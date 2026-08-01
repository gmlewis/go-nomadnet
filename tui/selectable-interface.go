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
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// InterfaceItemHeight is the fixed height of one SelectableInterfaceItem box:
// top border, title, status, type, divider, TX/RX, bottom border = 7 rows.
const InterfaceItemHeight = 7

// SelectableInterfaceItem represents a selectable interface entry in
// the interfaces list. Matches Python's SelectableInterfaceItem at
// Interfaces.py:1125. The embedded *tview.Box lets it render as a rounded
// bordered box; focused controls the ●/○ selection glyph.
type SelectableInterfaceItem struct {
	*tview.Box
	Name        string
	Icon        string
	IfaceType   string
	IsConnected bool
	IsEnabled   bool
	TX          int64
	RX          int64
	IfaceOpts   any
	focused     bool
	OnSelect    func(name string)
	OnActivate  func()
}

// NewSelectableInterfaceItem builds a selectable interface row box.
func NewSelectableInterfaceItem(name, ifaceType string, connected, enabled bool, tx, rx int64, icon string) *SelectableInterfaceItem {
	return &SelectableInterfaceItem{
		Box:         tview.NewBox(),
		Name:        name,
		IfaceType:   ifaceType,
		IsConnected: connected,
		IsEnabled:   enabled,
		TX:          tx,
		RX:          rx,
		Icon:        icon,
	}
}

// SetFocused sets whether this item shows the focused selection glyph.
func (s *SelectableInterfaceItem) SetFocused(f bool) { s.focused = f }

// Focused reports the focus state.
func (s *SelectableInterfaceItem) Focused() bool { return s.focused }

// StatusText returns the enabled/disabled status string.
func (s *SelectableInterfaceItem) StatusText() string {
	if s.IsEnabled {
		return "Enabled"
	}
	return "Disabled"
}

// ConnectedText returns the connected/disconnected status string.
func (s *SelectableInterfaceItem) ConnectedText() string {
	if s.IsConnected {
		return "Connected"
	}
	return "Disconnected"
}

// TitleText returns the display title with icon and name.
func (s *SelectableInterfaceItem) TitleText() string {
	return fmt.Sprintf("%v  %v", s.Icon, s.Name)
}

// TXText returns a formatted string of bytes sent.
func (s *SelectableInterfaceItem) TXText() string {
	return FormatBytes(float64(s.TX))
}

// RXText returns a formatted string of bytes received.
func (s *SelectableInterfaceItem) RXText() string {
	return FormatBytes(float64(s.RX))
}

// UpdateStats updates the TX/RX byte counters.
func (s *SelectableInterfaceItem) UpdateStats(tx, rx int64) {
	s.TX = tx
	s.RX = rx
}

