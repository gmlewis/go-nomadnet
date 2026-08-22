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
	"runtime"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestConfigDisplayExplainerText asserts the Config page shows Python's
// explainer text (Config.py:38-44) — "To change the configuration, edit the
// config file located at:" + the config path + "Restart Nomad Network for
// changes to take effect" — rather than an in-app editor.
func TestConfigDisplayExplainerText(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConfigDisplay(app, "/home/user/.nomadnetwork/config")

	text := cd.explainer.GetText()
	for _, want := range []string{
		"To change the configuration",
		"/home/user/.nomadnetwork/config",
		"Restart Nomad Network for changes to take effect",
	} {
		if !containsStr(text, want) {
			t.Errorf("explainer missing %q; got: %q", want, text)
		}
	}
}

// TestConfigDisplayOpenEditor asserts the "Open Editor" action is dispatchable
// via openEditor and routes through the OnOpenEditor seam (so the app can launch
// $EDITOR via Application.Suspend), matching Python's open_editor →
// EditorTerminal (Config.py:30-37).
func TestConfigDisplayOpenEditor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConfigDisplay(app, "/test/config")

	called := false
	cd.OnOpenEditor = func() { called = true }

	cd.openEditor()
	if !called {
		t.Error("openEditor() did not invoke OnOpenEditor")
	}
}

// TestResolveEditorCmdDarwin asserts that on Darwin the default "editor" alias
// is replaced with "nano", matching Python's EditorTerminal Darwin fallback
// (Config.py:65-68). A non-Darwin default stays "editor".
func TestResolveEditorCmdDefault(t *testing.T) {
	t.Parallel()

	got := ResolveEditorCmd("editor")
	switch runtime.GOOS {
	case "darwin":
		if got != "nano" {
			t.Errorf("ResolveEditorCmd(editor) on darwin = %q, want nano", got)
		}
	default:
		if got != "editor" {
			t.Errorf("ResolveEditorCmd(editor) = %q, want editor", got)
		}
	}

	// A custom editor is never rewritten to nano.
	if got := ResolveEditorCmd("vim"); got != "vim" {
		t.Errorf("ResolveEditorCmd(vim) = %q, want vim", got)
	}
}

// TestConfigDisplayLayout pins the Config page against Python's ConfigDisplay
// (Config.py:25-48) capture at 80×24: a TOP-filled (not vertically centered)
// Pile whose 7-line ceil-left-centered explainer sits at the top of the body —
// blank, "To change…", blank, the config path, blank, "Restart Nomad Network
// for changes to take effect", blank — followed by a floor-left-centered
// "< Open Editor >" button (urwid.Padding width=15 align=CENTER → 32 leading
// spaces at width 80). The rows below are blank. Capture-verified byte-identical
// to the original (only the config path differs per seed).
func TestConfigDisplayLayout(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConfigDisplay(app, "/test/config")

	rows := renderPrimitive(t, cd.Widget(), 80, 23)
	for i := range rows {
		rows[i] = trimTrailing(rows[i])
	}

	// Row 0: blank (the explainer's leading "\n").
	if rows[0] != "" {
		t.Errorf("row 0 = %q, want blank (leading \\n)", rows[0])
	}
	// Row 1: ceil-left-centered "To change…" — rw=61 → lead=(80-61+1)/2=10.
	if want := strings.Repeat(" ", 10) + "To change the configuration, edit the config file located at:"; rows[1] != want {
		t.Errorf("row 1 = %q, want %q", rows[1], want)
	}
	// Row 2: blank.
	if rows[2] != "" {
		t.Errorf("row 2 = %q, want blank", rows[2])
	}
	// Row 3: ceil-left-centered "/test/config" — rw=12 → lead=(80-12+1)/2=34.
	if want := strings.Repeat(" ", 34) + "/test/config"; rows[3] != want {
		t.Errorf("row 3 = %q, want %q", rows[3], want)
	}
	// Row 4: blank.
	if rows[4] != "" {
		t.Errorf("row 4 = %q, want blank", rows[4])
	}
	// Row 5: "Restart Nomad Network for changes to take effect" — rw=48 →
	// lead=(80-48+1)/2=16.
	if want := strings.Repeat(" ", 16) + "Restart Nomad Network for changes to take effect"; rows[5] != want {
		t.Errorf("row 5 = %q, want %q (Restart line must be present)", rows[5], want)
	}
	// Row 6: blank (the explainer's trailing "\n").
	if rows[6] != "" {
		t.Errorf("row 6 = %q, want blank (trailing \\n)", rows[6])
	}
	// Row 7: floor-left-centered "< Open Editor >" — button width 15 →
	// lead=(80-15)/2=32 (Padding CENTER is floor-left).
	if want := strings.Repeat(" ", 32) + "< Open Editor >"; rows[7] != want {
		t.Errorf("row 7 = %q, want %q", rows[7], want)
	}
	// Rows 8+: blank (TOP-filled, not vertically centered).
	for i := 8; i < 23; i++ {
		if rows[i] != "" {
			t.Errorf("row %v = %q, want blank (TOP-filled below the pile)", i, rows[i])
		}
	}
}

