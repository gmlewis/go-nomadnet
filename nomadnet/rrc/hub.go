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
	"github.com/gmlewis/go-reticulum/rns"
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
	Rooms          map[string]bool            // joined rooms (lowercased)
	Messages       map[string][]*RRCMessage   // room → messages
	Notices        []*RRCMessage              // global notices
	UnreadRooms    map[string]bool            // rooms with unread messages
	MentionRooms   map[string]bool            // rooms with unread mentions
	Members        map[string]map[string]bool // room → set of hash hex
	Nicks          map[string]string          // hash hex → nick
	PartedRooms    map[string]bool            // rooms with history but not joined
	AvailableRooms map[string]*string         // room → topic or nil

	// Auto-connect options
	AutoReconnect bool
	AutoList      bool
	AutoWho       bool
	NickOverride  string

	// Internal state
	lock              sync.Mutex
	sentIDs           *ring.Ring           // dedup ring buffer
	pendingPings      map[string]time.Time // body → send time
	pendingJoins      map[string]bool
	pendingParts      map[string]bool
	silentJoins       map[string]bool
	silentWhoRooms    map[string]bool
	savedHistoryPath  string
	link              *rns.Link
	onLinkEstablished func()
	onLinkClosed      func()
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
		Manager:             manager,
		HubHash:             hubHash,
		DestName:            destName,
		Name:                name,
		Status:              StatusDisconnected,
		StatusText:          "Disconnected",
		MaxNickBytes:        DefaultMaxNickBytes,
		MaxRoomNameBytes:    DefaultMaxRoomBytes,
		MaxMsgBodyBytes:     DefaultMaxMsgBytes,
		MaxRoomsPerSession:  DefaultMaxRooms,
		RateLimitMsgsPerMin: DefaultRatePerMinute,
		Rooms:               make(map[string]bool),
		Messages:            make(map[string][]*RRCMessage),
		UnreadRooms:         make(map[string]bool),
		MentionRooms:        make(map[string]bool),
		Members:             make(map[string]map[string]bool),
		Nicks:               make(map[string]string),
		PartedRooms:         make(map[string]bool),
		AvailableRooms:      make(map[string]*string),
		sentIDs:             ring.New(256),
		pendingPings:        make(map[string]time.Time),
		pendingJoins:        make(map[string]bool),
		pendingParts:        make(map[string]bool),
		silentJoins:         make(map[string]bool),
		silentWhoRooms:      make(map[string]bool),
	}

	return h
}

// SetOnLinkEstablished registers a callback invoked when the RNS link
// to the hub becomes active.
func (h *RRCHub) SetOnLinkEstablished(fn func()) {
	h.lock.Lock()
	h.onLinkEstablished = fn
	h.lock.Unlock()
}

// SetOnLinkClosed registers a callback invoked when the RNS link closes.
func (h *RRCHub) SetOnLinkClosed(fn func()) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.onLinkClosed = fn
}

// SetLink sets the RNS link used by this hub for sending data. This is
// used by server-side hubs that receive incoming links.
func (h *RRCHub) SetLink(link *rns.Link) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.link = link
}

// Connect establishes an RNS link to the hub's destination. The link
// handshake is asynchronous; use SetOnLinkEstablished to be notified
// when the link becomes active. After link establishment, a HELLO
// envelope is sent to the server to initiate the RRC handshake.
func (h *RRCHub) Connect(ts rns.Transport, dest *rns.Destination) error {
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		return err
	}

	h.lock.Lock()
	h.link = link
	h.Status = StatusConnecting
	h.StatusText = "Connecting"
	h.lock.Unlock()

	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		h.lock.Lock()
		h.Status = StatusConnected
		h.StatusText = "Connected"
		h.Welcomed = false
		cb := h.onLinkEstablished
		h.lock.Unlock()

		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			h.HandleData(data)
		})

		h.sendHello(l)

		if cb != nil {
			cb()
		}
	})

	link.SetLinkClosedCallback(func(l *rns.Link) {
		h.lock.Lock()
		h.Status = StatusDisconnected
		h.StatusText = "Disconnected"
		h.Welcomed = false
		cb := h.onLinkClosed
		h.lock.Unlock()
		if cb != nil {
			cb()
		}
	})

	return link.Establish()
}

