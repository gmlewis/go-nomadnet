// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Package conversation manages LXMF conversations and their messages,
// including attachment extraction and on-disk persistence.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/vmihailenco/msgpack/v5"
)

// MessageState represents the delivery state of a message.
type MessageState int

const (
	StateDraft      MessageState = 0
	StateGenerating MessageState = 1
	StatePending    MessageState = 2
	StateSent       MessageState = 3
	StateDelivered  MessageState = 4
	StateFailed     MessageState = 5
	StatePaper      MessageState = 6
)

// lxmfStatePaper is the LXMF PAPER state value (Python LXMessage.PAPER = 0x05).
// go-reticulum does not export it as a named state constant, so it is defined
// here to keep the state mapping faithful to the Python source.
const lxmfStatePaper = 0x05

// SignatureState represents the signature validation state.
type SignatureState int

const (
	SigValidated     SignatureState = 0
	SigSourceUnknown SignatureState = 1
	SigInvalid       SignatureState = 2
)

// Message represents a single message in a conversation.
type Message struct {
	FilePath string
	// Transport, when set, allows Load to parse the LXMF envelope from disk.
	// When nil, Load falls back to file-mtime metadata only.
	Transport rns.Transport
	// AttachmentPath, when set, overrides the global attachment directory
	// base for this message. Tests set it directly to avoid touching shared
	// global state; the app sets it when constructing messages.
	AttachmentPath string

	Loaded bool

	Timestamp     *float64
	SortTimestamp float64

	// Cached fields from the LXM
	CachedHash                []byte
	CachedState               *MessageState
	CachedTitle               string
	CachedContent             string
	CachedSourceHash          []byte
	CachedRawState            int // raw LXMF state int (lxm.State), for header rendering
	CachedTransportEncrypted  bool
	CachedTransportEncryption string
	CachedSignatureValidated  *bool
	CachedUnverifiedReason    any
	CachedMethod              int
	CachedHasAttachments      bool
	CachedAttachmentNames     []AttachmentInfo

	// lxm holds the parsed LXMF message, retained for field/render access.
	lxm *lxmf.Message
	// cachedFields holds the parsed fields map (file/image/audio).
	cachedFields map[any]any

	Renderer any // reserved for UI renderer cache
}

// AttachmentInfo describes a single attachment.
type AttachmentInfo struct {
	Type string // "file", "image", or "audio"
	Name string
	Size int
}

// NewMessage creates a Message for the given file path. The message cannot
// parse its LXMF envelope until a Transport is provided via SetTransport or
// NewMessageWithTransport.
func NewMessage(filePath string) *Message {
	info, err := os.Stat(filePath)
	sortTimestamp := float64(0)
	if err == nil {
		sortTimestamp = float64(info.ModTime().UnixNano()) / 1e9
	}

	return &Message{
		FilePath:      filePath,
		SortTimestamp: sortTimestamp,
	}
}

// NewMessageWithTransport creates a Message whose Load can parse the LXMF
// envelope using the supplied transport for identity recall.
func NewMessageWithTransport(filePath string, transport rns.Transport) *Message {
	m := NewMessage(filePath)
	m.Transport = transport
	return m
}

// SetTransport attaches a transport so Load can parse the LXMF envelope.
func (m *Message) SetTransport(transport rns.Transport) {
	m.Transport = transport
}

// GetTimestamp returns the message timestamp, loading from disk if needed.
func (m *Message) GetTimestamp() float64 {
	if m.Timestamp != nil {
		return *m.Timestamp
	}
	if !m.Loaded {
		m.Load()
	}
	if m.Timestamp != nil {
		return *m.Timestamp
	}
	return m.SortTimestamp
}

// GetTitle returns the message title, loading from disk if needed.
func (m *Message) GetTitle() string {
	if m.CachedTitle != "" {
		return m.CachedTitle
	}
	if !m.Loaded {
		m.Load()
	}
	return m.CachedTitle
}

// GetContent returns the message content, loading from disk if needed.
func (m *Message) GetContent() string {
	if m.CachedContent != "" {
		return m.CachedContent
	}
	if !m.Loaded {
		m.Load()
	}
	return m.CachedContent
}

// GetHash returns the message hash, loading from disk if needed.
func (m *Message) GetHash() []byte {
	if m.CachedHash != nil {
		return m.CachedHash
	}
	if !m.Loaded {
		m.Load()
	}
	return m.CachedHash
}

