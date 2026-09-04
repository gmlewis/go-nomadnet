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
	lock sync.Mutex
	// sentIDs mirrors Python RRCHub._sent_ids (RRC.py:247): the message ids
	// this client itself sent, so the hub's fanout echo of our own message is
	// skipped instead of recorded twice. Capped at 256 like Python's deque.
	sentIDs      map[string]bool
	sentIDsOrder []string
	// recvIDs caps received-message dedup: rrcd's member list can accumulate
	// duplicate joins from reconnect storms, fanning a message out N times.
	recvIDs      map[string]bool
	recvIDsOrder []string
	// fanoutGroups is the client-side collapse of rrcd 0.3.2's per-member
	// message fanout — an intentional, user-ordered deviation from Python
	// (TODO item 4), which renders every copy. rrcd fans each message out
	// once per room member with a unique mid, a per-copy rewritten source
	// hash and a registry-derived (often wrong or missing) nick, so fanout
	// copies of one message share only kind, room, body and approximately the
	// timestamp. Each group records the first copy's timestamp; copies of the
	// same body within fanoutWindowMs of that first arrival collapse into it.
	fanoutGroups map[string]*fanoutGroup
	fanoutOrder  []string
	// recentSentBodies remembers the bodies recently sent by this client so
	// the hub's fanout echoes of our own message (unique mids, possibly
	// rewritten source hashes) collapse with the local record instead of
	// duplicating it. Capped at 256 like Python's deques.
	recentSentBodies map[string]int64
	recentSentOrder  []string
	// pendingPing tracks the pings this client sent and has not had echoed
	// back (Python _pending_pings, RRC.py:254): keyed by the raw ping body,
	// each entry carries the send time for the 15 s expiry and the target
	// room (Python send_ping's room bookkeeping, RRC.py:592-600).
	pendingPings map[string]pendingPing
	pendingJoins map[string]bool
	pendingParts map[string]bool
	silentJoins  map[string]bool
	// silentWhoRooms holds the rooms with an outstanding AUTO-requested /who
	// (the periodic reconciliation sweep and the join-time auto_who). The
	// marker is consumed by EVERY who-reply copy for the room — rrcd fans the
	// who-notice out once per member, so a one-shot delete on the first copy
	// leaks the marker and every duplicate copy renders. It is cleared only
	// when a user-initiated /who for the room is answered, or by the link
	// closing. userWhoRooms is the distinct bookkeeping for USER-initiated
	// /who requests, whose replies must render.
	silentWhoRooms       map[string]bool
	userWhoRooms         map[string]bool
	resourceExpectations map[string]*resourceExpectation // rid → pending transfer
	savedHistoryPath     string
	transport            rns.Transport
	link                 *rns.Link
	manualDisconnect     bool
	reconnectAttempts    int
	reconnectTimer       *time.Timer
	// whoRefreshTimer/whoRefreshPending carry the armed periodic membership
	// reconciliation (scheduleWhoRefresh): the timer is stopped and both
	// fields cleared whenever the link closes or disconnects.
	whoRefreshTimer   *time.Timer
	whoRefreshPending func()
	connectTimeout    time.Duration // override the connect-worker recall deadline (tests)
	onLinkEstablished func()
	onLinkClosed      func()

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

	// serverSide marks hubs that act as the room server (the Go mini-hub used
	// by the cross-process tests). Such hubs are wired via SetLink after an
	// inbound link arrives, and their HandleData fans a received message back
	// out to the room like rrcd's router does. Client hubs never SetLink, so
	// they must not echo — echoing rrcd's fanout copies back would make the
	// hub re-forward them (Python's client has no such echo, RRC.py:1021).
	serverSide bool
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
		sentIDs:              make(map[string]bool),
		recvIDs:              make(map[string]bool),
		fanoutGroups:         make(map[string]*fanoutGroup),
		recentSentBodies:     make(map[string]int64),
		pendingPings:         make(map[string]pendingPing),
		pendingJoins:         make(map[string]bool),
		pendingParts:         make(map[string]bool),
		silentJoins:          make(map[string]bool),
		silentWhoRooms:       make(map[string]bool),
		userWhoRooms:         make(map[string]bool),
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
// used by server-side hubs that receive incoming links; marking the hub
// serverSide here is what enables its router-style fanout of received
// messages (client hubs, wired by the connect worker instead, never echo).
func (h *RRCHub) SetLink(link *rns.Link) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.link = link
	h.serverSide = true
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
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}

	link.SetLinkEstablishedCallback(h.onEstablished)
	link.SetLinkClosedCallback(func(l *rns.Link) { h.onClosed() })

	return link.Establish()
}

// onEstablished is the link-established callback, mirroring Python
// RRCHub._on_established: it registers the packet and resource callbacks,
// then sends the initial HELLO envelope. The status stays CONNECTING
// ("Identified, sending HELLO", Python RRC.py:415) until the WELCOME arrives
// — handleWelcome flips it to CONNECTED.
func (h *RRCHub) onEstablished(l *rns.Link) {
	// SetStatus (not a direct field write) so the change notification fires
	// and the channels hub list refreshes the connecting glyph.
	h.SetStatus(StatusConnecting, "Identified, sending HELLO")
	h.lock.Lock()
	h.Welcomed = false
	cb := h.onLinkEstablished
	h.lock.Unlock()
	// Identify the link to the hub, mirroring Python's _on_established
	// link.identify(self.manager.identity) (RRC.py:411): the hub needs the
	// client's signed identity on the link before it will respond to the
	// hello - without it real hubs silently ignore every packet.
	if h.Manager != nil {
		if id := h.Manager.Identity(); id != nil {
			// Python logs identify failures at ERROR (RRC.py:412-414) and
			// continues - identification is best-effort.
			if err := l.Identify(id); err != nil {
				log.Printf("[RRC %v] identify failed: %v", h.Name, err)
			}
		}
	}
	// Mirror Python's hello thread (RRC.py:421-441): the hello repeats every
	// 3 s, up to 5 attempts, until a WELCOME arrives - the hub may not have
	// registered the fresh link when the first hello lands.
	go h.helloLoop()

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
	for k := range h.userWhoRooms {
		delete(h.userWhoRooms, k)
	}
	if h.whoRefreshTimer != nil {
		h.whoRefreshTimer.Stop()
		h.whoRefreshTimer = nil
	}
	h.whoRefreshPending = nil
	shouldReconnect := h.AutoReconnect && !h.manualDisconnect
	cb := h.onLinkClosed
	h.Status = StatusDisconnected
	h.StatusText = "Disconnected"
	h.lock.Unlock()
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}

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
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}

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
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}

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

	// Python's _send_hello body values are TEXT strings (RRC.py:447-448);
	// byte strings here made the hub silently drop the hello.
	body := map[any]any{
		BHelloName: "nomadnet",
		BHelloVer:  "0.1",
		BHelloCaps: map[any]any{
			CapResourceEnvelope: true,
			CapAction:           true,
		},
	}

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeHello, srcHash, nil, nil, body, mid, ts)

	nick := h.effectiveNick()
	if nick != "" {
		// Python _send_hello (RRC.py:453-455): env[K_NICK] = nick — a TEXT
		// string. A byte string makes real rrcd hubs reject the whole hello
		// ("Bad packet ... err=nickname must be a string") and the welcome
		// never arrives.
		env[KeyNick] = nick
	}

	h.sendEnv(env)
}

