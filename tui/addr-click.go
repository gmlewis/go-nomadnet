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
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// addrRegionPrefix starts the tview region tag a clickable address row wraps an
// address in; the tag is the prefix plus the address's index on the row. Tags
// only have to be unique within their own view.
const addrRegionPrefix = "addr"

// addrHexLen is the hex length of a Reticulum hash: 16 bytes.
const addrHexLen = 32

// addrRun is one address found on a row: the bare lowercase hex and the byte
// offsets of the span to make clickable, which for RNS.prettyhexrep's "<hex>"
// form include the delimiters, so a click on the "<" works like a click on the
// hex itself.
type addrRun struct {
	addr       string
	start, end int
}

// addrSpan locates the first address on a rendered address row. See addrSpans
// for what counts as an address.
func addrSpan(text string) (addr string, start, end int, ok bool) {
	run, found := addrSpanAt(text, 0)
	return run.addr, run.start, run.end, found
}

// addrSpans returns every address on a row, in order — a row may carry several
// (the Local Peer dialog shows the LXMF address and the identity hash on
// consecutive lines).
func addrSpans(text string) []addrRun {
	var runs []addrRun
	for from := 0; from < len(text); {
		run, ok := addrSpanAt(text, from)
		if !ok {
			break
		}
		runs = append(runs, run)
		from = run.end
	}
	return runs
}

// addrSpanAt locates the first address at or after byte offset from: either
// RNS.prettyhexrep's "<hex>" form or a bare 32-hex run standing alone between
// spaces (or at a line's edges).
//
// Only a full 16-byte hash qualifies. A shorter run is a truncated label — the
// "<ef289bf8…>" in a sync notice, say — and a QR code of it could not be scanned
// as an address, so it is left alone. The bare form additionally requires space
// (or a line edge) on both sides, so a color tag's hex, a colon-separated browse
// URL, or any other hex run inside a longer word is never mistaken for a lone
// address.
func addrSpanAt(text string, from int) (addrRun, bool) {
	for i := max(from, 0); i < len(text); i++ {
		if text[i] == '<' && i+addrHexLen+1 < len(text) && text[i+addrHexLen+1] == '>' {
			if hex := text[i+1 : i+addrHexLen+1]; isHexRun(hex) {
				return addrRun{strings.ToLower(hex), i, i + addrHexLen + 2}, true
			}
		}
		if !isHexDigitAnyCase(text[i]) || i+addrHexLen > len(text) {
			continue
		}
		run := text[i : i+addrHexLen]
		if !isHexRun(run) {
			continue
		}
		if i > 0 && text[i-1] != ' ' {
			continue
		}
		if i+addrHexLen < len(text) && text[i+addrHexLen] != ' ' && text[i+addrHexLen] != '\n' {
			continue
		}
		return addrRun{strings.ToLower(run), i, i + addrHexLen}, true
	}
	return addrRun{}, false
}

// isHexRun reports whether every byte of s is a hex digit of either case.
func isHexRun(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHexDigitAnyCase(s[i]) {
			return false
		}
	}
	return true
}

// isHexDigitAnyCase reports whether b is one of the sixteen hex digits in either
// case, folding 'A'-'F' onto isHexDigit's lower-case set.
func isHexDigitAnyCase(b byte) bool {
	if b >= 'A' && b <= 'F' {
		b += 'a' - 'A'
	}
	return isHexDigit(b)
}

// ShowAddressActions copies addr — the bare hex, without the angle brackets the
// panels display it in — to the clipboard and opens the modal that shows the QR
// code the address encodes, the address in text beneath it, and the
// confirmation that it was copied. It is what a click on a rendered address
// does.
//
// This is a Go-only enhancement: Python nomadnet has a QR dialog for its own
// addresses, but no address in the UI is clickable and nothing but the mouse
// selection copies one. The QR carries the bare 32-hex address, which is exactly
// what an appliance's "< Scan >" button accepts (normalizeScannedAddress), so
// the code can be scanned straight into another device's address field.
func (a *App) ShowAddressActions(title, addr string) {
	a.showAddressActions(title, addr, nil)
}