// GetState returns the message state, mapping the LXMF state enum onto the
// conversation MessageState values, loading from disk if needed.
func (m *Message) GetState() MessageState {
	if m.CachedState != nil {
		return *m.CachedState
	}
	if !m.Loaded {
		m.Load()
	}
	if m.CachedState != nil {
		return *m.CachedState
	}
	return StateDraft
}

// SignatureValidated returns whether the message signature was validated.
func (m *Message) SignatureValidated() bool {
	if m.CachedSignatureValidated != nil {
		return *m.CachedSignatureValidated
	}
	if !m.Loaded {
		m.Load()
	}
	if m.CachedSignatureValidated != nil {
		return *m.CachedSignatureValidated
	}
	return false
}

// GetSignatureDescription returns a human-readable signature status, mirroring
// the Python NomadNet get_signature_description branches: verified, unknown
// origin, invalid signature, or an unknown failure.
func (m *Message) GetSignatureDescription() string {
	if !m.Loaded {
		m.Load()
	}
	if m.CachedSignatureValidated != nil && *m.CachedSignatureValidated {
		return "Signature Verified"
	}
	switch toIntOr(m.CachedUnverifiedReason, -1) {
	case lxmf.ReasonSourceUnknown:
		return "Unknown Origin"
	case lxmf.ReasonSignatureInvalid:
		return "Invalid Signature"
	}
	return "Unknown signature validation failure"
}

// GetTransportEncrypted returns whether the message transport was encrypted,
// loading from disk if needed.
func (m *Message) GetTransportEncrypted() bool {
	if m.Loaded {
		return m.CachedTransportEncrypted
	}
	m.Load()
	return m.CachedTransportEncrypted
}

// GetTransportEncryption returns the transport encryption method string,
// loading from disk if needed.
func (m *Message) GetTransportEncryption() string {
	if m.CachedTransportEncryption != "" {
		return m.CachedTransportEncryption
	}
	if !m.Loaded {
		m.Load()
	}
	return m.CachedTransportEncryption
}

// GetFields returns the parsed LXMF fields map (file/image/audio), loading
// from disk if needed. An empty map is returned when no fields are present.
func (m *Message) GetFields() map[any]any {
	if m.cachedFields != nil {
		return m.cachedFields
	}
	if !m.Loaded {
		m.Load()
	}
	return m.cachedFields
}

// ContentRenderer returns the cached content renderer for the message body, or
// nil when none is set. This mirrors the Python NomadNet content_renderer.
func (m *Message) ContentRenderer() any {
	if m.Renderer != nil {
		return m.Renderer
	}
	if !m.Loaded {
		m.Load()
	}
	return m.Renderer
}

// HasAttachments returns whether the message has any attachments.
func (m *Message) HasAttachments() bool {
	if !m.Loaded {
		m.Load()
	}
	if m.CachedHasAttachments {
		return true
	}
	fields := m.GetFields()
	_, hasFile := fieldLookup(fields, lxmf.FieldFileAttachments)
	_, hasImage := fieldLookup(fields, lxmf.FieldImage)
	_, hasAudio := fieldLookup(fields, lxmf.FieldAudio)
	return hasFile || hasImage || hasAudio
}

// Unload releases the loaded LXM data from memory.
func (m *Message) Unload() {
	m.Loaded = false
	m.lxm = nil
}

// Purge unloads and deletes the message file from disk.
func (m *Message) Purge() error {
	m.Unload()
	if _, err := os.Stat(m.FilePath); err == nil {
		return os.Remove(m.FilePath)
	}
	return nil
}