// helloLoop mirrors Python's hello_loop (RRC.py:421-441): the hello repeats
// every 3 s, up to 5 attempts, until a WELCOME arrives. A hub that never
// welcomes fails the hub with "WELCOME timeout" and tears the link down - a
// fresh link's first hello can race the hub-side registration, so a single
// send is not enough.
func (h *RRCHub) helloLoop() {
	const attempts = 5
	for i := range attempts {
		h.lock.Lock()
		link := h.link
		welcomed := h.Welcomed
		stopped := h.Manager != nil && h.Manager.IsStopped()
		status := h.Status
		h.lock.Unlock()

		if welcomed || stopped || status == StatusFailed {
			return
		}
		if link == nil {
			return
		}
		if i > 0 {
			h.sendHello(link)
		}
		time.Sleep(3 * time.Second)

		h.lock.Lock()
		welcomed = h.Welcomed
		h.lock.Unlock()
		if welcomed {
			return
		}
	}
	if !h.Welcomed {
		h.SetStatus(StatusFailed, "WELCOME timeout")
		h.lock.Lock()
		link := h.link
		h.lock.Unlock()
		if link != nil {
			link.Teardown()
		}
	}
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
	if h.whoRefreshTimer != nil {
		h.whoRefreshTimer.Stop()
		h.whoRefreshTimer = nil
	}
	h.whoRefreshPending = nil
	// No reply can be answered after a manual disconnect: drop the
	// outstanding auto- and user-initiated /who markers with the link.
	for k := range h.silentWhoRooms {
		delete(h.silentWhoRooms, k)
	}
	for k := range h.userWhoRooms {
		delete(h.userWhoRooms, k)
	}
	link := h.link
	h.link = nil
	h.Welcomed = false
	h.lock.Unlock()

	// SetStatus fires the change notification so the hub row flips to the
	// disconnected glyph immediately (Python's disconnect updates the UI the
	// same way via the status setter).
	h.SetStatus(StatusDisconnected, "Disconnected")

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
	h.Rooms[room] = true
	if h.Messages[room] == nil {
		h.Messages[room] = make([]*RRCMessage, 0)
	}
	mgr := h.Manager
	h.lock.Unlock()

	// Python add_room ends with manager.save() + _notify_change
	// (RRC.py:274-282): the room list persists across restarts.
	if mgr != nil {
		if err := mgr.Save(); err != nil {
			log.Printf("[RRC %v] could not save after add_room: %v", h.Name, err)
		}
		mgr.NotifyChange(h)
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

// SetMOTD stores the hub's message of the day and notifies the UI (Python
// assigns self.motd then manager._notify_change, RRC.py:1136-1141).
func (h *RRCHub) SetMOTD(text string) {
	h.lock.Lock()
	h.MOTD = text
	h.lock.Unlock()
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}
}

// applyWhoReply REPLACES the room's member set from a parsed /who reply,
// learning nicks by matching who-reply 12-hex prefixes against the known
// member hashes (Python RRC.py:1085-1100's nick walk).
//
// The replacement (not Python's merge) is deliberate and user-ordered: with
// rrcd's include_joined_member_list defaulting to OFF, JOINED/PARTED fanouts
// carry no member data, so the /who reply is the ONLY authoritative member
// source. Merging could never add members whose nick was known or remove
// departed ones, which froze every client's member count at its own
// join-time snapshot — the fleet's diverging "N users" counts (6/6/6/5/3/4
// on ONE hub, 2026-09-03). Each entry resolves to a full-hash member key
// when a known member carries that hash prefix (so conversations and the
// self-arrow keep working); unknown nicked members enter under their reply
// prefix so the COUNT is correct.
func (h *RRCHub) applyWhoReply(room string, entries []whoEntry) {
	room = strings.ToLower(room)
	h.lock.Lock()
	// Resolve each entry BEFORE replacing, so the prefix match can see the
	// outgoing set (the new set may carry reply-prefix keys that would
	// shadow real matches).
	resolved := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.HashHex == "" {
			continue
		}
		key := e.HashHex
		if e.Nick != "" {
			full := ""
			for ph := range h.Members[room] {
				if strings.HasPrefix(ph, e.HashHex) {
					full = ph
					break
				}
			}
			if full == "" {
				full = e.HashHex
			}
			key = full
			h.Nicks[full] = e.Nick
		}
		if key != "" {
			resolved[key] = true
		}
	}
	h.Members[room] = resolved
	h.lock.Unlock()
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}
}

// whoRefreshInterval is the cadence of the periodic silent membership
// reconciliation. rrcd's rate bucket allows 30 messages/minute per client;
// one /who per joined room per minute stays far below it.
const whoRefreshInterval = 60 * time.Second

// scheduleWhoRefresh arms the periodic membership reconciliation after a
// WELCOME. Python's auto_who fires /who only at self-join; nothing ever
// re-synced membership afterwards, so a client missed the later join/part
// fanouts (which carry no member data on this hub) and kept its stale count
// forever. The reconciliation re-requests the member list for every joined
// room so the Users count converges on the hub's live membership.
func (h *RRCHub) scheduleWhoRefresh() {
	h.lock.Lock()
	if h.whoRefreshTimer != nil {
		h.lock.Unlock()
		return
	}
	after := h.afterFunc
	if after == nil {
		after = time.AfterFunc
	}
	fire := h.whoRefreshFire()
	h.whoRefreshTimer = after(whoRefreshInterval, fire)
	h.whoRefreshPending = fire
	h.lock.Unlock()
}

// whoRefreshFire builds the reconciliation callback: one silent /who sweep,
// then re-arm — stopping (without rescheduling) when the welcome is lost
// (link closed or manual disconnect both clear Welcomed) or the manager has
// been shut down.
func (h *RRCHub) whoRefreshFire() func() {
	return func() {
		h.lock.Lock()
		h.whoRefreshTimer = nil
		h.whoRefreshPending = nil
		welcomed := h.Welcomed
		stopped := h.Manager != nil && h.Manager.IsStopped()
		h.lock.Unlock()
		if !welcomed || stopped {
			return
		}
		h.refreshRoomMembership()
		h.scheduleWhoRefresh()
	}
}

// refreshRoomMembership sends one silent /who per joined room (the reply is
// consumed without hitting the message log via silentWhoRooms, but its
// member set still heals through applyWhoReply).
func (h *RRCHub) refreshRoomMembership() {
	h.lock.Lock()
	if !h.Welcomed {
		h.lock.Unlock()
		return
	}
	rooms := sortedKeys(h.Rooms)
	h.lock.Unlock()

	for _, room := range rooms {
		h.markAutoWhoRequest(room)
		if err := h.SendCommand("/who "+room, room); err != nil {
			// The marker must not linger: a later user-initiated /who reply
			// for this room would otherwise be consumed silently.
			h.clearAutoWhoRequest(room)
			log.Printf("[RRC %v] SendCommand('/who %v'): %v", h.Name, room, err)
		}
	}
}

// markAutoWhoRequest marks the room as having an outstanding auto-requested
// /who: EVERY reply copy for the room is consumed silently (rrcd fans the
// who-notice out once per member, so a one-shot consume leaks the marker and
// the duplicates flood the conversation window every sweep). The marker is
// cleared when a user-initiated /who for the room is answered, or by the
// link closing.
func (h *RRCHub) markAutoWhoRequest(room string) {
	h.lock.Lock()
	h.silentWhoRooms[strings.ToLower(room)] = true
	h.lock.Unlock()
}

// clearAutoWhoRequest drops the room's outstanding auto-request marker.
func (h *RRCHub) clearAutoWhoRequest(room string) {
	h.lock.Lock()
	delete(h.silentWhoRooms, strings.ToLower(room))
	h.lock.Unlock()
}

// consumeWhoReply applies the /who reply bookkeeping and reports whether the
// reply must be consumed silently. A user-initiated reply RENDERS (it also
// clears the room's auto marker — the user's answer retires the stale
// auto-request); an outstanding auto-request consumes every copy while its
// marker stays set; any other who reply renders.
func (h *RRCHub) consumeWhoReply(room string) bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.userWhoRooms[room] {
		delete(h.userWhoRooms, room)
		delete(h.silentWhoRooms, room)
		return false
	}
	return h.silentWhoRooms[room]
}

// stopWhoRefresh cancels the armed reconciliation timer (link closed or
// manual disconnect).
func (h *RRCHub) stopWhoRefresh() {
	h.lock.Lock()
	if h.whoRefreshTimer != nil {
		h.whoRefreshTimer.Stop()
		h.whoRefreshTimer = nil
	}
	h.whoRefreshPending = nil
	h.lock.Unlock()
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

// RoomMemberInfo is one room member: the identity-hash hex (empty when the
// joiner's hash is unknown) and the display nick.
type RoomMemberInfo struct {
	HashHex string
	Nick    string
}

// GetRoomMembers returns the members of the given room as hash/nick pairs
// sorted case-insensitively by nick, mirroring the member list Python's
// RoomWidget._refresh_users_pane builds (Channels.py:663-724: entries sorted
// by name.lower(), each carrying the peer hash for the user-info dialog).
// The hash comes from the hub's nick table (keyed by the join envelope's
// source identity hash); a member with no known hash carries an empty HashHex.
func (h *RRCHub) GetRoomMembers(room string) []RoomMemberInfo {
	room = strings.ToLower(room)
	h.lock.Lock()
	defer h.lock.Unlock()

	set := h.Members[room]
	out := make([]RoomMemberInfo, 0, len(set))
	for hashHex := range set {
		// The member set is hash-keyed (Python members[room] is a set of
		// identity hashes, RRC.py:982-984); the display nick comes from the
		// learned nick table with Python display_name_for's hash-prefix
		// fallback.
		nick := h.Nicks[hashHex]
		if nick == "" {
			if len(hashHex) > 12 {
				nick = hashHex[:12]
			} else {
				nick = hashHex
			}
		}
		out = append(out, RoomMemberInfo{HashHex: hashHex, Nick: nick})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Nick) < strings.ToLower(out[j].Nick)
	})
	return out
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

