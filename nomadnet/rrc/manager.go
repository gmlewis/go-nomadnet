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
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/gmlewis/go-reticulum/rns"
)

// RRCManager manages multiple RRC hub connections and persistence.
type RRCManager struct {
	Hubs []*RRCHub

	storagePath    string
	identity       *rns.Identity
	identityHashFn func() []byte

	// History config, mirroring the Python hub's getattr(self.manager.app, …)
	// fallbacks: rrc_history_per_room_cap (0 = no cap), rrc_filter_loaded_history
	// (default true), rrc_ephemeral_notices (default SYS_NOTICE_TIMEOUT seconds).
	historyPerRoomCap    int
	filterLoadedHistory  bool
	ephemeralNoticesSecs int

	lock            sync.Mutex
	changeCallback  func()
	messageCallback func(hub *RRCHub, msg *RRCMessage)
	activeHub       *RRCHub
	activeRoom      string
	loaded          bool
	nickname        string
}

// NewManager creates a new RRCManager rooted at the given storage path.
func NewManager(storagePath string, identityHashFn func() []byte) *RRCManager {
	return &RRCManager{
		storagePath:          storagePath,
		identityHashFn:       identityHashFn,
		Hubs:                 make([]*RRCHub, 0),
		filterLoadedHistory:  true,
		ephemeralNoticesSecs: NoticeTimeout,
	}
}

// SetHistoryConfig configures the per-room message-history cap, whether loaded
// history is filtered (system/notice messages dropped on load), and how long
// ephemeral system/notice messages survive the periodic cleanup — mirroring
// Python's rrc_history_per_room_cap, rrc_filter_loaded_history and
// rrc_ephemeral_notices app attributes. A perRoomCap <= 0 disables the cap.
func (m *RRCManager) SetHistoryConfig(perRoomCap int, filterLoaded bool, ephemeralSecs int) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.historyPerRoomCap = perRoomCap
	m.filterLoadedHistory = filterLoaded
	if ephemeralSecs > 0 {
		m.ephemeralNoticesSecs = ephemeralSecs
	}
}

// HistoryPerRoomCap returns the per-room history cap, or 0 when no cap is set
// (matching Python _per_room_cap returning None).
func (m *RRCManager) HistoryPerRoomCap() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.historyPerRoomCap
}

// FilterLoadedHistory reports whether system/notice messages are dropped when
// loading history from disk.
func (m *RRCManager) FilterLoadedHistory() bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.filterLoadedHistory
}

// EphemeralNotices returns the age in seconds after which ephemeral
// system/notice messages are removed by the periodic cleanup.
func (m *RRCManager) EphemeralNotices() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.ephemeralNoticesSecs
}

// SetIdentity sets the local RNS identity, mirroring Python RRCManager, which
// obtains its identity from the owning app (self.app.identity). The identity
// is exposed via Identity and used as the source for outgoing envelopes and
// for link identification.
func (m *RRCManager) SetIdentity(id *rns.Identity) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.identity = id
}

// Identity returns the local RNS identity, mirroring Python's
// RRCManager.identity property (self.app.identity). It returns nil when no
// identity has been configured.
func (m *RRCManager) Identity() *rns.Identity {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.identity
}

// identityHash returns the local identity hash.
func (m *RRCManager) identityHash() []byte {
	if m.identityHashFn != nil {
		return m.identityHashFn()
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.identity != nil {
		return m.identity.Hash
	}
	return nil
}

// GetNickname returns the display nickname.
func (m *RRCManager) GetNickname() string {
	return m.nickname
}

// SetNickname sets the display nickname.
func (m *RRCManager) SetNickname(nick string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.nickname = nick
}

// SetChangeCallback registers a callback for hub state changes.
func (m *RRCManager) SetChangeCallback(fn func()) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.changeCallback = fn
}

// SetMessageCallback registers a callback for new messages.
func (m *RRCManager) SetMessageCallback(fn func(hub *RRCHub, msg *RRCMessage)) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.messageCallback = fn
}

// NotifyChange fires the change callback.
func (m *RRCManager) NotifyChange(hub *RRCHub) {
	m.lock.Lock()
	cb := m.changeCallback
	m.lock.Unlock()
	if cb != nil {
		cb()
	}
}

