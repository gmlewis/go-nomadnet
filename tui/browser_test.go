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
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestHandleLinkAnchor(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("#section1")
	if err != nil || dest != "anchor" || hash != "section1" {
		t.Errorf("HandleLink(#section1) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkLXMF(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("lxmf@aabb11223344")
	if err != nil || dest != "lxmf" || hash != "aabb11223344" {
		t.Errorf("HandleLink(lxmf@...) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkRRC(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("rrc://hub123")
	if err != nil || dest != "rrc" || hash != "hub123" {
		t.Errorf("HandleLink(rrc://...) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkPageHash(t *testing.T) {
	t.Parallel()
	hash := "aabb11223344556677889900aabb11223344556677889900aabb112233445566"
	dest, gotHash, err := HandleLink(hash)
	if err != nil || dest != "page" || gotHash != hash {
		t.Errorf("HandleLink(64hex) = %q,%q,%v", dest, gotHash, err)
	}
}

func TestHandleLinkEmpty(t *testing.T) {
	t.Parallel()
	_, _, err := HandleLink("")
	if err == nil {
		t.Error("HandleLink(empty) should return error")
	}
}

// TestBrowserPageViewWheelMultiplier pins the browser page's per-primitive
// wheel multiplier (applyWheelMultiplier, installed in newBrowserPageView): a
// wheel notch scrolls mouseWheelLines rows in one delivery via ScrollTo, and at
// the top/bottom boundary it declines to consume so tview skips the no-op
// redraw (the fix for the "scroll hangs at the ends" symptom and the
// trackEnd-jump-after-topic-switch bug — scrolling by N in one ScrollTo keeps
// trackEnd=false, so no leap to the bottom). mid-page notches consume and move
// the offset by N rows, not tview's default 1.
func TestBrowserPageViewWheelMultiplier(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	const delta = 3
	SetMouseWheelLines(delta)

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	var b strings.Builder
	for i := range 40 {
		b.WriteString("line ")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	bd.content.SetText(b.String())

	screen := newBenchScreen()
	defer screen.Fini()
	v := bd.content
	const w, h = 40, 10
	v.SetRect(0, 0, w, h)
	v.Draw(screen) // settle the line index + clamp lineOffset to 0

	_, _, cw, ch := v.GetInnerRect()
	_ = cw
	total := v.GetWrappedLineCount()
	posmax := total - ch
	if posmax <= 0 {
		t.Fatalf("need overflow for a boundary test: total=%v h=%v", total, ch)
	}

	handler := v.MouseHandler()
	ev := func() *tcell.EventMouse { return tcell.NewEventMouse(w/2, h/2, tcell.ButtonNone, tcell.ModNone) }
	setFocus := func(p tview.Primitive) {}

	if row, _ := v.GetScrollOffset(); row != 0 {
		t.Fatalf("after Draw at top: lineOffset=%v, want 0", row)
	}

	// At the top, scrolling up is a no-op: must NOT consume, lineOffset stays 0.
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); consumed {
		t.Error("ScrollUp at top: consumed=true, want false (no-op should skip redraw)")
	}
	if row, _ := v.GetScrollOffset(); row != 0 {
		t.Errorf("ScrollUp at top moved lineOffset to %v, want unchanged 0", row)
	}

	// Scroll to the bottom and re-Draw so lineOffset clamps to posmax.
	v.ScrollTo(1<<20, 0)
	v.Draw(screen)
	if row, _ := v.GetScrollOffset(); row != posmax {
		t.Fatalf("after ScrollTo end: lineOffset=%v, want posmax=%v (total=%v)", row, posmax, total)
	}

	// At the bottom, scrolling down is a no-op: must NOT consume.
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); consumed {
		t.Error("ScrollDown at bottom: consumed=true, want false (no-op should skip redraw)")
	}
	if row, _ := v.GetScrollOffset(); row != posmax {
		t.Errorf("ScrollDown at bottom moved lineOffset to %v, want unchanged %v", row, posmax)
	}

	// Mid-page: a wheel notch must consume AND move the offset by delta rows.
	mid := posmax / 2
	v.ScrollTo(mid, 0)
	v.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); !consumed {
		t.Error("ScrollDown at mid: consumed=false, want true")
	}
	if row, _ := v.GetScrollOffset(); row != mid+delta {
		t.Errorf("ScrollDown at mid moved lineOffset to %v, want %v", row, mid+delta)
	}

	// Scroll up from mid: must consume and decrement by delta.
	v.ScrollTo(mid, 0)
	v.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); !consumed {
		t.Error("ScrollUp at mid: consumed=false, want true")
	}
	if row, _ := v.GetScrollOffset(); row != mid-delta {
		t.Errorf("ScrollUp at mid moved lineOffset to %v, want %v", row, mid-delta)
	}
}

func TestBrowserPageHeaderAndMicronRendering(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	bd.LoadURL("nomadnetwork://44f0dbf2ec1c2ac47277995475217aed/page/status.mu")
	bd.RenderPage(">Welcome Node\n-")

	text := bd.content.GetText(true)
	if text == "" {
		t.Fatal("content text is empty after RenderPage")
	}
	if !testing.Short() && len(text) < 5 {
		t.Errorf("content text too short: %q", text)
	}
}

func TestHandleLinkUnknown(t *testing.T) {
	t.Parallel()
	_, _, err := HandleLink("ftp://example.com")
	if err == nil {
		t.Error("HandleLink(ftp://) should return error")
	}
}

func TestDetectPartials(t *testing.T) {
	t.Parallel()

	markup := "Hello world\n>>header\nSome text\n>>footer\nEnd"
	partials := DetectPartials(markup)
	if len(partials) != 2 {
		t.Fatalf("DetectPartials got %v, want 2", len(partials))
	}
	if partials[0] != "header" || partials[1] != "footer" {
		t.Errorf("DetectPartials = %v, want [header footer]", partials)
	}
}

func TestDetectPartialsNone(t *testing.T) {
	t.Parallel()
	partials := DetectPartials("No partials here")
	if len(partials) != 0 {
		t.Errorf("DetectPartials got %v, want 0", len(partials))
	}
}

func TestParseMicronColorsBgFg(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("#!bg=fff\n#!fg=000\nHello")
	if bg != "fff" || fg != "000" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want bg=fff fg=000", bg, fg)
	}
}

func TestParseMicronColorsNone(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("No colors here")
	if bg != "" || fg != "" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want empty", bg, fg)
	}
}

func TestParseMicronColorsHex6(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("#!bg=aabbcc\n#!fg=112233\n")
	if bg != "aabbcc" || fg != "112233" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want bg=aabbcc fg=112233", bg, fg)
	}
}