// SetHubName sets the hub's display name under the hub lock, mirroring
// Python edit_hub_dialog's confirmed() `hub.name = nm` assignment
// (Channels.py:2026).
func (h *RRCHub) SetHubName(name string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.Name = name
}

// GetServerName returns the hub's ADVERTISED server name (Python hub.hub_name,
// set from the welcome envelope), for the TUI hub-info and edit-hub dialogs.
// Empty until the hub connects and sends its welcome.
func (h *RRCHub) GetServerName() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.HubName
}

// GetHubVersion returns the hub's ADVERTISED version (Python hub.hub_version,
// set from the welcome envelope), for the room header and hub info panel.
// Empty until the hub connects.
func (h *RRCHub) GetHubVersion() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.HubVersion
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
	h.JoinRoomWithKey(room, silent, "")
}

// JoinRoomWithKey joins a room, optionally with a room key for keyed (+k)
// rooms (Python join_room's key parameter, RRC.py:569).
func (h *RRCHub) JoinRoomWithKey(room string, silent bool, key string) {
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
	// Python join_room (RRC.py:566-579): the JOIN carries the sender's
	// identity hash as the source and the effective nick as K_NICK — the hub
	// needs both to key the member set and to attach the advisory nick to the
	// JOINED fanout.
	var src []byte
	if h.Manager != nil {
		src = h.Manager.identityHash()
	}
	var nick []byte
	if n := h.effectiveNick(); n != "" {
		nick = []byte(n)
	}
	// Python join_room: the room key rides in the JOIN envelope's BODY
	// (RRC.py:571-572: body = key if key else None).
	var joinBody any
	if key != "" {
		joinBody = []byte(key)
	}
	env := MakeEnvelope(TypeJoin, src, []byte(room), nick, joinBody, mid, ts)
	h.sendEnv(env)
}

// HasRoom reports whether the room is joined (Python's `room in hub.rooms`).
func (h *RRCHub) HasRoom(room string) bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.Rooms[strings.ToLower(room)]
}

// PartRoom sends a T_PART for a room.
func (h *RRCHub) PartRoom(room string) {
	room = strings.ToLower(room)
	h.lock.Lock()
	h.pendingParts[room] = true
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	// Python part_room (RRC.py:604-610): the PART carries the sender's
	// identity hash so the hub can drop the hash-keyed member entry.
	var src []byte
	if h.Manager != nil {
		src = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypePart, src, []byte(room), nil, nil, mid, ts)
	h.sendEnv(env)
}

// SendMessage sends a T_MSG to a room and records it locally. Slash-prefixed
// text is a user command, not chat (Python's TUI intercepts it in
// RoomWidget._handle_slash_command before anything reaches send_message), so
// it routes through the user-command path: the command is not recorded as a
// chat message and its reply renders in the conversation window.
func (h *RRCHub) SendMessage(room, text string) string {
	room = strings.ToLower(room)
	if strings.HasPrefix(text, "/") {
		mid, err := h.sendUserCommand(text, room)
		if err != nil {
			log.Printf("[RRC %v] command %q: %v", h.Name, text, err)
		}
		return mid
	}
	mid := MsgID()
	ts := NowMs()

	nick := h.effectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeMsg, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h.rememberSentID(hexString(mid))
	// The body is remembered BEFORE the send so a fanout echo that races
	// back ahead of the local record is still collapsed (see
	// collapseSelfEcho).
	h.rememberSentBody("msg", room, text, ts)
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

	nick := h.effectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeAction, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h.rememberSentID(hexString(mid))
	h.rememberSentBody("action", room, text, ts)
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
// SendMessage it does not normalize the room or record the message locally —
// it is a thin send of the command text used for the silent /who and /list
// requests. The sent message id AND body are remembered like SendMessage so
// a hub that relays the command text back as a MSG (observed live: an
// unregistered session's "/who general" came back as a chat message 550
// times) is suppressed by the self-echo and fanout-collapse guards instead
// of being recorded as chat.
func (h *RRCHub) SendCommand(text, room string) error {
	_, err := h.sendCommand(text, room)
	return err
}

// SendUserCommand sends a user-initiated slash command (e.g. "/who" typed in
// the composer) to the hub as a T_MSG envelope and returns the sent message
// id hex. Unlike the silent SendCommand path it marks the target room as
// having an outstanding user-initiated /who, so the reply RENDERS in the
// conversation window and can never be swallowed by a stale auto-request
// marker. It is the entry point for the TUI's server-forwarded commands.
func (h *RRCHub) SendUserCommand(text, room string) (string, error) {
	return h.sendUserCommand(text, room)
}

// sendCommand is the shared command-send tail of SendCommand and
// SendUserCommand; it returns the sent message id hex.
func (h *RRCHub) sendCommand(text, room string) (string, error) {
	if !strings.HasPrefix(text, "/") {
		return "", errors.New("command must start with /")
	}
	mid := MsgID()
	ts := NowMs()
	nick := h.effectiveNick()
	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}
	env := MakeEnvelope(TypeMsg, srcHash, []byte(room), []byte(nick), text, mid, ts)
	h.rememberSentID(hexString(mid))
	// The body is remembered BEFORE the send (like SendMessage) so a fanout
	// echo that races back is still collapsed by collapseSelfEcho.
	h.rememberSentBody("msg", strings.ToLower(room), text, ts)
	h.sendEnv(env)
	return hexString(mid), nil
}

// sendUserCommand sends a user-initiated command, marking the target room as
// having an outstanding user-initiated /who request (cleared when the reply
// is answered, or on a failed send).
func (h *RRCHub) sendUserCommand(text, room string) (string, error) {
	target, isWho := userWhoTarget(text, room)
	if isWho {
		h.lock.Lock()
		if target != "" {
			h.userWhoRooms[target] = true
		} else {
			// A bare "/who" with no active room: the target room is only
			// known from the reply, so drop every stale auto marker — one
			// of them would otherwise swallow the user's reply.
			for r := range h.silentWhoRooms {
				delete(h.silentWhoRooms, r)
			}
		}
		h.lock.Unlock()
	}
	mid, err := h.sendCommand(text, room)
	if isWho && err != nil && target != "" {
		h.lock.Lock()
		delete(h.userWhoRooms, target)
		h.lock.Unlock()
	}
	return mid, err
}

// userWhoTarget parses the room a user /who-style command targets: "/who
// <room>" names the room in its argument, a bare "/who" targets the caller's
// active room. ok is false for any non-/who command.
func userWhoTarget(text, fallback string) (string, bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", false
	}
	cmd := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	if cmd != "who" && cmd != "names" {
		return "", false
	}
	if len(parts) > 1 {
		return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(parts[1]), "#")), true
	}
	return strings.ToLower(fallback), true
}

// pendingPing is one client-sent ping awaiting its PONG echo (Python
// _pending_pings' (now_ms, room) value, RRC.py:592-600).
type pendingPing struct {
	sentMs int64
	room   string
}

// pingExpiryMs is how long an un-echoed ping stays pending (Python's 15 s
// expiry in send_ping, RRC.py:597-599).
const pingExpiryMs = 15000

// SendPing sends a T_PING to a room (Python send_ping, RRC.py:592-600): the
// envelope carries the local identity as the source and NO room field, the
// 8-byte random body keys the pending-pings table, and pings older than 15 s
// expire at each send. The ping itself renders nothing; its ANSWERED pong
// records the round trip as a system row (RRC.py:878).
func (h *RRCHub) SendPing(room string) {
	room = strings.ToLower(room)
	mid := MsgID()
	ts := NowMs()

	body := make([]byte, 8)
	_, err := rand.Read(body)
	if err != nil {
		log.Printf("[RRC %v] ping body: %v", h.Name, err)
	}
	key := string(body)
	now := ts

	var srcHash []byte
	if h.Manager != nil {
		srcHash = h.Manager.identityHash()
	}

	h.lock.Lock()
	for k, p := range h.pendingPings {
		if now-p.sentMs > pingExpiryMs {
			delete(h.pendingPings, k)
		}
	}
	h.pendingPings[key] = pendingPing{sentMs: now, room: room}
	h.lock.Unlock()

	// Python's send_ping omits the room field (RRC.py:594): the envelope
	// carries only the source, the body, and the id.
	env := MakeEnvelope(TypePing, srcHash, nil, nil, body, mid, ts)
	h.sendEnv(env)
}

