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
	"container/ring"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// RRCHub represents a connection to a single RRC hub server.
type RRCHub struct {
	Manager *RRCManager

	HubHash  []byte // hub identity hash
	DestName string // RNS destination name
	Name     string // display name

	// Connection state
	Status     int
	StatusText string
	Welcomed   bool

	// Hub-reported info
	HubName    string
	HubVersion string
	HubCaps    map[any]any
	MOTD       string

	// Limits from WELCOME
	MaxNickBytes        int
	MaxRoomNameBytes    int
	MaxMsgBodyBytes     int
	MaxRoomsPerSession  int
	RateLimitMsgsPerMin int

	// Room state
	Rooms        map[string]bool   // joined rooms (lowercased)
	Messages     map[string][]*RRCMessage // room → messages
	Notices      []*RRCMessage     // global notices
	UnreadRooms  map[string]bool   // rooms with unread messages
	MentionRooms map[string]bool   // rooms with unread mentions
	Members      map[string]map[string]bool // room → set of hash hex
	Nicks        map[string]string  // hash hex → nick
	PartedRooms  map[string]bool   // rooms with history but not joined
	AvailableRooms map[string]*string // room → topic or nil

	// Auto-connect options
	AutoReconnect bool
	AutoList      bool
	AutoWho       bool
	NickOverride  string

	// Internal state
	lock              sync.Mutex
	sentIDs           *ring.Ring // dedup ring buffer
	pendingPings      map[string]time.Time // body → send time
	pendingJoins      map[string]bool
	pendingParts      map[string]bool
	silentJoins       map[string]bool
	silentWhoRooms    map[string]bool
	reconnectAttempts int
	historyPath       string
}

// NewHub creates a new RRCHub with default values.
func NewHub(manager *RRCManager, hubHash []byte, destName, name string) *RRCHub {
	if destName == "" {
		destName = DefaultDestName
	}
	if name == "" {
		name = hexString(hubHash)
	}

	h := &RRCHub{
		Manager:           manager,
		HubHash:           hubHash,
		DestName:          destName,
		Name:              name,
		Status:            StatusDisconnected,
		StatusText:        "Disconnected",
		MaxNickBytes:      DefaultMaxNickBytes,
		MaxRoomNameBytes:  DefaultMaxRoomBytes,
		MaxMsgBodyBytes:   DefaultMaxMsgBytes,
		MaxRoomsPerSession: DefaultMaxRooms,
		RateLimitMsgsPerMin: DefaultRatePerMinute,
		Rooms:             make(map[string]bool),
		Messages:          make(map[string][]*RRCMessage),
		UnreadRooms:       make(map[string]bool),
		MentionRooms:      make(map[string]bool),
		Members:           make(map[string]map[string]bool),
		Nicks:             make(map[string]string),
		PartedRooms:       make(map[string]bool),
		AvailableRooms:    make(map[string]*string),
		sentIDs:           ring.New(256),
		pendingPings:      make(map[string]time.Time),
		pendingJoins:      make(map[string]bool),
		pendingParts:      make(map[string]bool),
		silentJoins:       make(map[string]bool),
		silentWhoRooms:    make(map[string]bool),
	}

	return h
}

// AddRoom adds a room to the local state.
func (h *RRCHub) AddRoom(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()
	h.Rooms[room] = true
	if h.Messages[room] == nil {
		h.Messages[room] = make([]*RRCMessage, 0)
	}
}

// RemoveRoom removes a room and its history.
func (h *RRCHub) RemoveRoom(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()
	delete(h.Rooms, room)
	delete(h.Messages, room)
	delete(h.Members, room)
	delete(h.UnreadRooms, room)
	delete(h.MentionRooms, room)
	h._deleteHistory(room)
}

// ClearMessages clears the message buffer for a room.
func (h *RRCHub) ClearMessages(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()
	h.Messages[room] = make([]*RRCMessage, 0)
}

// GetMembers returns the member list for a room.
func (h *RRCHub) GetMembers(room string) []string {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()

	members := make([]string, 0)
	if m, ok := h.Members[room]; ok {
		for hash := range m {
			members = append(members, hash)
		}
	}
	return members
}

// DisplayNameFor returns the display name for a peer hash.
func (h *RRCHub) DisplayNameFor(peer []byte) string {
	h.lock.Lock()
	defer h.lock.Unlock()

	hex := hexString(peer)
	if nick, ok := h.Nicks[hex]; ok && nick != "" {
		return nick
	}
	// Return first 12 hex chars
	if len(hex) > 12 {
		return hex[:12]
	}
	return hex
}

// MarkRead clears unread/mention flags for a room.
func (h *RRCHub) MarkRead(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()
	delete(h.UnreadRooms, room)
	delete(h.MentionRooms, room)
}

// GetMessages returns the message buffer for a room.
func (h *RRCHub) GetMessages(room string) []*RRCMessage {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()

	msgs := h.Messages[room]
	result := make([]*RRCMessage, len(msgs))
	copy(result, msgs)
	return result
}

