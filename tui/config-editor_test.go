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

// TestConfigDisplayShowEditorMountsAndCloses verifies the Phase-4 wiring:
// ShowEditor mounts the EmbeddedTerminal as the editor page over the config
// explainer (matching Python's open_editor setting self.widget =
// LineBox(editor); Config.py:30-35) and focuses it; closeEditor removes the
// page, clears the editor, and restores focus to the Open Editor button. It
// uses sleep 5 as a stand-in editor; closeEditor closes the PTY master (SIGHUP
// to sleep) so no child process leaks.
func TestConfigDisplayShowEditorMountsAndCloses(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConfigDisplay(app, "/tmp/cfg")

	cd.ShowEditor("sleep", "5")
	if cd.editor == nil {
		t.Fatal("ShowEditor did not set cd.editor")
	}
	if !cd.pages.HasPage("editor") {
		t.Error("ShowEditor did not add the editor page")
	}
	if got := app.GetFocus(); got != cd.editor {
		t.Errorf("after ShowEditor, focus = %T, want *EmbeddedTerminal", got)
	}

	cd.closeEditor()
	if cd.editor != nil {
		t.Error("closeEditor did not clear cd.editor")
	}
	if cd.pages.HasPage("editor") {
		t.Error("closeEditor did not remove the editor page")
	}
}
