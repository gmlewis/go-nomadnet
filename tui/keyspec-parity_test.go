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

// Phase-1 keyspec parity: the Python UI source is the SPEC. A static AST scan
// (tooling/keyspec/extract_py.py) derives every keypress/mouse_event/
// unhandled_input handler with the keys it handles, plus the Ctrl-keys the
// shortcut bars advertise, fresh from the current Python checkout on every
// run. This test extracts the SAME surface from the Go port's source (the
// tcell key cases in each handler function and the [C-x] tokens in its
// shortcut-bar strings) and diffs the two, keyed by a static mapping of
// Python widget classes to the Go functions that port them.
//
// Phase 1 is purely static: neither TUI runs, so the two implementations
// never need to be alive at the same time (the shared RNS instance can host
// at most one client stack; these tests run neither).
//
// Out of scope (documented, not accepted-divergences):
//   - implicit widget bindings (urwid ListBox/Pile/Frame traversal vs tview
//     List/Flex defaults) — covered by the behavioral suites;
//   - mouse handlers beyond button presence (tview MouseHandlers are closures).
//
// urwid dispatches keys bottom-up (focused child first); tview runs ancestor
// InputCaptures top-down. Several accepted entries record where a Python
// handler's key landed in Go's ancestor-capture architecture instead.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// keyspecPair maps one Python spec handler to the Go function(s) that port
// it. The diff compares the UNION of their canonical handled-key sets against
// the Python set (Go often splits one urwid keypress across dispatcher/
// composer/widget functions to mirror Python's bottom-up dispatch).
type keyspecPair struct {
	pyFile  string   // basename of the Python UI file
	pyClass string   // Python widget class
	pyMeth  string   // Python method ("keypress"/"unhandled_input")
	goFile  string   // basename of the Go tui file
	goRecv  string   // Go receiver type
	goFuncs []string // Go function names; union of their handled keys
}

// keyspecDisplayPairs covers the display-level handlers plus the sub-widget
// handlers already verified as ported. Extend as new handlers are audited.
var keyspecDisplayPairs = []keyspecPair{
	{"Main.py", "MenuColumns", "keypress", "main-display.go", "MainDisplay", []string{"handleMenuInput"}},
	{"TextUI.py", "TextUI", "unhandled_input", "main-display.go", "MainDisplay", []string{"handleInput"}},
	{"Conversations.py", "ConversationsArea", "keypress", "conversations.go", "ConversationsDisplay", []string{"handleInput"}},
	{"Conversations.py", "ConversationWidget", "keypress", "conversation-widget.go", "ConversationWidget", []string{"handleInput", "handleWidgetKey", "handleComposerKey"}},
	{"Conversations.py", "MessageEdit", "keypress", "conversation-widget.go", "ConversationWidget", []string{"handleComposerKey"}},
	{"Network.py", "NetworkLeftPile", "keypress", "network.go", "NetworkDisplay", []string{"handleInput"}},
	{"Network.py", "AnnounceStream", "keypress", "network.go", "NetworkDisplay", []string{"handleInput"}},
	{"Network.py", "KnownNodes", "keypress", "network.go", "NetworkDisplay", []string{"handleInput"}},
	{"Network.py", "LXMFPeers", "keypress", "lxmf-peers.go", "LXMFPeersDisplay", []string{"handlePeerKey"}},
	{"Channels.py", "ChannelsListArea", "keypress", "channels.go", "ChannelsDisplay", []string{"handleInput"}},
	{"Channels.py", "RoomFrame", "keypress", "room-widget.go", "RoomWidget", []string{"handleInput"}},
	{"Channels.py", "RoomMessageEdit", "keypress", "room-widget.go", "RoomWidget", []string{"handleInput"}},
	{"Channels.py", "UsersBox", "keypress", "room-widget.go", "RoomWidget", []string{"handleInput"}},
	{"Browser.py", "BrowserFrame", "keypress", "browser.go", "BrowserDisplay", []string{"handleInput"}},
	{"Interfaces.py", "InterfaceFiller", "keypress", "interfaces.go", "InterfacesDisplay", []string{"handleInput"}},
	{"Log.py", "LogTerminal", "keypress", "log.go", "LogDisplay", []string{"handleInput"}},
	{"Config.py", "ConfigFiller", "keypress", "config.go", "ConfigDisplay", []string{"handleInput"}},
}

