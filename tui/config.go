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
	"os/exec"
	"runtime"

	"github.com/rivo/tview"
)

// ConfigDisplay shows the config-file explainer and an "Open Editor" button,
// matching Python's ConfigDisplay (Config.py:25-48). Python embeds an
// urwid.Terminal running the editor on the NomadNet config (Config.py:57-71);
// the Go port embeds it via the EmbeddedTerminal widget (the tview analogue of
// urwid.Terminal) so the menu bar + footer stay visible while editing.
type ConfigDisplay struct {
	app        *App
	widget     *tview.Flex
	explainer  *centeredText
	configPath string
	editorCmd  string
	openBtn    *UrwidButton

	// pages wraps the explainer so the embedded editor can swap in as the
	// "editor" page (Python's open_editor sets self.widget = LineBox(editor);
	// Config.py:30-35). "main" = the explainer; "editor" = the EmbeddedTerminal.
	pages *tview.Pages

	// editor is the embedded terminal running the external editor while the
	// "editor" page is mounted.
	editor *EmbeddedTerminal

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
	// Python wraps the explainer in `urwid.Text(("body_text", ...), CENTER)`
	// (Config.py:40); body_text is 3-hex #ddd / #222 (ui/TextUI.py:26,80),
	// cube-quantized to #d7d7d7 / #000000.
	cd.explainer = newCenteredText(
		GetThemeColors(app.Theme)["body_text"],
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
	cd.openBtn = NewUrwidButton("Open Editor").SetSelectedFunc(func() { cd.openEditor() })
	buttonRow := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(cd.openBtn, 15, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	// Pile [explainer(7), button(1)] = 8 rows, TOP-filled (blank below).
	pile := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.explainer, 7, 0, false).
		AddItem(buttonRow, 1, 0, true)

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pile, 8, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	cd.pages = tview.NewPages().AddPage("main", cd.widget, true, true)

	return cd
}

// Widget returns the tview primitive for this display (a Pages wrapping the
// explainer so the embedded editor can swap in).
func (cd *ConfigDisplay) Widget() tview.Primitive {
	return cd.pages
}

// openEditor launches the configured editor on the NomadNet config file. If
// OnOpenEditor is set it is called instead (test/app seam). Otherwise the editor
// is embedded in the body via the EmbeddedTerminal widget, matching Python's
// EditorTerminal (Config.py:30-35,57-71): the editor replaces the explainer
// while the menu bar + footer stay visible, and on editor exit the explainer is
// restored. The LineBox has no title (Python's urwid.LineBox(self.editor_term)
// passes no title; Config.py:32).
func (cd *ConfigDisplay) openEditor() {
	if cd.OnOpenEditor != nil {
		cd.OnOpenEditor()
		return
	}
	cd.ShowEditor(cd.editorCmd, cd.configPath)
}

// SetEditorCmd overrides the editor command (the app passes the configured
// [textui] editor resolved via ResolveEditorCmd; tests use the default).
func (cd *ConfigDisplay) SetEditorCmd(cmd string) { cd.editorCmd = cmd }

// ShowEditor embeds an external editor (editorCmd on filePath) inside the
// config display as a full-body page (no title, matching Python's
// urwid.LineBox(self.editor_term); Config.py:32). On editor exit the explainer
// is restored and focus returns to the "Open Editor" button (Python's
// quit_term restores self.parent.config_explainer; Config.py:74-78).
func (cd *ConfigDisplay) ShowEditor(editorCmd, filePath string) {
	if cd.editor != nil {
		cd.closeEditor()
	}
	_, _, w, h := cd.widget.GetRect()
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	cmd := exec.Command(editorCmd, filePath)
	et, err := NewEmbeddedTerminal(cd.app, cmd, w, h, "linux", cd.closeEditor)
	if err != nil {
		return
	}
	et.SetBorder(true)
	cd.editor = et
	cd.pages.AddPage("editor", et, true, true)
	cd.pages.SwitchToPage("editor")
	if cd.app != nil {
		cd.app.SetFocus(et)
	}
}

// closeEditor removes the embedded editor page and restores the explainer +
// focus. It is the onClose callback fired when the editor child exits
// (matching Python's urwid.Terminal 'closed' signal → quit_term).
func (cd *ConfigDisplay) closeEditor() {
	if cd.editor == nil {
		return
	}
	cd.editor.Close()
	cd.editor = nil
	cd.pages.RemovePage("editor")
	cd.pages.SwitchToPage("main")
	if cd.app != nil && cd.openBtn != nil {
		cd.app.SetFocus(cd.openBtn)
	}
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