// Load reads the message metadata from disk. When a Transport is configured the
// LXMF envelope is parsed (title, content, hash, state, signature, fields,
// transport encryption); otherwise it falls back to the file modification time
// only. This mirrors the Python NomadNet ConversationMessage.load.
func (m *Message) Load() {
	if m.Loaded {
		return
	}

	info, err := os.Stat(m.FilePath)
	if err != nil {
		return
	}
	m.SortTimestamp = float64(info.ModTime().UnixNano()) / 1e9

	if m.Transport == nil {
		ts := m.SortTimestamp
		m.Timestamp = &ts
		m.Loaded = true
		return
	}

	f, err := os.Open(m.FilePath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	lxm, err := lxmf.UnpackMessageFromFile(m.Transport, f)
	if err != nil {
		// Fall back to mtime-only metadata when the envelope cannot be parsed.
		ts := m.SortTimestamp
		m.Timestamp = &ts
		m.Loaded = true
		return
	}

	m.lxm = lxm
	m.Loaded = true

	ts := lxm.Timestamp
	m.Timestamp = &ts
	if len(lxm.Hash) > 0 {
		m.CachedHash = lxm.Hash
	}
	if len(lxm.SourceHash) > 0 {
		m.CachedSourceHash = lxm.SourceHash
	}
	m.CachedRawState = lxm.State
	m.CachedTitle = lxm.TitleString()
	m.CachedContent = lxm.ContentString()
	m.CachedTransportEncrypted = lxm.TransportEncrypted
	m.CachedTransportEncryption = lxm.TransportEncryption
	m.CachedMethod = lxm.Method
	m.cachedFields = lxm.Fields

	st := mapLXMFState(lxm.State)
	m.CachedState = &st

	validated := lxm.SignatureValidated
	m.CachedSignatureValidated = &validated
	m.CachedUnverifiedReason = lxm.UnverifiedReason

	// Preserve the renderer field when present in the parsed fields.
	if m.cachedFields != nil {
		if r, ok := fieldLookup(m.cachedFields, lxmf.FieldRenderer); ok {
			m.Renderer = r
		}
	}

	m.computeAttachmentCache()
}

// mapLXMFState maps an LXMF state constant onto a conversation MessageState,
// mirroring the Python NomadNet state semantics.
func mapLXMFState(lxmfState int) MessageState {
	switch lxmfState {
	case lxmf.StateGenerating:
		return StateGenerating
	case lxmf.StateOutbound, lxmf.StateSending:
		return StatePending
	case lxmf.StateSent:
		return StateSent
	case lxmf.StateDelivered:
		return StateDelivered
	case lxmf.StateFailed, lxmf.StateRejected, lxmf.StateCancelled:
		return StateFailed
	case lxmfStatePaper:
		return StatePaper
	}
	if lxmfState > lxmf.StateGenerating && lxmfState < lxmf.StateSent {
		return StatePending
	}
	return StateDraft
}

// computeAttachmentCache populates CachedHasAttachments and the attachment name
// list from the parsed LXMF fields, mirroring the Python NomadNet load
// attachment discovery.
func (m *Message) computeAttachmentCache() {
	m.CachedHasAttachments = false
	m.CachedAttachmentNames = nil
	if m.cachedFields == nil {
		return
	}
	if _, ok := fieldLookup(m.cachedFields, lxmf.FieldFileAttachments); ok {
		m.CachedHasAttachments = true
	}
	if _, ok := fieldLookup(m.cachedFields, lxmf.FieldImage); ok {
		m.CachedHasAttachments = true
	}
	if _, ok := fieldLookup(m.cachedFields, lxmf.FieldAudio); ok {
		m.CachedHasAttachments = true
	}
	if !m.CachedHasAttachments {
		return
	}

	if fv, ok := fieldLookup(m.cachedFields, lxmf.FieldFileAttachments); ok {
		fileAtts, _ := fv.([]any)
		for idx, att := range fileAtts {
			entry, ok := att.([]any)
			if !ok || len(entry) < 2 {
				continue
			}
			size := 0
			if b, ok := entry[1].([]byte); ok {
				size = len(b)
			}
			safe := SafeAttachmentName(entry[0], fmt.Sprintf("attachment_%v", idx))
			m.CachedAttachmentNames = append(m.CachedAttachmentNames, AttachmentInfo{Type: "file", Name: safe, Size: size})
		}
	}
	if imageField, ok := fieldLookup(m.cachedFields, lxmf.FieldImage); ok {
		fmtVal, data := UnpackMediaField(imageField)
		if data != nil {
			ext := ExtFromMediaFormat(fmtVal, data, false)
			safe := SafeAttachmentName("image"+ext, "image")
			m.CachedAttachmentNames = append(m.CachedAttachmentNames, AttachmentInfo{Type: "file", Name: safe, Size: len(data)})
		}
	}
	if audioField, ok := fieldLookup(m.cachedFields, lxmf.FieldAudio); ok {
		fmtVal, data := UnpackMediaField(audioField)
		if data != nil {
			ext := ExtFromMediaFormat(fmtVal, data, true)
			safe := SafeAttachmentName("audio"+ext, "audio")
			m.CachedAttachmentNames = append(m.CachedAttachmentNames, AttachmentInfo{Type: "file", Name: safe, Size: len(data)})
		}
	}
}

// ToIndexEntry serializes the message metadata for the index file.
func (m *Message) ToIndexEntry() map[string]any {
	var ts any
	if m.Timestamp != nil {
		ts = *m.Timestamp
	}

	var state any
	if m.CachedState != nil {
		state = int(*m.CachedState)
	}

	var sigValid any
	if m.CachedSignatureValidated != nil {
		sigValid = *m.CachedSignatureValidated
	}

	var renderer any
	if m.Renderer != nil {
		renderer = m.Renderer
	}

	attNames := make([]any, 0, len(m.CachedAttachmentNames))
	for _, a := range m.CachedAttachmentNames {
		attNames = append(attNames, []any{a.Type, a.Name, a.Size})
	}

	return map[string]any{
		"timestamp":            ts,
		"sort_timestamp":       m.SortTimestamp,
		"state":                state,
		"title":                m.CachedTitle,
		"content":              m.CachedContent,
		"source_hash":          m.CachedSourceHash,
		"transport_encrypted":  m.CachedTransportEncrypted,
		"transport_encryption": m.CachedTransportEncryption,
		"signature_validated":  sigValid,
		"unverified_reason":    m.CachedUnverifiedReason,
		"method":               m.CachedMethod,
		"renderer":             renderer,
		"has_attachments":      m.CachedHasAttachments,
		"attachment_names":     attNames,
	}
}

// RestoreFromIndex populates cached fields from an index entry.
func (m *Message) RestoreFromIndex(entry map[string]any) {
	if v, ok := entry["timestamp"]; ok && v != nil {
		if f, ok := v.(float64); ok {
			m.Timestamp = &f
		}
	}
	if v, ok := entry["sort_timestamp"]; ok {
		if f, ok := v.(float64); ok {
			m.SortTimestamp = f
		}
	}
	if v, ok := entry["state"]; ok && v != nil {
		if i, ok := toInt(v); ok {
			s := MessageState(i)
			m.CachedState = &s
		}
	}
	if v, ok := entry["title"]; ok {
		m.CachedTitle, _ = v.(string)
	}
	if v, ok := entry["content"]; ok {
		m.CachedContent, _ = v.(string)
	}
	if v, ok := entry["source_hash"]; ok {
		if b, ok := v.([]byte); ok {
			m.CachedSourceHash = b
		}
	}
	if v, ok := entry["transport_encrypted"]; ok {
		m.CachedTransportEncrypted, _ = v.(bool)
	}
	if v, ok := entry["transport_encryption"]; ok {
		m.CachedTransportEncryption, _ = v.(string)
	}
	if v, ok := entry["signature_validated"]; ok && v != nil {
		if b, ok := v.(bool); ok {
			m.CachedSignatureValidated = &b
		}
	}
	m.CachedUnverifiedReason = entry["unverified_reason"]
	if v, ok := entry["method"]; ok {
		m.CachedMethod, _ = toInt(v)
	}
	if v, ok := entry["has_attachments"]; ok {
		m.CachedHasAttachments, _ = v.(bool)
	}
}

// ReadIndex reads the message index from a conversation directory.
func ReadIndex(conversationPath string) map[string]any {
	indexPath := filepath.Join(conversationPath, ".index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return make(map[string]any)
	}

	var index map[string]any
	if err := msgpack.Unmarshal(data, &index); err != nil {
		return make(map[string]any)
	}
	return index
}

// WriteIndex writes the message index to a conversation directory.
func WriteIndex(conversationPath string, messages []*Message) error {
	indexPath := filepath.Join(conversationPath, ".index")

	// Read existing index
	existing := ReadIndex(conversationPath)

	// Update entries for messages with cached state
	for _, msg := range messages {
		if msg.CachedState != nil {
			key := filepath.Base(msg.FilePath)
			existing[key] = msg.ToIndexEntry()
		}
	}

	data, err := msgpack.Marshal(existing)
	if err != nil {
		return fmt.Errorf("encoding index: %w", err)
	}

	return os.WriteFile(indexPath, data, 0o644)
}

// IndexFilename returns the expected index filename for a conversation.
func IndexFilename() string {
	return ".index"
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int8:
		return int(val), true
	case int16:
		return int(val), true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}

// toIntOr returns the int value of v, or fallback when v is not an integer.
func toIntOr(v any, fallback int) int {
	if i, ok := toInt(v); ok {
		return i
	}
	return fallback
}