// keyspecAccepted records reviewed, deliberate divergences:
// "pyFile:pyClass|canonicalKey" → one-line reason. Anything not listed fails.
var keyspecAccepted = map[string]string{
	// --- Main / global ---------------------------------------------------
	// Python's unhandled_input intercepts ctrl-e and does `pass` (a global
	// swallow); Go deliberately does NOT swallow Ctrl-E globally — the
	// Conversations page consumes it (Conversations.py:89 has the page-level
	// binding), so a global swallow would break the Go page handler. Ctrl-E
	// reaches the page's peer-info action in both.
	"TextUI.py:TextUI|ctrl+e": "Python swallows ctrl-e globally (pass); Go delivers it to the focused page which handles it — same observable behavior",
	// Go additionally routes Ctrl-C to quit: Python's urwid loop turns Ctrl-C
	// into KeyboardInterrupt before unhandled_input ever runs (atexit then
	// saves the directory). Same observable behavior, different surface.
	"TextUI.py:TextUI|ctrl+c": "Go routes Ctrl-C to quit for SIGINT parity; Python relies on KeyboardInterrupt",
	// Go's dispatcher owns Esc (dialog dismissal) and Up-at-list-top (body →
	// menu collapse) app-wide; Python gets these from per-dialog keypresses
	// and urwid Frame/Pile traversal respectively.
	"TextUI.py:TextUI|esc": "Go's dispatcher routes Esc to DismissTop app-wide; Python handles Esc in each dialog's keypress",
	"TextUI.py:TextUI|up":  "Go's dispatcher owns Up-at-list-top → menu collapse; Python gets it from urwid Frame traversal",
	// MenuColumns forwards Tab/Down to the body and returns super().keypress;
	// urwid drops the forwarded key (verified against the live nomadnet pane —
	// Main.py:171-176 comment). Go consumes it after FocusBody. Left/Right/
	// Enter/Space menu navigation is explicit in Go; Python does it through
	// urwid Columns traversal and MenuButton ACTIVATE.
	"Main.py:MenuColumns|tab":        "Go consumes Tab after FocusBody, mirroring urwid's drop of the forwarded key",
	"Main.py:MenuColumns|down":       "Go consumes Down after FocusBody, mirroring urwid's drop of the forwarded key",
	"Main.py:MenuColumns|left":       "Go's menu dispatcher moves the highlight Left; Python does this in urwid Columns traversal",
	"Main.py:MenuColumns|right":      "Go's menu dispatcher moves the highlight Right; Python does this in urwid Columns traversal",
	"Main.py:MenuColumns|enter":      "Go's menu dispatcher activates the highlighted item on Enter; Python fires MenuButton's on_press via urwid ACTIVATE",
	"Main.py:MenuColumns|rune:space": "Go's menu dispatcher activates on Space; Python fires MenuButton's on_press via urwid ACTIVATE",

	// --- Conversations list area -----------------------------------------
	// Go's KeyDown/KeyLeft/KeyRight cases own the focus paths urwid gets from
	// Pile/Columns traversal (checkbox → list, tab buttons, conversation
	// column): Conversations.py relies on container traversal for these.
	"Conversations.py:ConversationsArea|down":  "Go owns the Pile/Columns focus paths explicitly (checkbox/tab-bar/list); Python uses urwid container traversal",
	"Conversations.py:ConversationsArea|left":  "Go owns the Columns focus paths explicitly; Python uses urwid Columns traversal",
	"Conversations.py:ConversationsArea|right": "Go owns the Columns focus paths explicitly (right → conversation column); Python uses urwid Columns traversal",

	// --- Conversation widget (frame/composer/widget split) ---------------
	// Go's widget-level switch also binds MessageEdit's C-d/C-f/C-p (and the
	// frame-focus arrows) so compose actions and focus paths work while the
	// BODY has focus; in Python those keys only act when the composer (footer)
	// has focus — urwid dispatches bottom-up. Documented deliberate extra.
	"Conversations.py:ConversationWidget|ctrl+d": "Go binds send/paper/attach at widget level so compose actions work with body focus; Python only reaches them from the composer",
	"Conversations.py:ConversationWidget|ctrl+f": "Go binds send/paper/attach at widget level so compose actions work with body focus; Python only reaches them from the composer",
	"Conversations.py:ConversationWidget|ctrl+p": "Go binds send/paper/attach at widget level so compose actions work with body focus; Python only reaches them from the composer",
	"Conversations.py:ConversationWidget|up":     "Go owns the Frame focus paths (editor → message list → banner → menu) explicitly; Python uses urwid Frame traversal",
	"Conversations.py:ConversationWidget|down":   "Go owns the Frame focus paths explicitly; Python uses urwid Frame traversal",
	"Conversations.py:ConversationWidget|left":   "Go owns the Frame focus paths explicitly; Python uses urwid Frame traversal",
	"Conversations.py:ConversationWidget|right":  "Go owns the Frame focus paths explicitly; Python uses urwid Frame traversal",
	// The composer's Down hands the key to the content editor in the full
	// editor (Python full_editor Pile traversal).
	"Conversations.py:MessageEdit|down": "Go's composer Down mirrors the full-editor title editor handing the key to the content editor (urwid Pile traversal in Python)",

	// --- Network ----------------------------------------------------------
	// Up-at-top → menubar is owned by Go's dispatcher (bodyListAtTop) instead
	// of each network sub-display's keypress.
	"Network.py:NetworkLeftPile|up": "Go's dispatcher owns Up-at-top → menu collapse; Python does it in the sub-display keypress",
	"Network.py:AnnounceStream|up":  "Go's dispatcher owns Up-at-top → menu collapse; Python does it in AnnounceStream.keypress",
	"Network.py:KnownNodes|up":      "Go's dispatcher owns Up-at-top → menu collapse; Python does it in KnownNodes.keypress",
	"Network.py:LXMFPeers|up":       "Go's dispatcher owns Up-at-top → menu collapse; Python does it in LXMFPeers.keypress",

	// --- Channels ----------------------------------------------------------
	// RoomFrame/RoomMessageEdit/UsersBox keys merged into RoomWidget's single
	// handler; C-y toggle-channel-list is owned by the ChannelsDisplay
	// ancestor capture (channels.go:453, Python ChannelsListArea:370) so the
	// room-level binding is redundant in Go's top-down architecture.
	"Channels.py:RoomFrame|ctrl+y":       "ChannelsDisplay's ancestor capture owns C-y (toggle channel list) app-wide; the room-level binding is redundant in Go",
	"Channels.py:RoomMessageEdit|ctrl+y": "ChannelsDisplay's ancestor capture owns C-y (toggle-channel-list); redundant at room level in Go",
	"Channels.py:UsersBox|ctrl+y":        "ChannelsDisplay's ancestor capture owns C-y (toggle-channel-list); redundant at room level in Go",
	// The room widget merges Python's three handlers: MessageEdit's send and
	// UsersBox's frame-focus keys all live in RoomWidget.handleInput.
	"Channels.py:RoomFrame|ctrl+d":       "RoomWidget merges MessageEdit's C-d send; Python reaches it via the composer's keypress",
	"Channels.py:RoomMessageEdit|ctrl+u": "RoomWidget merges UsersBox's C-u toggle-users; Python scopes it to the user box",
	"Channels.py:UsersBox|ctrl+d":        "RoomWidget merges MessageEdit's C-d send; Python scopes it to the room editor",
	"Channels.py:UsersBox|ctrl+x":        "RoomWidget merges RoomFrame's C-x leave; Python scopes it to the room frame",
	"Channels.py:UsersBox|f8":            "RoomWidget merges RoomFrame's F8 collapse; Python scopes it to the room frame",
	// Room composer Up-at-cursor-0 escape: Go's room composer hands the key to
	// the tview editor + room focus routing; behavioral suites cover it.
	"Channels.py:RoomMessageEdit|up": "Go room composer Up is handled by the underlying editor/focus routing; covered by behavioral suites",
	// ChannelsListArea: Up-at-top → menubar is owned by the dispatcher.
	"Channels.py:ChannelsListArea|up": "Go's dispatcher owns Up-at-top → menu collapse; Python does it in ChannelsListArea.keypress",
	// The channels-level ancestor capture owns C-d send for the whole page
	// (merged RoomMessageEdit binding); Python scopes it to the room editor.
	"Channels.py:ChannelsListArea|ctrl+d": "ChannelsDisplay's ancestor capture owns C-d send app-wide; Python scopes it to the room editor",

	// --- Network page shared dispatch ---------------------------------------
	// NetworkDisplay.handleInput is ONE switch serving every Network
	// sub-display (left pile, announce stream, known nodes), so each Python
	// class sees the others' keys in its Go counterpart. The per-sub-view
	// ownership matches Python at runtime: each switch branch targets the
	// sub-view that is actually focused.
	"Network.py:NetworkLeftPile|ctrl+x": "shared NetworkDisplay dispatch: C-x belongs to AnnounceStream/KnownNodes delete; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+e":  "shared NetworkDisplay dispatch: C-e belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+g":  "shared NetworkDisplay dispatch: C-g belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+l":  "shared NetworkDisplay dispatch: C-l belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+p":  "shared NetworkDisplay dispatch: C-p belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+s":  "shared NetworkDisplay dispatch: C-s belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+u":  "shared NetworkDisplay dispatch: C-u belongs to the left pile; the switch serves all network sub-views",
	"Network.py:AnnounceStream|ctrl+w":  "shared NetworkDisplay dispatch: C-w belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+e":      "shared NetworkDisplay dispatch: C-e belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+g":      "shared NetworkDisplay dispatch: C-g belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+l":      "shared NetworkDisplay dispatch: C-l belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+p":      "shared NetworkDisplay dispatch: C-p belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+s":      "shared NetworkDisplay dispatch: C-s belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+u":      "shared NetworkDisplay dispatch: C-u belongs to the left pile; the switch serves all network sub-views",
	"Network.py:KnownNodes|ctrl+w":      "shared NetworkDisplay dispatch: C-w belongs to the left pile; the switch serves all network sub-views",

	// The composer bubbles C-t/C-x/C-g/C-o to the widget-level shortcuts while
	// typing — exactly what Python's super() forwarding achieves.
	"Conversations.py:MessageEdit|ctrl+t": "Go's composer bubbles C-t/C-x/C-g/C-o to the widget-level shortcuts, mirroring Python's super() forwarding",
	"Conversations.py:MessageEdit|ctrl+x": "Go's composer bubbles C-t/C-x/C-g/C-o to the widget-level shortcuts, mirroring Python's super() forwarding",
	"Conversations.py:MessageEdit|ctrl+g": "Go's composer bubbles C-t/C-x/C-g/C-o to the widget-level shortcuts, mirroring Python's super() forwarding",
	"Conversations.py:MessageEdit|ctrl+o": "Go's composer bubbles C-t/C-x/C-g/C-o to the widget-level shortcuts, mirroring Python's super() forwarding",

	// --- Interfaces ---------------------------------------------------------
	// Go's interface list handles the navigation keys explicitly; Python gets
	// them from urwid ListBox traversal inside InterfaceFiller.
	"Interfaces.py:InterfaceFiller|up":    "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|down":  "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|home":  "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|end":   "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|pgup":  "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|pgdn":  "Go owns interface-list navigation explicitly; Python uses urwid ListBox traversal",
	"Interfaces.py:InterfaceFiller|enter": "Go opens the selected interface explicitly; Python uses urwid ACTIVATE on the list entry",
}