// NotifyMessage fires the message callback.
func (m *RRCManager) NotifyMessage(hub *RRCHub, msg *RRCMessage) {
	m.lock.Lock()
	cb := m.messageCallback
	m.lock.Unlock()
	if cb != nil {
		cb(hub, msg)
	}
}

// OnWelcome is called when a hub receives a WELCOME packet.
// It re-joins all stored rooms.
func (m *RRCManager) OnWelcome(hub *RRCHub) {
	hub.lock.Lock()
	rooms := sortedKeys(hub.Rooms)
	hub.lock.Unlock()

	for _, room := range rooms {
		hub.JoinRoom(room, false)
	}
}

// SetActive sets the active hub and room.
func (m *RRCManager) SetActive(hub *RRCHub, room string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.activeHub = hub
	m.activeRoom = strings.ToLower(room)
	if hub != nil {
		hub.MarkRead(room)
	}
}

// ActiveRoomFor returns the active room for the given hub.
func (m *RRCManager) ActiveRoomFor(hub *RRCHub) string {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.activeHub == hub {
		return m.activeRoom
	}
	return ""
}

// HasUnread returns true if any hub has unread messages.
func (m *RRCManager) HasUnread() bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, hub := range m.Hubs {
		hub.lock.Lock()
		hasUnread := len(hub.UnreadRooms) > 0
		hub.lock.Unlock()
		if hasUnread {
			return true
		}
	}
	return false
}

// AddHub creates or returns an existing hub for the given hash.
func (m *RRCManager) AddHub(hubHash []byte, destName, name string) *RRCHub {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Check if hub already exists
	for _, h := range m.Hubs {
		if bytesEqual(h.HubHash, hubHash) && h.DestName == destName {
			return h
		}
	}

	hub := NewHub(m, hubHash, destName, name)
	hub.savedHistoryPath = m.historyDir(hub)
	m.Hubs = append(m.Hubs, hub)
	return hub
}

// RemoveHub disconnects and removes a hub.
func (m *RRCManager) RemoveHub(hub *RRCHub) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for i, h := range m.Hubs {
		if h == hub {
			m.Hubs = append(m.Hubs[:i], m.Hubs[i+1:]...)
			break
		}
	}
}

// HubsSnapshot returns a locked copy of the hub slice, for the TUI to render
// the channels list without racing AddHub/RemoveHub mutations. The returned
// slice is a copy; mutating it does not affect the manager.
func (m *RRCManager) HubsSnapshot() []*RRCHub {
	m.lock.Lock()
	defer m.lock.Unlock()
	out := make([]*RRCHub, len(m.Hubs))
	copy(out, m.Hubs)
	return out
}

// FindHub looks up a hub by hash and destination name.
func (m *RRCManager) FindHub(hubHash []byte, destName string) *RRCHub {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, h := range m.Hubs {
		if bytesEqual(h.HubHash, hubHash) {
			if destName == "" || h.DestName == destName {
				return h
			}
		}
	}
	return nil
}

// HubInfo holds the serialized state of a hub for persistence.
type HubInfo struct {
	Hash          []byte   `cbor:"hash"`
	DestName      string   `cbor:"dest_name"`
	Name          string   `cbor:"name"`
	Rooms         []string `cbor:"rooms"`
	PartedRooms   []string `cbor:"parted_rooms"`
	AutoReconnect bool     `cbor:"auto_reconnect"`
	AutoList      bool     `cbor:"auto_list"`
	AutoWho       bool     `cbor:"auto_who"`
	Nick          string   `cbor:"nick,omitempty"`
}

// Save persists all hub configurations to disk.
func (m *RRCManager) Save() error {
	m.lock.Lock()
	hubs := make([]HubInfo, 0, len(m.Hubs))
	for _, h := range m.Hubs {
		h.lock.Lock()
		info := HubInfo{
			Hash:          h.HubHash,
			DestName:      h.DestName,
			Name:          h.Name,
			Rooms:         sortedKeys(h.Rooms),
			PartedRooms:   partedRoomKeys(h.Messages, h.Rooms),
			AutoReconnect: h.AutoReconnect,
			AutoList:      h.AutoList,
			AutoWho:       h.AutoWho,
			Nick:          h.NickOverride,
		}
		h.lock.Unlock()
		hubs = append(hubs, info)
	}
	m.lock.Unlock()

	data := map[string]any{"hubs": hubs}
	encoded, err := cbor.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding hub config: %w", err)
	}

	storePath := m.storePath()
	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating store dir: %w", err)
	}

	tmpPath := storePath + ".tmp"
	if err := os.WriteFile(tmpPath, encoded, 0o644); err != nil {
		return fmt.Errorf("writing hub config: %w", err)
	}

	return os.Rename(tmpPath, storePath)
}

