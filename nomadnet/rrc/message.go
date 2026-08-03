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

package rrc

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// RRCMessage represents a single chat message.
type RRCMessage struct {
	Kind    string // "msg", "action", "notice", "system", "error"
	Room    string // room name (lowercased), empty for global notices
	Src     []byte // sender identity hash, nil for system/notices
	Nick    string // sender display nick
	Text    string // message content
	Ts      int64  // timestamp in milliseconds since epoch
	Mention bool   // true if message mentions the local user
}

// NowMs returns the current time in milliseconds since epoch.
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// MsgID returns 8 random bytes for message deduplication.
func MsgID() []byte {
	b := make([]byte, 8)
	rand.Read(b)
	return b
}

// HistoryEntry returns a map suitable for CBOR encoding to the
// history file format.
func (m *RRCMessage) HistoryEntry() map[string]any {
	entry := map[string]any{
		HKind: m.Kind,
		HTS:   m.Ts,
		HText: m.Text,
	}
	if len(m.Src) > 0 {
		entry[HSrc] = m.Src
	}
	if m.Nick != "" {
		entry[HNick] = m.Nick
	}
	// if m.Room != "" {
	//   Room is stored implicitly in the file path, but included for clarity
	// }
	if m.Mention {
		entry[HMention] = true
	}
	return entry
}

// DecodeHistoryEntry creates an RRCMessage from a CBOR-decoded
// history entry map.
func DecodeHistoryEntry(entry map[string]any) *RRCMessage {
	msg := &RRCMessage{}

	if v, ok := entry[HKind].(string); ok {
		msg.Kind = v
	}
	if v, ok := entry[HTS].(int64); ok {
		msg.Ts = v
	} else if v, ok := entry[HTS].(uint64); ok {
		msg.Ts = int64(v)
	}
	if v, ok := entry[HText].(string); ok {
		msg.Text = v
	}
	if v, ok := entry[HSrc].([]byte); ok {
		msg.Src = v
	}
	if v, ok := entry[HNick].(string); ok {
		msg.Nick = v
	}
	if v, ok := entry[HMention].(bool); ok {
		msg.Mention = v
	}

	return msg
}

// MakeEnvelope constructs a CBOR-encodable envelope for the RRC protocol.
func MakeEnvelope(msgType int, src, room, nick []byte, body any, mid []byte, ts int64) map[any]any {
	env := map[any]any{
		KeyVersion:   RRCVersion,
		KeyType:      msgType,
		KeyTimestamp: ts,
	}
	if len(mid) > 0 {
		env[KeyMessageID] = mid
	}
	if len(src) > 0 {
		env[KeySource] = src
	}
	if len(room) > 0 {
		env[KeyRoom] = room
	}
	if body != nil {
		env[KeyBody] = body
	}
	if len(nick) > 0 {
		env[KeyNick] = nick
	}
	return env
}

// EncodeEnvelope serializes an envelope map to CBOR bytes.
func EncodeEnvelope(env map[any]any) ([]byte, error) {
	return cbor.Marshal(env)
}

// DecodeEnvelope deserializes CBOR bytes to an envelope map.
func DecodeEnvelope(data []byte) (map[any]any, error) {
	var env map[any]any
	if err := cbor.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decoding RRC envelope: %w", err)
	}
	return env, nil
}

// MentionRegex returns a simple pattern for detecting @mentions.
// The actual implementation uses word-boundary-aware matching.
func MentionRegex(nick string) string {
	return fmt.Sprintf(`(?i)\b@%v\b`, nick)
}
