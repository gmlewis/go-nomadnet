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
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
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
	// PendingChecker, when set, reports whether the given LXMF message hash is
	// still queued for outbound delivery (in the router's pending-outbound or
	// pending-deferred-stamps queue). Load uses it to mark interrupted
	// pending messages FAILED, mirroring Python's ConversationMessage.load
	// (Conversation.py:451-460): a message on disk whose state is between
	// GENERATING and SENT that is no longer in the pending queue is marked
	// FAILED. nil skips the check (headless/tests).
	PendingChecker func(hash []byte) bool
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

	// Presence flags for cached fields the index supplied. Python's lazy
	// getters check `_cached_X is not None` — an index-restored EMPTY string
	// or FALSE bool is still present and must not trigger a reload from disk
	// (Conversation.py get_title/get_content/get_transport_encrypted/
	// has_attachments). A reload would overwrite the index-restored state and
	// signature fields with freshly parsed envelope values, which can differ
	// (e.g. an interrupted outbound becomes FAILED via the pending check).
	titleSet   bool
	contentSet bool
	encSet     bool
	encMethSet bool
	attachSet  bool

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

	// Python __init__ seeds _cached_hash from the hex filename
	// (Conversation.py:443-447: bytes.fromhex(filename)), so get_hash never
	// needs to load from disk. Do the same: without it, GetHash's
	// "index didn't supply a hash" fallback reloads the envelope and clobbers
	// index-restored fields.
	var cachedHash []byte
	if filename := filepath.Base(filePath); len(filename) == 64 {
		if h, err := hex.DecodeString(filename); err == nil {
			cachedHash = h
		}
	}
	return &Message{
		FilePath:      filePath,
		SortTimestamp: sortTimestamp,
		CachedHash:    cachedHash,
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

// GetTitle returns the message title, loading from disk if needed. A title
// the index restored as an EMPTY string still counts as present (Python
// checks `_cached_title is not None`), so no reload clobbers the restored
// state/signature fields with freshly parsed envelope values.
func (m *Message) GetTitle() string {
	if m.titleSet || m.Loaded {
		return m.CachedTitle
	}
	if m.CachedTitle != "" {
		return m.CachedTitle
	}
	if !m.Loaded {
		m.Load()
	}
	return m.CachedTitle
}

// GetContent returns the message content, loading from disk if needed. Content
// the index restored as an EMPTY string still counts as present (Python checks
// `_cached_content is not None`), so it must not trigger a reload that would
// clobber index-restored state/signature fields.
func (m *Message) GetContent() string {
	if m.contentSet || m.Loaded {
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
// origin, invalid signature, or an unknown failure. When the index supplied
// CachedSignatureValidated (restore_from_index), use it without loading from
// disk — matching Python's lazy behavior.
func (m *Message) GetSignatureDescription() string {
	if m.CachedSignatureValidated == nil && !m.Loaded {
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
// loading from disk if needed. A FALSE value restored from the index still
// counts as present (Python checks `_cached_transport_encrypted is not None`).
func (m *Message) GetTransportEncrypted() bool {
	if m.encSet || m.Loaded {
		return m.CachedTransportEncrypted
	}
	m.Load()
	return m.CachedTransportEncrypted
}

// GetTransportEncryption returns the transport encryption method string,
// loading from disk if needed. An empty string restored from the index still
// counts as present (Python checks `_cached_transport_encryption is not None`).
func (m *Message) GetTransportEncryption() string {
	if m.encMethSet || (m.Loaded && m.CachedTransportEncryption != "") {
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

// HasAttachments returns whether the message has any attachments. When the
// index supplied CachedHasAttachments (restore_from_index), return it without
// loading from disk — matching Python's has_attachments which returns
// _cached_has_attachments when it is not None (so a restored FALSE is present
// and authoritative). Only load from disk when the index flag is unset.
func (m *Message) HasAttachments() bool {
	if m.attachSet || m.CachedHasAttachments {
		return m.CachedHasAttachments
	}
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

	// Mark interrupted pending messages FAILED, mirroring Python's
	// ConversationMessage.load (Conversation.py:451-460): a message on disk
	// whose LXMF state is between GENERATING (0x00) and SENT (0x04) — i.e.
	// OUTBOUND/SENDING — that is no longer in the router's pending-outbound
	// or pending-deferred-stamps queue is marked FAILED. Without this, an
	// outbound message interrupted mid-send (app restart, crash) renders
	// with the default "→" header branch instead of the "✕ →" FAILED glyph
	// (B16). Skipped when no PendingChecker is wired (headless/tests) and
	// when the message hash is unknown.
	rawState := lxm.State()
	if m.PendingChecker != nil && rawState > lxmf.StateGenerating && rawState < lxmf.StateSent {
		hash := m.CachedHash
		if hash == nil {
			hash = lxm.Hash
		}
		if hash != nil && !m.PendingChecker(hash) {
			rawState = lxmf.StateFailed
		}
	}

	m.CachedRawState = rawState
	m.CachedTitle = lxm.TitleString()
	m.CachedContent = lxm.ContentString()
	m.CachedTransportEncrypted = lxm.TransportEncrypted
	m.CachedTransportEncryption = lxm.TransportEncryption
	m.CachedMethod = lxm.Method()
	m.cachedFields = lxm.Fields

	st := mapLXMFState(rawState)
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

// ToIndexEntry serializes the message metadata for the index file. It returns an
// OrderedMap keyed in Python NomadNet's to_index_entry declaration order
// (timestamp, sort_timestamp, state, ...) so the on-disk msgpack is
// byte-identical to Python's insertion-ordered dict serialization — a plain Go
// map would randomize the key order. Integer fields (state, method, and the
// per-attachment size) are widened to uint so their msgpack encoding matches
// Python's unsigned encoding for non-negative values (a Go int packs values
// above 0x7f as signed int16/int32, e.g. a 1024-byte attachment size would
// become 0xd1 0x04 0x00 instead of Python's uint16 0xcd 0x04 0x00).
//
// The "state" field stores the RAW LXMF wire state (CachedRawState, e.g. 0x04
// SENT, 0x08 DELIVERED, 0xFF FAILED), matching Python's to_index_entry which
// stores self._cached_state = self.lxm.state (the raw LXMF constant). Storing
// the MAPPED conversation MessageState (0-6) here is incompatible: when
// Python reads a Go-written .index it interprets the value as a raw LXMF
// constant, and mapped values (3, 5) match no LXMF state (SENT=4,
// DELIVERED=8, FAILED=255), so every outbound header falls to the default
// branch.
func (m *Message) ToIndexEntry() rnsmsgpack.OrderedMap {
	var ts any
	if m.Timestamp != nil {
		ts = *m.Timestamp
	}

	// Store the raw LXMF state (matching Python's self._cached_state =
	// self.lxm.state). CachedRawState is populated by Load (from the on-disk
	// envelope) and by RestoreFromIndex (from the index). It is 0 for the
	// GENERATING state, which is a valid raw value, so guard on CachedState
	// being set (state is known) rather than on CachedRawState being non-zero.
	var state any
	if m.CachedState != nil {
		state = uint(m.CachedRawState)
	}

	var sigValid any
	if m.CachedSignatureValidated != nil {
		sigValid = *m.CachedSignatureValidated
	}

	var renderer any
	if m.Renderer != nil {
		renderer = m.Renderer
	}

	// source_hash is a []byte; a nil slice must serialize as msgpack nil (0xc0)
	// to match Python's None, not as an empty bin (0xc4 0x00) which is what a
	// typed nil []byte encodes to. Wrap it in an any so an absent hash stays an
	// untyped nil.
	var sourceHash any
	if m.CachedSourceHash != nil {
		sourceHash = m.CachedSourceHash
	}

	attNames := make([]any, 0, len(m.CachedAttachmentNames))
	for _, a := range m.CachedAttachmentNames {
		attNames = append(attNames, []any{a.Type, a.Name, uint(a.Size)})
	}

	return rnsmsgpack.OrderedMap{
		{Key: "timestamp", Value: ts},
		{Key: "sort_timestamp", Value: m.SortTimestamp},
		{Key: "state", Value: state},
		{Key: "title", Value: m.CachedTitle},
		{Key: "content", Value: m.CachedContent},
		{Key: "source_hash", Value: sourceHash},
		{Key: "transport_encrypted", Value: m.CachedTransportEncrypted},
		{Key: "transport_encryption", Value: m.CachedTransportEncryption},
		{Key: "signature_validated", Value: sigValid},
		{Key: "unverified_reason", Value: m.CachedUnverifiedReason},
		{Key: "method", Value: uint(m.CachedMethod)},
		{Key: "renderer", Value: renderer},
		{Key: "has_attachments", Value: m.CachedHasAttachments},
		{Key: "attachment_names", Value: attNames},
	}
}

// RestoreFromIndex populates cached fields from an index entry. The entry is
// the OrderedMap produced by the order-preserving Unpack variant, so fields
// are read by name via Get rather than by Go map indexing.
func (m *Message) RestoreFromIndex(entry rnsmsgpack.OrderedMap) {
	if v, ok := entry.Get("timestamp"); ok && v != nil {
		if f, ok := v.(float64); ok {
			m.Timestamp = &f
		}
	}
	if v, ok := entry.Get("sort_timestamp"); ok {
		if f, ok := v.(float64); ok {
			m.SortTimestamp = f
		}
	}
	if v, ok := entry.Get("state"); ok && v != nil {
		if i, ok := toInt(v); ok {
			// The index stores the RAW LXMF wire state (matching Python's
			// self._cached_state = self.lxm.state), so populate both the
			// raw state (for header rendering) and the mapped conversation
			// state (for GetState callers).
			m.CachedRawState = i
			s := mapLXMFState(i)
			m.CachedState = &s
		}
	}
	if v, ok := entry.Get("title"); ok {
		m.CachedTitle, _ = v.(string)
		m.titleSet = true
	}
	if v, ok := entry.Get("content"); ok {
		m.CachedContent, _ = v.(string)
		m.contentSet = true
	}
	if v, ok := entry.Get("source_hash"); ok {
		if b, ok := v.([]byte); ok {
			m.CachedSourceHash = b
		}
	}
	if v, ok := entry.Get("transport_encrypted"); ok {
		m.CachedTransportEncrypted, _ = v.(bool)
		m.encSet = true
	}
	if v, ok := entry.Get("transport_encryption"); ok {
		m.CachedTransportEncryption, _ = v.(string)
		m.encMethSet = true
	}
	if v, ok := entry.Get("signature_validated"); ok && v != nil {
		if b, ok := v.(bool); ok {
			m.CachedSignatureValidated = &b
		}
	}
	if v, ok := entry.Get("unverified_reason"); ok {
		m.CachedUnverifiedReason = v
	}
	if v, ok := entry.Get("method"); ok {
		m.CachedMethod, _ = toInt(v)
	}
	if v, ok := entry.Get("has_attachments"); ok {
		m.CachedHasAttachments, _ = v.(bool)
		m.attachSet = true
	}
}

// ReadIndex reads the message index from a conversation directory. It returns
// an OrderedMap (keyed by message filename) preserving the file's insertion
// order, mirroring Python NomadNet's read_index which returns the
// insertion-ordered dict from msgpack.unpackb. Nested per-message entries are
// also OrderedMap. An empty OrderedMap is returned when the index is absent or
// unreadable so callers can use Get uniformly.
func ReadIndex(conversationPath string) rnsmsgpack.OrderedMap {
	indexPath := filepath.Join(conversationPath, ".index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return rnsmsgpack.OrderedMap{}
	}

	raw, err := rnsmsgpack.UnpackPreserveBinMapKeyOrder(data)
	if err != nil {
		return rnsmsgpack.OrderedMap{}
	}
	om, ok := raw.(rnsmsgpack.OrderedMap)
	if !ok {
		return rnsmsgpack.OrderedMap{}
	}
	return om
}

// WriteIndex writes the message index to a conversation directory. The index
// is read back as an OrderedMap (preserving the existing file's key order),
// updated via Set so re-assigned filenames keep their insertion position and
// new filenames append (matching Python dict semantics), and packed via
// OrderedMap.MarshalMsgpack so the on-disk bytes are byte-identical to Python's
// write_index. Only messages with a cached state are written, matching
// Python's "if msg._cached_state is not None" guard.
func WriteIndex(conversationPath string, messages []*Message) error {
	indexPath := filepath.Join(conversationPath, ".index")

	existing := ReadIndex(conversationPath)

	for _, msg := range messages {
		if msg.CachedState != nil {
			key := filepath.Base(msg.FilePath)
			existing = existing.Set(key, msg.ToIndexEntry())
		}
	}

	data, err := rnsmsgpack.Pack(existing)
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
	case uint:
		return int(val), true
	case uint8:
		return int(val), true
	case uint16:
		return int(val), true
	case uint32:
		return int(val), true
	case uint64:
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
