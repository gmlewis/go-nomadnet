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

package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

func TestNewNetworkDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if nd == nil {
		t.Fatal("NewNetworkDisplay returned nil")
	}
	if nd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestNetworkDisplayWithEntries(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	announces := []AnnounceEntry{
		{DisplayName: "Node1", Type: "node", Timestamp: time.Now()},
		{DisplayName: "Peer1", Type: "peer", Timestamp: time.Now().Add(-time.Hour)},
	}
	nodes := []NodeEntry{
		{DisplayName: "Server1", TrustLevel: "trusted", Delivery: "direct"},
	}

	nd := NewNetworkDisplay(app, announces, nodes)
	if nd == nil {
		t.Fatal("NewNetworkDisplay returned nil")
	}
}

// TestNetworkDetailActionCallbacks verifies the AnnounceInfo in-detail buttons
// fire their callbacks and return to the stream view (Python's save_node /
// msg_op / use_pn / converse all pop back to the announce stream).
func TestNetworkDetailActionCallbacks(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	announces := []AnnounceEntry{
		{DisplayName: "Node1", Type: "node", SourceHash: "aa", Timestamp: time.Now()},
		{DisplayName: "PN1", Type: "pn", SourceHash: "bb", Timestamp: time.Now()},
		{DisplayName: "Peer1", Type: "peer", SourceHash: "cc", Timestamp: time.Now()},
	}
	nd := NewNetworkDisplay(app, announces, nil)

	var fired string
	nd.OnSaveNode = func() { fired = "save" }
	nd.OnMsgOp = func(string) { fired = "msgop" }
	nd.OnUseAsPN = func() { fired = "useaspn" }
	nd.OnConverse = func() { fired = "converse" }

	cases := []struct {
		name string
		idx  int
		call func()
		want string
	}{
		{"save node", 0, func() { nd.saveNode(announces[0]) }, "save"},
		{"msg op", 0, func() { nd.msgOpNode("") }, "msgop"},
		{"use as pn", 1, func() { nd.useAsPN(announces[1]) }, "useaspn"},
		{"converse", 2, func() { nd.converseWith(announces[2]) }, "converse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fired = ""
			nd.showAnnounceDetail(c.idx) // enter the AnnounceInfo detail view
			if !nd.inInfoView {
				t.Fatal("showAnnounceDetail did not enter info view")
			}
			c.call()
			if fired != c.want {
				t.Errorf("callback fired %q, want %q", fired, c.want)
			}
			if nd.inInfoView {
				t.Error("expected to return to stream view after action")
			}
		})
	}
}

func TestTruncateStr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 10, "hello w..."},
		{"hi", 2, "hi"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncateStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncateStr(%q, %v) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestFormatAnnounce(t *testing.T) {
	t.Parallel()

	ann := AnnounceEntry{
		DisplayName: "TestNode",
		Type:        "node",
		TrustLevel:  "trusted",
		SourceHash:  "abc123",
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		AppData:     "Test data",
	}

	result := formatAnnounce(ann)
	if result == "" {
		t.Error("formatAnnounce returned empty")
	}
}

func TestNewBrowserDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	if bd == nil {
		t.Fatal("NewBrowserDisplay returned nil")
	}
	if bd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestBrowserLoadURL(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("http://example.com")
	if bd.URLDisplayText() != "http://example.com" {
		t.Errorf("URL display = %q, want %q", bd.URLDisplayText(), "http://example.com")
	}
}

func TestBrowserHistory(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("url1")
	bd.LoadURL("url2")
	bd.LoadURL("url3")

	bd.GoBack()
	if bd.URLDisplayText() != "url2" {
		t.Errorf("After GoBack, URL = %q, want %q", bd.URLDisplayText(), "url2")
	}

	bd.GoForward()
	if bd.URLDisplayText() != "url3" {
		t.Errorf("After GoForward, URL = %q, want %q", bd.URLDisplayText(), "url3")
	}
}

func TestNewMicronViewDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMicronViewDisplay(app)

	if mvd == nil {
		t.Fatal("NewMicronViewDisplay returned nil")
	}
	if mvd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestMicronViewRenderPage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMicronViewDisplay(app)

	mvd.RenderPage("Hello World")
	// Should not panic
}

func TestMicronViewClear(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMicronViewDisplay(app)

	mvd.RenderPage("test")
	mvd.Clear()
	// Should not panic
}

func TestRenderNodes(t *testing.T) {
	t.Parallel()

	nodes := micron.Parse("Hello World")
	result := renderNodes(nodes)
	if result == "" {
		t.Error("renderNodes returned empty")
	}
}

func TestMapColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, want string
	}{
		{"red", "red"},
		{"f00", "red"},
		{"green", "green"},
		{"0f0", "green"},
		{"blue", "blue"},
		{"00f", "blue"},
		{"unknown", "#unknown"},
		{"#ff8080", "#ff8080"},
	}

	for _, tt := range tests {
		got := mapColor(tt.input)
		if got != tt.want {
			t.Errorf("mapColor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNetworkDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	var fired []string
	nd.OnToggleFullscreen = func() { fired = append(fired, "fullscreen") }
	nd.OnEditNode = func() { fired = append(fired, "edit_node") }
	nd.OnShowPeers = func() { fired = append(fired, "peers") }
	nd.OnDisconnect = func() { fired = append(fired, "disconnect") }
	nd.OnURLDialog = func() { fired = append(fired, "url") }
	nd.OnSaveNode = func() { fired = append(fired, "save") }
	nd.OnDeleteSelected = func() { fired = append(fired, "delete") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-l", tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModNone), ""},
		{"ctrl-g", tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone), "fullscreen"},
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit_node"},
		{"ctrl-p", tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModNone), "peers"},
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "disconnect"},
		{"ctrl-u", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), "url"},
		{"ctrl-s", tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModNone), "save"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := nd.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %v was not consumed", tt.name)
			}
			if tt.want == "" {
				if len(fired) != 0 {
					t.Errorf("key %v should not fire callbacks, fired %v", tt.name, fired)
				}
			} else {
				if len(fired) != 1 || fired[0] != tt.want {
					t.Errorf("key %v fired %v, want [%v]", tt.name, fired, tt.want)
				}
			}
		})
	}
}

// TestNetworkCtrlGFullscreenResizesLeftPane pins the Ctrl-G fullscreen fix for
// the Network page. Before the fix, NetworkDisplay.handleInput Ctrl-G only
// fired the OnToggleFullscreen callback, which the app layer never wired — so
// the keypress was a no-op and the left list pane never collapsed (Python's
// Network.toggle_fullscreen hides it so the in-page browser fills the page,
// Network.py:1678-1686). The fix makes the display self-toggle the resize, so
// no app-layer callback is needed. This dispatches Ctrl-G through the FULL
// production key path — nd.mainCols.InputHandler() (the same handler tview
// calls on a keypress), whose WrapInputHandler consults the SetInputCapture
// registered in NewNetworkDisplay (network.go mainCols.SetInputCapture) before
// forwarding to the focused child — with NO OnToggleFullscreen wired, and
// asserts the left column width actually flips 52 → 0 → 52. Dispatching through
// the capture chain (not handleInput directly) also pins that the capture is
// registered, so the resize can't be silently dropped by a missing callback OR
// a missing capture registration again.
func TestNetworkCtrlGFullscreenResizesLeftPane(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if nd.Fullscreen() {
		t.Fatal("display should not start fullscreen")
	}
	if w := nd.mainCols.FixedWidth(0); w != 52 {
		t.Fatalf("initial left pane width = %v, want 52", w)
	}

	// Ctrl-G through the full key path: mainCols.InputHandler() is what tview
	// invokes on a keypress; its WrapInputHandler runs the SetInputCapture
	// (nd.handleInput) before any child sees the key. No OnToggleFullscreen is
	// wired (mirroring production before the fix), so the resize must happen
	// inside the display itself, not via a callback.
	h := nd.mainCols.InputHandler()
	if h == nil {
		t.Fatal("mainCols has no InputHandler")
	}
	h(tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone), nil)
	if !nd.Fullscreen() {
		t.Error("Ctrl-G did not toggle the display to fullscreen")
	}
	if w := nd.mainCols.FixedWidth(0); w != 0 {
		t.Errorf("fullscreen left pane width = %v, want 0 (browser should fill page)", w)
	}

	// A second Ctrl-G restores the saved list width.
	h(tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone), nil)
	if nd.Fullscreen() {
		t.Error("second Ctrl-G did not restore the normal view")
	}
	if w := nd.mainCols.FixedWidth(0); w != 52 {
		t.Errorf("restored left pane width = %v, want 52", w)
	}
}

// TestNetworkToggleFullscreenFiresCallback verifies ToggleFullscreen still
// fires the display-level OnToggleFullscreen (kept for app-layer hooks), even
// though the resize no longer depends on it.
func TestNetworkToggleFullscreenFiresCallback(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	var fired int
	nd.OnToggleFullscreen = func() { fired++ }

	nd.ToggleFullscreen()
	nd.ToggleFullscreen()

	if fired != 2 {
		t.Errorf("OnToggleFullscreen fired %v times, want 2", fired)
	}
}

func TestShowLocalPeerDialog(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	// Should not panic
	nd.ShowLocalPeerDialog("addr123", "ident456", "MyPeer", "2h ago")
}

func TestShowLXMFPeersDialogEmpty(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	nd.ShowLXMFPeersDialog(nil)
}

func TestShowLXMFPeersDialogWithPeers(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	peers := []LXMFPeerEntry{
		{Hash: "aabb1122", Name: "Peer1", Alive: true, Pending: 3},
		{Hash: "ccdd3344", Name: "Peer2", Alive: false, Pending: 0},
	}
	nd.ShowLXMFPeersDialog(peers)
}