// GetEffectiveNick returns the override nick or the manager's nick.
func (h *RRCHub) GetEffectiveNick() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.NickOverride != "" {
		return h.NickOverride
	}
	if h.Manager != nil {
		return h.Manager.GetNickname()
	}
	return ""
}

// effectiveNick returns the effective nick truncated to the hub's active
// nick byte limit, so every outgoing envelope carries a nick the hub will
// accept (real hubs silently drop envelopes whose nick exceeds the limit,
// which registered over-long-named clients as bare hashes).
func (h *RRCHub) effectiveNick() string {
	return truncateUTF8(h.GetEffectiveNick(), h.effectiveNickLimit())
}

// effectiveNickLimit returns the hub's active nickname byte limit: the
// WELCOME-provided value when positive, falling back to the 32-byte default
// before the first WELCOME (and when the hub advertises a non-positive
// limit, matching Python's `max_nick_bytes or 32` fallback).
func (h *RRCHub) effectiveNickLimit() int {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.MaxNickBytes > 0 {
		return h.MaxNickBytes
	}
	return DefaultMaxNickBytes
}

// truncateUTF8 cuts s to at most maxBytes UTF-8 bytes rune-safely: the cut
// never splits a multi-byte rune and the result is always valid UTF-8 (a
// partial trailing rune, or a hostile invalid sequence, is dropped).
func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return strings.ToValidUTF8(s, "")
	}
	return strings.ToValidUTF8(s[:maxBytes], "")
}

// SetNickOverride sets a per-hub nick override.
func (h *RRCHub) SetNickOverride(nick string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.NickOverride = nick
}

// HasNickOverride reports whether a per-hub nick override is set (Python's
// nick_override check in Channels.py:1073).
func (h *RRCHub) HasNickOverride() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.NickOverride != ""
}

// MaxNickLimit returns the hub's active nickname byte limit (the
// WELCOME-provided value when positive, else 32) for client-side validation
// (Python Channels.py:1078: `self.hub.max_nick_bytes or 32`).
func (h *RRCHub) MaxNickLimit() int {
	return h.effectiveNickLimit()
}