// sendHello sends a HELLO envelope on the given link. Matches Python's
// RRCHub._send_hello which sends client name, version, caps, and nick.
func (h *RRCHub) sendHello(link *rns.Link) {
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}

	body := map[any]any{
		BHelloName: []byte("nomadnet"),
		BHelloVer:  []byte("0.1"),
		BHelloCaps: map[any]any{
			CapResourceEnvelope: true,
			CapAction:           true,
		},
	}

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeHello, srcHash, nil, nil, body, mid, ts)

	nick := h.GetEffectiveNick()
	if nick != "" {
		env[KeyNick] = []byte(nick)
	}

	h.sendEnv(env)
}

// Disconnect tears down the RNS link and resets hub status.
func (h *RRCHub) Disconnect() {
	h.lock.Lock()
	link := h.link
	h.link = nil
	h.Status = StatusDisconnected
	h.StatusText = "Disconnected"
	h.Welcomed = false
	h.lock.Unlock()

	if link != nil {
		link.Teardown()
	}
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
	h.deleteHistory(room)
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
	h.sendEnv(env)
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
	h.sendEnv(env)
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
	h.sendEnv(env)

	msg := &RRCMessage{
		Kind: "msg",
		Room: room,
		Src:  srcHash,
		Nick: nick,
		Text: text,
		Ts:   ts,
	}
	h.recordMessage(msg, true)

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
	h.sendEnv(env)

	msg := &RRCMessage{
		Kind: "action",
		Room: room,
		Src:  srcHash,
		Nick: nick,
		Text: text,
		Ts:   ts,
	}
	h.recordMessage(msg, true)

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
	h.sendEnv(env)
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

func (h *RRCHub) sendEnv(env map[any]any) {
	data, err := EncodeEnvelope(env)
	if err != nil {
		return
	}
	h.lock.Lock()
	link := h.link
	h.lock.Unlock()
	if link == nil {
		return
	}
	p := rns.NewPacketWithTransport(link.GetTransport(), link, data)
	if err := p.Pack(); err != nil {
		return
	}
	_ = link.SendPacket(p)
}

// HandleData decodes a CBOR-encoded RRC envelope and dispatches it
// to the appropriate handler based on the message type.
func (h *RRCHub) HandleData(data []byte) {
	env, err := DecodeEnvelope(data)
	if err != nil {
		return
	}

	msgType := intVal(env, KeyType)
	src := byteVal(env, KeySource)
	room := byteVal(env, KeyRoom)
	nick := byteVal(env, KeyNick)
	body := envVal(env, KeyBody)
	ts := int64Val(env, KeyTimestamp)
	mid := byteVal(env, KeyMessageID)

	roomStr := strings.ToLower(string(room))
	nickStr := string(nick)
	srcHex := hexString(src)

	switch msgType {
	case TypeHello:
		h.handleHello(src, nick, body)

	case TypeJoin:
		h.handleJoin(src, nick, room, body)

	case TypeMsg:
		var textStr string
		switch b := body.(type) {
		case []byte:
			textStr = string(b)
		case string:
			textStr = b
		}
		msg := &RRCMessage{
			Kind: "msg",
			Room: roomStr,
			Src:  src,
			Nick: nickStr,
			Text: textStr,
			Ts:   ts,
		}
		h.recordMessage(msg, false)
		if h.Manager != nil && h.Manager.messageCallback != nil {
			h.Manager.messageCallback(h, msg)
		}

		h.echoMessage(src, room, nick, body, mid, ts)

	case TypePart:
		h.handlePart(src, nick, room, body)

	case TypeJoined:
		h.lock.Lock()
		if h.Members[roomStr] == nil {
			h.Members[roomStr] = make(map[string]bool)
		}
		h.Members[roomStr][nickStr] = true
		h.Nicks[srcHex] = nickStr
		h.lock.Unlock()

	case TypeParted:
		h.lock.Lock()
		if h.Members[roomStr] != nil {
			delete(h.Members[roomStr], nickStr)
		}
		h.lock.Unlock()

	case TypeWelcome:
		h.handleWelcome(body)

	case TypePong:
		h.lock.Lock()
		var bodyStr string
		switch b := body.(type) {
		case []byte:
			bodyStr = string(b)
		case string:
			bodyStr = b
		}
		delete(h.pendingPings, bodyStr)
		h.lock.Unlock()

	case TypePing:
		h.sendEnv(MakeEnvelope(TypePong, src, nil, nil, body, mid, NowMs()))

	case TypeNotice:
		var textStr string
		switch b := body.(type) {
		case []byte:
			textStr = string(b)
		case string:
			textStr = b
		}
		msg := &RRCMessage{
			Kind: "notice",
			Room: roomStr,
			Src:  src,
			Nick: nickStr,
			Text: textStr,
			Ts:   ts,
		}
		h.recordMessage(msg, false)
	}
}

// intVal extracts an int value from a CBOR-decoded map, handling both
// int and uint64 key types produced by fxamacker/cbor.
func intVal(env map[any]any, key int) int {
	if v, ok := env[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
		if u, ok := v.(uint64); ok {
			return int(u)
		}
	}
	if v, ok := env[uint64(key)]; ok {
		if i, ok := v.(int); ok {
			return i
		}
		if u, ok := v.(uint64); ok {
			return int(u)
		}
	}
	return 0
}

// envVal extracts a value from a CBOR-decoded map using both int and
// uint64 key variants, since fxamacker/cbor decodes integer keys as uint64.
func envVal(env map[any]any, key int) any {
	if v, ok := env[key]; ok {
		return v
	}
	if v, ok := env[uint64(key)]; ok {
		return v
	}
	return nil
}

// byteVal extracts a []byte value from a CBOR-decoded map.
func byteVal(env map[any]any, key int) []byte {
	if v, ok := env[key]; ok {
		return toBytes(v)
	}
	if v, ok := env[uint64(key)]; ok {
		return toBytes(v)
	}
	return nil
}

// int64Val extracts an int64 value from a CBOR-decoded map.
func int64Val(env map[any]any, key int) int64 {
	if v, ok := env[key]; ok {
		return toInt64(v)
	}
	if v, ok := env[uint64(key)]; ok {
		return toInt64(v)
	}
	return 0
}

func toBytes(v any) []byte {
	switch b := v.(type) {
	case []byte:
		return b
	case string:
		return []byte(b)
	}
	return nil
}

func toInt64(v any) int64 {
	switch i := v.(type) {
	case int:
		return int64(i)
	case int64:
		return i
	case uint64:
		return int64(i)
	}
	return 0
}

// handleWelcome processes a WELCOME envelope from the hub server.
func (h *RRCHub) handleWelcome(body any) {
	bodyMap, ok := body.(map[any]any)
	if !ok {
		return
	}

	h.lock.Lock()
	h.Welcomed = true
	h.lock.Unlock()

	if hubName, ok := bodyMap[BWelcomeHub].([]byte); ok {
		h.lock.Lock()
		h.HubName = string(hubName)
		h.lock.Unlock()
	}
	if hubVer, ok := bodyMap[BWelcomeVer].([]byte); ok {
		h.lock.Lock()
		h.HubVersion = string(hubVer)
		h.lock.Unlock()
	}
	if caps, ok := bodyMap[BWelcomeCaps].(map[any]any); ok {
		h.lock.Lock()
		h.HubCaps = caps
		h.lock.Unlock()
	}
	if limits, ok := bodyMap[BWelcomeLimits].(map[any]any); ok {
		if v, ok := limits[LMaxNickBytes].(int); ok {
			h.MaxNickBytes = v
		}
		if v, ok := limits[LMaxRoomNameBytes].(int); ok {
			h.MaxRoomNameBytes = v
		}
		if v, ok := limits[LMaxMsgBodyBytes].(int); ok {
			h.MaxMsgBodyBytes = v
		}
		if v, ok := limits[LMaxRoomsPerSession].(int); ok {
			h.MaxRoomsPerSession = v
		}
		if v, ok := limits[LRateLimitMsgsPerMinute].(int); ok {
			h.RateLimitMsgsPerMin = v
		}
	}

	if h.Manager != nil {
		h.Manager.OnWelcome(h)
	}
}

// handleHello processes a HELLO envelope from a connecting client.
// It stores the client's nick and sends a WELCOME response. Matches
// Python's RRC server behavior when receiving a HELLO from a client.
func (h *RRCHub) handleHello(src, nick []byte, body any) {
	srcHex := hexString(src)
	nickStr := string(nick)

	h.lock.Lock()
	if nickStr != "" {
		h.Nicks[srcHex] = nickStr
	}
	hubName := h.Name
	h.lock.Unlock()

	welcomeBody := map[any]any{
		BWelcomeHub:  []byte(hubName),
		BWelcomeVer:  []byte("0.1"),
		BWelcomeCaps: map[any]any{},
		BWelcomeLimits: map[any]any{
			LMaxNickBytes:           DefaultMaxNickBytes,
			LMaxRoomNameBytes:       DefaultMaxRoomBytes,
			LMaxMsgBodyBytes:        DefaultMaxMsgBytes,
			LMaxRoomsPerSession:     DefaultMaxRooms,
			LRateLimitMsgsPerMinute: DefaultRatePerMinute,
		},
	}

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeWelcome, nil, nil, nil, welcomeBody, mid, ts)
	h.sendEnv(env)
}

