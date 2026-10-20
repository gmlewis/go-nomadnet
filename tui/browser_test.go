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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
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
// redraw. Scrolling by N in one ScrollTo keeps trackEnd=false, so there is no
// leap to the bottom. Mid-page notches consume and move the offset by N rows,
// not tview's default 1.
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

// TestBrowserPageHeaderAndMicronRendering pins that a page's section header and
// its body markup both reach the render: the heading keeps its text and the
// theme's heading1 style, a bare "-" line becomes a divider, and the painted
// text carries the header rather than only the line model.
func TestBrowserPageHeaderAndMicronRendering(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	bd.LoadURL("nomadnetwork://44f0dbf2ec1c2ac47277995475217aed/page/status.mu")
	bd.RenderPage(">Welcome Node\n-")

	if len(bd.currentLines) < 2 {
		t.Fatalf("rendered %v lines, want the heading and the divider", len(bd.currentLines))
	}

	// >Welcome Node is a level-1 section header: its text intact, carrying the theme's
	// heading1 style (Python's heading_1 palette entry, TextUI.py).
	heading := bd.currentLines[0]
	if heading.HeadingLevel != 1 {
		t.Errorf("first line heading level = %v, want 1", heading.HeadingLevel)
	}
	if len(heading.Spans) != 1 || heading.Spans[0].Text != "Welcome Node" {
		t.Errorf("header spans = %+v, want one span reading %q", heading.Spans, "Welcome Node")
	}
	if heading.Spans[0].FG != "#222222" || heading.Spans[0].BG != "#bbbbbb" {
		t.Errorf("header fg/bg = %v/%v, want the heading1 style #222222/#bbbbbb",
			heading.Spans[0].FG, heading.Spans[0].BG)
	}

	// A bare "-" line is a micron divider.
	if !bd.currentLines[1].Divider {
		t.Errorf("second line is not a divider: %+v", bd.currentLines[1])
	}

	// And the painted text carries the header, so the content is really on screen
	// rather than only in the line model.
	if text := bd.content.GetText(true); !strings.Contains(text, "Welcome Node") {
		t.Errorf("painted content does not contain the header text: %q", text)
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

// TestBrowserDisplayPartialSplicing pins the partial substitution: each fetched
// partial is rendered as its own document and its lines replace the ⧖
// placeholder line, so formatting state cannot cross the boundary. Python parses
// every partial through markup_to_attrmaps with the page's default colors
// (Browser.py:699-702).
func TestBrowserDisplayPartialSplicing(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)

	// An unfetched partial keeps its placeholder line.
	bd.RenderPage(">Page\n`!bold \n`{url`5}\nafter")
	if !lineTextsContain(bd.currentLines, "⧖") {
		t.Fatalf("an unfetched partial did not render its placeholder: %v", lineTexts(bd.currentLines))
	}

	// Once the content arrives, its own lines replace the placeholder. The
	// page's unclosed bold must NOT reach the partial, and the partial must not
	// reset the page's bold for the line after it.
	bd.partialContents = map[string]string{"`{url`5}": "partial text"}
	bd.renderPage()

	var partialLine, afterLine *micron.StyledLine
	for _, sl := range bd.currentLines {
		switch styledLineText(sl) {
		case "partial text":
			partialLine = sl
		case "after":
			afterLine = sl
		}
	}
	if partialLine == nil {
		t.Fatalf("partial content was not spliced in: %v", lineTexts(bd.currentLines))
	}
	for _, span := range partialLine.Spans {
		if span.Text != "" && span.Bold {
			t.Errorf("partial span %q inherited the page's bold formatting", span.Text)
		}
	}
	if afterLine == nil {
		t.Fatalf("the page line after the partial is missing: %v", lineTexts(bd.currentLines))
	}
	bold := false
	for _, span := range afterLine.Spans {
		if span.Text != "" && span.Bold {
			bold = true
		}
	}
	if !bold {
		t.Errorf("the partial reset the page's formatting state; the line after it lost the page's bold")
	}
	if lineTextsContain(bd.currentLines, "⧖") {
		t.Errorf("the placeholder survived a fetched partial: %v", lineTexts(bd.currentLines))
	}

	// The SAME directive may appear more than once in one page: every
	// occurrence must receive the fetched content (a deliberate divergence from
	// Python's hash-keyed page_partials, which patches only the last pile).
	bd.RenderPage(">A\n`{p1}\nmid\n`{p1}\nend")
	bd.partialContents = map[string]string{"`{p1}": "COUNT"}
	bd.renderPage()
	if got := countText(bd.currentLines, "COUNT"); got != 2 {
		t.Errorf("repeated directive rendered %v copies of its content, want 2", got)
	}
}

// styledLineText concatenates a line's span text.
func styledLineText(l *micron.StyledLine) string {
	var sb strings.Builder
	for _, s := range l.Spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// lineTexts returns the concatenated text of each rendered line.
func lineTexts(lines []*micron.StyledLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = styledLineText(l)
	}
	return out
}

// lineTextsContain reports whether any line's text contains sub.
func lineTextsContain(lines []*micron.StyledLine, sub string) bool {
	for _, l := range lines {
		if strings.Contains(styledLineText(l), sub) {
			return true
		}
	}
	return false
}

// countText counts how many lines equal text.
func countText(lines []*micron.StyledLine, text string) int {
	n := 0
	for _, l := range lines {
		if styledLineText(l) == text {
			n++
		}
	}
	return n
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
// node (Python sets status=DISCONECTED and clears request_data).
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
	// Wide content rect so "Link to <target>`<fields>" is not ellipsized here —
	// this test targets the link-peek text + fields, not truncation.
	bd.content.SetRect(0, 0, 100, 10)

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
