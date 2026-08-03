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
