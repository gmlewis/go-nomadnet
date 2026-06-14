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

package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
)

// SignatureState represents the signature validation state.
type SignatureState int

const (
	SigValidated    SignatureState = 0
	SigSourceUnknown SignatureState = 1
	SigInvalid      SignatureState = 2
)

// Message represents a single message in a conversation.
type Message struct {
	FilePath string

	Loaded bool

	Timestamp    *float64
	SortTimestamp float64

	// Cached fields from the LXM
	CachedHash               []byte
	CachedState              *MessageState
	CachedTitle              string
	CachedContent            string
	CachedSourceHash         []byte
	CachedTransportEncrypted bool
	CachedSignatureValidated *bool
	CachedUnverifiedReason   any
	CachedMethod             int
	CachedHasAttachments     bool
	CachedAttachmentNames    []AttachmentInfo

	Renderer any // reserved for UI renderer cache
}

// AttachmentInfo describes a single attachment.
type AttachmentInfo struct {
	Type string // "file", "image", or "audio"
	Name string
	Size int
}

// NewMessage creates a Message for the given file path.
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

// GetState returns the message state, loading from disk if needed.
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

// GetSignatureDescription returns a human-readable signature status.
func (m *Message) GetSignatureDescription() string {
	if !m.Loaded {
		m.Load()
	}
	if m.CachedSignatureValidated == nil {
		return "Unknown signature validation failure"
	}
	if *m.CachedSignatureValidated {
		return "Signature Verified"
	}
	if reason, ok := m.CachedUnverifiedReason.(string); ok {
		switch reason {
		case "source_unknown":
			return "Unknown Origin"
		case "signature_invalid":
			return "Invalid Signature"
		}
	}
	return "Unknown signature validation failure"
}

// HasAttachments returns whether the message has any attachments.
func (m *Message) HasAttachments() bool {
	if !m.Loaded {
		m.Load()
	}
	return m.CachedHasAttachments && len(m.CachedAttachmentNames) > 0
}

// Unload releases the loaded LXM data from memory.
func (m *Message) Unload() {
	m.Loaded = false
}

// Purge unloads and deletes the message file from disk.
func (m *Message) Purge() error {
	m.Unload()
	if _, err := os.Stat(m.FilePath); err == nil {
		return os.Remove(m.FilePath)
	}
	return nil
}

// Load reads the message metadata from disk. This is a placeholder
// that will be implemented when LXMF support is added.
func (m *Message) Load() {
	if m.Loaded {
		return
	}

	// Extract timestamp from the file modification time
	info, err := os.Stat(m.FilePath)
	if err != nil {
		return
	}

	ts := float64(info.ModTime().UnixNano()) / 1e9
	m.Timestamp = &ts
	m.Loaded = true
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
		"timestamp":              ts,
		"sort_timestamp":         m.SortTimestamp,
		"state":                  state,
		"title":                  m.CachedTitle,
		"content":                m.CachedContent,
		"source_hash":            m.CachedSourceHash,
		"transport_encrypted":    m.CachedTransportEncrypted,
		"signature_validated":    sigValid,
		"unverified_reason":      m.CachedUnverifiedReason,
		"method":                 m.CachedMethod,
		"renderer":               renderer,
		"has_attachments":        m.CachedHasAttachments,
		"attachment_names":       attNames,
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

// now returns the current time as a Unix timestamp.
func now() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
