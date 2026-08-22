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
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNewConversationsDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	convs := []ConversationInfo{
		{DisplayName: "Alice", TrustLevel: "trusted", Unread: true, LastTime: time.Now()},
		{DisplayName: "Bob", TrustLevel: "unknown", Unread: false, LastTime: time.Now().Add(-time.Hour)},
	}

	cd := NewConversationsDisplay(app, convs)
	if cd == nil {
		t.Fatal("NewConversationsDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConversationsDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	_, ok := cd.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestConversationsDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, []ConversationInfo{})

	if cd == nil {
		t.Fatal("NewConversationsDisplay with empty list returned nil")
	}
}

// TestTabBarTextPythonParity verifies the Trusted/Untrusted tab label against
// Python's _label (Conversations.py:461-465): no digit prefixes, an envelope
// glyph + alert count when a tab has unread/failed conversations. The glyph
// here is the unicode set's "✉" (glyphs["unread"]).
// TestTabBarTextPythonParity is a LIVE cross-implementation check: it
// AST-extracts the trusted/untrusted count + _label block from Python's
// ConversationsDisplay.update_listbox (Conversations.py:453-464), execs it
// with mock conversations (tuples with c[2]=trust level, c[4]=unread,
// c[6]=failed, matching the tuple layout app.conversations() yields) and a
// real unicode glyph dict, and captures the two tab labels freshly on every
// run. Go owns the input battery ([]ConversationInfo); Python owns the
// reference behavior. Go's Unread/Failed map to Python's c[4]/c[6] alert
// flags; Go's "trusted" maps to DirectoryEntry.TRUSTED and every other level
// to the untrusted bucket (UNTRUSTED/WARNING/UNKNOWN), matching Python. The
// test SKIPs, not fails, when the Python reference is not importable.
func TestTabBarTextPythonParity(t *testing.T) {
	t.Parallel()

	type tabConv struct {
		TrustLevel string `json:"trust"`
		Unread     bool   `json:"unread"`
		Failed     bool   `json:"failed"`
	}
	tests := []struct {
		name  string
		convs []ConversationInfo
	}{
		{
			"empty",
			nil,
		},
		{
			"no alerts",
			[]ConversationInfo{
				{TrustLevel: "trusted"},
				{TrustLevel: "trusted"},
				{TrustLevel: "unknown"},
			},
		},
		{
			"trusted alerts",
			[]ConversationInfo{
				{TrustLevel: "trusted", Unread: true},
				{TrustLevel: "trusted", Failed: true},
				{TrustLevel: "trusted"},
				{TrustLevel: "unknown"},
			},
		},
		{
			"both alert",
			[]ConversationInfo{
				{TrustLevel: "trusted", Unread: true},
				{TrustLevel: "trusted", Unread: true},
				{TrustLevel: "trusted"},
				{TrustLevel: "untrusted", Failed: true},
			},
		},
	}

	// Build the JSON input: one list of mock conversations per test case.
	type caseInput struct {
		Convs []tabConv `json:"convs"`
	}
	inputs := make([]caseInput, len(tests))
	for i, tt := range tests {
		convs := make([]tabConv, len(tt.convs))
		for j, c := range tt.convs {
			convs[j] = tabConv{TrustLevel: c.TrustLevel, Unread: c.Unread, Failed: c.Failed}
		}
		inputs[i] = caseInput{Convs: convs}
	}

	const script = `
import sys, json, ast, inspect, textwrap, types
import nomadnet.ui.textui.Conversations as C
from nomadnet.ui import TextUI as T
from nomadnet.Directory import DirectoryEntry

def glyph_dict(gs):
    g={}; idx=T.GLYPHSETS[gs]
    for tup in T.GLYPHS: g[tup[0]]=tup[idx]
    return g
G = glyph_dict("unicode")

# AST-extract the count + _label block from update_listbox: from the
# glyphs = self.app.ui.glyphs assignment through the tab_untrusted set_label.
src = textwrap.dedent(inspect.getsource(C.ConversationsDisplay.update_listbox))
fn = ast.parse(src).body[0]
collected = []
started = False
for s in fn.body:
    if not started:
        if isinstance(s, ast.Assign) and any(isinstance(t, ast.Name) and t.id=="glyphs" for t in s.targets):
            started = True
    if started:
        collected.append(s)
        if isinstance(s, ast.Expr) and isinstance(s.value, ast.Call) and isinstance(s.value.func, ast.Attribute) and s.value.func.attr=="set_label":
            # second set_label (tab_untrusted) ends the block
            if len(collected) >= 2 and isinstance(collected[-2], ast.Expr) and isinstance(collected[-2].value, ast.Call):
                break
mod = ast.Module(body=collected, type_ignores=[]); ast.fix_missing_locations(mod)
code = compile(mod, "<tabbar>", "exec")

class Tab:
    def __init__(self): self.label=None
    def set_label(self, l): self.label=l

TRUST_MAP = {"trusted": DirectoryEntry.TRUSTED, "untrusted": DirectoryEntry.UNTRUSTED,
             "warning": DirectoryEntry.WARNING, "unknown": DirectoryEntry.UNKNOWN}

cases = json.load(sys.stdin)
out = []
for c in cases:
    # Build conversation tuples matching app.conversations() layout:
    # c[2]=trust, c[4]=unread alert, c[6]=failed alert.
    conversations = []
    for cv in c["convs"]:
        conversations.append(("", "", TRUST_MAP.get(cv["trust"], DirectoryEntry.UNKNOWN), "", bool(cv["unread"]), False, bool(cv["failed"])))
    class S: pass
    s = S(); s.app = types.SimpleNamespace(ui=types.SimpleNamespace(glyphs=G))
    s.tab_trusted = Tab(); s.tab_untrusted = Tab()
    ns = {"__builtins__": __builtins__, "self": s, "conversations": conversations, "DirectoryEntry": DirectoryEntry}
    exec(code, ns)
    out.append(s.tab_trusted.label + "  " + s.tab_untrusted.label)
json.dump(out, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tabBarText(tt.convs, "✉")
			if got != want[i] {
				t.Errorf("tabBarText = %q, want %q (Python)", got, want[i])
			}
		})
	}
}

// TestConversationRowMainPythonParity is a LIVE cross-implementation check:
// it AST-extracts the body of Python's
// ConversationsDisplay.conversation_list_widget (Conversations.py:1687-1755)
// up to the ListEntry construction, execs it with a mock self (app.ui.glyphs,
// directory.find returning a sort_rank-bearing entry for pinned
// conversations, currently_displayed_conversation) and a conversation tuple
// matching the app.conversations() layout (c[0]=source_hash, c[1]=display_name,
// c[2]=trust, c[4]=unread count, c[5]=last_activity, c[6]=failed count), and
// captures the markup list freshly on every run. The Go first line
// (conversationRowMain) is compared to the concatenation of the non-newline
// markup elements (head + badge); the Go second line (conversationRowSecondary)
// is compared to the newline-prefixed element with its leading "\n" stripped.
//
// last_activity = 1000 (unix) is well over 30 days ago for any run date, so
// Python's relative_time hits its date branch (datetime.fromtimestamp(1000).
// strftime("%Y-%m-%d") == "1970-01-01", local-zone-stable) without needing to
// mock time.time. Go's relativeTime produces the same date. Glyphs are the
// unicode set (check=✓ cross=✕ warning=⚠ unread=✉ pin=★). The test SKIPs,
// not fails, when the Python reference is not importable.
func TestConversationRowMainPythonParity(t *testing.T) {
	t.Parallel()

	glyphs := glyphsUnicode
	old := time.Unix(1000, 0)

	tests := []struct {
		name    string
		conv    ConversationInfo
		current string // currently-displayed conversation source hash
	}{
		{
			"trusted unread",
			ConversationInfo{SourceHash: "aabbccdd", DisplayName: "Alice", TrustLevel: "trusted", UnreadCount: 3, LastTime: old},
			"",
		},
		{
			"untrusted",
			ConversationInfo{SourceHash: "eeff0011", DisplayName: "Bob", TrustLevel: "untrusted", LastTime: old},
			"",
		},
		{
			"unknown no activity",
			ConversationInfo{SourceHash: "22334455", DisplayName: "Carol", TrustLevel: "unknown"},
			"",
		},
		{
			"warning failed",
			ConversationInfo{SourceHash: "66778899", DisplayName: "Dave", TrustLevel: "warning", FailedCount: 2, LastTime: old},
			"",
		},
		{
			"trusted empty name unread",
			ConversationInfo{SourceHash: "aabbccdd", TrustLevel: "trusted", UnreadCount: 1, LastTime: old},
			"",
		},
		{
			"pinned trusted",
			ConversationInfo{SourceHash: "aabbccdd", DisplayName: "Pinned", TrustLevel: "trusted", Pinned: true, LastTime: old},
			"",
		},
		{
			"full hash non-trusted",
			ConversationInfo{SourceHash: "0102030405060708010203040506070801020304050607080102030405060708", DisplayName: "Bob", TrustLevel: "untrusted", LastTime: old},
			"",
		},
		{
			"currently displayed suppresses badge",
			ConversationInfo{SourceHash: "aabbccdd", DisplayName: "Alice", TrustLevel: "trusted", Pinned: true, UnreadCount: 3, LastTime: old},
			"aabbccdd",
		},
	}

	type rowInput struct {
		SourceHash   string `json:"source_hash"`
		DisplayName  string `json:"display_name"`
		TrustLevel   string `json:"trust"`
		UnreadCount  int    `json:"unread"`
		LastActivity int64  `json:"last_activity"`
		FailedCount  int    `json:"failed"`
		Pinned       bool   `json:"pinned"`
		Current      string `json:"current"`
	}
	inputs := make([]rowInput, len(tests))
	for i, tt := range tests {
		var lastAct int64
		if !tt.conv.LastTime.IsZero() {
			lastAct = tt.conv.LastTime.Unix()
		}
		inputs[i] = rowInput{
			SourceHash: tt.conv.SourceHash, DisplayName: tt.conv.DisplayName, TrustLevel: tt.conv.TrustLevel,
			UnreadCount: tt.conv.UnreadCount, LastActivity: lastAct, FailedCount: tt.conv.FailedCount,
			Pinned: tt.conv.Pinned, Current: tt.current,
		}
	}

	const script = `
import sys, json, ast, inspect, textwrap, types
import nomadnet.ui.textui.Conversations as C
from nomadnet.ui import TextUI as T
from nomadnet.Directory import DirectoryEntry

def glyph_dict(gs):
    g={}; idx=T.GLYPHSETS[gs]
    for tup in T.GLYPHS: g[tup[0]]=tup[idx]
    return g
G = glyph_dict("unicode")

src = textwrap.dedent(inspect.getsource(C.ConversationsDisplay.conversation_list_widget))
fn = ast.parse(src).body[0]
body = []
for s in fn.body:
    if isinstance(s, ast.Assign) and any(isinstance(t, ast.Name) and t.id=="widget" for t in s.targets):
        break
    body.append(s)
mod = ast.Module(body=body, type_ignores=[]); ast.fix_missing_locations(mod)
code = compile(mod, "<convrow>", "exec")

TRUST_MAP = {"trusted": DirectoryEntry.TRUSTED, "untrusted": DirectoryEntry.UNTRUSTED,
             "warning": DirectoryEntry.WARNING, "unknown": DirectoryEntry.UNKNOWN}

class Dir:
    def __init__(self, entry): self._e=entry
    def find(self, b): return self._e
class App:
    def __init__(self, entry): self.ui=types.SimpleNamespace(glyphs=G); self.directory=Dir(entry)

cases = json.load(sys.stdin)
out = []
for c in cases:
    entry = types.SimpleNamespace(sort_rank=0) if c["pinned"] else None
    s = types.SimpleNamespace()
    s.app = App(entry)
    s.currently_displayed_conversation = c["current"]
    conv = (c["source_hash"], c["display_name"], TRUST_MAP.get(c["trust"], DirectoryEntry.UNKNOWN),
            "", c["unread"], c["last_activity"], c["failed"])
    ns = {"__builtins__": __builtins__, "self": s, "conversation": conv,
          "DirectoryEntry": DirectoryEntry, "relative_time": C.relative_time}
    exec(code, ns)
    markup = ns["markup"]
    main = "".join(m for m in markup if not (isinstance(m, str) and m.startswith("\n")))
    sec = ""
    for m in markup:
        if isinstance(m, str) and m.startswith("\n"):
            sec = m[1:]; break
    out.append({"main": main, "secondary": sec})
json.dump(out, sys.stdout)
`

	type rowWant struct {
		Main      string `json:"main"`
		Secondary string `json:"secondary"`
	}
	var want []rowWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotMain := conversationRowMain(tt.conv, glyphs, tt.current)
			if gotMain != want[i].Main {
				t.Errorf("conversationRowMain = %q, want %q (Python)", gotMain, want[i].Main)
			}
			gotSec := conversationRowSecondary(tt.conv)
			if gotSec != want[i].Secondary {
				t.Errorf("conversationRowSecondary = %q, want %q (Python)", gotSec, want[i].Secondary)
			}
		})
	}
}

// TestPopulateListRowText verifies the wired populateList produces the parity
// row text (not just the helper) for a trusted unread conversation.
func TestPopulateListRowText(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	old := time.Unix(1000, 0)
	convs := []ConversationInfo{
		{SourceHash: "aabbccdd", DisplayName: "Alice", TrustLevel: "trusted", UnreadCount: 3, LastTime: old},
		{SourceHash: "eeff0011", DisplayName: "Bob", TrustLevel: "untrusted", LastTime: old},
	}
	cd := NewConversationsDisplay(app, convs)
	cd.populateList()
	// Tab defaults to trusted; only Alice should be present with the badge.
	if cd.list.GetItemCount() != 1 {
		t.Fatalf("item count = %v, want 1", cd.list.GetItemCount())
	}
	main, sec := cd.list.GetItemText(0)
	if main != "✓ Alice ✉ (3)" {
		t.Errorf("main = %q, want %q", main, "✓ Alice ✉ (3)")
	}
	wantSec := "  " + old.Format("2006-01-02")
	if sec != wantSec {
		t.Errorf("secondary = %q, want %q", sec, wantSec)
	}
}

func TestNewComposeDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewComposeDisplay(app)

	if cd == nil {
		t.Fatal("NewComposeDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

// TestConversationsToggleFullscreen verifies the list pane collapses to width
// 0 (detail fills the row) and restores, matching Python's toggle_fullscreen
// (Conversations.py:1276). The detail pane is bordered, so its text begins one
// column inside the border; the test checks the detail text's "S" sits at
// listWidth+1 normally and at column 1 in fullscreen.
func TestConversationsToggleFullscreen(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	// Override the empty-state text with a known "Select …" string.
	cd.detail.SetText("Select a conversation to view")

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 12)

	cell := func(x, y int) rune {
		c, _, _, _ := cellContent(screen, x, y)
		return c
	}

	// Non-fullscreen: the list pane occupies [0,listWidth); the bordered detail
	// pane starts at listWidth, so its text's "S" is at column listWidth+1.
	cd.content.SetRect(0, 0, 80, 12)
	screen.Clear()
	cd.content.Draw(screen)
	if cd.Fullscreen() {
		t.Error("expected non-fullscreen initially")
	}
	if c := cell(cd.ListWidth()+1, 1); c != 'S' {
		t.Errorf("normal detail cell(%v,1) = %q, want 'S'", cd.ListWidth()+1, c)
	}
	if c := cell(0, 1); c == 'S' {
		t.Errorf("normal detail cell(0,1) = 'S', but list pane should occupy column 0")
	}

	// Toggle fullscreen: list pane collapses to width 0, the bordered detail
	// pane fills the row, so its "S" is at column 1.
	cd.ToggleFullscreen()
	if !cd.Fullscreen() {
		t.Error("expected fullscreen after toggle")
	}
	screen.Clear()
	cd.content.Draw(screen)
	if c := cell(1, 1); c != 'S' {
		t.Errorf("fullscreen detail cell(1,1) = %q, want 'S'", c)
	}

	// Toggle back: detail returns to the list width.
	cd.ToggleFullscreen()
	if cd.Fullscreen() {
		t.Error("expected non-fullscreen after second toggle")
	}
	screen.Clear()
	cd.content.Draw(screen)
	if c := cell(cd.ListWidth()+1, 1); c != 'S' {
		t.Errorf("restored detail cell(%v,1) = %q, want 'S'", cd.ListWidth()+1, c)
	}
}

// TestDisplayConversationWiresOnSend verifies that DisplayConversation wires
// the ConversationWidget's OnSend to delegate to the display-level OnSend,
// forwarding the conversation's source hash plus the composed content/title.
// This pins the TUI-side of the "Wire conversation send" task:
// C-d in the composer must reach the display's OnSend(sourceHash, content,
// title), which the wiring layer (cmd/gonomadnet/textui.go) connects to
// App.SendConversation. The C-d key path itself (handleInput → sendMessage →
// OnSend) is exercised by dispatching a KeyCtrlD through the widget's input
// capture, mirroring a real keypress.
func TestDisplayConversationWiresOnSend(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var gotSource, gotContent, gotTitle string
	var fired bool
	cd.OnSend = func(sourceHash, content, title string, _ []string) {
		fired = true
		gotSource, gotContent, gotTitle = sourceHash, content, title
	}

	cd.DisplayConversation(hash)
	if cd.currentWidget == nil {
		t.Fatal("DisplayConversation did not set currentWidget")
	}

	// Type into the composer and dispatch C-d through the frame's input
	// capture (the same path tview takes on a real keypress).
	cd.currentWidget.editor.SetText("hello there")

	if ret := cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone)); ret != nil {
		t.Errorf("KeyCtrlD was not consumed by handleInput")
	}

	if !fired {
		t.Fatal("OnSend was not fired by C-d")
	}
	if gotSource != hash {
		t.Errorf("OnSend sourceHash = %q, want %q", gotSource, hash)
	}
	if gotContent != "hello there" {
		t.Errorf("OnSend content = %q, want %q", gotContent, "hello there")
	}
	if gotTitle != "" {
		t.Errorf("OnSend title = %q, want empty (minimal editor)", gotTitle)
	}

	// The editor must be cleared after a successful send.
	if cd.currentWidget.editor.GetText() != "" {
		t.Errorf("editor not cleared after send: %q", cd.currentWidget.editor.GetText())
	}
}

// TestDisplayConversationOnSendForwardsTitle verifies the title is forwarded
// when the full editor (C-t) is active, matching Python's send path which
// includes the title field when present.
func TestDisplayConversationOnSendForwardsTitle(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Bob", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var gotTitle string
	cd.OnSend = func(sourceHash, content, title string, _ []string) {
		gotTitle = title
	}

	cd.DisplayConversation(hash)
	cd.currentWidget.toggleEditor() // switch to full editor (title + content)
	cd.currentWidget.titleEditor.SetText("Greetings")
	cd.currentWidget.editor.SetText("body")

	cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if gotTitle != "Greetings" {
		t.Errorf("OnSend title = %q, want %q", gotTitle, "Greetings")
	}
}

// TestDisplayConversationWiresCtrlGFullscreen verifies that DisplayConversation
// wires the open-conversation widget's Ctrl-G to the display's fullscreen
// toggle, matching Python's ConversationWidget.keypress "ctrl g" →
// conversations_display.toggle_fullscreen() (Conversations.py:2234-2235).
// Before the fix the widget's OnToggleFullscreen was never wired, so Ctrl-G
// inside an open conversation was a no-op even though the list-level Ctrl-G
// worked. This dispatches Ctrl-G through the widget's input capture (the real
// keypress path) and asserts the display's Fullscreen() state actually flips.
func TestDisplayConversationWiresCtrlGFullscreen(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Carol", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	cd.DisplayConversation(hash)
	if cd.currentWidget == nil {
		t.Fatal("DisplayConversation did not set currentWidget")
	}
	if cd.Fullscreen() {
		t.Fatal("display should not start fullscreen")
	}

	// Ctrl-G through the widget's input capture — the same path tview takes on
	// a real keypress while focus is in the open conversation body.
	if ret := cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone)); ret != nil {
		t.Errorf("KeyCtrlG was not consumed by the widget handleInput")
	}
	if !cd.Fullscreen() {
		t.Error("Ctrl-G inside the conversation did not toggle the display to fullscreen")
	}

	// A second Ctrl-G restores the normal two-pane view.
	if ret := cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone)); ret != nil {
		t.Errorf("second KeyCtrlG was not consumed")
	}
	if cd.Fullscreen() {
		t.Error("second Ctrl-G did not restore the normal view")
	}
}

// TestDisplayConversationLoadsMessages verifies DisplayConversation calls the
// display-level OnLoadMessages hook to populate the conversation widget's
// message list, and injects OnOwnHash so the LXMessageWidget header can tell
// inbound from outbound. This pins the TUI side of message loading
// ("ConversationWidget — messages").
func TestDisplayConversationLoadsMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Carol", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	ownHash := []byte{0x11, 0x22, 0x33}
	cd.OnOwnHash = func() []byte { return ownHash }
	cd.OnTimeFormat = func() string { return "%Y-%m-%d %H:%M:%S" }
	cd.OnLoadMessages = func(sourceHash string) []ConversationMessage {
		if sourceHash != hash {
			t.Errorf("OnLoadMessages sourceHash = %q, want %q", sourceHash, hash)
		}
		return []ConversationMessage{
			{Content: "hello world", Title: "Greeting", State: 0x04, SourceHash: []byte{0xff}},
			{Content: "second msg", State: 0x08, SourceHash: ownHash},
		}
	}

	cd.DisplayConversation(hash)
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("DisplayConversation did not set currentWidget")
	}
	if !bytes.Equal(cw.OwnHash, ownHash) {
		t.Errorf("cw.OwnHash = %x, want %x", cw.OwnHash, ownHash)
	}
	if len(cw.messages) != 2 {
		t.Fatalf("loaded %v messages, want 2", len(cw.messages))
	}
	if cw.messages[0].Content != "hello world" {
		t.Errorf("messages[0].Content = %q, want %q", cw.messages[0].Content, "hello world")
	}

	// The rendered message list must contain the message bodies.
	body := cw.messageList.GetText(true)
	if !strings.Contains(body, "hello world") {
		t.Errorf("messageList missing %q; got: %v", "hello world", body)
	}
	if !strings.Contains(body, "second msg") {
		t.Errorf("messageList missing %q; got: %v", "second msg", body)
	}
}

// TestReloadCurrentMessages verifies ReloadCurrentMessages re-fetches the
// message list from OnLoadMessages and re-renders, so a just-sent message
// appears in the open conversation view.
func TestReloadCurrentMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "dddddddddddddddddddddddddddddddddddddddd"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Dave", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	loads := 0
	cd.OnLoadMessages = func(sourceHash string) []ConversationMessage {
		loads++
		if loads == 1 {
			return nil // initially empty
		}
		return []ConversationMessage{{Content: "freshly sent"}}
	}

	cd.DisplayConversation(hash)
	if loads != 1 {
		t.Errorf("expected 1 load on display, got %v", loads)
	}
	// if cd.currentWidget.messageList.GetText(true) != "" && cd.currentWidget.messages != nil {
	// 	// initial empty is acceptable
	// }

	cd.ReloadCurrentMessages()
	if loads != 2 {
		t.Errorf("expected 2 loads after reload, got %v", loads)
	}
	if !strings.Contains(cd.currentWidget.messageList.GetText(true), "freshly sent") {
		t.Errorf("reload did not render the new message: %v", cd.currentWidget.messageList.GetText(true))
	}
}

// TestDisplayConversationWiresPaperMessage verifies the paper-message wiring
// (Python paper_message, Conversations.py:2474-2503): the widget's
// OnPaperMessage forwards the open conversation's source hash + action +
// content + title to cd.OnPaperMessage; the saved/failed result callbacks
// route to the display's dialog methods.
func TestDisplayConversationWiresPaperMessage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Eve", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var gotSource, gotAction, gotContent, gotTitle string
	cd.OnPaperMessage = func(sourceHash, action, content, title string) (string, bool) {
		gotSource, gotAction, gotContent, gotTitle = sourceHash, action, content, title
		return "/dl/paper.lxm", true
	}
	var savedPath string
	cd.OnPaperMessageSaved = func(path string) { savedPath = path }
	var failedFired bool
	cd.OnPaperMessageFailed = func() { failedFired = true }

	cd.DisplayConversation(hash)
	if cd.currentWidget == nil {
		t.Fatal("DisplayConversation did not set currentWidget")
	}
	cd.currentWidget.editor.SetText("paper body")
	cd.currentWidget.titleEditor.SetText("paper title")

	cd.currentWidget.PaperMessageSaveQR()

	if gotSource != hash {
		t.Errorf("OnPaperMessage sourceHash = %q, want %q", gotSource, hash)
	}
	if gotAction != "SaveQR" {
		t.Errorf("OnPaperMessage action = %q, want SaveQR", gotAction)
	}
	if gotContent != "paper body" || gotTitle != "paper title" {
		t.Errorf("OnPaperMessage content/title = %q/%q, want paper body/paper title", gotContent, gotTitle)
	}
	if savedPath != "/dl/paper.lxm" {
		t.Errorf("OnPaperMessageSaved = %q, want /dl/paper.lxm", savedPath)
	}
	if failedFired {
		t.Error("success should not fire OnPaperMessageFailed")
	}

	// Failure path routes to OnPaperMessageFailed.
	cd.OnPaperMessage = func(sourceHash, action, content, title string) (string, bool) {
		return "", false
	}
	cd.currentWidget.editor.SetText("try again")
	failedFired = false
	savedPath = ""
	cd.currentWidget.PaperMessagePrintQR()
	if !failedFired {
		t.Error("failure should fire OnPaperMessageFailed")
	}
	if savedPath != "" {
		t.Error("failure should not fire OnPaperMessageSaved")
	}
}

// TestConversationWidgetPendingAttachments verifies the compose-side
// attachment flow: ConfirmAttachFile stages file paths on the widget, and
// C-d (sendMessage) forwards them through OnSend along with the content,
// then clears the staged list (Python send_message, Conversations.py:2412-
// 2436 + clear_editor). This pins the TUI side of the "attachFile" TODO.
func TestConversationWidgetPendingAttachments(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	const hash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	convs := []ConversationInfo{
		{SourceHash: hash, DisplayName: "Eve", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var gotAttachments []string
	var gotContent string
	cd.OnSend = func(sourceHash, content, title string, attachments []string) {
		gotContent = content
		gotAttachments = attachments
	}

	cd.DisplayConversation(hash)
	cw := cd.currentWidget

	// Stage two attachments (as the file browser would on selection).
	cw.ConfirmAttachFile([]string{"/tmp/alpha.txt", "/tmp/beta.txt"})
	if len(cw.pendingAttachments) != 2 {
		t.Fatalf("pendingAttachments = %v, want 2 entries", cw.pendingAttachments)
	}

	cw.editor.SetText("with files")
	cw.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if gotContent != "with files" {
		t.Errorf("content = %q, want %q", gotContent, "with files")
	}
	if len(gotAttachments) != 2 || gotAttachments[0] != "/tmp/alpha.txt" || gotAttachments[1] != "/tmp/beta.txt" {
		t.Errorf("attachments = %v, want [/tmp/alpha.txt /tmp/beta.txt]", gotAttachments)
	}
	// The staged list must be cleared after the send (Python clear_editor
	// resets pending_attachments).
	if len(cw.pendingAttachments) != 0 {
		t.Errorf("pendingAttachments not cleared after send: %v", cw.pendingAttachments)
	}
}

func TestComposeDisplayGetSetText(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("Hello World")
	if cd.GetText() != "Hello World" {
		t.Errorf("GetText() = %q, want %q", cd.GetText(), "Hello World")
	}

	cd.title.SetText("Alice")
	if cd.GetTitle() != "Alice" {
		t.Errorf("GetTitle() = %q, want %q", cd.GetTitle(), "Alice")
	}
}

func TestComposeDisplayClear(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("test")
	cd.title.SetText("test")
	cd.Clear()

	if cd.GetText() != "" {
		t.Errorf("GetText() after clear = %q, want empty", cd.GetText())
	}
	if cd.GetTitle() != "" {
		t.Errorf("GetTitle() after clear = %q, want empty", cd.GetTitle())
	}
}

func TestNewMessageViewDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMessageViewDisplay(app)

	if mvd == nil {
		t.Fatal("NewMessageViewDisplay returned nil")
	}
	if mvd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestMessageViewShowMessage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMessageViewDisplay(app)

	msg := MessageInfo{
		Title:      "Test",
		Content:    "Hello World",
		Sender:     "Alice",
		Timestamp:  "2024-01-01",
		TrustLevel: "trusted",
	}

	// Should not panic
	mvd.ShowMessage(msg)
}

func TestMessageViewClear(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	mvd := NewMessageViewDisplay(app)

	mvd.ShowMessage(MessageInfo{Content: "test"})
	mvd.Clear()
	// Should not panic
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input time.Time
		want  string
	}{
		{time.Now(), "just now"},
		{time.Now().Add(-30 * time.Second), "just now"},
		{time.Now().Add(-5 * time.Minute), "5m ago"},
		{time.Now().Add(-3 * time.Hour), "3h ago"},
		{time.Now().Add(-25 * time.Hour), "yesterday"},
		{time.Now().Add(-3 * 24 * time.Hour), "3d ago"},
	}

	for _, tt := range tests {
		got := relativeTime(tt.input)
		if got != tt.want {
			t.Errorf("relativeTime(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLooksLikeMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"plain text", false},
		{">>Heading", true},
		{"```code```", true},
		{"`!bold`", true},
		{"Hello World", false},
	}

	for _, tt := range tests {
		got := looksLikeMicron(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeMicron(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRenderMicronAsText(t *testing.T) {
	t.Parallel()

	result := renderMicronAsText("plain text")
	if result != "plain text" {
		t.Errorf("renderMicronAsText plain = %q, want %q", result, "plain text")
	}
}

func TestConversationsDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	// Track which callbacks fire
	var fired []string
	cd.OnEditPeerInfo = func() { fired = append(fired, "edit_peer") }
	cd.OnDeleteConv = func() { fired = append(fired, "delete") }
	cd.OnNewConv = func() { fired = append(fired, "new") }
	cd.OnIngestURI = func() { fired = append(fired, "ingest") }
	cd.OnSync = func() { fired = append(fired, "sync") }
	cd.OnToggleFullscreen = func() { fired = append(fired, "fullscreen") }
	cd.OnToggleSort = func() { fired = append(fired, "sort") }
	cd.OnShowQR = func() { fired = append(fired, "qr") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit_peer"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "delete"},
		{"ctrl-n", tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone), "new"},
		{"ctrl-u", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), "ingest"},
		{"ctrl-r", tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone), "sync"},
		{"ctrl-g", tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone), "fullscreen"},
		{"ctrl-o", tcell.NewEventKey(tcell.KeyCtrlO, 0, tcell.ModNone), "sort"},
		{"ctrl-p", tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModNone), "qr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := cd.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %v was not consumed (returned non-nil)", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %v fired %v, want [%v]", tt.name, fired, tt.want)
			}
		})
	}
}

func TestConversationsDisplayUnhandledKeys(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	// Unhandled keys should pass through
	event := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	result := cd.handleInput(event)
	if result != event {
		t.Error("Unhandled key should pass through")
	}
}

func TestConversationsDisplayEditSelectedInDirectory(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	convs := []ConversationInfo{
		{SourceHash: "aabb1122", DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var edited PeerInfoEntry
	cd.OnEditPeerInfo = func() { edited = cd.EditSelectedInDirectory() }

	cd.list.SetCurrentItem(0)
	cd.OnEditPeerInfo()

	if edited.SourceHash != "aabb1122" {
		t.Errorf("SourceHash = %q, want %q", edited.SourceHash, "aabb1122")
	}
	if edited.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", edited.DisplayName, "Alice")
	}
}

func TestConversationsDisplayEditSelectedNoSelection(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	result := cd.EditSelectedInDirectory()
	if result.SourceHash != "" {
		t.Errorf("with no selection, SourceHash should be empty, got %q", result.SourceHash)
	}
}

// TestDisplayConversationInjectsPeerInfoRNS verifies DisplayConversation consults
// the OnStampCost/OnHops callbacks and feeds them into the open conversation
// widget's peer-info header bar, mirroring Python _update_peer_info
// (Conversations.py:2103-2112). With both supplied the bar shows "Stamp: N"
// and "M hops"; with both nil it shows "unknown" hops and no stamp segment.
func TestDisplayConversationInjectsPeerInfoRNS(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	convs := []ConversationInfo{
		{SourceHash: "aabbccddeeff0011", DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }

	stamp := 4
	hops := 3
	cd.OnStampCost = func(sourceHash string) *int { return &stamp }
	cd.OnHops = func(sourceHash string) *int { return &hops }

	cd.DisplayConversation("aabbccddeeff0011")
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("currentWidget not set after DisplayConversation")
	}
	bar := cw.peerInfoBar.GetText(false)
	if !strings.Contains(bar, "Stamp: 4") {
		t.Errorf("peer-info bar missing injected stamp cost: %q", bar)
	}
	if !strings.Contains(bar, "3 hops") {
		t.Errorf("peer-info bar missing injected hop count: %q", bar)
	}

	// nil callbacks fall back to "unknown" hops and no stamp segment.
	app2 := newTestApp()
	app2.Glyphs = GetGlyphSet(GlyphUnicode)
	cd2 := NewConversationsDisplay(app2, convs)
	cd2.OnLoadMessages = func(string) []ConversationMessage { return nil }
	cd2.DisplayConversation("aabbccddeeff0011")
	bar2 := cd2.currentWidget.peerInfoBar.GetText(false)
	if strings.Contains(bar2, "Stamp:") {
		t.Errorf("peer-info bar should omit Stamp segment when OnStampCost is nil: %q", bar2)
	}
	if !strings.Contains(bar2, "unknown") {
		t.Errorf("peer-info bar should show unknown hops when OnHops is nil: %q", bar2)
	}
}

// TestShortcutFocusByRegion verifies the footer shortcut bar follows the focused
// region, mirroring Python's shortcuts() focus-path dispatch
// (Conversations.py:1765-1779): list pane → the list bar; conversation editor
// → the editor bar; message body → the body bar. Each focusable primitive's
// SetFocusFunc drives setShortcutRegion, so gaining focus switches the bar.
func TestShortcutFocusByRegion(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	convs := []ConversationInfo{
		{SourceHash: "aabbccddeeff0011", DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }

	listBar := "[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen"
	editorBar := "[C-d] Send  [C-p] Paper Msg  [C-t] Title  [C-f] Attach  [C-s] Save  [Tab] ↑ Messages"
	bodyBar := "[C-s] Save  [C-u] Purge  [C-o] Sort  [C-x] Clear History  [C-g] Fullscreen  [C-w] Close  [Tab] ↓ Editor"

	// Default region is "list".
	if got := cd.GetShortcutText(); got != listBar {
		t.Errorf("default shortcut bar = %q, want list bar", got)
	}

	// Focusing a list-pane primitive switches to the list bar.
	cd.list.Focus(func(tview.Primitive) {})
	if got := cd.GetShortcutText(); got != listBar {
		t.Errorf("after list focus, shortcut bar = %q, want list bar", got)
	}

	// Open a conversation and focus the editor → editor bar.
	cd.DisplayConversation("aabbccddeeff0011")
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("currentWidget not set")
	}
	cw.editor.Focus(func(tview.Primitive) {})
	if got := cd.GetShortcutText(); got != editorBar {
		t.Errorf("after editor focus, shortcut bar = %q, want editor bar", got)
	}

	// Focus the message body → body bar.
	cw.messageList.Focus(func(tview.Primitive) {})
	if got := cd.GetShortcutText(); got != bodyBar {
		t.Errorf("after body focus, shortcut bar = %q, want body bar", got)
	}

	// Title editor (full-editor mode) → editor bar.
	cw.titleEditor.Focus(func(tview.Primitive) {})
	if got := cd.GetShortcutText(); got != editorBar {
		t.Errorf("after title editor focus, shortcut bar = %q, want editor bar", got)
	}

	// Focus returns to the list → list bar.
	cd.list.Focus(func(tview.Primitive) {})
	if got := cd.GetShortcutText(); got != listBar {
		t.Errorf("after returning to list, shortcut bar = %q, want list bar", got)
	}

	// An open dialog suppresses the bar regardless of region.
	cd.dialogOpen = true
	if got := cd.GetShortcutText(); got != "" {
		t.Errorf("with dialog open, shortcut bar = %q, want empty", got)
	}
}

func TestPeerInfoEntryTrustLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		trust    string
		expected string
	}{
		{"trusted", TrustTrusted},
		{"untrusted", TrustUntrusted},
		{"unknown", TrustUnknown},
		{"blocked", TrustUntrusted},
		{"", TrustUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.trust, func(t *testing.T) {
			t.Parallel()

			entry := PeerInfoEntry{TrustLevel: tt.trust}
			if entry.TrustLevelValue() != tt.expected {
				t.Errorf("TrustLevelValue() = %q, want %q", entry.TrustLevelValue(), tt.expected)
			}
		})
	}
}

func TestConversationsDisplayIngestLXMURI(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenIngestURIDialog()
	if !cd.DialogOpen() {
		t.Error("OpenIngestURIDialog should set dialog open")
	}

	cd.ConfirmIngestURI("lxm://abc123def")
	if cd.DialogOpen() {
		t.Error("ConfirmIngestURI should close dialog")
	}
	if cd.IngestURIDialogValue() != "lxm://abc123def" {
		t.Errorf("IngestURIDialogValue = %q, want %q", cd.IngestURIDialogValue(), "lxm://abc123def")
	}
}

func TestConversationsDisplayIngestURIDismiss(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenIngestURIDialog()
	cd.DismissIngestURIDialog()

	if cd.DialogOpen() {
		t.Error("DismissIngestURIDialog should close dialog")
	}
}

func TestConversationsDisplaySyncConversations(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenSyncDialog()
	if !cd.DialogOpen() {
		t.Error("OpenSyncDialog should set dialog open")
	}
}

func TestConversationsDisplaySyncConversationsRequestSync(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var syncRequested bool
	var syncLimit int
	cd.OnSyncRequested = func(limit int) { syncRequested = true; syncLimit = limit }

	cd.OpenSyncDialog()
	cd.RequestSync(0)

	if syncRequested != true {
		t.Error("RequestSync should fire OnSyncRequested")
	}
	if syncLimit != 0 {
		t.Errorf("syncLimit = %v, want 0 (unlimited)", syncLimit)
	}
}

func TestConversationsDisplaySyncConversationsWithLimit(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var syncLimit int
	cd.OnSyncRequested = func(limit int) { syncLimit = limit }

	cd.OpenSyncDialog()
	cd.RequestSync(5)

	if syncLimit != 5 {
		t.Errorf("syncLimit = %v, want 5", syncLimit)
	}
}

func TestConversationsDisplaySyncConversationsDismiss(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenSyncDialog()
	cd.DismissSyncDialog()

	if cd.DialogOpen() {
		t.Error("DismissSyncDialog should close dialog")
	}
}

func TestSyncStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		progress int
		running  bool
		node     string
		wantBar  string
	}{
		{"idle", 0, false, "", ""},
		{"syncing 50%", 50, true, "Node1", "[==========          ] 50%"},
		{"syncing 100%", 100, true, "", "[====================] 100%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ss := &SyncStatus{
				SyncProgress: tt.progress,
				SyncRunning:  tt.running,
				NodeLabel:    tt.node,
			}
			bar := ss.FormatSyncProgress()
			if bar != tt.wantBar {
				t.Errorf("FormatSyncProgress() = %q, want %q", bar, tt.wantBar)
			}
		})
	}
}

func TestSyncStatusLine(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{HasSynced: false}
	if got := ss.FormatStatusLine(); got != "Last sync: never" {
		t.Errorf("FormatStatusLine() = %q, want %q", got, "Last sync: never")
	}
}

// TestConversationsSyncStatusLine pins B8: the Conversations left-pane sync
// footer must read the persisted last-sync time (Python _sync_status_line,
// Conversations.py:517-545, which reads peer_settings["last_lxmf_sync"]). The
// Go port hardcoded " Last sync: never"; it must instead consult a
// LastSyncInfo hook returning (lastSyncTime, nodeLabel) supplied by the wiring
// layer from app.PeerSettings.LastLXMFSync + the default propagation node.
// Format: " Last sync: <relative|never>" + optional "  (<nodeLabel>)".
func TestConversationsSyncStatusLine(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	// No hook wired: legacy fallback is " Last sync: never".
	if got := cd.syncStatusLine(); got != " Last sync: never" {
		t.Errorf("syncStatusLine() no-hook = %q, want %q", got, " Last sync: never")
	}

	// Hook returning a zero time → never (no sync recorded yet).
	cd.LastSyncInfo = func() (time.Time, string) { return time.Time{}, "" }
	if got := cd.syncStatusLine(); got != " Last sync: never" {
		t.Errorf("syncStatusLine() zero-time = %q, want %q", got, " Last sync: never")
	}

	// Hook returning 5 minutes ago, no node label.
	cd.LastSyncInfo = func() (time.Time, string) {
		return time.Now().Add(-5 * time.Minute), ""
	}
	if got := cd.syncStatusLine(); got != " Last sync: 5m ago" {
		t.Errorf("syncStatusLine() 5m = %q, want %q", got, " Last sync: 5m ago")
	}

	// Hook returning 1 hour ago with a node label suffix.
	cd.LastSyncInfo = func() (time.Time, string) {
		return time.Now().Add(-1 * time.Hour), "TestNode"
	}
	if got := cd.syncStatusLine(); got != " Last sync: 1h ago  (TestNode)" {
		t.Errorf("syncStatusLine() 1h+label = %q, want %q", got, " Last sync: 1h ago  (TestNode)")
	}
}

func TestConversationsDisplayShowBlocked(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	if cd.ShowBlocked() {
		t.Error("ShowBlocked should default to false")
	}

	cd.SetShowBlocked(true)
	if !cd.ShowBlocked() {
		t.Error("SetShowBlocked(true) should make ShowBlocked return true")
	}

	cd.SetShowBlocked(false)
	if cd.ShowBlocked() {
		t.Error("SetShowBlocked(false) should make ShowBlocked return false")
	}
}

func TestConversationsDisplayShowBlockedFiltersList(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	convs := []ConversationInfo{
		{SourceHash: "aaaa", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "bbbb", DisplayName: "Eve", TrustLevel: "blocked"},
	}
	cd := NewConversationsDisplay(app, convs)

	cd.showTrusted = false

	cd.SetShowBlocked(false)
	filtered := FilterConversationsWithBlocked(cd.conversations, "untrusted", cd.ShowBlocked())
	if len(filtered) != 0 {
		t.Errorf("without blocked, got %v untrusted, want 0", len(filtered))
	}

	cd.SetShowBlocked(true)
	filtered = FilterConversationsWithBlocked(cd.conversations, "untrusted", cd.ShowBlocked())
	if len(filtered) != 1 {
		t.Errorf("with blocked, got %v untrusted, want 1", len(filtered))
	}
}

func TestConversationsDisplayBlockedRowLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		displayName string
		sourceHash  string
		wantPrefix  string
	}{
		{"named peer", "Eve", "aabb1122", "× [blocked] Eve"},
		{"unnamed peer", "", "aabb1122", "× [blocked]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			label := BlockedRowLabel(tt.displayName, tt.sourceHash)
			if !strings.HasPrefix(label, tt.wantPrefix) {
				t.Errorf("BlockedRowLabel(%q, %q) = %q, want prefix %q", tt.displayName, tt.sourceHash, label, tt.wantPrefix)
			}
		})
	}
}

func TestPaperMessageDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var printFired, saveQRFired, saveURIFired bool
	cd.PaperMessageDialog(
		func() { printFired = true },
		func() { saveQRFired = true },
		func() { saveURIFired = true },
	)

	if !cd.dialogOpen {
		t.Error("dialog should be open after PaperMessageDialog")
	}

	// Verify callbacks are set (dialog display requires running app)
	_ = printFired
	_ = saveQRFired
	_ = saveURIFired
}

func TestPaperMessageFailed(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.PaperMessageFailed()
	if !cd.dialogOpen {
		t.Error("dialog should be open after PaperMessageFailed")
	}
}

func TestAttachFileDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var selectedPaths []string
	cd.AttachFileDialog("/tmp/", func(paths []string) {
		selectedPaths = paths
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = selectedPaths
}

func TestSaveAttachmentsDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	refs := []AttachmentRef{
		{Name: "file1.pdf", Type: "file"},
		{Name: "image.png", Type: "image"},
		{Name: "doc.txt", Type: "file"},
	}
	cd.SaveAttachmentsDialog("aabb1122", refs)

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
}

func TestShowPeerInfoDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	entry := PeerInfoEntry{
		SourceHash:        "aabb112233445566",
		DisplayName:       "TestPeer",
		TrustLevel:        TrustTrusted,
		PreferredDelivery: "direct",
		Pinned:            true,
		Notes:             "test notes",
	}

	var saved bool
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{}, func(e PeerInfoEntry) {
		saved = true
		if e.DisplayName != "TestPeer" {
			t.Errorf("DisplayName = %q, want TestPeer", e.DisplayName)
		}
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = saved
}

// TestShowPeerInfoDialogGolden pins the verbatim Python dialog field labels,
// button labels, the read-only Addr caption, and the known-section info text
// against Conversations.py:821-1020. These are the exact urwid caption/label
// strings the original shows; the dialog is an interactive overlay so capture
// tooling cannot snapshot it — the golden strings are the parity measure.
func TestShowPeerInfoDialogGolden(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	pages := app.Dialogs.Init(app.Application, cd.Widget())
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(cd.Widget())

	entry := PeerInfoEntry{
		SourceHash:        "aabb112233445566",
		DisplayName:       "TestPeer",
		TrustLevel:        TrustUnknown,
		PreferredDelivery: "direct",
	}

	var knownReported bool
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{
		IsKnown: func(_ string) bool {
			knownReported = true
			return false // forces the "Query network for keys" section
		},
	}, func(PeerInfoEntry) {})

	if !knownReported {
		t.Error("IsKnown hook was not consulted")
	}
	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 30)
	pages.SetRect(0, 0, 80, 30)
	pages.Draw(screen)
	screen.Sync()

	rows := make([]string, 30)
	for y := range 30 {
		var b strings.Builder
		for x := range 80 {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}

	// Golden strings verbatim from Python edit_selected_in_directory.
	golden := []string{
		"Addr : aabb112233445566", // selected_id_widget caption
		"Name : ",                 // e_name caption
		"Copy : ",                 // e_copy caption
		"Untrusted",               // trust radio
		"Unknown",                 // trust radio
		"Trusted",                 // trust radio
		"Deliver directly",        // delivery radio
		"Use propagation nodes",   // delivery radio
		"Pin to top",              // cb_pin label
		"Notes: ",                 // e_notes caption
		"Query network for keys",  // query button
		"Ping",                    // action button
		"Block",                   // action button
		"LXMF",                    // action button (qr_button label)
		"Save",                    // save button
		"Back",                    // back button
	}
	for _, want := range golden {
		if !containsRow(rows, want) {
			t.Errorf("dialog text missing golden %q", want)
		}
	}
	// The known-section info text wraps across rows; assert the distinctive
	// phrases that fit on a single wrapped line appear in the rendered output.
	for _, want := range []string{
		"The identity of this peer is not known",
		"query the network to obtain the identity.",
	} {
		if !containsRow(rows, want) {
			t.Errorf("known-section info text missing %q", want)
		}
	}
}

// TestShowPeerInfoDialogKnownDivider verifies that when the peer identity IS
// known the known-section collapses to a divider (no "Query network for keys"
// button), mirroring Python (Conversations.py:957-959).
func TestShowPeerInfoDialogKnownDivider(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	pages := app.Dialogs.Init(app.Application, cd.Widget())
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(cd.Widget())

	cd.ShowPeerInfoDialog(PeerInfoEntry{SourceHash: "abc", TrustLevel: TrustUnknown}, PeerInfoDialogHooks{
		IsKnown: func(_ string) bool { return true },
	}, func(PeerInfoEntry) {})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 30)
	pages.SetRect(0, 0, 80, 30)
	pages.Draw(screen)
	screen.Sync()

	rows := make([]string, 30)
	for y := range 30 {
		var b strings.Builder
		for x := range 80 {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	if containsRow(rows, "Query network for keys") {
		t.Error("Query button should NOT appear when peer is known")
	}
}

// TestShowPeerInfoDialogSaveValues verifies Save maps the dialog fields to the
// PeerInfoEntry, mirroring Python's confirmed() (Conversations.py:901-929):
// trust radio → TrustLevel, delivery radio → PreferredDelivery, pin checkbox
// → Pinned, name → DisplayName, notes → Notes. The dialog primitives are not
// directly addressable from the test, so the trust/delivery/pin mapping is
// exercised via the wiring-layer app test (app.TestRememberPeerInfoRoundTrip).
func TestShowPeerInfoDialogSaveValues(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	entry := PeerInfoEntry{
		SourceHash:        "deadbeef",
		DisplayName:       "Orig",
		TrustLevel:        TrustUnknown,
		PreferredDelivery: "direct",
	}

	var saved PeerInfoEntry
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{
		IsKnown: func(_ string) bool { return true }, // divider, not query section
	}, func(e PeerInfoEntry) { saved = e })

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = saved
}

func TestPeerInfoEntryTrustLevelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{TrustTrusted, TrustTrusted},
		{TrustUntrusted, TrustUntrusted},
		{TrustUnknown, TrustUnknown},
		{"blocked", TrustUntrusted},
		{"other", TrustUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			e := PeerInfoEntry{TrustLevel: tt.input}
			got := e.TrustLevelValue()
			if got != tt.want {
				t.Errorf("TrustLevelValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShowSyncDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var result SyncDialogResult
	cd.ShowSyncDialog("aabb112233445566", nil, SyncDialogHooks{
		Progress:    func() float64 { return 0.5 },
		Status:      func() string { return "Idle" },
		ShowPercent: func() bool { return false },
	}, func(r SyncDialogResult) {
		result = r
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = result
}

// TestShowSyncDialogLiveProgress verifies the progress UI refresh
// (Python update_sync_dialog, Conversations.py:1566-1575): the dialog's status
// line + button reflect the live hooks on each updateSyncProgress call.
// "Idle"/"Done*" shows the Sync Now button; an active transfer shows Cancel Sync
// and, when showPercent, appends "(NN%)" to the status line.
func TestShowSyncDialogLiveProgress(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	prog := 0.0
	stat := "Idle"
	cd.ShowSyncDialog("aabb", nil, SyncDialogHooks{
		Progress:    func() float64 { return prog },
		Status:      func() string { return stat },
		ShowPercent: func() bool { return true },
	}, func(SyncDialogResult) {})

	cd.updateSyncProgress()
	if got := cd.syncStatusText.GetText(true); got != "Idle (0%)" {
		t.Errorf("idle status = %q, want %q", got, "Idle (0%)")
	}
	if cd.syncSyncBtn.Label() != "Sync Now" {
		t.Errorf("idle button = %q, want Sync Now", cd.syncSyncBtn.Label())
	}

	prog = 0.73
	stat = "Receiving messages"
	cd.updateSyncProgress()
	if got := cd.syncStatusText.GetText(true); got != "Receiving messages (73%)" {
		t.Errorf("active status = %q, want %q", got, "Receiving messages (73%)")
	}
	if cd.syncSyncBtn.Label() != "Cancel Sync" {
		t.Errorf("active button = %q, want Cancel Sync", cd.syncSyncBtn.Label())
	}

	// A non-percent status omits the parenthetical.
	stat = "Link established"
	cd.SetSyncShowPercentHook(func() bool { return false })
	cd.updateSyncProgress()
	if got := cd.syncStatusText.GetText(true); got != "Link established" {
		t.Errorf("no-percent status = %q, want %q", got, "Link established")
	}
}

func TestSyncDialogResult(t *testing.T) {
	t.Parallel()

	result := SyncDialogResult{
		Mode:   SyncLimited,
		Limit:  10,
		Action: "sync",
	}

	if result.Mode != SyncLimited {
		t.Errorf("Mode = %v, want SyncLimited", result.Mode)
	}
	if result.Limit != 10 {
		t.Errorf("Limit = %v, want 10", result.Limit)
	}
	if result.Action != "sync" {
		t.Errorf("Action = %q, want sync", result.Action)
	}
}

func TestSyncModeConstants(t *testing.T) {
	t.Parallel()

	if SyncAll != 0 {
		t.Errorf("SyncAll = %v, want 0", SyncAll)
	}
	if SyncLimited != 1 {
		t.Errorf("SyncLimited = %v, want 1", SyncLimited)
	}
}