// Load reads hub configurations from disk.
func (m *RRCManager) Load() error {
	if m.loaded {
		return nil
	}
	m.loaded = true

	storePath := m.storePath()
	data, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading hub config: %w", err)
	}

	var raw map[string]any
	if err := cbor.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decoding hub config: %w", err)
	}

	hubsRaw, ok := raw["hubs"].([]any)
	if !ok {
		return nil
	}

	for _, hRaw := range hubsRaw {
		hubMap, ok := hRaw.(map[any]any)
		if !ok {
			continue
		}

		hash, _ := hubMap["hash"].([]byte)
		destName, _ := hubMap["dest_name"].(string)
		name, _ := hubMap["name"].(string)

		hub := m.AddHub(hash, destName, name)

		hub.lock.Lock()
		if v, ok := hubMap["auto_reconnect"].(bool); ok {
			hub.AutoReconnect = v
		}
		if v, ok := hubMap["auto_list"].(bool); ok {
			hub.AutoList = v
		}
		if v, ok := hubMap["auto_who"].(bool); ok {
			hub.AutoWho = v
		}
		if v, ok := hubMap["nick"].(string); ok {
			hub.NickOverride = v
		}
		hub.lock.Unlock()

		// Joined rooms: Python calls hub.add_room(r), which normalizes the
		// name and ensures an empty message buffer exists. AddRoom mirrors that
		// (it lowercases and creates the buffer), so call it unlocked.
		if rooms, ok := hubMap["rooms"].([]any); ok {
			for _, r := range rooms {
				if rs, ok := r.(string); ok {
					hub.AddRoom(rs)
				}
			}
		}
		// Parted rooms: Python does hub.messages.setdefault(rn, []) — the room
		// gets an empty message buffer but is NOT added to the joined set.
		if parted, ok := hubMap["parted_rooms"].([]any); ok {
			for _, r := range parted {
				if rs, ok := r.(string); ok {
					rs = strings.ToLower(strings.TrimSpace(rs))
					if rs == "" {
						continue
					}
					hub.lock.Lock()
					if hub.Messages[rs] == nil {
						hub.Messages[rs] = make([]*RRCMessage, 0)
					}
					hub.lock.Unlock()
				}
			}
		}

		// Load per-room history now that the room buffers exist, mirroring
		// Python's hub._load_history() call at the end of each load entry.
		hub.loadHistory()
	}

	return nil
}

// Shutdown disconnects all hubs.
func (m *RRCManager) Shutdown() {
	m.lock.Lock()
	hubs := make([]*RRCHub, len(m.Hubs))
	copy(hubs, m.Hubs)
	m.lock.Unlock()

	for _, hub := range hubs {
		hub.Disconnect()
	}
}

func (m *RRCManager) storePath() string {
	return filepath.Join(m.storagePath, "rrc_hubs")
}

func (m *RRCManager) historyRoot() string {
	return filepath.Join(m.storagePath, "rrc_history")
}

// historyDir mirrors Python RRCManager._history_dir: the per-hub history
// directory is keyed by the hub hash hex, with a "__<dest_name hash>" suffix
// appended when the hub has a non-default destination name, so hubs sharing a
// hash but differing in dest name keep separate histories.
func (m *RRCManager) historyDir(hub *RRCHub) string {
	hub.lock.Lock()
	defer hub.lock.Unlock()
	key := hexString(hub.HubHash)
	if hub.DestName != "" && hub.DestName != DefaultDestName {
		sum := sha256.Sum256([]byte(hub.DestName))
		key = key + "__" + fmt.Sprintf("%x", sum[:4])
	}
	return filepath.Join(m.historyRoot(), key)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
