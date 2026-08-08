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
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// LinkSpec holds metadata for a clickable link within LinkableText.
// Matches Python's LinkSpec at MicronParser.py:856.
type LinkSpec struct {
	Label  string
	Target string
	Fields string
}

// LinkDelegate mirrors the Python url_delegate interface used by LinkableText
// (MicronParser.py:881-918): link activation, footer link-peek, and focus
// release back to the owning micron view. MarkedLink with an empty target
// clears the peek (Python marked_link(None)).
type LinkDelegate interface {
	HandleLink(target, fields string)
	MarkedLink(target, fields string)
	MicronReleasedFocus()
}

// linkPart is one navigable run of a LinkableText line: a text run plus an
// optional link spec. It is the Go equivalent of one urwid (style, length)
// run from LinkableText.get_text (MicronParser.py:921-929).
type linkPart struct {
	Text string
	Link *micron.LinkSpec
}

// LinkableText displays text with embedded clickable link regions.
// It wraps tview.TextView and tracks link positions so that mouse
// clicks can resolve and dispatch the correct link action.
// Matches Python's LinkableText at MicronParser.py:866.
//
// The nav* fields implement the Python cursor-navigation model: the cursor
// steps through part positions (plain runs AND links), left at position 0
// releases focus, and a 2s key-timeout governs cursor visibility + footer
// peek (MicronParser.py:910-977,982-992).
type LinkableText struct {
	*tview.TextView
	links    []LinkSpec
	onHandle func(target, fields string)

	// Cursor-navigation state (mirrors LinkableText keypress/render).
	parts        []linkPart
	cursor       int
	inColumns    bool
	delegate     LinkDelegate
	keyTimeout   time.Duration
	lastKeypress time.Time
	hasKeypress  bool
}

// NewLinkableText creates a selectable text view with link support.
// The onHandle callback is invoked when a link is activated;
// it receives the link target and optional fields string.
func NewLinkableText(onHandle func(target, fields string)) *LinkableText {
	lt := &LinkableText{
		TextView: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetRegions(true).
			SetTextColor(tcell.NewHexColor(0xbbbbbb)),
		onHandle:   onHandle,
		keyTimeout: 2 * time.Second, // Python key_timeout = 2
	}

	lt.SetHighlightedFunc(func(added, removed, remaining []string) {
		if len(added) > 0 {
			lt.activateRegion(added[0])
		}
	})

	return lt
}

// AddLink registers a link with a label and target. Links are
// numbered sequentially starting from 0 and rendered as tview
// region tags [\"N\"]...[\"\"] within the text.
func (lt *LinkableText) AddLink(label, target string) {
	lt.links = append(lt.links, LinkSpec{
		Label:  label,
		Target: target,
	})
}

// AddLinkWithFields registers a link with an additional fields
// string that is passed to the handler on activation.
func (lt *LinkableText) AddLinkWithFields(label, target, fields string) {
	lt.links = append(lt.links, LinkSpec{
		Label:  label,
		Target: target,
		Fields: fields,
	})
}

// Links returns a copy of the current link list.
func (lt *LinkableText) Links() []LinkSpec {
	result := make([]LinkSpec, len(lt.links))
	copy(result, lt.links)
	return result
}

// PlainText returns the text without tview color/region tags.
func (lt *LinkableText) PlainText() string {
	return lt.GetText(false)
}

// RenderedText returns the text including all tview tags.
func (lt *LinkableText) RenderedText() string {
	return lt.GetText(true)
}

// SetText sets the display text. Callers should embed tview region
// tags like [\"0\"]...[\"\"] that reference link indices added
// via AddLink.
func (lt *LinkableText) SetText(text string) {
	lt.TextView.SetText(text)
}

// Clear removes all text and registered links.
func (lt *LinkableText) Clear() {
	lt.TextView.SetText("")
	lt.links = nil
}

// HandleLinkByIndex activates the link at the given index.
// This is the programmatic equivalent of clicking a link.
func (lt *LinkableText) HandleLinkByIndex(idx int) {
	if idx < 0 || idx >= len(lt.links) {
		return
	}
	link := lt.links[idx]
	if lt.onHandle != nil {
		lt.onHandle(link.Target, link.Fields)
	}
}

// GetRegionByMouse resolves a mouse event to a tview region tag ID.
// It returns the currently highlighted region ID, which tview
// sets when a region is clicked or navigated to.
func (lt *LinkableText) GetRegionByMouse(event *tcell.EventMouse) string {
	highlights := lt.GetHighlights()
	if len(highlights) > 0 {
		return highlights[0]
	}
	return ""
}

