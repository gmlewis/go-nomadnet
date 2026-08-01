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

// Package tui implements the NomadNet terminal user interface.
//
// Extras display: ports Python's Extras.py IntroDisplay — a 1-second splash
// showing the program name as urwid BigText (HalfBlock5x4Font), the version, a
// divider, and "-= Starting =-", all centered (urwid.Filler). Shown for
// intro_time seconds before the main display (TextUI.py:223-232).

package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// IntroDisplay shows the splash/intro screen, matching Python's
// IntroDisplay (Extras.py:3-19).
type IntroDisplay struct {
	widget       tview.Primitive
	bigView      *tview.TextView
	versionView  *tview.TextView
	startingView *tview.TextView
}

// NewIntroDisplay creates a new intro display with the title rendered as
// half-block big text and the given version string.
func NewIntroDisplay(title string, version string) *IntroDisplay {
	id := &IntroDisplay{}

	// BigText(title, HalfBlock5x4Font). The untrimmed render keeps every row
	// the same width so AlignCenter aligns the glyphs consistently (trimming
	// trailing spaces per row would shift narrower rows left). intro_title is
	// not in nomadnet's palette, so urwid renders it in the default color.
	id.bigView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(strings.Join(halfBlock5x4Render(title), "\n"))

	id.versionView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf("Version %s", version))

	id.startingView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("-= Starting =- ")

	// Pile: big text (font height 4) + blank + version + divider + starting.
	// Matches Extras.py:11-17 (BigText, Text(version), Divider, Text(starting)).
	pile := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(id.bigView, 4, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(id.versionView, 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(id.startingView, 1, 0, false)

	// urwid.Filler centers the pile vertically; tview equivalent is a row Flex
	// with weight-1 spacers above and below. No border (Extras.py:19 Filler).
	id.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(pile, 8, 0, false).
		AddItem(tview.NewBox(), 0, 1, false)

	return id
}

// Widget returns the tview primitive for this display.
func (id *IntroDisplay) Widget() tview.Primitive {
	return id.widget
}
