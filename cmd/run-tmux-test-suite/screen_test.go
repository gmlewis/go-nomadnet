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

func TestAnnounceInfoAddr(t *testing.T) {
	bar := menuBar()
	info := "┌─ Announce Info " + strings.Repeat("─", 20) + "┐\n"
	addr := "Addr  : <0123456789abcdef>\n"
	raw := bar + "\n" + info + addr
	v := viewOf(raw, 0, 2)
	hash, ok := v.AnnounceInfoAddr()
	if !ok {
		t.Fatal("AnnounceInfoAddr not ok")
	}
	if hash != "0123456789abcdef" {
		t.Errorf("addr = %q, want 0123456789abcdef", hash)
	}
}

func TestFocusedActionButton(t *testing.T) {
	bar := menuBar()
	// Button row with < Back > < Connect > < Msg Op > < Save > (all default style).
	buttons := "< Back >  < Connect >  < Msg Op >  < Save >\n"
	raw := bar + "\n" + buttons
	s := parseScreen([]byte(raw))
	// Find the column of "Connect" and place the cursor on it.
	connectCol := strings.Index(s.rowText(1), "< Connect >")
	v := &View{Screen: s, CursorX: connectCol + 2, CursorY: 1, CursorOK: true}
	got, ok := v.FocusedActionButton()
	if !ok || got != "Connect" {
		t.Errorf("FocusedActionButton = (%q,%t), want (Connect,true)", got, ok)
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

func TestGuideTopicRendered(t *testing.T) {
	// Full-width screen (W=60): left third (cols 0..19) is the topics list,
	// the reader occupies cols 20..59. The first non-blank reader line is the
	// topic title, which starts at col 20 (= W/3). All rows are 60 ASCII chars
	// so display columns == rune count.
	const W = 60
	blankRow := strings.Repeat(" ", W)
	titleRow := strings.Repeat(" ", 20) + "Nomad Network" + strings.Repeat(" ", W-20-13)
	raw := blankRow + "\n" + blankRow + "\n" + titleRow + "\n" + blankRow
	v := viewOf(raw, 0, 1)
	got := strings.TrimSpace(v.GuideTopicRendered())
	if got != "Nomad Network" {
		t.Errorf("GuideTopicRendered = %q, want Nomad Network", got)
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