// MaxMsgBodyLimit returns the hub's active message-body byte limit for
// client-side validation (Python Channels.py:1058:
// `self.hub.max_msg_body_bytes or 350`).
func (h *RRCHub) MaxMsgBodyLimit() int {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.MaxMsgBodyBytes > 0 {
		return h.MaxMsgBodyBytes
	}
	return DefaultMaxMsgBytes
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
	log.Printf("DEBUG rrc HandleData: %d bytes: %x", len(data), data[:min(len(data), 40)])
	env, err := DecodeEnvelope(data)
	if err != nil {
		log.Printf("DEBUG rrc HandleData decode failed: %v", err)
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

	switch msgType {
	case TypeHello:
		h.handleHello(src, nick, body)

	case TypeJoin:
		h.handleJoin(src, nick, room, body)

	case TypeMsg:
		// Python RRC.py:1060-1064: skip the hub's echo of our own message
		// (src == own identity and mid in _sent_ids). The recvIDs guard also
		// collapses duplicate fanout copies (rrcd's member list can carry the
		// same identity repeatedly after reconnect storms).
		if h.isOwnEcho(src, hexString(mid)) {
			return
		}
		if hexString(mid) != "" && h.seenReceivedID(hexString(mid)) {
			return
		}
		textStr, ok := bodyText(body)
		if !ok {
			// Python only records string MSG bodies (RRC.py:1060); other
			// payloads are opaque and ignored.
			return
		}
		// Python stamps every inbound message with its own _now_ms()
		// arrival time (RRC.py:1043) — the sender's envelope ts is only
		// kept as the fanout-dedupe window key (rrcd's per-member fanout
		// copies share it, and the chronological render sort is stable so
		// replay order is preserved).
		collapseKey := ts
		if collapseKey <= 0 {
			collapseKey = NowMs()
		}
		arrival := NowMs()
		// Python T_MSG bookkeeping (RRC.py:1031-1035): every copy with a
		// source hash and a non-empty nick learns nicks[src] and adds src to
		// the room's member set. This runs BEFORE the fanout collapse skips
		// rendering, so the member set converges on the per-member fanout
		// copies exactly like Python's does.
		if len(src) > 0 && nickStr != "" {
			h.learnMsgPeer(roomStr, src, nickStr)
		}
		// rrcd 0.3.2 fans each message out once per room member with a unique
		// mid, a per-copy rewritten source hash and a registry-derived nick,
		// so neither mid- nor src-based dedupe can collapse the copies. The
		// client-side fanout collapse keys on kind+room+body with a small
		// timestamp window instead — an intentional, user-ordered deviation
		// from Python, which renders every copy (TODO item 4). Server-side
		// hubs skip it: their inbound copies are one per client and must all
		// be recorded and re-broadcast.
		msg := &RRCMessage{
			Kind: "msg",
			Room: roomStr,
			Src:  src,
			Nick: nickStr,
			Text: textStr,
			Ts:   arrival,
		}
		if !h.isServerSide() {
			if h.collapseSelfEcho("msg", roomStr, textStr, ts) || h.collapseFanout("msg", roomStr, textStr, collapseKey, nickStr, msg) {
				return
			}
		}
		h.recordMessage(msg, false)
		if h.Manager != nil && h.Manager.messageCallback != nil {
			h.Manager.messageCallback(h, msg)
		}

		if h.isServerSide() {
			h.echoMessage(src, room, nick, body, mid, ts)
		}

	case TypeAction:
		// Python T_ACTION branch (RRC.py:1054-1085): action messages record
		// exactly like msgs, with the same self-echo and fanout guards.
		if h.isOwnEcho(src, hexString(mid)) {
			return
		}
		if hexString(mid) != "" && h.seenReceivedID(hexString(mid)) {
			return
		}
		textStr, ok := bodyText(body)
		if !ok {
			// Python only records string ACTION bodies (RRC.py:1054).
			return
		}
		// Python T_ACTION bookkeeping (RRC.py:1054-1068): same nick/member
		// learning as msgs, and the same arrival-time stamping (the
		// envelope ts is only the fanout-dedupe window key, RRC.py:1054).
		if len(src) > 0 && nickStr != "" {
			h.learnMsgPeer(roomStr, src, nickStr)
		}
		collapseKey := ts
		if collapseKey <= 0 {
			collapseKey = NowMs()
		}
		msg := &RRCMessage{
			Kind: "action",
			Room: roomStr,
			Src:  src,
			Nick: nickStr,
			Text: textStr,
			Ts:   NowMs(),
		}
		if !h.isServerSide() {
			if h.collapseSelfEcho("action", roomStr, textStr, ts) || h.collapseFanout("action", roomStr, textStr, collapseKey, nickStr, msg) {
				return
			}
		}
		h.recordMessage(msg, false)
		if h.Manager != nil && h.Manager.messageCallback != nil {
			h.Manager.messageCallback(h, msg)
		}
		if h.isServerSide() {
			h.echoMessage(src, room, nick, body, mid, ts)
		}

	case TypePart:
		h.handlePart(src, nick, room, body)

	case TypeJoined:
		h.handleJoinedNotification(roomStr, nickStr, body)

	case TypeParted:
		h.handlePartedNotification(roomStr, nickStr, body)

	case TypeWelcome:
		h.handleWelcome(body)

	case TypePong:
		// Python T_PONG branch (RRC.py:865-880): the echoed body keys the
		// pending-pings table; a matching entry is cleared and its round trip
		// is recorded as a system row in the ping's room — "Pong from hub:
		// <rtt> ms" (RRC.py:878). Hub-initiated pings (nothing pending) stay
		// silent: only the client's own answered pings render.
		if bodyStr, ok := bodyText(body); ok {
			h.lock.Lock()
			pending, answered := h.pendingPings[bodyStr]
			delete(h.pendingPings, bodyStr)
			h.lock.Unlock()
			if answered {
				rttMs := max(int64(0), NowMs()-pending.sentMs)
				h.recordMessage(&RRCMessage{
					Kind: "system", Room: pending.room,
					Text: "Pong from hub: " + strconv.FormatInt(rttMs, 10) + " ms",
					Ts:   NowMs(),
				}, true)
			}
		}

	case TypePing:
		// Python T_PING branch (RRC.py:857-863): the PONG replies with the
		// RESPONDER's own identity hash (the hub attributes the pong to the
		// client — answering with the PING's source, the hub's identity,
		// made every hub treat the client as dead, tear the link down, and
		// spam every room member with re-join fanouts), echoes the body back
		// UNCHANGED, omits the room field, and carries a FRESH message id.
		var srcHash []byte
		if h.Manager != nil {
			srcHash = h.Manager.identityHash()
		}
		h.sendEnv(MakeEnvelope(TypePong, srcHash, nil, nil, body, MsgID(), NowMs()))

	case TypeError:
		var textStr string
		if s, ok := bodyText(body); ok {
			textStr = s
		} else {
			// Python T_ERROR (RRC.py:1148): a non-string body records the
			// placeholder "(error)".
			textStr = "(error)"
		}
		h.handleError(roomStr, textStr)

	case TypeNotice:
		textStr, ok := bodyText(body)
		if !ok {
			// Python only records string NOTICE bodies (RRC.py:1104).
			return
		}
		// Python _process_notice_text (RRC.py:843-848): /list replies populate
		// the hub's advertised room set for the info panel. They are consumed
		// silently in every case (the room list itself must never render).
		if rooms := ParseRoomListNotice(textStr); rooms != nil {
			h.SetAvailableRooms(rooms)
			return
		}
		// Python _parse_who_notice (RRC.py:1084-1105): /who replies replace
		// the room's member set and learn the nicks via the 12-hex prefix
		// match against the existing members. Auto-requested replies are
		// consumed without hitting the message log (EVERY copy, see
		// consumeWhoReply); user-initiated replies render.
		if whoRoom, entries, isWho := ParseWhoNotice(textStr); isWho {
			h.applyWhoReply(whoRoom, entries)
			if h.consumeWhoReply(whoRoom) {
				return
			}
		}
		// rrcd's server-command acks ("room <name>: registered; mode=+nrt;
		// topic=(none)" and the /mode and /topic acks) are protocol traffic,
		// not conversation: consume them without rendering.
		if isProtocolControlNotice(textStr) {
			return
		}
		// Python (RRC.py:1128-1136): a roomless (global) notice sets the
		// hub's MOTD and notifies — the hub's greeting rides as the first
		// global notice after the welcome.
		if roomStr == "" && strings.TrimSpace(textStr) != "" {
			h.SetMOTD(textStr)
		}
		// Python stamps notices with the _now_ms() arrival time too
		// (RRC.py:1043 — every inbound kind); the envelope ts is only the
		// fanout-dedupe window key.
		collapseKey := ts
		if collapseKey <= 0 {
			collapseKey = NowMs()
		}
		// Fanout collapse for notices too: rrcd's per-member fanout applies
		// to hub notices ("room test: unregistered…" arrives once per fanout
		// copy per join, TODO item 5).
		msg := &RRCMessage{
			Kind: "notice",
			Room: roomStr,
			Src:  src,
			Nick: nickStr,
			Text: textStr,
			Ts:   NowMs(),
		}
		if !h.isServerSide() && h.collapseFanout("notice", roomStr, textStr, collapseKey, nickStr, msg) {
			return
		}
		// Python _record_notice (RRC.py:817-839, reached from the T_NOTICE
		// branch at RRC.py:1138): a ROOMLESS notice is attributed to the
		// manager's ACTIVE room and joins that room's buffer — rrcd's global
		// "󰙎 Welcome to the RaspPi Local Hub!" MOTD notice renders in the
		// open room on the Python SOT (2026-09-03 12:32 capture, mac row 24).
		h.recordNotice(msg)

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

	// Python T_WELCOME (RRC.py:906-908): the status flips to CONNECTED only
	// when the WELCOME arrives, and the reconnect attempt counter resets.
	h.SetStatus(StatusConnected, "Connected")
	h.lock.Lock()
	h.reconnectAttempts = 0
	h.lock.Unlock()

	if h.Manager != nil {
		h.Manager.OnWelcome(h)
	}
	// Arm the periodic membership reconciliation (the auto_who sweep fires
	// once here; the timer keeps the member list converging on the hub's
	// live membership afterwards).
	h.scheduleWhoRefresh()
	// Python handle_welcome → auto_list (RRC.py:950-959): fetch the hub's
	// public room list right after the welcome so the hub info panel can show
	// which rooms exist. The reply NOTICE is consumed silently.
	if h.AutoList {
		go func() {
			if err := h.SendCommand("/list", ""); err != nil {
				log.Printf("SendCommand('/list'): %v", err)
			}
		}()
	}
}

// whoEntry is one entry parsed from a "/who" reply NOTICE: the display nick
// (empty when the entry is a bare hash) and the 12-hex or 32-hex hash text.
type whoEntry struct {
	Nick    string // empty for bare-hash entries
	HashHex string
}

// isHashText reports whether every byte of s is a hex digit.
func isHashText(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// ParseWhoNotice parses a hub "/who" reply NOTICE (Python
// _parse_who_notice, RRC.py:110-125): the body is
// "members in <room>: nick (hash12), ...". Returns the room and the parsed
// entries, or a nil result when the text is not a who reply.
func ParseWhoNotice(text string) (string, []whoEntry, bool) {
	const prefix = "members in "
	if !strings.HasPrefix(text, prefix) {
		return "", nil, false
	}
	sep := strings.Index(text[len(prefix):], ": ")
	if sep < 0 {
		return "", nil, false
	}
	room := strings.ToLower(strings.TrimSpace(text[len(prefix) : len(prefix)+sep]))
	if room == "" {
		return "", nil, false
	}
	body := strings.TrimSpace(text[len(prefix)+sep+2:])
	var entries []whoEntry
	if body != "" && body != "(none)" {
		// rrcd's /who reply entries are "nick (hash12prefix)" or a bare
		// 32-hex identity hash, comma-separated (Python _WHO_ENTRY_RE,
		// RRC.py:118-120). Go's regexp lacks the lookahead the Python
		// pattern uses, so the entries are parsed by splitting.
		for part := range strings.SplitSeq(body, ", ") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) == 32 && isHashText(part) {
				entries = append(entries, whoEntry{HashHex: strings.ToLower(part)})
				continue
			}
			if open := strings.LastIndex(part, " ("); open > 0 && strings.HasSuffix(part, ")") {
				hashText := part[open+2 : len(part)-1]
				nick := strings.TrimSpace(part[:open])
				if len(hashText) == 12 && isHashText(hashText) && nick != "" {
					entries = append(entries, whoEntry{Nick: nick, HashHex: strings.ToLower(hashText)})
				}
			}
		}
	}
	return room, entries, true
}

// ParseRoomListNotice parses a hub "/list" reply NOTICE into the advertised
// rooms (Python _parse_room_list_notice, RRC.py:805-820): the body starts
// with "Registered public rooms" followed by "name - topic" lines, or is the
// single line "No public rooms registered". Returns nil when the text is not
// a room list.
func ParseRoomListNotice(text string) map[string]*string {
	stripped := strings.TrimSpace(text)
	if stripped == "No public rooms registered" {
		return map[string]*string{}
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimLeft(lines[0], " \t"), "Registered public rooms") {
		return nil
	}
	rooms := map[string]*string{}
	for _, line := range lines[1:] {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if name, topic, found := strings.Cut(s, " - "); found {
			name = strings.TrimSpace(name)
			topic = strings.TrimSpace(topic)
			rooms[strings.ToLower(name)] = &topic
		} else {
			rooms[strings.ToLower(strings.TrimPrefix(s, "#"))] = nil
		}
	}
	return rooms
}

// protocolControlNoticeRe matches rrcd's server-command acknowledgement
// notices — the registration acks ("room <name>: registered; mode=+nrt;
// topic=(none)" and its unregistration twin, observed in the 2026-09-03
// captures) and the /mode and /topic acks of the same "room <name>: …" ack
// family. They are protocol traffic, not conversation: consumed without
// rendering.
var protocolControlNoticeRe = regexp.MustCompile(
	`(?i)^room [^:]+: (unregistered|registered|mode |topic )`)

// isProtocolControlNotice reports whether a NOTICE body is an rrcd
// server-command acknowledgement that must update no conversation buffer.
func isProtocolControlNotice(text string) bool {
	return protocolControlNoticeRe.MatchString(text)
}

// handleError processes a T_ERROR envelope (Python T_ERROR branch,
// RRC.py:1145-1170): the text is recorded as an error notice in the affected
// room (or the active room when roomless, via recordNotice), a pending join
// for that room is rolled back, and a refusal error fails the hub per doc
// 4-RRC ("an ERROR that clearly indicates refusal … the client enters the
// Disconnected state"). It is never treated as chat.
func (h *RRCHub) handleError(roomStr, text string) {
	h.lock.Lock()
	rollback := h.pendingJoins[roomStr]
	delete(h.pendingJoins, roomStr)
	delete(h.silentJoins, roomStr)
	delete(h.pendingParts, roomStr)
	if rollback {
		delete(h.Rooms, roomStr)
	}
	h.lock.Unlock()

	if rollback {
		if h.Manager != nil {
			if err := h.Manager.Save(); err != nil {
				log.Printf("[RRC %v] could not save after join rollback: %v", h.Name, err)
			}
		}
	}

	h.recordNotice(&RRCMessage{Kind: "error", Room: roomStr, Text: text, Ts: NowMs()})

	if errorIndicatesRefusal(text) {
		h.SetStatus(StatusFailed, "Refused: "+text)
	}
}

// refusalIndicators are the lowercased substrings that mark an ERROR message
// as a hub refusal (doc 4-RRC: "an ERROR that clearly indicates refusal or
// fatal failure" ends the session).
var refusalIndicators = []string{
	"refus", "reject", "denied", "not allowed", "forbidden",
	"unauthorized", "banned", "kicked", "rate limit", "too many",
	"limit exceeded", "declined",
}

// errorIndicatesRefusal reports whether an ERROR text indicates a hub
// refusal or fatal failure.
func errorIndicatesRefusal(text string) bool {
	lower := strings.ToLower(text)
	for _, indicator := range refusalIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
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

// handleJoin processes a JOIN envelope. On the server side, it adds the
// joiner's identity hash to the room's member set and fans out a JOINED
// notification whose body is a single-element list of the joiner's hash, with
// the joiner's nick as the advisory K_NICK — the shape rrcd 0.3.2 sends and
// the Python client's T_JOINED branch parses (RRC.py:968-1000). On the client
// side, JOINED is handled in the main HandleData switch.
func (h *RRCHub) handleJoin(src, nick, room []byte, _ any) {
	roomStr := strings.ToLower(string(room))
	srcHex := hexString(src)
	nickStr := string(nick)

	h.lock.Lock()
	if h.Members[roomStr] == nil {
		h.Members[roomStr] = make(map[string]bool)
	}
	if srcHex != "" {
		h.Members[roomStr][srcHex] = true
	}
	if srcHex != "" && nickStr != "" {
		h.Nicks[srcHex] = nickStr
	}
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeJoined, src, []byte(roomStr), nick, []any{src}, mid, ts)
	h.sendEnv(env)
}

// handleJoinedNotification processes a JOINED fanout on the client side,
// mirroring Python's T_JOINED branch (RRC.py:968-1000): the body is a list of
// the room's member identity hashes, the member set is hash-keyed, and the
// client's own identity hash always joins the set. The advisory K_NICK is
// learned only for a fanout about a single other joiner. Join/leave events
// render like the Python capture: a self-join records "You joined #<room>"
// (unless the join was silent) and a single other joiner records
// "→ <nick> joined" with the 12-hex hash-prefix fallback for unknown nicks.
func (h *RRCHub) handleJoinedNotification(roomStr, nickStr string, body any) {
	bodyHashes := bodyHashList(body)

	ownHash := ""
	if h.Manager != nil {
		ownHash = hexString(h.Manager.identityHash())
	}

	selfJoin := false
	silent := false
	h.lock.Lock()
	selfJoin = h.pendingJoins[roomStr]
	if selfJoin {
		delete(h.pendingJoins, roomStr)
	}
	if h.silentJoins[roomStr] {
		silent = true
		delete(h.silentJoins, roomStr)
	}
	h.Rooms[roomStr] = true
	if h.Messages[roomStr] == nil {
		h.Messages[roomStr] = make([]*RRCMessage, 0)
	}
	if h.Members[roomStr] == nil {
		h.Members[roomStr] = make(map[string]bool)
	}
	for _, hb := range bodyHashes {
		h.Members[roomStr][hb] = true
	}
	if ownHash != "" {
		h.Members[roomStr][ownHash] = true
	}
	if !selfJoin && nickStr != "" && len(bodyHashes) == 1 {
		jh := bodyHashes[0]
		if ownHash == "" || jh != ownHash {
			h.Nicks[jh] = nickStr
		}
	}
	autoWho := h.AutoWho && selfJoin
	h.lock.Unlock()

	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}
	// Python T_JOINED (RRC.py:956-958, 972-975): the self-join records
	// "You joined #<room>" unless the join was silent (the WELCOME re-join
	// loop), and a fanout about a single other joiner records its arrival.
	// Both are recorded as Kind "system" (Python _record_system) WITHOUT an
	// arrow in the text — the renderer derives the arrow_r/arrow_l icon from
	// the " joined"/" left" suffix (Channels.py:1294), and Python's F8
	// join/leave collapse only collapses Kind "system" rows
	// (_is_joinpart_system, Channels.py:1240). The nick resolves through the
	// just-learned nick table with display_name_for's hash-prefix fallback.
	joiner := ""
	if !selfJoin && len(bodyHashes) == 1 && (ownHash == "" || bodyHashes[0] != ownHash) {
		joiner = h.displayNameForHash(bodyHashes[0])
	}
	switch {
	case selfJoin && !silent:
		h.recordMessage(&RRCMessage{
			Kind: "system", Room: roomStr, Text: "You joined #" + roomStr, Ts: NowMs(),
		}, true)
	case joiner != "":
		h.recordMessage(&RRCMessage{
			Kind: "system", Room: roomStr, Text: joiner + " joined", Ts: NowMs(),
		}, true)
	}
	// Python handle_joined → auto_who (RRC.py:1006-1012): fetch the room's
	// member list right after joining; every reply copy is consumed silently.
	if autoWho {
		h.markAutoWhoRequest(roomStr)
		if err := h.SendCommand("/who "+roomStr, roomStr); err != nil {
			log.Printf("[RRC %v] SendCommand('/who %v'): %v", h.Name, roomStr, err)
			h.clearAutoWhoRequest(roomStr)
		}
	}
}

