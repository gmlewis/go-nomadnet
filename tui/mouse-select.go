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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// selectionTracker implements mouse text selection for the whole TUI — a
// Go-only enhancement (Python nomadnet has no TUI selection support).
//
// Three gestures, all standard terminal behavior:
//
//   - Drag: press anywhere, move, release. The rectangular cell range is
//     highlighted while dragging and its on-screen text is copied to the
//     system clipboard on release ("scan the current TUI layout": the text is
//     read straight from the tcell screen buffer, so it works over every
//     widget — lists, dialogs, the browser, the log, top to bottom).
//   - Double-click: select AND copy the whitespace-delimited word at the cell
//     — e.g. a full LXMF address. If the word is displayed wrapped in angle
//     brackets (`<hash>`, RNS.prettyhexrep style) the COPY drops the brackets
//     so the pasted text is the bare address; the highlight stays verbatim.
//   - Triple-click: select AND copy the entire row.
//
// Plain clicks (press+release without movement) are forwarded unchanged so
// every existing mouse interaction (menu, dialogs, browser links) keeps
// working; drag motions and the selection release are consumed so widgets
// never see them.
//
// Wiring (App.NewApp): tviewApp.SetMouseCapture(tracker.capture) — the
// app-level capture runs before every widget handler — and
// tviewApp.SetAfterDrawFunc(tracker.paintAfter), which re-paints the
// highlight over each frame so widget redraws never lose it.
type selectionTracker struct {
	app *App

	// screen is the live tcell screen, captured from the after-draw hook (and
	// set directly by tests). Extraction and the highlight paint read/write it.
	screen tcell.Screen

	// active is true while a selection is highlighted (drag moved, or a
	// word/line selection).
	active   bool
	dragging bool
	// pressing is true between MouseLeftDown and MouseLeftUp.
	pressing bool
	anchorX  int
	anchorY  int
	endX     int
	endY     int

	// clickCount distinguishes single/double/triple clicks at the same cell.
	clickCount   int
	lastClickX   int
	lastClickY   int
	lastClickAt  time.Time
	doubleClickI time.Duration
}

// newSelectionTracker builds the tracker with the given double-click window.
func newSelectionTracker(app *App, doubleClick time.Duration) *selectionTracker {
	return &selectionTracker{app: app, doubleClickI: doubleClick}
}

// capture is the app-level mouse capture. Returning a nil event consumes the
// event (tview fireMouseActions drops it and triggers a redraw).
func (s *selectionTracker) capture(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if event == nil {
		return event, action
	}
	x, y := event.Position()

	switch action {
	case tview.MouseLeftDown:
		s.markClick(x, y)
		s.pressing = true
		s.dragging = false
		s.anchorX, s.anchorY = x, y
		s.endX, s.endY = x, y
		if s.clickCount >= 3 {
			// Triple-click: select AND copy the entire row.
			s.selectLine(x, y)
			return nil, action
		}
		// Single press: clear any previous selection and let the widget see
		// the press. The selection rect is (re)established on motion or the
		// double-click action.
		s.clearRect()
		return event, action

	case tview.MouseLeftDoubleClick:
		// Double-click: select AND copy the whole word.
		s.selectWord(x, y)
		return nil, action

	case tview.MouseLeftUp:
		if s.pressing && s.dragging {
			// Drag release: copy the rectangular selection; consume the Up so
			// the widget under it does not fire a click.
			s.copySelection()
			s.pressing = false
			return nil, action
		}
		s.pressing = false
		return event, action

	case tview.MouseMove:
		if s.pressing && event.Buttons()&tcell.ButtonPrimary != 0 {
			if x != s.endX || y != s.endY {
				s.endX, s.endY = x, y
				if x != s.anchorX || y != s.anchorY {
					s.dragging = true
					s.active = true // the highlight paints from now on
				}
				if s.dragging {
					// Consume drag motions so widgets never react to them;
					// fireMouseActions triggers a redraw because the event
					// was consumed, and paintAfter re-paints the highlight.
					return nil, action
				}
			}
		}
	}
	return event, action
}

// markClick updates the click counter for double/triple-click detection.
func (s *selectionTracker) markClick(x, y int) {
	now := time.Now()
	if s.lastClickX == x && s.lastClickY == y && now.Sub(s.lastClickAt) <= s.doubleClickI {
		s.clickCount++
	} else {
		s.clickCount = 1
	}
	s.lastClickX, s.lastClickY = x, y
	s.lastClickAt = now
}

// selectWord highlights the whitespace-delimited word at (x, y) and copies it.
func (s *selectionTracker) selectWord(x, y int) {
	x1, x2, ok := s.wordRun(x, y)
	if !ok {
		return
	}
	s.anchorX, s.anchorY = x1, y
	s.endX, s.endY = x2, y
	s.dragging = false
	s.active = true
	s.copySelection()
}

// selectLine highlights the entire row at (x, y) and copies it.
func (s *selectionTracker) selectLine(x, y int) {
	s.anchorX, s.anchorY = 0, y
	s.endX, s.endY = s.width()-1, y
	s.dragging = false
	s.active = true
	s.copySelection()
}

// maxOSCPayload caps the OSC 52 clipboard payload. Terminals and tmux bound
// the escape length; a 64 KiB cap comfortably covers text selections while
// keeping the escape well inside every passthrough limit. Oversized
// selections skip the terminal path (the system-clipboard write still
// covers them).
const maxOSCPayload = 64 * 1024