// activateRegion dispatches the link handler for a region tag.
// Region tags use numeric IDs matching the link index.
func (lt *LinkableText) activateRegion(regionID string) {
	if regionID == "" {
		return
	}
	idx := 0
	for _, c := range regionID {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
		} else {
			return
		}
	}
	lt.HandleLinkByIndex(idx)
}

// NewLinkableTextFromSpans builds a LinkableText whose navigable parts are the
// given styled spans (the real part structure from micron.RenderToStyledLines),
// wired to a LinkDelegate for link activation, footer peek, and focus release.
// This is the Python-parity constructor: the cursor steps through every part
// (plain runs and links), mirroring LinkableText.get_text's (style, length)
// runs (MicronParser.py:921-929).
func NewLinkableTextFromSpans(spans []micron.StyledSpan, delegate LinkDelegate) *LinkableText {
	lt := &LinkableText{
		TextView: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetRegions(true),
		delegate:   delegate,
		keyTimeout: 2 * time.Second, // Python key_timeout = 2
	}
	lt.SetSpans(spans)
	return lt
}

// SetSpans rebuilds the navigable parts from styled spans. Each span becomes
// one linkPart carrying its text and optional link spec.
func (lt *LinkableText) SetSpans(spans []micron.StyledSpan) {
	lt.parts = make([]linkPart, 0, len(spans))
	for _, s := range spans {
		lt.parts = append(lt.parts, linkPart{Text: s.Text, Link: s.Link})
	}
	lt.cursor = 0
	lt.hasKeypress = false
}

// SetDelegate installs the LinkDelegate used for activation, peek, and focus
// release.
func (lt *LinkableText) SetDelegate(d LinkDelegate) { lt.delegate = d }

// Text returns the concatenated text of all parts (the plain-text content of
// the line, mirroring urwid Text.get_text's text component).
func (lt *LinkableText) Text() string {
	var n int
	for _, p := range lt.parts {
		n += len(p.Text)
	}
	b := make([]byte, 0, n)
	for _, p := range lt.parts {
		b = append(b, p.Text...)
	}
	return string(b)
}

// Cursor returns the current cursor position (a char offset into Text).
func (lt *LinkableText) Cursor() int { return lt.cursor }

// SetCursor sets the cursor position directly (e.g. from a mouse click).
func (lt *LinkableText) SetCursor(n int) {
	if n < 0 {
		n = 0
	}
	lt.cursor = n
}

// SetInColumns toggles columnar-layout mode, where left/right propagate the
// key instead of wrapping/stepping (MicronParser.py:956-957,966-967).
func (lt *LinkableText) SetInColumns(v bool) { lt.inColumns = v }

// PartPositions returns the cumulative part-position table: [0] followed by
// each part's length + running total. Mirrors LinkableText.keypress's
// part_positions build (MicronParser.py:921-929).
func (lt *LinkableText) PartPositions() []int {
	pos := []int{0}
	total := 0
	for _, p := range lt.parts {
		total += len(p.Text)
		pos = append(pos, total)
	}
	return pos
}

// findNextPartPos returns the first part position greater than pos, or pos if
// none. Mirrors find_next_part_pos (MicronParser.py:885-889).
func findNextPartPos(pos int, positions []int) int {
	for _, p := range positions {
		if p > pos {
			return p
		}
	}
	return pos
}

// findPrevPartPos returns the last part position less than pos, or pos if none.
// Mirrors find_prev_part_pos (MicronParser.py:891-896).
func findPrevPartPos(pos int, positions []int) int {
	next := pos
	for _, p := range positions {
		if p < pos {
			next = p
		}
	}
	return next
}

// findItemAtPos returns the part whose char range contains pos, or nil.
// Mirrors find_item_at_pos (MicronParser.py:898-908): total <= pos < total+len.
func (lt *LinkableText) findItemAtPos(pos int) *linkPart {
	total := 0
	for i := range lt.parts {
		length := len(lt.parts[i].Text)
		if total <= pos && pos < total+length {
			return &lt.parts[i]
		}
		total += length
	}
	return nil
}

// LinkAtCursor returns the link spec at the current cursor position, or nil if
// the cursor is on a plain (non-link) part. This exposes both the target URL
// and the display label text (task 3.2 target/display-text).
func (lt *LinkableText) LinkAtCursor() *micron.LinkSpec {
	if part := lt.findItemAtPos(lt.cursor); part != nil {
		return part.Link
	}
	return nil
}

// Activate dispatches the link at the cursor (if any) to the delegate — the
// ACTIVATE/enter path. Returns true when a link was at the cursor.
// Mirrors MicronParser.py:937-941 + handle_link (881-883).
func (lt *LinkableText) Activate() bool {
	link := lt.LinkAtCursor()
	if link == nil {
		return false
	}
	if lt.delegate != nil {
		lt.delegate.HandleLink(link.URL, link.Fields)
	}
	return true
}

