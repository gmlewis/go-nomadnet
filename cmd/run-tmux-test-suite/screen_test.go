// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"testing"
)

// screen_test.go unit-tests the ANSI SGR parser and the View query helpers
// (menu focus via cursor, list selected row via #aaaaaa, border-title page
// detection, Announce Info addr, focused action button, browser state, Guide
// topic rendered title) against synthetic `tmux capture-pane -e` fixtures that
// mirror the real TUI's emitted escapes — so the parser is correct without
// needing a live app.

const (
	esc   = "\x1b"
	reset = "\x1b[0m"
	defFG = "\x1b[39m"
	defBG = "\x1b[49m"
)

func fgRGB(hex int32) string {
	r, g, b := (hex>>16)&0xff, (hex>>8)&0xff, hex&0xff
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func bgRGB(hex int32) string {
	r, g, b := (hex>>16)&0xff, (hex>>8)&0xff, hex&0xff
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// styled emits text with the given fg/bg (colorDefault skips that channel).
func styled(fg, bg int32, text string) string {
	var b strings.Builder
	if fg == colorDefault {
		b.WriteString(defFG)
	} else {
		b.WriteString(fgRGB(fg))
	}
	if bg == colorDefault {
		b.WriteString(defBG)
	} else {
		b.WriteString(bgRGB(bg))
	}
	b.WriteString(text)
	b.WriteString(reset)
	return b.String()
}

const (
	cMenubarFG = int32(0x111111)
	cMenubarBG = int32(0xbbbbbb)
	cFocusFG   = int32(0x111111)
	cFocusBG   = int32(0xaaaaaa)
)

// menuBar builds the row-0 menu bar text: a leading indicator + "[ Label ]"
// buttons separated by single spaces, all in the menubar style.
func menuBar() string {
	var b strings.Builder
	b.WriteString(styled(cMenubarFG, cMenubarBG, "•")) // decoration_menu indicator (1 col)
	for _, label := range menuLabels {
		b.WriteString(styled(cMenubarFG, cMenubarBG, " [ "+label+" ]"))
	}
	return b.String()
}

// viewOf builds a View from raw ANSI lines + a cursor position.
func viewOf(raw string, cx, cy int) *View {
	s := parseScreen([]byte(raw))
	return &View{Screen: s, CursorX: cx, CursorY: cy, CursorOK: true}
}

func TestParseScreenColors(t *testing.T) {
	raw := styled(cFocusFG, cFocusBG, "Introduction") + "\n"
	s := parseScreen([]byte(raw))
	if s.H < 1 || len(s.Rows[0]) < len("Introduction") {
		t.Fatalf("screen too small: H=%d row0=%d", s.H, len(s.Rows[0]))
	}
	// First content cell must carry the #aaaaaa bg (list focus).
	c := s.Rows[0][0]
	if c.BG != cFocusBG {
		t.Errorf("cell bg = %#06x, want %#06x (list_focus)", c.BG, cFocusBG)
	}
	if c.FG != cFocusFG {
		t.Errorf("cell fg = %#06x, want %#06x", c.FG, cFocusFG)
	}
	if c.Ch != 'I' {
		t.Errorf("cell ch = %q, want 'I'", string(c.Ch))
	}
}

func TestParseScreenDefaultColor(t *testing.T) {
	raw := styled(colorDefault, colorDefault, "hello") + "\n"
	s := parseScreen([]byte(raw))
	c := s.Rows[0][0]
	if c.BG != colorDefault {
		t.Errorf("default bg = %#06x, want colorDefault", c.BG)
	}
	if c.FG != colorDefault {
		t.Errorf("default fg = %#06x, want colorDefault", c.FG)
	}
	if c.Ch != 'h' {
		t.Errorf("ch = %q, want 'h'", string(c.Ch))
	}
}

func TestMenuFocusedButton(t *testing.T) {
	bar := menuBar()
	s := parseScreen([]byte(bar + "\n"))
	row0 := s.rowText(0)
	// Cursor on the first letter of "Network" (which is button index 1).
	netCol := strings.Index(row0, "[ Network ]")
	if netCol < 0 {
		t.Fatalf("Network button not found in row 0: %q", row0)
	}
	cx := netCol + 2 // first letter inside "[ "
	v := &View{Screen: s, CursorX: cx, CursorY: 0, CursorOK: true}
	idx, ok := v.MenuFocusedButton()
	if !ok {
		t.Errorf("MenuFocusedButton not ok at cursor x=%d (row0=%q)", cx, row0)
	}
	if idx != 1 {
		t.Errorf("MenuFocusedButton = %d, want 1 (Network); row0=%q", idx, row0)
	}

	// Cursor off row 0 => not focused.
	v.CursorY = 3
	if _, ok := v.MenuFocusedButton(); ok {
		t.Errorf("MenuFocusedButton ok on row 3, want not ok (menu is row 0)")
	}
}

func TestMenuFocusedButtonAllIndices(t *testing.T) {
	bar := menuBar()
	s := parseScreen([]byte(bar + "\n"))
	row0 := s.rowText(0)
	for i, label := range menuLabels {
		target := "[ " + label + " ]"
		col := strings.Index(row0, target)
		if col < 0 {
			t.Fatalf("button %q not in row 0", label)
		}
		v := &View{Screen: s, CursorX: col + 2, CursorY: 0, CursorOK: true}
		idx, ok := v.MenuFocusedButton()
		if !ok || idx != i {
			t.Errorf("label %q (index %d): got (%d,%t), want (%d,true)", label, i, idx, ok, i)
		}
	}
}

func TestActivePageGuide(t *testing.T) {
	// Row 0 menu bar; row 1 a Guide "Topics" border title.
	bar := menuBar()
	border := "┌─ Topics " + strings.Repeat("─", 20) + "┐"
	raw := bar + "\n" + border + "\n"
	v := viewOf(raw, 0, 1)
	if got := v.ActivePage(); got != "guide" {
		t.Errorf("ActivePage = %q, want guide", got)
	}
}

func TestActivePageNetwork(t *testing.T) {
	bar := menuBar()
	border := "┌─ Announce Stream " + strings.Repeat("─", 20) + "┐"
	raw := bar + "\n" + border + "\n"
	v := viewOf(raw, 0, 1)
	if got := v.ActivePage(); got != "network" {
		t.Errorf("ActivePage = %q, want network", got)
	}
}

func TestListSelectedRow(t *testing.T) {
	// A list with one selected (#aaaaaa) row and two default rows.
	row0 := "• [ Conversations ] ...\n"
	sel := styled(cFocusFG, cFocusBG, "  Introduction  ") + reset + "\n"
	other := styled(colorDefault, colorDefault, "  Concepts  ") + reset + "\n"
	raw := row0 + sel + other + other
	v := viewOf(raw, 0, 1)
	rows := v.ListSelectedRows()
	if len(rows) != 1 {
		t.Fatalf("ListSelectedRows = %d rows, want 1", len(rows))
	}
	if rows[0].Text != "Introduction" {
		t.Errorf("selected text = %q, want Introduction", rows[0].Text)
	}
	if !rows[0].Selected {
		t.Error("selected row not marked Selected")
	}
}

func TestAnnounceNodeName(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{"named node with borders", "│16:32:23 Ⓝ  Rostov                                │", "Rostov"},
		{"re-announce with new timestamp keeps name", "│16:33:01 Ⓝ  Rostov                                │", "Rostov"},
		{"date timestamp other day", "│2026-08-01 Ⓝ  Helsinki                            │", "Helsinki"},
		{"hash in show-destination mode", "│16:32:23 Ⓝ  c5379340701ac6fff391de24d42db64f    │", "c5379340701ac6fff391de24d42db64f"},
		{"no borders", "16:32:23 Ⓝ  Rostov", "Rostov"},
		{"empty name falls back to <hash>", "│16:32:23 Ⓝ  <c5379340701ac6fff391de24d42db64f>  │", "<c5379340701ac6fff391de24d42db64f>"},
		// announceNodeName only runs on the selected #aaaaaa node row, so a
		// non-announce string never reaches it; the contract is "return the 3rd
		// space-separated field", so a 3-word string yields its 3rd field.
		{"three-word string yields 3rd field", "just some text", "text"},
		{"two-word row has no name field", "16:32:23 Ⓝ", ""},
		{"blank", "      ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := announceNodeName(tt.row)
			if got != tt.want {
				t.Errorf("announceNodeName(%q) = %q, want %q", tt.row, got, tt.want)
			}
		})
	}
}