// keyspecCanonical maps an urwid key name to the canonical token used on both
// sides of the diff.
func keyspecCanonical(pyKey string) string {
	k := strings.ToLower(strings.TrimSpace(pyKey))
	switch k {
	case "activate":
		return "activate"
	case "enter":
		return "enter"
	case "esc":
		return "esc"
	case "tab":
		return "tab"
	case "shift tab":
		return "shift+tab"
	case "up":
		return "up"
	case "down":
		return "down"
	case "left":
		return "left"
	case "right":
		return "right"
	case "page up":
		return "pgup"
	case "page down":
		return "pgdn"
	case "home":
		return "home"
	case "end":
		return "end"
	case "backspace":
		return "backspace"
	case "delete":
		return "delete"
	case "space":
		return "rune:space"
	}
	if strings.HasPrefix(k, "ctrl ") && len(k) > 5 {
		return "ctrl+" + k[5:]
	}
	if strings.HasPrefix(k, "f") && len(k) <= 3 {
		if _, err := strconv.Atoi(k[1:]); err == nil {
			return k
		}
	}
	if len(k) == 1 {
		return "rune:" + k
	}
	return "raw:" + k
}

// keyspecGoCanonical maps a tcell key constant name to the same canonical form.
func keyspecGoCanonical(name string) string {
	n := strings.TrimPrefix(name, "Key")
	switch {
	case n == "Enter":
		return "enter"
	case n == "Esc", n == "Escape", n == "CtrlLeftSq":
		return "esc"
	case n == "Tab":
		return "tab"
	case n == "Backtab":
		return "shift+tab"
	case n == "Up", n == "Down", n == "Left", n == "Right":
		return strings.ToLower(n)
	case n == "PgUp":
		return "pgup"
	case n == "PgDn":
		return "pgdn"
	case n == "Home":
		return "home"
	case n == "End":
		return "end"
	case n == "Backspace", n == "Backspace2":
		return "backspace"
	case n == "Delete":
		return "delete"
	case strings.HasPrefix(n, "Ctrl") && len(n) > 4:
		return "ctrl+" + strings.ToLower(n[4:])
	case strings.HasPrefix(n, "F") && len(n) <= 3:
		if _, err := strconv.Atoi(n[1:]); err == nil {
			return strings.ToLower(n)
		}
	}
	return "raw:Key" + n
}

