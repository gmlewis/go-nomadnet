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
	"bytes"
	"container/ring"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gmlewis/go-reticulum/rns"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
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
	AvailableRooms map[string]*string         // room → topic or nil

	// Auto-connect options
	AutoReconnect bool
	AutoList      bool
	AutoWho       bool
	NickOverride  string

	// Internal state
	lock                 sync.Mutex
	sentIDs              *ring.Ring           // dedup ring buffer
	pendingPings         map[string]time.Time // body → send time
	pendingJoins         map[string]bool
	pendingParts         map[string]bool
	silentJoins          map[string]bool
	silentWhoRooms       map[string]bool
	resourceExpectations map[string]*resourceExpectation // rid → pending transfer
	savedHistoryPath     string
	transport            rns.Transport
	link                 *rns.Link
	manualDisconnect     bool
	reconnectAttempts    int
	reconnectTimer       *time.Timer
	connectTimeout       time.Duration // override the connect-worker recall deadline (tests)
	onLinkEstablished    func()
	onLinkClosed         func()

	// onSend, when set, is invoked by sendEnv with each outbound envelope
	// before it is encoded and transmitted. It is an observability seam used by
	// tests (and optionally the TUI) to inspect outgoing traffic; it is nil in
	// normal operation.
	onSend func(env map[any]any)

	// lastHistoryClean/cleanLastRemoved are accessed atomically because
	// cleanHistory runs concurrently from the inbound link-callback path
	// (HandleData→recordMessage) and from SendMessage→recordMessage. Python
	// guards these with the GIL; Go ports them to atomic.Int64 so the
	// unlocked check/update in cleanHistory (which mirrors Python's
	// structure) stays race-free without taking h.lock on every message.
	lastHistoryClean atomic.Int64 // unix seconds of last cleanHistory call (Python _last_history_clean)
	cleanLastRemoved atomic.Int64 // unix seconds when cleanup last removed messages

	// Testing seams (nil in production). They mirror Python dependencies that
	// otherwise require a live RNS transport or background goroutine, allowing
	// the connection lifecycle to be exercised deterministically in unit tests.
	afterFunc        func(d time.Duration, f func()) *time.Timer
	connectFn        func() // reconnect fire target; defaults to ConnectAsync
	connectWorkerFn  func() // stubs connectWorker from ConnectAsync
	hasPathFn        func(hash []byte) bool
	requestPathFn    func(hash []byte) error
	recallIdentityFn func(hash []byte) *rns.Identity
	buildDestFn      func(id *rns.Identity) (*rns.Destination, error)
}

// NewHub creates a new RRCHub with default values.
func NewHub(manager *RRCManager, hubHash []byte, destName, name string) *RRCHub {
	if destName == "" {
		destName = DefaultDestName
	}
	if name == "" {
		name = hexString(hubHash)
	}

	// The manager may be nil in tests; the transport (when present) is
	// inherited so a hub created after SetTransport can connect.
	var inheritedTransport rns.Transport
	if manager != nil {
		inheritedTransport = manager.transport
	}
	h := &RRCHub{
		Manager:              manager,
		HubHash:              hubHash,
		DestName:             destName,
		Name:                 name,
		transport:            inheritedTransport,
		Status:               StatusDisconnected,
		StatusText:           "Disconnected",
		MaxNickBytes:         DefaultMaxNickBytes,
		MaxRoomNameBytes:     DefaultMaxRoomBytes,
		MaxMsgBodyBytes:      DefaultMaxMsgBytes,
		MaxRoomsPerSession:   DefaultMaxRooms,
		RateLimitMsgsPerMin:  DefaultRatePerMinute,
		Rooms:                make(map[string]bool),
		Messages:             make(map[string][]*RRCMessage),
		UnreadRooms:          make(map[string]bool),
		MentionRooms:         make(map[string]bool),
		Members:              make(map[string]map[string]bool),
		Nicks:                make(map[string]string),
		AvailableRooms:       make(map[string]*string),
		sentIDs:              ring.New(256),
		pendingPings:         make(map[string]time.Time),
		pendingJoins:         make(map[string]bool),
		pendingParts:         make(map[string]bool),
		silentJoins:          make(map[string]bool),
		silentWhoRooms:       make(map[string]bool),
		resourceExpectations: make(map[string]*resourceExpectation),
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

	link.SetLinkEstablishedCallback(h.onEstablished)
	link.SetLinkClosedCallback(func(l *rns.Link) { h.onClosed() })

	return link.Establish()
}

// onEstablished is the link-established callback, mirroring Python
// RRCHub._on_established: it registers the packet and resource callbacks,
// then sends the initial HELLO envelope.
func (h *RRCHub) onEstablished(l *rns.Link) {
	h.lock.Lock()
	h.Status = StatusConnected
	h.StatusText = "Connected"
	h.Welcomed = false
	cb := h.onLinkEstablished
	h.lock.Unlock()

	l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
		h.HandleData(data)
	})

	// Accept resource transfers via the app callback, mirroring Python's
	// _on_established registration of the resource callbacks.
	_ = l.SetResourceStrategy(rns.AcceptApp)
	l.SetResourceCallback(h.resourceAdvertised)
	l.SetResourceStartedCallback(func(_ *rns.Resource) {})
	l.SetResourceConcludedCallback(h.resourceConcluded)

	h.sendHello(l)

	if cb != nil {
		cb()
	}
}