// handlePartedNotification processes a PARTED fanout on the client side,
// mirroring Python's T_PARTED branch (RRC.py:978-1040): the body is a list of
// the parted member identity hashes, and a self-part drops the room together
// with its member set. A single other parter records "← <nick> left" (with
// the 12-hex hash-prefix fallback for unknown nicks); a self-part records
// nothing (Python parity).
func (h *RRCHub) handlePartedNotification(roomStr, nickStr string, body any) {
	bodyHashes := bodyHashList(body)

	ownHash := ""
	if h.Manager != nil {
		ownHash = hexString(h.Manager.identityHash())
	}

	selfPart := false
	h.lock.Lock()
	selfPart = h.pendingParts[roomStr]
	if selfPart {
		delete(h.pendingParts, roomStr)
	}
	if !selfPart && nickStr != "" && len(bodyHashes) == 1 {
		ph := bodyHashes[0]
		if ownHash == "" || ph != ownHash {
			h.Nicks[ph] = nickStr
		}
	}
	if h.Members[roomStr] != nil {
		for _, hb := range bodyHashes {
			delete(h.Members[roomStr], hb)
		}
	}
	if selfPart {
		delete(h.Rooms, roomStr)
		delete(h.Members, roomStr)
	}
	h.lock.Unlock()

	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}
	// Python T_PARTED (RRC.py:1015-1018): a fanout about a single other
	// parter records the departure as Kind "system" (Python _record_system)
	// without an arrow in the text — the renderer derives the arrow_l icon
	// from the " left" suffix (Channels.py:1294). The nick resolves through
	// the just-learned nick table with the hash-prefix fallback.
	if !selfPart && len(bodyHashes) == 1 && (ownHash == "" || bodyHashes[0] != ownHash) {
		h.recordMessage(&RRCMessage{
			Kind: "system", Room: roomStr, Text: h.displayNameForHash(bodyHashes[0]) + " left", Ts: NowMs(),
		}, true)
	}
}