// TestAnnounceNodeNameDedupStability confirms the dedup key is stable across a
// re-announce (new timestamp, same name) — the exact bug that made the old
// suite reconnect the same node repeatedly.
func TestAnnounceNodeNameDedupStability(t *testing.T) {
	first := announceNodeName("│16:32:23 Ⓝ  Rostov      │")
	again := announceNodeName("│16:33:55 Ⓝ  Rostov      │")
	if first != again || first != "Rostov" {
		t.Errorf("dedup key changed across re-announce: %q vs %q (want Rostov)", first, again)
	}
}

func TestAnnounceInfoAddr(t *testing.T) {
	bar := menuBar()
	info := "┌─ Announce Info " + strings.Repeat("─", 20) + "┐\n"
	// The real app renders Announce Info inside a bordered box, so each content
	// row is prefixed with the '│' border char (which TrimSpace does NOT strip).
	addr := "│Addr  : <0123456789abcdef>   │\n"
	raw := bar + "\n" + info + addr
	v := viewOf(raw, 0, 2)
	hash, ok := v.AnnounceInfoAddr()
	if !ok {
		t.Fatal("AnnounceInfoAddr not ok (leading │ not handled?)")
	}
	if hash != "0123456789abcdef" {
		t.Errorf("addr = %q, want 0123456789abcdef", hash)
	}
}

