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

	text := cd.explainer.GetText(true)
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

	got := resolveEditorCmdDefault("editor")
	switch runtime.GOOS {
	case "darwin":
		if got != "nano" {
			t.Errorf("resolveEditorCmdDefault(editor) on darwin = %q, want nano", got)
		}
	default:
		if got != "editor" {
			t.Errorf("resolveEditorCmdDefault(editor) = %q, want editor", got)
		}
	}

	// A custom editor is never rewritten to nano.
	if got := resolveEditorCmdDefault("vim"); got != "vim" {
		t.Errorf("resolveEditorCmdDefault(vim) = %q, want vim", got)
	}
}
