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
	"testing"
)

// TestInterfacesDisplayShowEditorMountsAndCloses verifies the Phase-3 wiring:
// ShowEditor mounts the EmbeddedTerminal as the editor page (matching Python's
// open_config_editor swapping self.widget for a LineBox(urwid.Terminal)) and
// moves focus onto it; closeEditor removes the page and clears the editor. It
// uses sleep 5 as a stand-in editor that stays alive long enough to inspect the
// page state; closeEditor closes the PTY master (SIGHUP to sleep) so no child
// process leaks.
func TestInterfacesDisplayShowEditorMountsAndCloses(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	id.ShowEditor("sleep", "5")
	if id.editor == nil {
		t.Fatal("ShowEditor did not set id.editor")
	}
	if !id.pages.HasPage("editor") {
		t.Error("ShowEditor did not add the editor page")
	}
	if got := app.GetFocus(); got != id.editor {
		t.Errorf("after ShowEditor, focus = %T, want *EmbeddedTerminal", got)
	}

	id.closeEditor()
	if id.editor != nil {
		t.Error("closeEditor did not clear id.editor")
	}
	if id.pages.HasPage("editor") {
		t.Error("closeEditor did not remove the editor page")
	}
}