// Draw renders the rounded box and its five content rows, mirroring Python's
// SelectableInterfaceItem layout (Interfaces.py:1125-1224): a rounded border
// with 2-col padding, then title (selection glyph + icon + name), status
// (Enabled/Disabled | Connected/Disconnected), type, a dashed divider, and
// TX/RX byte counts.
//
// When the allocated height is less than InterfaceItemHeight the box is drawn
// PARTIAL — only the top rows that fit, with no bottom border — matching urwid's
// ListBox, which renders a bottom-clipped last visible item (the off-screen rows
// including the bottom border are simply not drawn).
func (s *SelectableInterfaceItem) Draw(screen tcell.Screen) {
	if s.Box == nil {
		s.Box = tview.NewBox()
	}
	s.Box.DrawForSubclass(screen, s)
	x, y, w, h := s.GetRect()
	if w < 4 || h < 1 {
		return
	}
	style := tcell.StyleDefault
	full := h >= InterfaceItemHeight

	// Top border (row 0), always drawn.
	screen.SetContent(x, y, BorderTopLeftRounded, nil, style)
	screen.SetContent(x+w-1, y, BorderTopRightRounded, nil, style)
	for i := 1; i < w-1; i++ {
		screen.SetContent(x+i, y, BorderHorizontal, nil, style)
	}

	// Content area: border(1) + pad(2) on each side.
	cx := x + 3
	cw := w - 6

	sel := "○"
	if s.focused {
		sel = "●"
	}
	contentRows := []string{
		fmt.Sprintf("%-4s%s  %s", sel, s.Icon, s.Name),
		fmt.Sprintf("%-10s%-10s | %s", "Status: ", s.StatusText(), s.ConnectedText()),
		fmt.Sprintf("%-10s%s", "Type:", s.IfaceType),
		strings.Repeat("-", cw),
		fmt.Sprintf("%-10s%-15s%-10s%s", "TX:", s.TXText(), "RX:", s.RXText()),
	}

	// Render rows 1..h-1. For a full box the last row is the bottom border; for
	// a partial box every row below the top border is an interior content row
	// flanked by verticals (the bottom border is off-screen / clipped).
	lastContentRow := h - 1 // index of the last row to fill
	if full {
		lastContentRow = h - 2 // reserve the last row for the bottom border
	}
	for r := 1; r <= lastContentRow; r++ {
		screen.SetContent(x, y+r, BorderVertical, nil, style)
		screen.SetContent(x+w-1, y+r, BorderVertical, nil, style)
		if r-1 < len(contentRows) {
			s.printRow(screen, cx, y+r, cw, contentRows[r-1], style)
		}
	}

	// Bottom border, only for a full (unclipped) box.
	if full {
		screen.SetContent(x, y+h-1, BorderBottomLeftRounded, nil, style)
		screen.SetContent(x+w-1, y+h-1, BorderBottomRightRounded, nil, style)
		for i := 1; i < w-1; i++ {
			screen.SetContent(x+i, y+h-1, BorderHorizontal, nil, style)
		}
	}
}

// printRow writes s left-aligned at (x,y) padded to width w with spaces,
// using runewidth so wide glyphs (e.g. the interface icon emoji) advance the
// correct number of columns, matching urwid's left-aligned fill.
func (s *SelectableInterfaceItem) printRow(screen tcell.Screen, x, y, w int, str string, style tcell.Style) {
	col := 0
	for _, r := range str {
		if col >= w {
			break
		}
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if col+rw > w {
			break
		}
		screen.SetContent(x+col, y, r, nil, style)
		col += rw
	}
	for col < w {
		screen.SetContent(x+col, y, ' ', nil, style)
		col++
	}
}

// InputHandler lets the item participate in focus; activation is handled by
// the parent list, so this is a no-op delegate.
func (s *SelectableInterfaceItem) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return s.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {})
}

// InterfaceItemRowText renders an item's five content rows (without the
// rounded border) as plain text, each left-aligned to the content width
// derived from the full box width w. Used for golden comparison against
// Python's SelectableInterfaceItem.
func InterfaceItemRowText(s *SelectableInterfaceItem, w int) []string {
	cw := w - 6
	sel := "○"
	if s.focused {
		sel = "●"
	}
	rows := []string{
		fmt.Sprintf("%-4s%s  %s", sel, s.Icon, s.Name),
		fmt.Sprintf("%-10s%-10s | %s", "Status: ", s.StatusText(), s.ConnectedText()),
		fmt.Sprintf("%-10s%s", "Type:", s.IfaceType),
		"",
		fmt.Sprintf("%-10s%-15s%-10s%s", "TX:", s.TXText(), "RX:", s.RXText()),
	}
	for i, r := range rows {
		if i == 3 {
			rows[i] = strings.Repeat("-", cw)
		} else {
			rows[i] = padToWidth(r, ' ', cw)
		}
	}
	return rows
}

// padToWidth left-aligns s and pads it to display width w with pad, using
// runewidth so wide glyphs are accounted for.
func padToWidth(s string, pad rune, w int) string {
	dw := runewidth.StringWidth(s)
	if dw >= w {
		return runewidth.Truncate(s, w, "")
	}
	return s + strings.Repeat(string(pad), w-dw)
}
