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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// IntroDisplay shows the splash/intro screen.
type IntroDisplay struct {
	widget tview.Primitive
}

// NewIntroDisplay creates a new intro display with title and version.
func NewIntroDisplay(title string, version string) *IntroDisplay {
	id := &IntroDisplay{}

	titleView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0x00a533)).
		SetText(fmt.Sprintf("[::b]%s[-]", title))

	versionView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(fmt.Sprintf("Version %s", version))

	startingView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("-= Starting =-")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titleView, 0, 1, false).
		AddItem(versionView, 1, 0, false).
		AddItem(startingView, 1, 0, false)
	layout.SetBorder(true)

	id.widget = layout
	return id
}

// Widget returns the tview primitive for this display.
func (id *IntroDisplay) Widget() tview.Primitive {
	return id.widget
}