// JoinRoom sends a T_JOIN for a room.
func (h *RRCHub) JoinRoom(room string, silent bool) {
	room = strings.ToLower(room)
	h.lock.Lock()
	h.Rooms[room] = true
	if h.Messages[room] == nil {
		h.Messages[room] = make([]*RRCMessage, 0)
	}
	h.pendingJoins[room] = true
	if silent {
		h.silentJoins[room] = true
	}
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeJoin, nil, []byte(room), nil, nil, mid, ts)
	h._sendEnv(env)
}

// PartRoom sends a T_PART for a room.
func (h *RRCHub) PartRoom(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	h.pendingParts[room] = true
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypePart, nil, []byte(room), nil, nil, mid, ts)
	h._sendEnv(env)
}

// SendMessage sends a T_MSG to a room and records it locally.
func (h *RRCHub) SendMessage(room, text string) string {
	room = strings.ToLower(room)
	mid := MsgID()
	ts := NowMs()

	nick := h.GetEffectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeMsg, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h._sendEnv(env)

	msg := &RRCMessage{
		Kind: "msg",
		Room: room,
		Src:  srcHash,
		Nick: nick,
		Text: text,
		Ts:   ts,
	}
	h._recordMessage(msg, true)

	return hexString(mid)
}

// SendAction sends a T_ACTION to a room.
func (h *RRCHub) SendAction(room, text string) string {
	room = strings.ToLower(room)
	mid := MsgID()
	ts := NowMs()

	nick := h.GetEffectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeAction, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h._sendEnv(env)

	msg := &RRCMessage{
		Kind: "action",
		Room: room,
		Src:  srcHash,
		Nick: nick,
		Text: text,
		Ts:   ts,
	}
	h._recordMessage(msg, true)

	return hexString(mid)
}

// SendPing sends a T_PING to a room.
func (h *RRCHub) SendPing(room string) {
	room = strings.ToLower(room)
	mid := MsgID()
	ts := NowMs()

	body := make([]byte, 8)
	rand.Read(body)
	key := hexString(body)

	h.lock.Lock()
	h.pendingPings[key] = time.Now()
	h.lock.Unlock()

	env := MakeEnvelope(TypePing, nil, []byte(room), nil, body, mid, ts)
	h._sendEnv(env)
}

// GetEffectiveNick returns the override nick or the manager's nick.
func (h *RRCHub) GetEffectiveNick() string {
	if h.NickOverride != "" {
		return h.NickOverride
	}
	if h.Manager != nil {
		return h.Manager.GetNickname()
	}
	return ""
}

// SetNickOverride sets a per-hub nick override.
func (h *RRCHub) SetNickOverride(nick string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.NickOverride = nick
}

func (h *RRCHub) _sendEnv(env map[any]any) {
	// Placeholder: actual send via RNS link
	_ = env
}

func (h *RRCHub) _recordMessage(msg *RRCMessage, local bool) {
	h.lock.Lock()
	defer h.lock.Unlock()

	room := strings.ToLower(msg.Room)
	if room == "" {
		// Global notice
		h.Notices = append(h.Notices, msg)
		if len(h.Notices) > 100 {
			h.Notices = h.Notices[len(h.Notices)-100:]
		}
		return
	}

	// Cap message buffer at 256
	msgs := h.Messages[room]
	if len(msgs) >= 256 {
		msgs = msgs[len(msgs)-255:]
	}
	h.Messages[room] = append([]*RRCMessage{msg}, msgs...)

	// Mark unread for non-local messages
	if !local && h.Manager != nil {
		activeRoom := h.Manager.ActiveRoomFor(h)
		if activeRoom != room {
			h.UnreadRooms[room] = true
			if msg.Mention {
				h.MentionRooms[room] = true
			}
		}
	}

	// Append to history
	h._appendHistory(room, msg)
}

func (h *RRCHub) _appendHistory(room string, msg *RRCMessage) {
	if h.historyPath == "" {
		return
	}

	room = strings.ToLower(room)
	entry := msg.HistoryEntry()
	data, err := cbor.Marshal(entry)
	if err != nil {
		return
	}

	path := h._historyPath(room)
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

func (h *RRCHub) _deleteHistory(room string) {
	if h.historyPath == "" {
		return
	}
	path := h._historyPath(room)
	os.Remove(path)
}

func (h *RRCHub) _historyPath(room string) string {
	if h.historyPath == "" {
		return ""
	}
	sanitized := sanitizeRoomName(room)
	hash := sha256.Sum256([]byte(room))
	prefix := fmt.Sprintf("%x", hash[:4])
	return filepath.Join(h.historyPath, sanitized+"_"+prefix+".log")
}

// SetStatus updates the connection status.
func (h *RRCHub) SetStatus(status int, text string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.Status = status
	if text != "" {
		h.StatusText = text
	}
}

func sanitizeRoomName(name string) string {
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	return re.ReplaceAllString(strings.ToLower(name), "_")
}

func hexString(hash []byte) string {
	const hexDigits = "0123456789abcdef"
	buf := make([]byte, len(hash)*2)
	for i, b := range hash {
		buf[i*2] = hexDigits[b>>4]
		buf[i*2+1] = hexDigits[b&0x0f]
	}
	return string(buf)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