// onClosed is the link-closed handler, mirroring Python RRCHub._on_closed: it
// clears link-derived state, marks the hub disconnected, and schedules a
// reconnect when auto-reconnect is enabled and the close was not the result of
// a manual disconnect. The optional onLinkClosed seam is fired before the
// reconnect decision.
func (h *RRCHub) onClosed() {
	h.lock.Lock()
	h.link = nil
	h.Welcomed = false
	h.MOTD = ""
	for k := range h.Members {
		delete(h.Members, k)
	}
	for k := range h.resourceExpectations {
		delete(h.resourceExpectations, k)
	}
	for k := range h.pendingJoins {
		delete(h.pendingJoins, k)
	}
	for k := range h.pendingParts {
		delete(h.pendingParts, k)
	}
	for k := range h.silentJoins {
		delete(h.silentJoins, k)
	}
	for k := range h.silentWhoRooms {
		delete(h.silentWhoRooms, k)
	}
	shouldReconnect := h.AutoReconnect && !h.manualDisconnect
	cb := h.onLinkClosed
	h.Status = StatusDisconnected
	h.StatusText = "Disconnected"
	h.lock.Unlock()

	if cb != nil {
		cb()
	}
	// Do not schedule a reconnect after the manager (and its transport) have
	// been torn down: the closed callback is dispatched asynchronously by
	// go-reticulum and can fire after RRCManager.Shutdown returned, so without
	// this guard the reconnect worker would drive a stopped TransportSystem.
	if shouldReconnect && h.Manager != nil && !h.Manager.IsStopped() {
		h.scheduleReconnect()
	}
}

// scheduleReconnect schedules a reconnect attempt after an exponential backoff,
// mirroring Python RRCHub._schedule_reconnect. Each call increments the attempt
// counter and recomputes the backoff; the scheduled fire is a no-op if a manual
// disconnect happened or auto-reconnect was disabled in the meantime.
func (h *RRCHub) scheduleReconnect() {
	h.lock.Lock()
	h.reconnectAttempts++
	backoff := reconnectBackoff(h.reconnectAttempts)
	if h.reconnectTimer != nil {
		h.reconnectTimer.Stop()
	}
	h.Status = StatusDisconnected
	h.StatusText = "Reconnect in " + strconv.Itoa(int(backoff.Seconds())) + "s"
	afterFunc := h.afterFunc
	connectFn := h.connectFn
	h.lock.Unlock()

	fire := func() {
		h.lock.Lock()
		h.reconnectTimer = nil
		proceed := h.AutoReconnect && !h.manualDisconnect
		h.lock.Unlock()
		if !proceed {
			return
		}
		// A shutdown between arming and firing must not launch a connectWorker
		// against the now-stopped transport.
		if h.Manager != nil && h.Manager.IsStopped() {
			return
		}
		if connectFn != nil {
			connectFn()
		} else {
			h.ConnectAsync()
		}
	}

	if afterFunc != nil {
		h.lock.Lock()
		h.reconnectTimer = afterFunc(backoff, fire)
		h.lock.Unlock()
	} else {
		h.lock.Lock()
		h.reconnectTimer = time.AfterFunc(backoff, fire)
		h.lock.Unlock()
	}
}

// reconnectBackoff computes the reconnect delay for the given (post-increment)
// attempt count, matching Python's backoff = min(60.0, max(1.0, 2.0 ** min(attempts, 6))).
func reconnectBackoff(attempts int) time.Duration {
	exp := max(min(attempts, 6), 0)
	secs := min(
		// 2 ** exp
		max(

			1<<uint(exp), 1), 60)
	return time.Duration(secs) * time.Second
}

