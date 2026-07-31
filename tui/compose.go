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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ComposeDisplay provides a message compose area.
type ComposeDisplay struct {
	app    *App
	widget tview.Primitive
	editor *ReadlineEdit
	title  *ReadlineEdit
}

// NewComposeDisplay creates a new compose display.
func NewComposeDisplay(app *App) *ComposeDisplay {
	cd := &ComposeDisplay{app: app}

	// Title field
	cd.title = NewReadlineEdit(app.killRing, "To: ", "recipient")
	cd.title.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cd.title.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Message editor
	cd.editor = NewReadlineEdit(app.killRing, "", "Type your message...")
	cd.editor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cd.editor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Layout
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.title, 1, 0, false).
		AddItem(cd.editor, 0, 1, true)

	cd.widget = layout
	return cd
}

// Widget returns the tview primitive for this display.
func (cd *ComposeDisplay) Widget() tview.Primitive {
	return cd.widget
}

// GetText returns the composed message text.
func (cd *ComposeDisplay) GetText() string {
	return cd.editor.GetText()
}

// GetTitle returns the message title/recipient.
func (cd *ComposeDisplay) GetTitle() string {
	return cd.title.GetText()
}

// Clear clears both fields.
func (cd *ComposeDisplay) Clear() {
	cd.title.SetText("")
	cd.editor.SetText("")
}