// TestConfigDisplayUpToMenu asserts the config page ports Python's
// ConfigFiller.keypress "up" escape (Config.py:19-23): pressing Up while the
// config body has focus — specifically while the "Open Editor" button is
// focused, as it is after quitting the embedded editor (Config.py:74-78
// quit_term restores the explainer and re-focuses the button) — moves focus
// to the menu bar (MainFrame.focus_position = "header"). The centralized
// MainDisplay.bodyListAtTop only recognizes *tview.List / IndicativeListBox /
// centeredText, so the button-focused config page must own this transition
// itself, exactly as Python's ConfigFiller wrapper does. This reproduces the
// reported bug where, after Ctrl-x exits the editor and the cursor lands back
// on "Open Editor", Up could no longer reach the main menu.
func TestConfigDisplayUpToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	cd := NewConfigDisplay(app, "/test/config")

	// The "Open Editor" button is the config body's only focusable primitive.
	app.SetFocus(cd.openBtn)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)

	capture := cd.widget.GetInputCapture()
	if capture == nil {
		t.Fatal("config display widget has no InputCapture")
	}

	got := capture(up)
	if got != nil {
		t.Errorf("Up on config body = %v, want nil (consumed → menu)", got)
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("focusRegion = %q, want menu", app.Main.focusRegion)
	}
	if app.GetFocus() != app.Main.menuBar {
		t.Errorf("app focus = %v, want menuBar", app.GetFocus())
	}
}

// TestConfigDisplayUpToMenuDialogOpen asserts the Up→menu transition does
// NOT fire while a modal dialog overlay is open (the dispatcher must not steal
// focus from an open dialog), matching the guard in LogDisplay.handleInput.
func TestConfigDisplayUpToMenuDialogOpen(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	cd := NewConfigDisplay(app, "/test/config")

	app.SetFocus(cd.openBtn)
	app.Main.focusRegion = "body"

	app.Dialogs.stack = append(app.Dialogs.stack, &dialogEntry{})
	defer func() { app.Dialogs.stack = app.Dialogs.stack[:0] }()

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	capture := cd.widget.GetInputCapture()
	if capture == nil {
		t.Fatal("config display widget has no InputCapture")
	}
	got := capture(up)
	if got == nil {
		t.Error("Up on config body consumed while dialog open; want forwarded")
	}
	if app.Main.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body (dialog keeps focus)", app.Main.focusRegion)
	}
}

// TestConfigDisplayUpNotConsumedWhenEditorOpen asserts the Up→menu capture
// does not intercept keys while the embedded editor is mounted. The capture
// is attached to cd.widget (the "main" page); tview's Pages dispatches input
// only to the focused page, so while the "editor" page is mounted and focused
// the main page is outside the dispatch path and its InputCapture cannot fire.
// We verify that structural guarantee: while the editor is shown, cd.widget
// does not have focus and the EmbeddedTerminal does. Uses sleep 5 as a
// stand-in editor; closeEditor closes the PTY master (SIGHUP) so no child
// leaks.
func TestConfigDisplayUpNotConsumedWhenEditorOpen(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConfigDisplay(app, "/tmp/cfg")

	cd.ShowEditor("sleep", "5")
	defer cd.closeEditor()

	if cd.editor == nil {
		t.Fatal("ShowEditor did not mount the editor")
	}
	if got := app.GetFocus(); got != cd.editor {
		t.Errorf("editor open: focus = %T, want *EmbeddedTerminal", got)
	}
	if cd.widget.HasFocus() {
		t.Error("main page has focus while editor is mounted; its InputCapture could steal Up from the editor")
	}
}