// displayNameForHash resolves a member's display name from the nick table,
// falling back to the 12-hex prefix of the identity hash (Python
// display_name_for, RRC.py:307-313). Callers must not hold h.lock.
func (h *RRCHub) displayNameForHash(hexHash string) string {
	h.lock.Lock()
	defer h.lock.Unlock()
	if nick := h.Nicks[hexHash]; nick != "" {
		return nick
	}
	if len(hexHash) > 12 {
		return hexHash[:12]
	}
	return hexHash
}

// bodyHashList extracts identity hashes from a JOINED/PARTED fanout body,
// which rrcd sends as a CBOR list of byte strings.
func bodyHashList(body any) []string {
	list, ok := body.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if hb, ok := e.([]byte); ok && len(hb) > 0 {
			out = append(out, hexString(hb))
		}
	}
	return out
}

// handlePart processes a PART envelope. On the server side, it drops the
// parter's identity hash from the room's member set and fans out a PARTED
// notification whose body is a single-element list of the parter's hash with
// the learned nick as the advisory K_NICK (RRC.py:978-1040).
func (h *RRCHub) handlePart(src, nick, room []byte, _ any) {
	roomStr := strings.ToLower(string(room))
	srcHex := hexString(src)

	h.lock.Lock()
	if h.Members[roomStr] != nil {
		delete(h.Members[roomStr], srcHex)
	}
	nickBytes := nick
	if len(nickBytes) == 0 && srcHex != "" {
		if learned, ok := h.Nicks[srcHex]; ok {
			nickBytes = []byte(learned)
		}
	}
	h.lock.Unlock()

	mid := MsgID()
	ts := NowMs()
	env := MakeEnvelope(TypeParted, src, []byte(roomStr), nickBytes, []any{src}, mid, ts)
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
	// Python _record_message APPENDS to the room buffer (RRC.py:790
	// buf.append(msg)) and trims the overflow from the FRONT, so the buffer
	// stays oldest→newest (TODO item 1: the Go port used to prepend, which
	// rendered the newest message at the top).
	cap := h.perRoomCap()
	buf := h.Messages[room]
	if cap > 0 && len(buf) >= cap {
		buf = buf[len(buf)-cap+1:]
	}
	h.Messages[room] = append(buf, msg)

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

	// Python _add_message ends with manager._notify_messages(self, msg)
	// (RRC.py:831): the UI updates the room view per message.
	if h.Manager != nil {
		h.Manager.NotifyMessage(h, msg)
	}
}

// rememberSentID records one of our own outgoing message ids (Python
// _sent_ids.append, RRC.py:633,650), capped at 256 like Python's deque.
func (h *RRCHub) rememberSentID(mid string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.sentIDs[mid] {
		return
	}
	h.sentIDs[mid] = true
	h.sentIDsOrder = append(h.sentIDsOrder, mid)
	if len(h.sentIDsOrder) > 256 {
		oldest := h.sentIDsOrder[0]
		h.sentIDsOrder = h.sentIDsOrder[1:]
		delete(h.sentIDs, oldest)
	}
}

// seenReceivedID returns true when the message id was already processed
// (the hub's duplicate fanout), recording it otherwise.
func (h *RRCHub) seenReceivedID(mid string) bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.recvIDs[mid] {
		return true
	}
	h.recvIDs[mid] = true
	h.recvIDsOrder = append(h.recvIDsOrder, mid)
	if len(h.recvIDsOrder) > 256 {
		oldest := h.recvIDsOrder[0]
		h.recvIDsOrder = h.recvIDsOrder[1:]
		delete(h.recvIDs, oldest)
	}
	return false
}

// isOwnEcho reports whether a received copy is the hub's echo of a message
// this client sent: the source hash is our own identity AND the message id
// is one we remembered (Python RRC.py:1060-1064). The sent-ids table is
// guarded by the hub lock.
func (h *RRCHub) isOwnEcho(src []byte, mid string) bool {
	if h.Manager == nil {
		return false
	}
	own := hexString(h.Manager.identityHash())
	if own == "" || hexString(src) != own {
		return false
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.sentIDs[mid]
}

// bodyText decodes a message body as its text form: CBOR text strings and
// byte strings both carry the text (hubs relay bodies opaquely), while any
// other payload kind reports not-ok so the caller can ignore it the way
// Python's isinstance(body, str) guards do.
func bodyText(body any) (string, bool) {
	switch b := body.(type) {
	case []byte:
		return string(b), true
	case string:
		return b, true
	}
	return "", false
}

// fanoutWindowMs is the collapse window for rrcd fanout copies, which arrive
// within well under a second of each other (the rrcd Forwarded log shows a
// full six-member room fanout spanning less than 0.5s). Copies of the same
// body outside the window are distinct messages.
const fanoutWindowMs = 3000

// selfEchoWindowMs is how long after a local send the hub's fanout echoes of
// that send are collapsed with the local record. Echo copies ride the same
// fanout as everyone else's copies (sub-second on this fleet), but the window
// leaves headroom for relay-path latency while keeping the false-positive
// surface (another member sending the identical body just after us) small.
const selfEchoWindowMs = 5000

// fanoutMaxKeys caps the fanout and sent-body dedupe indexes at 256 entries,
// mirroring Python's 256-entry deques.
const fanoutMaxKeys = 256

// fanoutGroup tracks the first-arrival timestamp of one fanout key.
type fanoutGroup struct {
	firstTs int64
	// msg is the kept (first-arrived) copy of the burst; a later copy can
	// backfill its empty nick (rrcd's per-copy rewritten source hashes mean
	// the nick rides on an arbitrary copy).
	msg *RRCMessage
}

// isServerSide reports whether this hub acts as the room server (wired via
// SetLink after an inbound link). Server hubs record and re-broadcast every
// inbound message; client hubs collapse rrcd's fanout copies instead.
func (h *RRCHub) isServerSide() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.serverSide
}

// fanoutKey builds the collapse key for a received message copy: kind, room
// and body. The source hash and nick are deliberately excluded — rrcd
// rewrites both per fanout copy, so they differ across copies of one message.
func fanoutKey(kind, room, body string) string {
	return kind + "\x00" + room + "\x00" + body
}

// learnMsgPeer mirrors Python T_MSG/T_ACTION bookkeeping (RRC.py:1031-1035):
// the copy's source hash learns the copy's nick and joins the room's member
// set (guarded by the caller on a non-empty nick).
func (h *RRCHub) learnMsgPeer(roomStr string, src []byte, nick string) {
	srcHex := hexString(src)
	h.lock.Lock()
	defer h.lock.Unlock()
	h.Nicks[srcHex] = nick
	if roomStr == "" {
		return
	}
	if h.Members[roomStr] == nil {
		h.Members[roomStr] = make(map[string]bool)
	}
	h.Members[roomStr][srcHex] = true
}

// collapseFanout returns true when the copy should be dropped as a duplicate
// fanout copy of an already-recorded message: same kind, room and body as a
// copy seen within fanoutWindowMs. A same-body copy arriving outside the
// window starts a fresh window (legitimate repeats keep rendering).
func (h *RRCHub) collapseFanout(kind, room, body string, ts int64, nick string, msg *RRCMessage) bool {
	key := fanoutKey(kind, room, body)
	h.lock.Lock()
	var backfill *RRCMessage
	if g, ok := h.fanoutGroups[key]; ok {
		if ts >= g.firstTs-fanoutWindowMs && ts <= g.firstTs+fanoutWindowMs {
			// Python learns the (src, nick) pair from EVERY fanout copy
			// before its own dedupe (RRC.py:1031-1035), and rrcd's per-copy
			// rewritten source hashes mean later copies can carry the
			// sender's registry nick even when the first-arrived copy's was
			// empty — backfill the kept copy so it renders ONCE, WITH the
			// sender's nick (the A2 capture symptom: the kept copy rendered
			// the bare <hash>).
			if nick != "" && g.msg != nil && g.msg.Nick == "" {
				g.msg.Nick = nick
				backfill = g.msg
			}
			h.lock.Unlock()
			if backfill != nil && h.Manager != nil {
				// Re-notify so the room view re-renders the backfilled
				// nick (Python re-renders on every copy's _notify_messages).
				h.Manager.NotifyMessage(h, backfill)
			}
			return true
		}
		// A same-body copy outside the window is a legitimate repeat: a
		// fresh burst whose kept copy is this one.
		g.firstTs = ts
		g.msg = msg
		h.lock.Unlock()
		return false
	}
	h.fanoutGroups[key] = &fanoutGroup{firstTs: ts, msg: msg}
	h.fanoutOrder = append(h.fanoutOrder, key)
	if len(h.fanoutOrder) > fanoutMaxKeys {
		oldest := h.fanoutOrder[0]
		h.fanoutOrder = h.fanoutOrder[1:]
		delete(h.fanoutGroups, oldest)
	}
	h.lock.Unlock()
	return false
}