func TestAnnounceInfoAddrAboveLocalPeer(t *testing.T) {
	// When Announce Info is open, both its "Addr  : <nodehash>" row and the
	// Local Peer Info "LXMF Addr : <localhash>" row contain "Addr"; the node
	// hash (in the Announce Info box, above) must win.
	bar := menuBar()
	raw := bar + "\n" +
		"┌─ Announce Info " + strings.Repeat("─", 20) + "┐\n" +
		"│Addr  : <nodehash1234>           │\n" +
		"┌─ Local Peer Info " + strings.Repeat("─", 18) + "┐\n" +
		"│LXMF Addr : <localhash5678>      │\n"
	v := viewOf(raw, 0, 3)
	hash, ok := v.AnnounceInfoAddr()
	if !ok {
		t.Fatal("AnnounceInfoAddr not ok")
	}
	if hash != "nodehash1234" {
		t.Errorf("addr = %q, want nodehash1234 (Local Peer Addr must not win)", hash)
	}
}

func TestFocusedActionButton(t *testing.T) {
	bar := menuBar()
	// The real app pads buttons to a fixed inner width, so the on-screen text is
	// "< Back    >  < Connect >  < Msg Op  >  < Save    >" — NOT "< Back >". The
	// parser must scan "< ... >" spans generically and trim the inner label.
	buttons := "< Back    >  < Connect >  < Msg Op  >  < Save    >\n"
	raw := bar + "\n" + buttons
	s := parseScreen([]byte(raw))
	rowText := s.rowText(1)
	for _, want := range []string{"Back", "Connect", "Msg Op", "Save"} {
		// Place the cursor on the label's first letter inside "< ... >".
		// Back/Msg Op/Save are padded with trailing spaces; Connect is not.
		start := strings.Index(rowText, "< "+want)
		if start < 0 {
			t.Fatalf("button %q not found in %q", want, rowText)
		}
		cx := start + 2 // first letter of label
		v := &View{Screen: s, CursorX: cx, CursorY: 1, CursorOK: true}
		got, ok := v.FocusedActionButton()
		if !ok || got != want {
			t.Errorf("cursor on %q: FocusedActionButton = (%q,%t), want (%q,true) [row=%q cx=%d]",
				want, got, ok, want, rowText, cx)
		}
	}
}