// PeekLink reports the focused link's target+fields to the delegate via
// MarkedLink, or clears the peek (empty target) when the cursor is on a plain
// part. Mirrors peek_link (MicronParser.py:910-918).
func (lt *LinkableText) PeekLink() {
	if lt.delegate == nil {
		return
	}
	if link := lt.LinkAtCursor(); link != nil {
		lt.delegate.MarkedLink(link.URL, link.Fields)
	} else {
		lt.delegate.MarkedLink("", "")
	}
}

// CursorVisible reports whether the cursor should be rendered, mirroring the
// render focus condition (MicronParser.py:982-992): visible only when focused,
// and (with a delegate) only within key_timeout of the last keypress. Without a
// delegate the cursor is always visible when focused.
func (lt *LinkableText) CursorVisible(now time.Time, focused bool) bool {
	if !focused {
		return false
	}
	if lt.delegate == nil {
		return true
	}
	if !lt.hasKeypress {
		return false // delegate.last_keypress == 0 → now >= 0+timeout
	}
	return now.Before(lt.lastKeypress.Add(lt.keyTimeout))
}

// Draw renders the text view and, when this LinkableText is focused and the
// cursor is visible (within the key-timeout window), re-positions the terminal
// hardware cursor at the (x,y) the cursor offset maps to under urwid's
// wrap="space" layout. This mirrors Python LinkableText.render setting
// canvas.cursor = get_cursor_coords(size) (MicronParser.py:982-992).
//
// tview's Application.Draw hides the cursor each frame, so the focused widget
// must re-show it on every draw — the same pattern ReadlineEdit.Draw uses
// (readline.go:152). The (x,y) is computed with CalcCoords (the Go port of
// urwid calc_coords) over the model text wrapped to the inner-rect width; the
// cursor byte offset is converted to a rune (codepoint) offset to match
// Python's _cursor_position semantics.
//
// CAVEAT — best-effort, NOT golden-tested: tmux capture-pane records the cell
// buffer, not the terminal hardware cursor, so capture parity tooling is blind
// to it (see TODO.md Phase 0 cursor task). The vertical scroll offset of the
// underlying tview.TextView is internal and not exposed, so when the page is
// scrolled the caret row is approximate. LinkableText is not yet wired into the
// live page tree (the browser/guide bodies render via guideReader +
// StyledLinesToTviewText into a plain TextView), so this override is currently
// dormant; it becomes active once a Phase 2/3 change hosts a LinkableText as the
// focusable page primitive. The verified, testable deliverable for cursor
// parity is the golden CalcCoords table in cursor-coords_test.go.
func (lt *LinkableText) Draw(screen tcell.Screen) {
	lt.TextView.Draw(screen)
	if !lt.HasFocus() {
		return
	}
	if !lt.CursorVisible(time.Now(), true) {
		return
	}
	text := lt.Text()
	pos := min(max(lt.cursor, 0), len(text))
	// lt.cursor is a byte offset into the concatenated part text; CalcCoords
	// takes a rune (codepoint) offset, matching Python's _cursor_position.
	pos = utf8.RuneCountInString(text[:pos])
	runes := []rune(text)
	if pos > len(runes) {
		pos = len(runes)
	}
	x0, y0, width, _ := lt.GetInnerRect()
	if width < 1 {
		return
	}
	relX, relY := CalcCoords(text, width, pos)
	screen.ShowCursor(x0+relX, y0+relY)
}

// HandleKey processes one keystroke and returns either "" (consumed) or the
// key name to propagate to the parent. now is the keystroke timestamp, used
// for the key-timeout cursor-visibility model. Mirrors LinkableText.keypress
// (MicronParser.py:921-977).
func (lt *LinkableText) HandleKey(key string, now time.Time) string {
	if lt.delegate != nil {
		lt.lastKeypress = now
		lt.hasKeypress = true
	}

	positions := lt.PartPositions()

	switch key {
	case "enter":
		lt.Activate()
		return ""

	case "up":
		lt.cursor = 0
		return "up"

	case "down":
		lt.cursor = 0
		return "down"

	case "right":
		old := lt.cursor
		lt.cursor = findNextPartPos(lt.cursor, positions)
		if lt.cursor == old {
			if lt.inColumns {
				return "right"
			}
			lt.cursor = 0
			return "down"
		}
		return ""

	case "left":
		if lt.cursor > 0 {
			if lt.inColumns {
				return "left"
			}
			lt.cursor = findPrevPartPos(lt.cursor, positions)
			return ""
		}
		if lt.delegate != nil {
			lt.delegate.MicronReleasedFocus()
		}
		return ""

	default:
		return key
	}
}