// ConnectAsync initiates a connection to the hub's destination asynchronously,
// mirroring Python RRCHub.connect: it is a no-op when already connecting or
// connected, clears the manual-disconnect flag, cancels any pending reconnect
// timer, sets the status to Connecting, and launches the connect worker. The
// transport must have been configured via SetTransport.
func (h *RRCHub) ConnectAsync() {
	// Refuse to (re)connect once the manager has been shut down: the transport
	// is gone and a connectWorker would dereference a stopped TransportSystem.
	if h.Manager != nil && h.Manager.IsStopped() {
		return
	}
	h.lock.Lock()
	if h.Status == StatusConnecting || h.Status == StatusConnected {
		h.lock.Unlock()
		return
	}
	h.manualDisconnect = false
	if h.reconnectTimer != nil {
		h.reconnectTimer.Stop()
		h.reconnectTimer = nil
	}
	text := "Connecting"
	if h.reconnectAttempts > 0 {
		text = "Reconnecting (attempt " + strconv.Itoa(h.reconnectAttempts) + ")"
	}
	h.Status = StatusConnecting
	h.StatusText = text
	workerFn := h.connectWorkerFn
	h.lock.Unlock()

	if workerFn != nil {
		go workerFn()
	} else {
		go h.connectWorker()
	}
}

// SetTransport configures the RNS transport used by the async connection
// worker (ConnectAsync). The synchronous Connect entry takes its transport
// argument directly; this setter is for the parameterless Python-style path.
func (h *RRCHub) SetTransport(ts rns.Transport) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.transport = ts
}