var (
	keyspecGoFuncRe = regexp.MustCompile(`^func \((\w+) \*?(\w+)\) (\w+)\(`)
	keyspecCaseRe   = regexp.MustCompile(`tcell\.Key(\w+)`)
	keyspecEqRe     = regexp.MustCompile(`[!=]=\s*tcell\.Key(\w+)`)
	keyspecRuneRe   = regexp.MustCompile(`event\.Rune\(\) == '([^']+)'`)
	keyspecAdvertRe = regexp.MustCompile(`\[C-([a-zA-Z])\]`)
)

// keyspecPyHandler is one record of the extracted Python spec.
type keyspecPyHandler struct {
	File   string `json:"file"`
	Class  string `json:"class"`
	Method string `json:"method"`
	Line   int    `json:"line"`
	Keys   []struct {
		Key    string `json:"key"`
		Action string `json:"action"`
	} `json:"keys"`
}

type keyspecPySpec struct {
	Handlers   []keyspecPyHandler `json:"handlers"`
	Advertised []struct {
		File string   `json:"file"`
		Line int      `json:"line"`
		Keys []string `json:"keys"`
		Bar  string   `json:"bar"`
	} `json:"advertised"`
}

// keyspecLoadSpec runs the extractor against the Python checkout and returns
// the full spec (handlers + advertised bars).
func keyspecLoadSpec(t *testing.T) *keyspecPySpec {
	t.Helper()
	testutils.SkipIfNoPythonNomadnet(t)

	sot := ""
	for _, cand := range []string{
		"../original-nomadnet-repo",
		filepath.Join(os.Getenv("HOME"), "src/github.com/markqvist/nomadnet"),
	} {
		if st, err := os.Stat(filepath.Join(cand, "nomadnet", "ui", "textui")); err == nil && st.IsDir() {
			sot = cand
			break
		}
	}
	if sot == "" {
		t.Skip("no Python nomadnet source checkout available for static spec extraction")
	}

	script := filepath.Join("..", "tooling", "keyspec", "extract_py.py")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("extractor %s not accessible from package cwd: %v", script, err)
	}
	cmd := exec.Command(testutils.PythonNomadnetExe(), script, sot)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("keyspec extractor failed: %v\nstderr:\n%s", err, stderr.String())
	}
	var spec keyspecPySpec
	if err := json.Unmarshal(stdout, &spec); err != nil {
		t.Fatalf("decode keyspec extractor output: %v\nstderr:\n%s", err, stderr.String())
	}
	return &spec
}

