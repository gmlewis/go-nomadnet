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
// Config display: ports Python's Config.py. The page shows an explainer (where
// the config file lives + "restart" notice) and an "Open Editor" button that
// launches $EDITOR on the config path (nano on Darwin when the editor is the
// "editor" alias), mirroring Python's open_editor → EditorTerminal.

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfigDisplay shows the config-file explainer and an "Open Editor" button,
// matching Python's ConfigDisplay (Config.py:25-48). Python embeds an
// urwid.Terminal running the editor; tview has no embedded terminal widget, so
// the button launches $EDITOR via Application.Suspend (screen paused, editor
// runs on the real terminal, then the TUI resumes) — same user-visible effect.
type ConfigDisplay struct {
	app        *App
	widget     *tview.Flex
	explainer  *tview.TextView
	configPath string
	editorCmd  string

	// OnOpenEditor, if set, is invoked by openEditor instead of launching the
	// editor. The app can use it to customize launch; tests use it to avoid
	// spawning a real process.
	OnOpenEditor func()
}

// NewConfigDisplay creates a config display centered on the explainer + button.
func NewConfigDisplay(app *App, configPath string) *ConfigDisplay {
	cd := &ConfigDisplay{
		app:        app,
		configPath: configPath,
		editorCmd:  resolveEditorCmdDefault("editor"),
	}

	cd.explainer = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(fmt.Sprintf(
			"\nTo change the configuration, edit the config file located at:\n\n%s\n\n"+
				"Restart Nomad Network for changes to take effect\n",
			configPath,
		))

	openBtn := tview.NewButton(" Open Editor ")
	openBtn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	openBtn.SetBackgroundColor(tcell.NewHexColor(0x333333))
	openBtn.SetSelectedFunc(func() { cd.openEditor() })

	// Center the pile vertically: a top spacer (weight 1), the explainer + button
	// (fixed heights), and a bottom spacer (weight 1). Python wraps the pile in
	// a urwid.Filler (vertical center). No outer border (Python's config_explainer
	// is a bare Filler, no LineBox).
	pile := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.explainer, 5, 0, false).
		AddItem(openBtn, 1, 0, false)

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(pile, 7, 0, false).
		AddItem(tview.NewBox(), 0, 1, false)

	return cd
}

// Widget returns the tview primitive for this display.
func (cd *ConfigDisplay) Widget() tview.Primitive {
	return cd.widget
}

// openEditor launches the configured editor on the config file. If OnOpenEditor
// is set it is called instead (test/app seam). Otherwise the editor is run with
// the screen suspended via Application.Suspend so it owns the real terminal,
// matching Python's EditorTerminal (Config.py:50-71).
func (cd *ConfigDisplay) openEditor() {
	if cd.OnOpenEditor != nil {
		cd.OnOpenEditor()
		return
	}
	if cd.app == nil || cd.app.Application == nil {
		return
	}
	editor := cd.editorCmd
	configPath := cd.configPath
	cd.app.Application.Suspend(func() {
		cmd := exec.Command(editor, configPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	})
}

// resolveEditorCmdDefault resolves the editor command the way Python's
// EditorTerminal does (Config.py:60-68): the configured editor is used as-is,
// except on Darwin the unavailable "editor" alias is replaced with "nano".
func resolveEditorCmdDefault(editor string) string {
	if runtime.GOOS == "darwin" && editor == "editor" {
		return "nano"
	}
	return editor
}