// copySelection extracts the selected text and writes it to the clipboard.
func (s *selectionTracker) copySelection() {
	if s.app == nil {
		return
	}
	text := s.extractSelection()
	if text == "" {
		return
	}
	normalized := normalizeCopiedWord(text)
	// The system-clipboard write below lands on the machine gonomadnet RUNS
	// on. Over SSH (the glenn-mac-mini-m2 and every other fleet session) that
	// is the remote box's pasteboard — Cmd-V on the machine the user is
	// typing on never sees it (fleet bug #11). Also post OSC 52 THROUGH the
	// terminal: the escape travels app → tmux (set-clipboard external
	// forwards it) → outer terminal, which sets the clipboard of the machine
	// the user actually types on. Running locally both paths write the same
	// clipboard; over SSH OSC 52 is the only one that works.
	s.app.clipboard.WriteText(normalized)
	s.postOSCClipboard(normalized)
}

// postOSCClipboard emits the OSC 52 "set clipboard" escape through tcell's
// own output channel, so the sequence is written under the screen lock and
// interleaved safely with frame updates — a raw fmt.Print would race tcell's
// buffered terminal writes. The live tcell screen is captured from the
// after-draw hook; tests get the simulation screen, whose SetClipboard
// records the payload for assertion.
func (s *selectionTracker) postOSCClipboard(text string) {
	if s.screen == nil || text == "" || len(text) > maxOSCPayload {
		return
	}
	s.screen.SetClipboard([]byte(text))
}

// normalizeCopiedWord drops one surrounding angle-bracket pair when the
// selection is a single line displayed as `<…>` (RNS.prettyhexrep address
// style), so a double-click on a displayed LXMF address pastes the bare
// address. Multi-line selections are returned verbatim.
func normalizeCopiedWord(text string) string {
	if !strings.Contains(text, "\n") && strings.HasPrefix(text, "<") && strings.HasSuffix(text, ">") && len(text) >= 2 {
		return text[1 : len(text)-1]
	}
	return text
}

// rect returns the normalized (inclusive) selection rectangle.
func (s *selectionTracker) rect() (x1, y1, x2, y2 int) {
	x1, x2 = s.anchorX, s.endX
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	y1, y2 = s.anchorY, s.endY
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	return x1, y1, x2, y2
}

// extractSelection reads the selected cells from the screen. Wide runes
// (width 2) are taken once; their continuation columns are skipped, and
// trailing spaces are trimmed per row (urwid-style padding is not copied).
func (s *selectionTracker) extractSelection() string {
	if s.app == nil || s.screen == nil {
		return ""
	}
	x1, y1, x2, y2 := s.rect()
	var rows []string
	for y := y1; y <= y2; y++ {
		var b strings.Builder
		for x := x1; x <= x2; x++ {
			str, _, w := s.screen.Get(x, y)
			if w <= 0 || str == "" {
				continue // wide-rune continuation column
			}
			b.WriteString(str)
			if w > 1 {
				x += w - 1
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	// Drop trailing empty rows (a bottom-anchored drag past the last text).
	for len(rows) > 1 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return strings.Join(rows, "\n")
}

// wordRun expands from (x, y) to the whitespace-delimited run on that row,
// returning the inclusive [x1, x2] columns. ok is false when the cell holds
// nothing (a space).
func (s *selectionTracker) wordRun(x, y int) (int, int, bool) {
	if s.app == nil || s.screen == nil {
		return 0, 0, false
	}
	w, _ := s.screen.Size()
	isWord := func(x int) bool {
		str, _, rw := s.screen.Get(x, y)
		return rw > 0 && str != "" && str != " "
	}
	if !isWord(x) {
		return 0, 0, false
	}
	x1 := x
	for x1 > 0 && isWord(x1-1) {
		x1--
	}
	x2 := x
	for x2 < w-1 && isWord(x2+1) {
		x2++
	}
	return x1, x2, true
}

// width returns the current screen width (0 before the screen exists).
func (s *selectionTracker) width() int {
	if s.screen == nil {
		return 0
	}
	w, _ := s.screen.Size()
	return w
}

// paintAfter re-paints the selection highlight over the frame's widgets so
// widget redraws never lose it. The highlight swaps each cell's
// foreground/background — the classic terminal selection look.
func (s *selectionTracker) paintAfter(screen tcell.Screen) {
	if screen == nil {
		return
	}
	s.screen = screen
	if !s.active {
		return
	}
	x1, y1, x2, y2 := s.rect()
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			str, style, w := screen.Get(x, y)
			if w <= 0 || str == "" {
				continue
			}
			fg, bg, _ := style.Decompose()
			swapped := style.Foreground(bg).Background(fg)
			screen.Put(x, y, str, swapped)
		}
	}
}

// active reports whether a selection is currently highlighted.
func (s *selectionTracker) activeState() bool { return s.active }

// clearRect removes the highlight. The next frame's after-draw paints nothing,
// and tcell's cell diffing restores the underlying widget pixels.
func (s *selectionTracker) clearRect() { s.active = false }

// clearOnKey removes the selection when any keyboard event arrives (standard
// terminal behavior: typing abandons the selection).
func (s *selectionTracker) clearOnKey() {
	if s.active || s.dragging {
		s.active = false
		s.dragging = false
		s.pressing = false
	}
}