// connectWorker resolves the hub destination and establishes an RNS link to it,
// mirroring Python RRCHub._connect_worker: it ensures a path is known, recalls
// the hub identity, builds the destination from the configured destination name,
// verifies the resolved hash matches the stored hub hash, then establishes a
// link with the established/closed callbacks wired. On any resolution failure it
// sets the FAILED status with a diagnostic message.
func (h *RRCHub) connectWorker() {
	defer func() {
		if r := recover(); r != nil {
			h.SetStatus(StatusFailed, "Connect error: "+fmt.Sprintf("%v", r))
		}
	}()

	hubHash := h.HubHash

	hasPath := func(hash []byte) bool {
		if fn := h.hasPathFn; fn != nil {
			return fn(hash)
		}
		h.lock.Lock()
		ts := h.transport
		h.lock.Unlock()
		return ts != nil && ts.HasPath(hash)
	}
	requestPath := func(hash []byte) error {
		if fn := h.requestPathFn; fn != nil {
			return fn(hash)
		}
		h.lock.Lock()
		ts := h.transport
		h.lock.Unlock()
		if ts == nil {
			return errors.New("no transport configured")
		}
		return ts.RequestPath(hash)
	}
	recallIdentity := func(hash []byte) *rns.Identity {
		if fn := h.recallIdentityFn; fn != nil {
			return fn(hash)
		}
		h.lock.Lock()
		ts := h.transport
		h.lock.Unlock()
		if ts == nil {
			return nil
		}
		return rns.RecallIdentity(ts, hash)
	}
	buildDest := func(id *rns.Identity) (*rns.Destination, error) {
		if fn := h.buildDestFn; fn != nil {
			return fn(id)
		}
		return h.buildDestination(id)
	}

	const timeout = 20 * time.Second
	h.lock.Lock()
	override := h.connectTimeout
	h.lock.Unlock()
	recallTimeout := timeout
	if override > 0 {
		recallTimeout = override
	}
	deadline := time.Now().Add(recallTimeout)

	if !hasPath(hubHash) {
		_ = requestPath(hubHash)
		pathDeadline := time.Now().Add(5 * time.Second)
		if override > 0 && override < 5*time.Second {
			pathDeadline = time.Now().Add(override)
		}
		if pathDeadline.After(deadline) {
			pathDeadline = deadline
		}
		for time.Now().Before(pathDeadline) {
			if hasPath(hubHash) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	var hubIdentity *rns.Identity
	for time.Now().Before(deadline) {
		hubIdentity = recallIdentity(hubHash)
		if hubIdentity != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if hubIdentity == nil {
		h.SetStatus(StatusFailed, "Hub identity unknown")
		return
	}

	hubDest, err := buildDest(hubIdentity)
	if err != nil {
		h.SetStatus(StatusFailed, "Connect error: "+err.Error())
		return
	}

	if !bytes.Equal(hubDest.Hash, hubHash) {
		h.SetStatus(StatusFailed, "Hash/destination name mismatch")
		return
	}

	ts := h.transport
	if ts == nil {
		h.SetStatus(StatusFailed, "Connect error: no transport configured")
		return
	}

	if err := h.establishLink(ts, hubDest); err != nil {
		h.SetStatus(StatusFailed, "Connect error: "+err.Error())
	}
}

// buildDestination constructs the RNS destination for this hub from the
// configured destination name, mirroring Python's
// RNS.Destination.app_and_aspects_from_name + RNS.Destination(...) call: the
// name is split on "." into an app name followed by zero or more aspects.
func (h *RRCHub) buildDestination(id *rns.Identity) (*rns.Destination, error) {
	parts := strings.Split(h.DestName, ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("empty destination name")
	}
	appName := parts[0]
	aspects := parts[1:]
	h.lock.Lock()
	ts := h.transport
	h.lock.Unlock()
	if ts == nil {
		return nil, errors.New("no transport configured")
	}
	return rns.NewDestination(ts, id, rns.DestinationOut, rns.DestinationSingle, appName, aspects...)
}

// establishLink creates and starts an RNS link to dest, wiring the established
// and closed callbacks. It is the shared tail of both the synchronous Connect
// entry and the async connectWorker.
func (h *RRCHub) establishLink(ts rns.Transport, dest *rns.Destination) error {
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		return err
	}

	h.lock.Lock()
	h.link = link
	h.Status = StatusConnecting
	h.StatusText = "Connecting"
	h.lock.Unlock()

	link.SetLinkEstablishedCallback(h.onEstablished)
	link.SetLinkClosedCallback(func(l *rns.Link) { h.onClosed() })

	return link.Establish()
}

// sendHello sends a HELLO envelope on the given link. Matches Python's
// RRCHub._send_hello which sends client name, version, caps, and nick.
func (h *RRCHub) sendHello(_ *rns.Link) {
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

// packetWouldFit reports whether payload, packed as a link data packet, would
// fit within the link MTU — mirroring Python RRC._packet_would_fit, which
// builds RNS.Packet(link, payload) and attempts to pack it. Pack fails when
// the packed size (header plus the encrypted ciphertext) exceeds the MTU, so
// a nil error means the payload fits.
func (h *RRCHub) packetWouldFit(link *rns.Link, payload []byte) bool {
	if link == nil {
		return false
	}
	p := rns.NewPacketWithTransport(link.GetTransport(), link, payload)
	return p.Pack() == nil
}

// Disconnect tears down the RNS link and resets hub status, mirroring Python
// RRCHub.disconnect: it marks the disconnect as manual (so onClosed will not
// schedule a reconnect), resets the attempt counter, cancels any pending
// reconnect timer, and tears down the active link.
func (h *RRCHub) Disconnect() {
	h.lock.Lock()
	h.manualDisconnect = true
	h.reconnectAttempts = 0
	if h.reconnectTimer != nil {
		h.reconnectTimer.Stop()
		h.reconnectTimer = nil
	}
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

// SetAutoReconnect toggles automatic reconnection, mirroring Python
// RRCHub.set_auto_reconnect: when disabled it cancels any pending reconnect
// timer, persists the change to disk when save is true, and notifies the
// manager of the change.
func (h *RRCHub) SetAutoReconnect(enabled, save bool) {
	h.lock.Lock()
	h.AutoReconnect = enabled
	if !enabled && h.reconnectTimer != nil {
		h.reconnectTimer.Stop()
		h.reconnectTimer = nil
	}
	mgr := h.Manager
	h.lock.Unlock()

	if save && mgr != nil {
		_ = mgr.Save()
	}
	if mgr != nil {
		mgr.NotifyChange(h)
	}
}

// SetAutoList toggles the auto-list option, persisting and notifying, mirroring
// Python RRCHub.set_auto_list.
func (h *RRCHub) SetAutoList(enabled, save bool) {
	h.lock.Lock()
	h.AutoList = enabled
	mgr := h.Manager
	h.lock.Unlock()

	if save && mgr != nil {
		_ = mgr.Save()
	}
	if mgr != nil {
		mgr.NotifyChange(h)
	}
}

// SetAutoWho toggles the auto-who option, persisting and notifying, mirroring
// Python RRCHub.set_auto_who.
func (h *RRCHub) SetAutoWho(enabled, save bool) {
	h.lock.Lock()
	h.AutoWho = enabled
	mgr := h.Manager
	h.lock.Unlock()

	if save && mgr != nil {
		_ = mgr.Save()
	}
	if mgr != nil {
		mgr.NotifyChange(h)
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

// GetHubName returns the hub's display name under the hub lock, for the TUI
// HubView adapter (mirrors Python hub.name in Channels._compose_list_widgets).
func (h *RRCHub) GetHubName() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.Name
}

// GetHubStatus returns the hub's connection status under the hub lock, for the
// TUI HubView adapter (mirrors Python hub.status). The int is the Status*
// enum (StatusDisconnected … StatusFailed).
func (h *RRCHub) GetHubStatus() int {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.Status
}

// JoinedRoomList returns the sorted list of joined room names, for the TUI
// HubView adapter (mirrors Python hub.rooms).
func (h *RRCHub) JoinedRoomList() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	out := make([]string, 0, len(h.Rooms))
	for r := range h.Rooms {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// MessageRoomList returns the sorted list of rooms that have message buffers
// (joined rooms get an empty buffer on AddRoom), for the TUI HubView adapter
// (mirrors Python set(hub.messages.keys())).
func (h *RRCHub) MessageRoomList() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	out := make([]string, 0, len(h.Messages))
	for r := range h.Messages {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// UnreadRoomList returns the sorted list of rooms with unread messages, for
// the TUI HubView adapter (mirrors Python hub.unread_rooms).
func (h *RRCHub) UnreadRoomList() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	out := make([]string, 0, len(h.UnreadRooms))
	for r := range h.UnreadRooms {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// MentionRoomList returns the sorted list of rooms with unread mentions, for
// the TUI HubView adapter (mirrors Python hub.mention_rooms).
func (h *RRCHub) MentionRoomList() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	out := make([]string, 0, len(h.MentionRooms))
	for r := range h.MentionRooms {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
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

// SendCommand mirrors Python RRCHub.send_command: it sends a raw command
// string (which must begin with "/") to the hub as a T_MSG envelope. Unlike
// SendMessage it does not normalize the room, record the message locally, or
// track the message ID for dedup — it is a thin send of the command text.
func (h *RRCHub) SendCommand(text, room string) error {
	if !strings.HasPrefix(text, "/") {
		return errors.New("command must start with /")
	}
	mid := MsgID()
	ts := NowMs()
	nick := h.GetEffectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeMsg, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h.sendEnv(env)
	return nil
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
	if h.onSend != nil {
		h.onSend(env)
	}
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
		log.Printf("rrc: dropping envelope send over link: %v", err)
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

	case TypeResourceEnvelope:
		h.recordResourceExpectation(env)
	}
}

// recordResourceExpectation mirrors the T_RESOURCE_ENVELOPE branch of Python
// RRCHub._on_packet: it records a pending resource expectation keyed by the
// resource id, capturing kind, size, sha256, encoding and (lowercased) room,
// expiring after 30 seconds.
func (h *RRCHub) recordResourceExpectation(env map[any]any) {
	body, ok := envVal(env, KeyBody).(map[any]any)
	if !ok {
		return
	}
	rid := byteVal(body, ResKeyID)
	kind, _ := envVal(body, ResKeyKind).(string)
	size := int(int64Val(body, ResKeySize))
	if len(rid) == 0 || kind == "" || size <= 0 {
		return
	}
	sha := byteVal(body, ResKeySHA256)
	encoding, _ := envVal(body, ResKeyEncoding).(string)
	if encoding == "" {
		encoding = "utf-8"
	}
	room := ""
	if r := byteVal(env, KeyRoom); len(r) > 0 {
		room = strings.ToLower(strings.TrimSpace(string(r)))
	}
	h.lock.Lock()
	h.resourceExpectations[string(rid)] = &resourceExpectation{
		kind:     kind,
		size:     size,
		sha256:   sha,
		encoding: encoding,
		room:     room,
		expires:  time.Now().Add(30 * time.Second),
	}
	h.lock.Unlock()
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

	// fxamacker/cbor decodes integer map keys as uint64, so every lookup must
	// go through the int/uint64-tolerant helpers (byteVal/intVal/envVal).
	// Direct bodyMap[intKey] indexing silently misses every field.
	if hubName := byteVal(bodyMap, BWelcomeHub); hubName != nil {
		h.lock.Lock()
		h.HubName = string(hubName)
		h.lock.Unlock()
	}
	if hubVer := byteVal(bodyMap, BWelcomeVer); hubVer != nil {
		h.lock.Lock()
		h.HubVersion = string(hubVer)
		h.lock.Unlock()
	}
	if caps, ok := envVal(bodyMap, BWelcomeCaps).(map[any]any); ok {
		h.lock.Lock()
		h.HubCaps = caps
		h.lock.Unlock()
	}
	if limits, ok := envVal(bodyMap, BWelcomeLimits).(map[any]any); ok {
		h.MaxNickBytes = intVal(limits, LMaxNickBytes)
		h.MaxRoomNameBytes = intVal(limits, LMaxRoomNameBytes)
		h.MaxMsgBodyBytes = intVal(limits, LMaxMsgBodyBytes)
		h.MaxRoomsPerSession = intVal(limits, LMaxRoomsPerSession)
		h.RateLimitMsgsPerMin = intVal(limits, LRateLimitMsgsPerMinute)
	}

	if h.Manager != nil {
		h.Manager.OnWelcome(h)
	}
}

// handleHello processes a HELLO envelope from a connecting client.
// It stores the client's nick and sends a WELCOME response. Matches
// Python's RRC server behavior when receiving a HELLO from a client.
func (h *RRCHub) handleHello(src, nick []byte, _ any) {
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
func (h *RRCHub) handleJoin(src, nick, room []byte, _ any) {
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
func (h *RRCHub) handlePart(src, nick, room []byte, _ any) {
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
	room := strings.ToLower(msg.Room)
	if room == "" {
		h.lock.Lock()
		// Global notice
		h.Notices = append(h.Notices, msg)
		if len(h.Notices) > 100 {
			h.Notices = h.Notices[len(h.Notices)-100:]
		}
		h.lock.Unlock()
		return
	}

	// Snapshot the manager's active room BEFORE acquiring h.lock so the lock
	// order stays manager.lock → hub.lock (SetActive/Save/HasUnread acquire in
	// that order). Calling ActiveRoomFor (which takes manager.lock) while
	// holding h.lock would invert that order and AB/BA-deadlock against
	// SetActive. A TOCTOU here can at worst transiently mis-flag the unread
	// indicator for a message whose room is switched to/from active as it
	// arrives — cosmetic, not a correctness hazard.
	var activeRoom string
	if !local && h.Manager != nil {
		activeRoom = h.Manager.ActiveRoomFor(h)
	}

	h.lock.Lock()
	// Cap message buffer at 256
	msgs := h.Messages[room]
	if len(msgs) >= 256 {
		msgs = msgs[len(msgs)-255:]
	}
	h.Messages[room] = append([]*RRCMessage{msg}, msgs...)

	// Mark unread for non-local messages
	if !local && h.Manager != nil {
		if activeRoom != room {
			h.UnreadRooms[room] = true
			if msg.Mention {
				h.MentionRooms[room] = true
			}
		}
	}
	h.lock.Unlock()

	// Append to history and clean up — outside the lock, mirroring Python,
	// which calls _append_history and _clean_history after the `with self._lock`
	// block. cleanHistory acquires the lock itself.
	h.appendHistory(room, msg)
	h.cleanHistory()
}

// perRoomCap mirrors Python RRCHub._per_room_cap: it returns the configured
// per-room history cap, or 0 when no cap is set (Python returns None). The
// value comes from the manager, which reads rrc_history_per_room_cap from the
// app config.
func (h *RRCHub) perRoomCap() int {
	if h.Manager == nil {
		return 0
	}
	return h.Manager.HistoryPerRoomCap()
}

// filterHistory mirrors Python RRCHub._filter_history: whether system/notice
// messages are dropped when loading history from disk. Defaults to true.
func (h *RRCHub) filterHistory() bool {
	if h.Manager == nil {
		return true
	}
	return h.Manager.FilterLoadedHistory()
}

// ephemeralNoticesHistory mirrors Python RRCHub._ephemeral_notices_history:
// the age in seconds after which ephemeral system/notice messages are removed
// by the periodic cleanup. Defaults to SYS_NOTICE_TIMEOUT.
func (h *RRCHub) ephemeralNoticesHistory() int {
	if h.Manager == nil {
		return NoticeTimeout
	}
	return h.Manager.EphemeralNotices()
}

// cleanHistory mirrors Python RRCHub._clean_history. At most once per
// CLEAN_HISTORY_INTERVAL seconds it scans every room's message buffer and
// removes system/notice messages older than the ephemeral-notices timeout.
func (h *RRCHub) cleanHistory() {
	now := time.Now().Unix()
	cleaned := false
	removeAfter := int64(h.ephemeralNoticesHistory())
	if now > h.lastHistoryClean.Load()+CleanHistoryInterval {
		h.lock.Lock()
		for r := range h.Messages {
			kept := h.Messages[r][:0]
			removed := false
			for _, m := range h.Messages[r] {
				shouldFilter := m.Kind == "system" || m.Kind == "notice"
				if shouldFilter {
					age := now - m.Ts/1000
					if age > removeAfter {
						removed = true
						continue
					}
				}
				kept = append(kept, m)
			}
			if removed {
				h.Messages[r] = kept
				cleaned = true
			}
		}
		h.lock.Unlock()
	}
	h.lastHistoryClean.Store(now)
	if cleaned {
		h.cleanLastRemoved.Store(now)
	}
}

// recordNotice mirrors Python RRCHub._record_notice. A notice is appended to
// the global notices list (capped at 200) and, when it has a target room, to
// that room's message buffer (capped at perRoomCap), marked unread when the
// room is not active, persisted to history, and followed by a history cleanup.
func (h *RRCHub) recordNotice(msg *RRCMessage) {
	targetRoom := strings.ToLower(msg.Room)

	// Snapshot the active room BEFORE acquiring h.lock so the lock order stays
	// manager.lock → hub.lock (SetActive/Save/HasUnread acquire in that order).
	// Calling ActiveRoomFor (which takes manager.lock) while holding h.lock
	// would invert that order and AB/BA-deadlock against SetActive. The same
	// snapshot serves the no-target-room fallback below and the unread check
	// inside the lock; a TOCTOU can at worst transiently mis-flag the unread
	// indicator — cosmetic, not a correctness hazard.
	var activeRoom string
	if h.Manager != nil {
		activeRoom = strings.ToLower(h.Manager.ActiveRoomFor(h))
	}
	if targetRoom == "" && activeRoom != "" {
		targetRoom = activeRoom
		msg.Room = activeRoom
	}

	cap := h.perRoomCap()
	h.lock.Lock()
	h.Notices = append(h.Notices, msg)
	if len(h.Notices) > 200 {
		h.Notices = h.Notices[len(h.Notices)-200:]
	}
	if targetRoom != "" {
		buf := h.Messages[targetRoom]
		if buf == nil {
			buf = make([]*RRCMessage, 0)
		}
		buf = append(buf, msg)
		if cap > 0 && len(buf) > cap {
			buf = buf[len(buf)-cap:]
		}
		h.Messages[targetRoom] = buf
		if h.Manager != nil && targetRoom != activeRoom {
			h.UnreadRooms[targetRoom] = true
		}
	}
	notify := h.Manager != nil
	h.lock.Unlock()

	if notify {
		h.Manager.NotifyMessage(h, msg)
	}
	if targetRoom != "" {
		h.appendHistory(targetRoom, msg)
		h.cleanHistory()
	}
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

// persistableRoom mirrors Python RRCHub._persistable_room: a room is persistable
// when it is a non-empty string other than the "*" catch-all.
func persistableRoom(room string) bool {
	return room != "" && room != "*"
}

// loadHistory mirrors Python RRCHub._load_history. For each room that currently
// has a message buffer, it reads the per-room CBOR history file, keeps only the
// last perRoomCap entries (truncating at the first decode error), drops
// system/notice entries when the loaded-history filter is enabled, and
// replaces the in-memory buffer with the result.
func (h *RRCHub) loadHistory() {
	h.lock.Lock()
	rooms := make([]string, 0, len(h.Messages))
	for r := range h.Messages {
		rooms = append(rooms, r)
	}
	h.lock.Unlock()

	cap := h.perRoomCap()
	filter := h.filterHistory()

	for _, room := range rooms {
		if !persistableRoom(room) {
			continue
		}
		path := h.historyPath(room)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		var window []*RRCMessage
		dec := cbor.NewDecoder(f)
		for {
			var entry map[string]any
			if err := dec.Decode(&entry); err != nil {
				break // EOF or decode error: stop, keeping the valid prefix
			}
			m := DecodeHistoryEntry(entry)
			if m == nil {
				continue
			}
			m.Room = room
			window = append(window, m)
			if cap > 0 && len(window) > cap {
				window = window[len(window)-cap:]
			}
		}
		_ = f.Close()

		msgs := make([]*RRCMessage, 0, len(window))
		for _, m := range window {
			if filter && (m.Kind == "system" || m.Kind == "notice") {
				continue
			}
			msgs = append(msgs, m)
		}

		h.lock.Lock()
		h.Messages[room] = msgs
		h.lock.Unlock()
	}
}

func (h *RRCHub) historyPath(room string) string {
	if h.savedHistoryPath == "" {
		return ""
	}
	room = strings.ToLower(room)
	sanitized := sanitizeRoomName(room)
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	hash := sha256.Sum256([]byte(room))
	prefix := fmt.Sprintf("%x", hash[:4]) // 8 hex chars, matching Python's [:8]
	var filename string
	if sanitized != "" {
		filename = sanitized + "_" + prefix + ".log"
	} else {
		filename = prefix + ".log"
	}
	return filepath.Join(h.savedHistoryPath, filename)
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

// resourceExpectation records a pending inbound resource transfer announced via
// a T_RESOURCE_ENVELOPE, mirroring Python RRCHub._resource_expectations. It is
// matched (by size) and consumed when the transfer concludes.
type resourceExpectation struct {
	kind     string
	size     int
	sha256   []byte
	encoding string
	room     string
	expires  time.Time
}

// resourceAdvertised mirrors Python RRCHub._resource_advertised: it accepts an
// incoming resource advertisement when its data size is within the 262144-byte
// cap, and rejects larger transfers.
func (h *RRCHub) resourceAdvertised(adv *rns.ResourceAdvertisement) bool {
	if adv == nil {
		return false
	}
	size := adv.D
	if size == 0 {
		size = adv.T
	}
	return size <= 262144
}

// resourceConcluded mirrors Python RRCHub._resource_concluded: on a completed
// transfer it passes the received data to the testable core handler. Non-complete
// resources are dropped.
func (h *RRCHub) resourceConcluded(resource *rns.Resource) {
	if resource == nil || resource.Status() != rns.ResourceStatusComplete {
		return
	}
	h.handleConcludedResource(resource.Data())
}

// handleConcludedResource is the testable core of _resource_concluded: it
// matches the received data against a pending resource expectation (by size,
// purging expired ones first), verifies the sha256 when present, and for
// notice/MOTD kinds decodes the text and records it (MOTD also updates the hub
// motd and fires a change notification).
func (h *RRCHub) handleConcludedResource(data []byte) {
	if len(data) == 0 {
		return
	}

	now := time.Now()
	h.lock.Lock()
	for k, exp := range h.resourceExpectations {
		if now.After(exp.expires) {
			delete(h.resourceExpectations, k)
		}
	}
	var matched *resourceExpectation
	for k, exp := range h.resourceExpectations {
		if exp.size == len(data) {
			matched = exp
			delete(h.resourceExpectations, k)
			break
		}
	}
	h.lock.Unlock()

	kind := ResKindBlob
	room := ""
	encoding := "utf-8"
	var sha []byte
	if matched != nil {
		kind = matched.kind
		room = matched.room
		encoding = matched.encoding
		sha = matched.sha256
	}
	if len(sha) > 0 {
		sum := sha256.Sum256(data)
		if !bytes.Equal(sum[:], sha) {
			return
		}
	}

	if kind != ResKindNotice && kind != ResKindMOTD {
		return
	}
	text := decodeText(data, encoding)
	if kind == ResKindMOTD {
		h.lock.Lock()
		h.MOTD = text
		h.lock.Unlock()
		if h.Manager != nil {
			h.Manager.NotifyChange(h)
		}
	}
	h.recordNotice(&RRCMessage{Kind: "notice", Room: room, Text: text, Ts: NowMs()})
}

// decodeText decodes data using the given charset name with U+FFFD replacement
// for invalid bytes, mirroring Python's data.decode(encoding, errors="replace").
// Unknown or unavailable encodings fall back to UTF-8-with-replacement.
func decodeText(data []byte, encoding string) string {
	if encoding == "" {
		encoding = "utf-8"
	}
	encoding = strings.ToLower(encoding)
	if canon, ok := encodingAliases[encoding]; ok {
		encoding = canon
	}
	if e, err := htmlindex.Get(encoding); err == nil {
		if s, _, err := transform.String(e.NewDecoder(), string(data)); err == nil {
			return s
		}
	}
	return strings.ToValidUTF8(string(data), "�")
}

// encodingAliases maps Python codec names that Go's htmlindex does not
// recognize to their WHATWG canonical names, so decodeText matches Python's
// data.decode for the common charsets.
var encodingAliases = map[string]string{
	"latin-1": "iso-8859-1",
	"latin1":  "iso-8859-1",
	"ascii":   "us-ascii",
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

// partedRoomKeys mirrors Python RRCManager.save, where parted_rooms is derived
// as sorted(set(h.messages.keys()) - joined): every room that has a message
// buffer but is not currently joined. Both maps must already be under the hub
// lock when this is called.
func partedRoomKeys(messages map[string][]*RRCMessage, joined map[string]bool) []string {
	keys := make([]string, 0, len(messages))
	for r := range messages {
		if !joined[r] {
			keys = append(keys, r)
		}
	}
	sort.Strings(keys)
	return keys
}