// keyspecGoHandlers scans the Go tui source and returns, per
// "file:Type:func", the canonical keys handled by explicit case/comparison.
func keyspecGoHandlers(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read tui dir: %v", err)
	}
	out := map[string]map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		cur := ""
		for _, line := range strings.Split(string(data), "\n") {
			if m := keyspecGoFuncRe.FindStringSubmatch(line); m != nil {
				cur = fmt.Sprintf("%s:%s:%s", name, m[2], m[3])
				if out[cur] == nil {
					out[cur] = map[string]bool{}
				}
				continue
			}
			if cur == "" {
				continue
			}
			trimmed := strings.TrimSpace(line)
			isCase := strings.HasPrefix(trimmed, "case ") && strings.Contains(trimmed, "tcell.Key")
			isEq := keyspecEqRe.MatchString(trimmed)
			if isCase {
				for _, m := range keyspecCaseRe.FindAllStringSubmatch(trimmed, -1) {
					if m[1] == "Rune" {
						continue // runes only count via a Rune() comparison
					}
					out[cur][keyspecGoCanonical("Key"+m[1])] = true
				}
			}
			if isEq {
				for _, m := range keyspecEqRe.FindAllStringSubmatch(trimmed, -1) {
					if m[1] == "Rune" {
						continue
					}
					out[cur][keyspecGoCanonical("Key"+m[1])] = true
				}
			}
			for _, m := range keyspecRuneRe.FindAllStringSubmatch(trimmed, -1) {
				r := m[1]
				if r == " " {
					r = "space"
				}
				out[cur]["rune:"+r] = true
			}
		}
	}
	return out
}

