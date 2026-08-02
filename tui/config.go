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
	explainer  *centeredText
	configPath string
	editorCmd  string

	// OnOpenEditor, if set, is invoked by openEditor instead of launching the
	// editor. The app can use it to customize launch; tests use it to avoid
	// spawning a real process.
	OnOpenEditor func()
}

// NewConfigDisplay builds the config page: a TOP-filled Pile (Python wraps the
// pile in `urwid.Filler(pile, urwid.TOP)`, Config.py:46) of a 7-line centered
// explainer (`urwid.Text(("body_text", …), align=CENTER)`, Config.py:38-44) and
// a centered `< Open Editor >` button (`urwid.Padding(urwid.Button("Open
// Editor"), width=15, align=CENTER)`, Config.py:45). No outer border — Python's
// config_explainer is a bare Filler.
//
// urwid.Text(align=CENTER) is CEIL-left (extra column to the LEFT on odd slack)
// while urwid.Padding(align=CENTER) is FLOOR-left, so the explainer uses the
// ceil-left `centeredText` primitive and the button row uses a Flex of
// [spacer, button(15), spacer] (tview's leftover-to-LAST gives left=floor,
// right=ceil), matching Python's leading-space counts exactly.
func NewConfigDisplay(app *App, configPath string) *ConfigDisplay {
	cd := &ConfigDisplay{
		app:        app,
		configPath: configPath,
		editorCmd:  ResolveEditorCmd("editor"),
	}

	// Explainer text (Config.py:38-44): leading/trailing blank + the
	// blank-separated body lines, all body_text colored, ceil-left centered.
	cd.explainer = newCenteredText(
		tcell.NewHexColor(0xbbbbbb),
		"",
		"To change the configuration, edit the config file located at:",
		"",
		configPath,
		"",
		"Restart Nomad Network for changes to take effect",
		"",
	)

	// "Open Editor" as a flat urwid "< Open Editor >" button (15 wide), then
	// floor-left-center it in a Flex row (Padding CENTER is floor-left).
	openBtn := NewUrwidButton("Open Editor").SetSelectedFunc(func() { cd.openEditor() })
	buttonRow := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(openBtn, 15, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	// Pile [explainer(7), button(1)] = 8 rows, TOP-filled (blank below).
	pile := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.explainer, 7, 0, false).
		AddItem(buttonRow, 1, 0, true)

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pile, 8, 0, true).
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

// ResolveEditorCmd resolves the editor command the way Python's EditorTerminal
// does (Config.py:60-68, and Interfaces.py open_config_editor:3163-3167): the
// configured editor is used as-is, except on Darwin the unavailable "editor"
// alias is replaced with "nano". Shared by the Config page's "Open Editor"
// button and the Interfaces page's C-w "Open Text Editor" action.
func ResolveEditorCmd(editor string) string {
	if runtime.GOOS == "darwin" && editor == "editor" {
		return "nano"
	}
	return editor
}