func TestBrowserState(t *testing.T) {
	bar := menuBar()
	tests := []struct {
		name    string
		body    string
		want    string
		wantURL string
	}{
		{"disconnected", "Disconnected\n<-  ->", bsDisconnected, ""},
		{"retrieving", "Retrieving\n[abc]", bsRetrieving, ""},
		{"rendered", "URL: abc123\nBrowser\nsome content", bsRendered, "abc123"},
		{"error-nopath", "No path to destination known", "error:No path to destination known", ""},
		{"error-linktimeout", "Link establishment timed out", "error:Link establishment timed out", ""},
		{"error-reqfailed", "Request failed", "error:Request failed", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := bar + "\n" + tt.body + "\n"
			v := viewOf(raw, 0, 1)
			got, url := v.BrowserState()
			if got != tt.want {
				t.Errorf("state = %q, want %q", got, tt.want)
			}
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestBrowserURLParsing(t *testing.T) {
	bar := menuBar()
	raw := bar + "\nURL: 7d2c9a4b\nBrowser title\n"
	v := viewOf(raw, 0, 1)
	_, url := v.BrowserState()
	if url != "7d2c9a4b" {
		t.Errorf("url = %q, want 7d2c9a4b", url)
	}
}

// TestBrowserURLBordered confirms browserURL strips BOTH the leading '││' border
// AND the trailing '   ││' border of the URL bar row — the bug that made
// Phase 3 record 0 successes (url came back as "<hash>││", never matching the
// target hash, so every connect timed out as state="").
func TestBrowserURLBordered(t *testing.T) {
	bar := menuBar()
	// Nested node-page box: "││URL: <hash>   ││" with leading AND trailing
	// borders. TrimSpace leaves the trailing "││" attached.
	rows := []string{
		"││URL: 7eb47d629a3b84984750355da9efb7fb   ││",
		"││                                          ││",
		"││  Nomad Network                           ││",
	}
	raw := bar + "\n" + strings.Join(rows, "\n") + "\n"
	v := viewOf(raw, 0, 1)
	st, url := v.BrowserState()
	if url != "7eb47d629a3b84984750355da9efb7fb" {
		t.Errorf("bordered url = %q, want the bare hash (trailing ││ must be stripped)", url)
	}
	if st != bsRendered {
		t.Errorf("bordered state = %q, want %q", st, bsRendered)
	}
}

func TestGuideTopicRendered(t *testing.T) {
	// Model the real Python Guide layout (W=60): a Topics LineBox (cols 0..19)
	// and a reader LineBox (cols 20..59), both bordered. Row 1 holds both top
	// borders (┌ at col 0 and ┌ at col 20); the reader's first content row (row
	// 2) holds the topic title. GuideTopicRendered must locate the reader's
	// left edge (the SECOND ┌) and return the first content line stripped of
	// its │ borders.
	const W = 60
	blankRow := strings.Repeat(" ", W)
	borderRow := "┌" + strings.Repeat("─", 18) + "┐" + "┌" + strings.Repeat("─", 38) + "┐"
	// Topics box content (cols 0..19) + reader content (cols 20..59).
	readerContent := "Nomad Network" + strings.Repeat(" ", 38-13)
	contentRow := "│Introduction" + strings.Repeat(" ", 6) + "│" + "│" + readerContent + "│"
	raw := blankRow + "\n" + borderRow + "\n" + contentRow + "\n" + blankRow
	v := viewOf(raw, 0, 1)
	got := strings.TrimSpace(v.GuideTopicRendered())
	if got != "Nomad Network" {
		t.Errorf("GuideTopicRendered = %q, want Nomad Network", got)
	}
}

// TestGuideTopicRenderedBordered confirms the reader title is extracted cleanly
// when the reader pane is a bordered box — every content row is wrapped in
// leading '│' and trailing '│' (or '┃' scrollbar) chars that TrimSpace does
// NOT strip. Without stripping the trailing border the title compared as
// "Nomad Network │", falsely reporting the scroll-reset bug on every topic.
func TestGuideTopicRenderedBordered(t *testing.T) {
	const W = 60
	blankRow := strings.Repeat(" ", W)
	borderRow := "┌" + strings.Repeat("─", 18) + "┐" + "┌" + strings.Repeat("─", 38) + "┐"
	// Title indented 2 inside the reader box.
	indented := "  Nomad Network" + strings.Repeat(" ", 38-2-13)
	contentRow := "│Introduction" + strings.Repeat(" ", 6) + "│" + "│" + indented + "│"
	raw := blankRow + "\n" + borderRow + "\n" + contentRow + "\n" + blankRow
	v := viewOf(raw, 0, 1)
	got := strings.TrimSpace(v.GuideTopicRendered())
	if got != "Nomad Network" {
		t.Errorf("bordered GuideTopicRendered = %q, want Nomad Network (trailing │ must be stripped)", got)
	}
}

func TestXterm256(t *testing.T) {
	// 256-color truecyan-ish sanity: index 51 = (0,5,5) cube -> 0,255,255.
	c := xterm256(51)
	if c != 0x00ffff {
		t.Errorf("xterm256(51) = %#06x, want 0x00ffff", c)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"135x32", 135, 32, true},
		{"80x24", 80, 24, true},
		{"32 135", 135, 32, true}, // stty size: rows cols
		{"bogus", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			w, h, ok := parseSize(tt.in)
			if ok != tt.ok || (ok && (w != tt.w || h != tt.h)) {
				t.Errorf("parseSize(%q) = (%d,%d,%t), want (%d,%d,%t)", tt.in, w, h, ok, tt.w, tt.h, tt.ok)
			}
		})
	}
}
