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
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfigDisplay shows the configuration file with an in-app editor.
type ConfigDisplay struct {
	app        *App
	widget     tview.Primitive
	editor     *tview.TextArea
	configPath string
}

// NewConfigDisplay creates a new config display with an editable text area.
// Matches Python's ConfigDisplay at Config.py:25 with in-app editor.
func NewConfigDisplay(app *App, configPath string) *ConfigDisplay {
	cd := &ConfigDisplay{app: app, configPath: configPath}

	// Title
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]Configuration[-]")

	// Load config file content
	content := ""
	data, err := os.ReadFile(configPath)
	if err == nil {
		content = string(data)
	} else {
		content = fmt.Sprintf("# Error reading config: %v\n# File: %s", err, configPath)
	}

	// Editor
	cd.editor = tview.NewTextArea()
	cd.editor.SetText(content, true)
	cd.editor.SetBackgroundColor(tcell.NewHexColor(0x1a1a1a))
	cd.editor.SetTextStyle(tcell.StyleDefault.Foreground(tcell.NewHexColor(0xbbbbbb)))

	// Status bar
	statusBar := tview.NewTextView()
	statusBar.SetDynamicColors(true)
	statusBar.SetTextColor(tcell.NewHexColor(0x999999))
	statusBar.SetText(fmt.Sprintf("[yellow]Ctrl-S[-] Save  [yellow]Ctrl-Q[-] Back  [gray]%s[-]", configPath))

	// Save button
	saveBtn := tview.NewButton("[Save]")
	saveBtn.SetBackgroundColor(tcell.NewHexColor(0x444444))
	saveBtn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	saveBtn.SetSelectedFunc(func() {
		cd.saveConfig()
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(title, 1, 0, false)
	layout.AddItem(cd.editor, 0, 1, true)
	layout.AddItem(statusBar, 1, 0, false)
	layout.SetBorder(true)

	cd.widget = layout
	return cd
}

// Widget returns the tview primitive.
func (cd *ConfigDisplay) Widget() tview.Primitive {
	return cd.widget
}

// saveConfig writes the editor content to the config file.
func (cd *ConfigDisplay) saveConfig() {
	content := cd.editor.GetText()
	err := os.WriteFile(cd.configPath, []byte(content), 0o644)
	if err != nil {
		cd.showMessage(fmt.Sprintf("Error saving: %v", err))
		return
	}
	cd.showMessage("Config saved. Restart Nomad Network for changes to take effect.")
}

// showMessage displays a temporary message.
func (cd *ConfigDisplay) showMessage(msg string) {
	modal := tview.NewModal()
	modal.SetText(msg)
	modal.AddButtons([]string{"OK"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		cd.app.Application.SetRoot(cd.widget, true)
	})
	cd.app.Application.SetRoot(modal, true)
}