// TestParseMicronColorsGolden pins the Go port of Python Browser.load_page's
// #!bg=/#!fg= page-color extraction (Browser.py:1247-1267 + 1326-1346) against
// golden values captured from the installed Python nomadnet. The Python code
// finds the directive, takes the slice up to the NEXT newline, and sets the
// color only when that slice is exactly 3 or 6 chars. CRITICAL edge cases:
//   - A directive with NO trailing newline (end of markup) yields NO color in
//     Python (str.find returns -1 → length check is negative). The Go port must
//     NOT fall back to end-of-markup there.
//   - A value followed by other text on the same line ("abc ", "abc; extra")
//     has length != 3/6 → no color.
//   - \r\n: the value includes the \r → length 4 → no color.
func TestParseMicronColorsGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		markup string
		wantBG string
		wantFG string
	}{
		{"bg3", "#!bg=abc\nbody", "abc", ""},
		{"bg6", "#!bg=abcdef\nbody", "abcdef", ""},
		{"bg wrong len 2", "#!bg=ab\nbody", "", ""},
		{"bg too long 7", "#!bg=abcdefg\nbody", "", ""},
		{"bg no newline len3", "#!bg=abc", "", ""},
		{"bg no newline len6", "#!bg=abcdef", "", ""},
		{"both", "#!fg=f00\n#!bg=111\nbody", "111", "f00"},
		{"no colors", "no colors here", "", ""},
		{"bg crlf", "#!bg=abc\r\nbody", "", ""},
		{"bg trailing text", "#!bg=abc # comment\nbody", "", ""},
		{"fg6", "#!fg=112233\nbody", "", "112233"},
		{"fg no newline", "#!fg=112233", "", ""},
		{"bg mid markup", ">Title\n#!bg=abc\nContent", "abc", ""},
		{"first directive wins", "#!bg=abc\n#!bg=def\nbody", "abc", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bg, fg := ParseMicronColors(c.markup)
			if bg != c.wantBG || fg != c.wantFG {
				t.Errorf("ParseMicronColors(%q) = bg=%q fg=%q, want bg=%q fg=%q",
					c.markup, bg, fg, c.wantBG, c.wantFG)
			}
		})
	}
}

func TestBrowserDisplayCurrentURL(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	if bd.CurrentURL() != "" {
		t.Error("CurrentURL() should be empty initially")
	}

	bd.LoadURL("test://page1")
	if bd.CurrentURL() != "test://page1" {
		t.Errorf("CurrentURL() = %q, want test://page1", bd.CurrentURL())
	}
}

