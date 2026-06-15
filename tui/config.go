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

// ConfigDisplay shows the configuration file path and an editor button.
type ConfigDisplay struct {
	app    *tview.Application
	widget tview.Primitive
}

// NewConfigDisplay creates a new config display.
func NewConfigDisplay(app *tview.Application, configPath string) *ConfigDisplay {
	cd := &ConfigDisplay{app: app}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("Configuration")

	// Instructions
	info := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(fmt.Sprintf(
			"\nTo change the configuration, edit the config file located at:\n\n%s\n\nRestart Nomad Network for changes to take effect.\n",
			configPath,
		))

	// Open editor button
	editorBtn := tview.NewButton("Open Editor")
	editorBtn.SetLabelColor(tcell.NewHexColor(0x00a533))
	editorBtn.SetBackgroundColor(tcell.ColorDefault)
	editorBtn.SetSelectedFunc(func() {
		// In a real implementation, this would open an editor
		// For now, show a message
		cd.showMessage(fmt.Sprintf("Edit: %s", configPath))
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(info, 0, 1, false).
		AddItem(editorBtn, 1, 0, true)
	layout.SetBorder(true)

	cd.widget = layout
	return cd
}

// Widget returns the tview primitive for this display.
func (cd *ConfigDisplay) Widget() tview.Primitive {
	return cd.widget
}

// showMessage displays a temporary message.
func (cd *ConfigDisplay) showMessage(msg string) {
	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			cd.app.SetRoot(cd.widget, true)
		})
	cd.app.SetRoot(modal, true)
}