// rememberSentBody records a locally sent message body so the hub's fanout
// echoes of it (unique mids, possibly rewritten source hashes) collapse with
// the local record instead of duplicating it.
func (h *RRCHub) rememberSentBody(kind, room, body string, ts int64) {
	key := fanoutKey(kind, room, body)
	h.lock.Lock()
	defer h.lock.Unlock()
	if _, ok := h.recentSentBodies[key]; !ok {
		h.recentSentOrder = append(h.recentSentOrder, key)
	}
	h.recentSentBodies[key] = ts
	if len(h.recentSentOrder) > fanoutMaxKeys {
		oldest := h.recentSentOrder[0]
		h.recentSentOrder = h.recentSentOrder[1:]
		delete(h.recentSentBodies, oldest)
	}
}

// collapseSelfEcho returns true when the copy is a fanout echo of a message
// this client sent within selfEchoWindowMs: matching the room and body is
// enough because rrcd rewrites the source hash per copy, so the echo copies
// of our own send can carry a peer's hash. Copies that arrive before the
// local send cannot match (the body is remembered before sendEnv).
func (h *RRCHub) collapseSelfEcho(kind, room, body string, ts int64) bool {
	if h.Manager == nil {
		return false
	}
	h.lock.Lock()
	sentTs, ok := h.recentSentBodies[fanoutKey(kind, room, body)]
	h.lock.Unlock()
	if !ok {
		return false
	}
	return ts >= sentTs-selfEchoWindowMs && ts <= sentTs+selfEchoWindowMs
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
				// The hub's greeting MOTD notice is pinned: standing hub
				// info that rrcd re-sends on every WELCOME, exempt from the
				// ephemeral-notice purge (the purge previously erased it
				// minutes after a connect, which made some fleet nodes
				// appear MOTD-less).
				shouldFilter := (m.Kind == "system" || m.Kind == "notice") && !m.Pinned
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
//
// Roomless notices are the hub's greeting MOTD (rrcd sends them with no room
// right after the WELCOME). Python attributes them to the manager's ACTIVE
// room; on a fresh boot no room is active, so the greeting landed in NO
// buffer and each fleet node's MOTD visibility depended on which room was
// active at arrival (2026-09-03 captures). When no room is active the
// greeting is now recorded into EVERY joined room's buffer — pinned, and
// without unread flags, so it is visible in whichever room the user opens.
func (h *RRCHub) recordNotice(msg *RRCMessage) {
	// Roomless notices ARE the hub's greeting: pin them against the
	// ephemeral purge regardless of which room they land in.
	if strings.ToLower(msg.Room) == "" {
		msg.Pinned = true
	}
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
	notify := h.Manager != nil
	// Rooms to record the notice into. A roomless greeting with no active
	// room fans out to every joined room; a roomed notice (or an active-room
	// attribution) stays single-room with Python's unread marking.
	rooms := []string{targetRoom}
	markUnread := true
	if targetRoom == "" {
		h.lock.Lock()
		rooms = sortedKeys(h.Rooms)
		h.lock.Unlock()
		markUnread = false
	}

	h.lock.Lock()
	h.Notices = append(h.Notices, msg)
	if len(h.Notices) > 200 {
		h.Notices = h.Notices[len(h.Notices)-200:]
	}
	for _, room := range rooms {
		if room == "" {
			continue
		}
		m := *msg
		m.Room = room
		buf := h.Messages[room]
		if buf == nil {
			buf = make([]*RRCMessage, 0)
		}
		buf = append(buf, &m)
		if cap > 0 && len(buf) > cap {
			buf = buf[len(buf)-cap:]
		}
		h.Messages[room] = buf
		if markUnread && notify && room != activeRoom {
			h.UnreadRooms[room] = true
		}
	}
	h.lock.Unlock()

	if notify {
		h.Manager.NotifyMessage(h, msg)
	}
	for _, room := range rooms {
		if room == "" {
			continue
		}
		h.appendHistory(room, msg)
	}
	h.cleanHistory()
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
			// Collapse duplicates with the same fanout rule the live path
			// uses (same kind+room+body within fanoutWindowMs): rrcd's
			// fanout copies share the sender's envelope timestamp, so the
			// per-member copies written by older builds collapse here, and a
			// join-time history replay of an already-loaded message does not
			// duplicate it (rrcd preserves the original ts on replay copies).
			// History entries carry no message id, so the key is the tuple.
			if h.collapseFanout(m.Kind, room, m.Text, m.Ts, m.Nick, m) {
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
	h.Status = status
	if text != "" {
		h.StatusText = text
	}
	mgr := h.Manager
	h.lock.Unlock()

	// The status transitions (Connecting/Connected/Failed/Disconnected) must
	// reach the UI: the channels hub list renders the status glyph + label and
	// only refreshes on this change notification (Python's hub status changes
	// propagate through the delegate the same way). Fired AFTER the hub lock
	// is released — the callback path ends in SetHubs/HubsSnapshot, which
	// takes the hub lock and would deadlock otherwise.
	if mgr != nil {
		mgr.NotifyChange(h)
	}
}

// Snapshot returns a consistent view of the hub's connection and room state for
// observers (tests, the TUI) that read fields concurrently with the connect
// worker and inbound-link callbacks. Rooms is copied because it is mutated in
// place; callers must not mutate the returned map.
func (h *RRCHub) Snapshot() (status int, statusText, motd string, rooms map[string]bool) {
	h.lock.Lock()
	defer h.lock.Unlock()
	rooms = make(map[string]bool, len(h.Rooms))
	for room := range h.Rooms {
		rooms[room] = h.Rooms[room]
	}
	return h.Status, h.StatusText, h.MOTD, rooms
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
	// The greeting rides as a roomless MOTD-kind resource; both notice kinds
	// route through recordNotice, with the MOTD pinned against the ephemeral
	// purge (the greeting is standing hub info re-sent on every WELCOME).
	h.recordNotice(&RRCMessage{
		Kind:   "notice",
		Room:   room,
		Text:   text,
		Ts:     NowMs(),
		Pinned: kind == ResKindMOTD,
	})
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

// HubAddressHex returns the hub's destination hash as lowercase hex
// (Python hub.hub_hash.hex(), shown in the hub info panel).
func (h *RRCHub) HubAddressHex() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return hexString(h.HubHash)
}

// GetStatusText returns the detailed connection status text (e.g.
// "Connected", "WELCOME timeout") shown next to the status label.
func (h *RRCHub) GetStatusText() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.StatusText
}

// GetMOTD returns the hub's message of the day (empty before the WELCOME).
func (h *RRCHub) GetMOTD() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.MOTD
}

// GetAutoReconnect reports the auto-reconnect toggle state.
func (h *RRCHub) GetAutoReconnect() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.AutoReconnect
}

// GetAutoList reports the auto room-list toggle state.
func (h *RRCHub) GetAutoList() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.AutoList
}

// GetAutoWho reports the auto who toggle state.
func (h *RRCHub) GetAutoWho() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.AutoWho
}

// SetAvailableRooms replaces the hub's advertised room set and notifies the
// UI (Python _process_notice_text assigns available_rooms then notifies).
func (h *RRCHub) SetAvailableRooms(rooms map[string]*string) {
	h.lock.Lock()
	h.AvailableRooms = rooms
	h.lock.Unlock()
	if h.Manager != nil {
		h.Manager.NotifyChange(h)
	}
}

// GetAvailableRoomList returns the sorted names of the rooms the hub
// advertises but the client has not joined.
func (h *RRCHub) GetAvailableRoomList() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	names := make([]string, 0, len(h.AvailableRooms))
	for name := range h.AvailableRooms {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