// handleJoin processes a JOIN envelope. On the server side, it
// broadcasts a JOINED notification back to the joining client with
// the joiner's identity hash and nick. On the client side, JOINED
// is handled in the main HandleData switch.
func (h *RRCHub) handleJoin(src, nick, room []byte, body any) {
	roomStr := strings.ToLower(string(room))
	nickStr := string(nick)

	h.lock.Lock()
	if h.Members[roomStr] == nil {
		h.Members[roomStr] = make(map[string]bool)
	}
	h.Members[roomStr][nickStr] = true
	h.Nicks[hexString(src)] = nickStr
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeJoined, src, []byte(roomStr), nick, src, mid, ts)
	h.sendEnv(env)
}

// handlePart processes a PART envelope. On the server side, it
// broadcasts a PARTED notification back to the parting client.
func (h *RRCHub) handlePart(src, nick, room []byte, body any) {
	roomStr := strings.ToLower(string(room))
	nickStr := string(nick)

	h.lock.Lock()
	if h.Members[roomStr] != nil {
		delete(h.Members[roomStr], nickStr)
	}
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeParted, src, []byte(roomStr), nick, src, mid, ts)
	h.sendEnv(env)
}

// echoMessage echoes a received MSG envelope back on the link.
// This simulates the rrcd server broadcasting messages to connected
// clients. The echoed message uses the same source, room, nick,
// and body so that other clients can display it.
func (h *RRCHub) echoMessage(src, room, nick []byte, body any, mid []byte, ts int64) {
	env := MakeEnvelope(TypeMsg, src, room, nick, body, mid, ts)
	h.sendEnv(env)
}

func (h *RRCHub) recordMessage(msg *RRCMessage, local bool) {
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
	h.appendHistory(room, msg)
}

func (h *RRCHub) appendHistory(room string, msg *RRCMessage) {
	if h.savedHistoryPath == "" {
		return
	}

	room = strings.ToLower(room)
	entry := msg.HistoryEntry()
	data, err := cbor.Marshal(entry)
	if err != nil {
		return
	}

	path := h.historyPath(room)
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(data)
}

func (h *RRCHub) deleteHistory(room string) {
	if h.savedHistoryPath == "" {
		return
	}
	path := h.historyPath(room)
	_ = os.Remove(path)
}

func (h *RRCHub) historyPath(room string) string {
	if h.savedHistoryPath == "" {
		return ""
	}
	sanitized := sanitizeRoomName(room)
	hash := sha256.Sum256([]byte(room))
	prefix := fmt.Sprintf("%x", hash[:4])
	return filepath.Join(h.savedHistoryPath, sanitized+"_"+prefix+".log")
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