func TestBrowserDisplayBackForward(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("page1")
	bd.LoadURL("page2")
	bd.LoadURL("page3")

	bd.GoBack()
	if bd.CurrentURL() != "page2" {
		t.Errorf("GoBack() CurrentURL = %q, want page2", bd.CurrentURL())
	}

	bd.GoBack()
	if bd.CurrentURL() != "page1" {
		t.Errorf("GoBack() CurrentURL = %q, want page1", bd.CurrentURL())
	}

	bd.GoForward()
	if bd.CurrentURL() != "page2" {
		t.Errorf("GoForward() CurrentURL = %q, want page2", bd.CurrentURL())
	}

	bd.LoadURL("page4")
	bd.GoForward()
	if bd.CurrentURL() != "page4" {
		t.Errorf("GoForward after LoadURL should not move, got %q", bd.CurrentURL())
	}
}

func TestBrowserDisplayReload(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("test://page")
	bd.Reload()
	if bd.CurrentURL() != "test://page" {
		t.Errorf("Reload() CurrentURL = %q, want test://page", bd.CurrentURL())
	}
}

// TestBrowserDisplayEffectiveMarkup pins the partial-substitution logic: the
// original markup (with directives) is kept, and each fetched partial's content
// replaces its directive (Partial.Raw) for rendering. Unfetched partials keep
// their directive (the micron renderer shows the ⧖ placeholder). Mirrors
// Python's per-Pile partial replacement without the urwid widget tree.
func TestBrowserDisplayEffectiveMarkup(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)

	// No partial contents → effectiveMarkup == currentMarkup.
	bd.currentMarkup = ">Page\n`{url`5}\nEnd"
	if got := bd.effectiveMarkup(); got != bd.currentMarkup {
		t.Errorf("effectiveMarkup (no contents) = %q, want %q", got, bd.currentMarkup)
	}

	// Substitute one partial's content for its directive.
	bd.partialContents = map[string]string{"`{url`5}": ">Partial\nBody"}
	got := bd.effectiveMarkup()
	want := ">Page\n>Partial\nBody\nEnd"
	if got != want {
		t.Errorf("effectiveMarkup (one partial) = %q, want %q", got, want)
	}

	// Two partials substitute independently.
	bd.currentMarkup = ">A\n`{p1}\nmid\n`{p2}\nend"
	bd.partialContents = map[string]string{"`{p1}": "X", "`{p2}": "Y"}
	got = bd.effectiveMarkup()
	want = ">A\nX\nmid\nY\nend"
	if got != want {
		t.Errorf("effectiveMarkup (two partials) = %q, want %q", got, want)
	}
}

// TestBrowserDisplayPartialsInertWithoutCallback verifies that RenderPage with
// no OnFetchPartial wired does not start any refresh goroutines (partials stay
// as ⧖ placeholders) — the common case for the unit-test harness and any
// BrowserDisplay whose app layer hasn't wired the fetch backend.
func TestBrowserDisplayPartialsInertWithoutCallback(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	if bd.OnFetchPartial != nil {
		t.Fatal("OnFetchPartial should be nil by default")
	}
	// RenderPage with a partial directive must not panic or hang.
	bd.RenderPage(">Page\n`{url`5}\nEnd")
	if bd.partialCancel != nil {
		t.Error("startPartials should not start a loop without OnFetchPartial")
	}
}

// TestBrowserDisplayDisconnect pins Python Browser.disconnect (Browser.py:862-
// 881): tearing down the link clears the history, resets the history pointer,
// and shows the disconnected state. The current-destination hint is also cleared
// so a subsequent relative ":<path>" URL does not resolve against the stale
// node (Python sets status=DISCONECTED and clears request_data; the Go port
// keeps no persistent link to tear down, but must still reset navigation).
func TestBrowserDisplayDisconnect(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("page1")
	bd.LoadURL("page2")
	bd.LoadURL("page3")
	if len(bd.history) != 3 {
		t.Fatalf("history len = %v, want 3", len(bd.history))
	}

	bd.Disconnect()

	if len(bd.history) != 0 {
		t.Errorf("after Disconnect history len = %v, want 0 (Python clears history)", len(bd.history))
	}
	if bd.histIdx != 0 {
		t.Errorf("after Disconnect histIdx = %v, want 0", bd.histIdx)
	}
	if bd.CurrentURL() != "" {
		t.Errorf("after Disconnect CurrentURL = %q, want empty", bd.CurrentURL())
	}
	if bd.CurrentDest() != nil {
		t.Errorf("after Disconnect CurrentDest = %x, want nil", bd.CurrentDest())
	}
}

func TestBrowserDisplayMarkedLink(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	app.Main.SetDisplay("browser", bd.Widget())
	app.Main.activePage = "browser"

	bd.MarkedLink("http://example.com", "f1|f2")
	got := bd.footerStatus.GetText(true)
	if got != "Link to http://example.com`f1|f2" {
		t.Errorf("MarkedLink footer = %q, want 'Link to http://example.com`f1|f2'", got)
	}

	bd.MarkedLink("", "")
	gotCleared := bd.footerStatus.GetText(true)
	if gotCleared == "Link to http://example.com`f1|f2" {
		t.Errorf("MarkedLink empty did not clear footer target")
	}
}