// keyspecGoAdvertised returns, per Go tui file, the set of Ctrl-keys its
// shortcut-bar strings advertise.
func keyspecGoAdvertised(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read tui dir: %v", err)
	}
	out := map[string]map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		set := map[string]bool{}
		for _, m := range keyspecAdvertRe.FindAllStringSubmatch(string(data), -1) {
			set["ctrl+"+strings.ToLower(m[1])] = true
		}
		if len(set) > 0 {
			out[name] = set
		}
	}
	return out
}

func keyspecSorted(set map[string]bool) []string {
	var ks []string
	for k := range set {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// keyspecPyKeys returns the canonical handled-key set of one Python handler.
func keyspecPyKeys(h keyspecPyHandler) map[string]bool {
	set := map[string]bool{}
	for _, k := range h.Keys {
		set[keyspecCanonical(k.Key)] = true
	}
	return set
}

// keyspecPyActionOf returns the recorded action for one canonical key.
func keyspecPyActionOf(h keyspecPyHandler, canonical string) string {
	for _, k := range h.Keys {
		if keyspecCanonical(k.Key) == canonical {
			if k.Action == "" {
				return "focus/forward"
			}
			return k.Action
		}
	}
	return "?"
}

// TestKeyspecGoAdvertisedKeysAreHandled pins the Go port's self-consistency:
// every Ctrl-key a Go shortcut bar advertises must be handled by an explicit
// handler case somewhere in the app. An advertised-but-dead key is exactly how
// the Ctrl-X delete bug presented to the user — the bar promised an action the
// state could never deliver.
func TestKeyspecGoAdvertisedKeysAreHandled(t *testing.T) {
	t.Parallel()

	handlers := keyspecGoHandlers(t)
	handledAnywhere := map[string]bool{}
	handledByFile := map[string]map[string]bool{}
	for id, keys := range handlers {
		file := strings.SplitN(id, ":", 2)[0]
		if handledByFile[file] == nil {
			handledByFile[file] = map[string]bool{}
		}
		for k := range keys {
			handledAnywhere[k] = true
			handledByFile[file][k] = true
		}
	}

	advertised := keyspecGoAdvertised(t)
	files := make([]string, 0, len(advertised))
	for f := range advertised {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		for _, k := range keyspecSorted(advertised[f]) {
			if !handledAnywhere[k] {
				t.Errorf("Go file %s advertises [%v] in a shortcut bar but no handler in the app consumes it (advertised-but-dead key)", f, strings.TrimPrefix(k, "ctrl+"))
				continue
			}
			if !handledByFile[f][k] {
				t.Logf("note: %s advertises [%v], handled in a sibling page file — fine", f, strings.TrimPrefix(k, "ctrl+"))
			}
		}
	}
}

// TestKeyspecDisplayHandlersMatchPython diffs each display-level handler pair.
// Missing Go keys are missing behavior; extra Go keys are port inventions.
// Both fail unless listed in keyspecAccepted with a reason.
func TestKeyspecDisplayHandlersMatchPython(t *testing.T) {
	t.Parallel()

	spec := keyspecLoadSpec(t)
	py := map[string]keyspecPyHandler{}
	for _, h := range spec.Handlers {
		py[fmt.Sprintf("%s:%s:%s", filepath.Base(h.File), h.Class, h.Method)] = h
	}
	goHandlers := keyspecGoHandlers(t)

	for _, pair := range keyspecDisplayPairs {
		pyID := fmt.Sprintf("%s:%s:%s", pair.pyFile, pair.pyClass, pair.pyMeth)
		goID := fmt.Sprintf("%s:%s:%v", pair.goFile, pair.goRecv, pair.goFuncs)

		pyh, ok := py[pyID]
		if !ok {
			t.Errorf("Python spec lost %s (extractor drift?)", pyID)
			continue
		}
		goKeys := map[string]bool{}
		for _, fn := range pair.goFuncs {
			gk, ok := goHandlers[fmt.Sprintf("%s:%s:%s", pair.goFile, pair.goRecv, fn)]
			if !ok {
				t.Errorf("Go handler %s:%s:%s not found (renamed? update keyspecDisplayPairs)", pair.goFile, pair.goRecv, fn)
				continue
			}
			for k := range gk {
				goKeys[k] = true
			}
		}

		pyKeys := keyspecPyKeys(pyh)
		pairName := pair.pyFile + ":" + pair.pyClass
		for _, k := range keyspecSorted(pyKeys) {
			if !goKeys[k] {
				if _, acc := keyspecAccepted[pairName+"|"+k]; acc {
					continue
				}
				t.Errorf("%s: Python handles %q (%v) but Go %s does not — missing port behavior",
					pairName, k, keyspecPyActionOf(pyh, k), goID)
			}
		}
		for _, k := range keyspecSorted(goKeys) {
			if !pyKeys[k] {
				if _, acc := keyspecAccepted[pairName+"|"+k]; acc {
					continue
				}
				t.Errorf("%s: Go %s handles %q but Python %s does not — port invention (accept with a reason or remove)",
					pairName, goID, k, pyID)
			}
		}
	}
}

// TestKeyspecPythonAdvertisedKeysReported runs the Python side of the
// advertised-vs-handled check as a REPORT (not a failure): Python's own bars
// advertise keys some of its handlers never consume (e.g. the Network bar's
// browser keys C-d/C-f/C-r). Those are spec oddities to be aware of, not Go
// bugs; the Go-side advertised check is the one that must hold.
func TestKeyspecPythonAdvertisedKeysReported(t *testing.T) {
	t.Parallel()

	spec := keyspecLoadSpec(t)

	handledByFile := map[string]map[string]bool{}
	for _, h := range spec.Handlers {
		file := filepath.Base(h.File)
		if handledByFile[file] == nil {
			handledByFile[file] = map[string]bool{}
		}
		for _, k := range h.Keys {
			handledByFile[file][keyspecCanonical(k.Key)] = true
		}
	}

	for _, adv := range spec.Advertised {
		file := filepath.Base(adv.File)
		for _, tok := range adv.Keys {
			k := "ctrl+" + strings.ToLower(tok)
			if !handledByFile[file][k] {
				t.Logf("Python spec note: %s:%d advertises [%v] but no keypress in %s handles it (Python's own dead advertisement)",
					file, adv.Line, tok, file)
			}
		}
	}
}