// showAddressActions is ShowAddressActions with the dialog's dismissal hook, so
// a caller that displaced the keyboard focus to get here can hand it back.
func (a *App) showAddressActions(title, addr string, onDismiss func()) {
	if a == nil || a.Dialogs == nil {
		return
	}
	bare := normalizeCopiedWord(strings.TrimSpace(addr))
	if bare == "" {
		return
	}
	// The clipboard is written first: the modal then confirms something that has
	// already happened, so a click can never look like it copied when it did not.
	if a.clipboard != nil {
		a.clipboard.WriteText(bare)
	}
	if title == "" {
		title = "Address"
	}
	code, err := GenerateQRASCII(bare)
	if err != nil {
		code = "" // no code, but the address text below it still carries the address
	}
	body, rows := qrDialogBodyWithNote(code, bare, "Copied to clipboard")
	a.Dialogs.ShowDialog(title, body, qrDialogWidth, rows+2, onDismiss)
}

// NewAddrRow builds a left-aligned address row — the shape the panels' textRow
// helper produces — already wired so a click on the address it carries copies
// the address and opens the QR modal.
func NewAddrRow(app *App, title, text string) *tview.TextView {
	tv := tview.NewTextView().
		SetDynamicColors(false).
		SetTextAlign(tview.AlignLeft)
	MakeAddrClickable(app, title, tv, text)
	return tv
}

// SetAddrRowText sets the text of a row wired by MakeAddrClickable, wrapping the
// addresses in it as clickable regions. Use it — instead of SetText — wherever a
// wired row's text changes, so a refreshed row keeps its clickable addresses.
func SetAddrRowText(tv *tview.TextView, text string) {
	if tv == nil {
		return
	}
	tv.SetText(tagAddrText(text))
}

// tagAddrText returns text with a tview region around every address on it. Text
// with no address is returned unchanged, so a row that shows none renders
// exactly as it did before it was wired.
func tagAddrText(text string) string {
	runs := addrSpans(text)
	if len(runs) == 0 {
		return text
	}
	var b strings.Builder
	prev := 0
	for i, run := range runs {
		b.WriteString(text[prev:run.start])
		fmt.Fprintf(&b, `["%v%v"]`, addrRegionPrefix, i)
		b.WriteString(text[run.start:run.end])
		b.WriteString(`[""]`)
		prev = run.end
	}
	b.WriteString(text[prev:])
	return b.String()
}

// MakeAddrClickable turns a row's TextView into a clickable address row: every
// address found in text is wrapped in a tview region, and a click on one copies
// the bare address and opens the QR modal. text is the row's whole displayed
// text, label included.
//
// Wire a row ONCE, from its constructor; change its text afterwards with
// SetAddrRowText. Wiring is not idempotent: each call composes another layer
// around whatever mouse capture the row already had, and a row that re-wires on
// every refresh would nest one layer per refresh.
//
// A row whose text holds no address is rendered exactly as before.
//
// The hit test is tview's own region resolution, so it follows the text through
// wrapping and scrolling. Resolving a region on click also focuses the TextView
// (TextView.MouseHandler calls setFocus on the press), which would leave a
// non-selectable text row holding the keyboard and swallow the pane's Up/Down
// afterwards; the focus that had it is remembered and handed back when the modal
// closes.
func MakeAddrClickable(app *App, title string, tv *tview.TextView, text string) {
	if tv == nil {
		return
	}
	// The row may already have a capture — the Hub Info view's wheel multiplier,
	// for instance — and that one keeps its events: this wiring only looks at the
	// press and then passes everything along.
	prior := tv.GetMouseCapture()
	var before tview.Primitive
	tv.SetRegions(true)
	tv.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftDown && app != nil && tv.InRect(event.Position()) {
			before = app.GetFocus()
		}
		if prior != nil {
			return prior(action, event)
		}
		return action, event
	})
	tv.SetHighlightedFunc(func(added, _, _ []string) {
		dispatchAddrRegion(app, tv, title, before, added)
	})
	SetAddrRowText(tv, text)
}

// dispatchAddrRegion runs the address click for a highlighted region. It is
// shared with views that install their own highlight handler (the hub-info
// panel's room links), because tview keeps only one handler per TextView and
// offers no getter to compose with.
func dispatchAddrRegion(app *App, tv *tview.TextView, title string, before tview.Primitive, added []string) {
	if len(added) == 0 || app == nil || !strings.HasPrefix(added[0], addrRegionPrefix) {
		return
	}
	// tview inverts a highlighted region on the next draw; clear it before
	// the modal covers the row, the way the browser clears a clicked link.
	tv.Highlight()
	idx, ok := addrRegionIndex(added[0])
	// Read the addresses back off the row rather than closing over them, so a
	// row whose text was replaced still copies what it displays. GetText(true)
	// strips the region tags again.
	showing := addrSpans(tv.GetText(true))
	if !ok || idx >= len(showing) {
		return
	}
	app.showAddressActions(title, showing[idx].addr, func() {
		if before != nil {
			app.SetFocus(before)
		}
	})
}

// addrRegionIndex parses the address's index out of an address region tag.
func addrRegionIndex(id string) (int, bool) {
	digits := strings.TrimPrefix(id, addrRegionPrefix)
	if digits == "" {
		return 0, false
	}
	idx, err := strconv.Atoi(digits)
	if err != nil || idx < 0 {
		return 0, false
	}
	return idx, true
}

// mouseCaptureFunc is the mouse-capture hook tview's Box installs. It is named
// here because SetMouseCapture returns the concrete *Box, so a method value
// cannot be passed where this function type is wanted.
type mouseCaptureFunc func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse)

// MakeCenterAddrClickable turns a centered address line into a clickable one.
// urwidCenterText draws its own urwid "space" wrap — its ceil-left centering and
// its wrapped row count are parity-tested — so the hit test replays that wrap
// instead of handing the text to tview.
//
// The click is consumed before the widget's default handler runs, so the line
// never takes the keyboard and nothing has to be handed back afterwards.
func MakeCenterAddrClickable(app *App, title string, c *urwidCenterText) {
	if c == nil {
		return
	}
	wireWrapAddrClick(app, title, func(cap mouseCaptureFunc) { c.SetMouseCapture(cap) }, c.addrAt)
}

// MakeLeftAddrClickable is MakeCenterAddrClickable for a left-aligned urwid text
// row (urwidLeftText), which the dialogs use for their read-only address lines.
func MakeLeftAddrClickable(app *App, title string, c *urwidLeftText) {
	if c == nil {
		return
	}
	wireWrapAddrClick(app, title, func(cap mouseCaptureFunc) { c.SetMouseCapture(cap) }, c.addrAt)
}

// wireWrapAddrClick installs the click behavior on a widget that draws its own
// urwid wrap: addrAt says whether a screen position is on the address the widget
// displays, and a hit is consumed so the widget's default handler never takes
// focus.
func wireWrapAddrClick(app *App, title string, setCapture func(mouseCaptureFunc), addrAt func(x, y int) (string, bool)) {
	setCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if app == nil || (action != tview.MouseLeftDown && action != tview.MouseLeftClick) {
			return action, event
		}
		addr, ok := addrAt(event.Position())
		if !ok {
			return action, event
		}
		if action == tview.MouseLeftClick {
			app.ShowAddressActions(title, addr)
		}
		return tview.MouseConsumed, nil
	})
}

// addrAt reports the address whose cells contain the given screen position, or
// "" when the position is off the address. It maps the position back onto the
// wrapped rows the way the widget draws them: the address's rune offsets are
// inverted through CalcCoords (the port of urwid's calc_coords), so a click on a
// continuation row resolves like a click on the first row.
//
// centered selects the per-row left inset the widget draws with: urwid's
// ceil-left centering for urwidCenterText, nothing for urwidLeftText.
func addrAt(text string, rectX, rectY, w, h, x, y int, centered bool) (string, bool) {
	addr, startByte, endByte, ok := addrSpan(text)
	if !ok {
		return "", false
	}
	if w <= 0 || h <= 0 {
		return "", false
	}
	lines := urwidSpaceWrap(text, w)
	relY := y - rectY
	if relY < 0 || relY >= len(lines) {
		return "", false
	}
	left := 0
	if centered {
		// The row's content starts past the centering pad — the same pad Draw
		// computes for this row, with the odd leftover column on the left.
		left = max((w-min(stringWidth(lines[relY]), w)+1)/2, 0)
	}
	relX := x - rectX - left
	if relX < 0 {
		return "", false
	}
	start := utf8.RuneCountInString(text[:startByte])
	end := utf8.RuneCountInString(text[:endByte])
	for pos := start; pos < end; pos++ {
		if cx, cy := CalcCoords(text, w, pos); cy == relY && cx == relX {
			return addr, true
		}
	}
	return "", false
}

// addrAt reports the address cells the given screen position falls on, in the
// centered urwid text row.
func (c *urwidCenterText) addrAt(x, y int) (string, bool) {
	rectX, rectY, w, h := c.GetRect()
	return addrAt(c.text, rectX, rectY, w, h, x, y, true)
}

// addrAt reports the address cells the given screen position falls on, in the
// left-aligned urwid text row.
func (c *urwidLeftText) addrAt(x, y int) (string, bool) {
	rectX, rectY, w, h := c.GetRect()
	return addrAt(c.text, rectX, rectY, w, h, x, y, false)
}
