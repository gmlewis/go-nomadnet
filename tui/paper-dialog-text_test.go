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
	"strings"
	"testing"
)

// TestPaperMessageDialogText pins the Create Paper Message dialog body against the
// reference. Python builds it as
//
//	"Select the desired paper message output method.\nSaved files will be written
//	 to:\n\n" + str(self.app.downloads_path) + "\n"
//
// SOT: ui/textui/Conversations.py:2538-2541 (nomadnet 1.2.8). The port showed only
// the first sentence, so a user was never told where the file would be written —
// and an empty composer closes the dialog without doing anything at all, which is
// baffling without that line.
func TestPaperMessageDialogText(t *testing.T) {
	t.Parallel()

	const downloads = "/home/user/Downloads"
	got := paperMessageDialogText(downloads)

	want := "Select the desired paper message output method.\n" +
		"Saved files will be written to:\n\n" + downloads + "\n"
	if got != want {
		t.Errorf("paperMessageDialogText(%q) =\n%q\nwant\n%q", downloads, got, want)
	}
	if !strings.Contains(got, downloads) {
		t.Errorf("dialog text %q does not name the downloads directory", got)
	}
	// The sentence the port used to show on its own must not be the whole body.
	if got == "Select the desired paper message output method." {
		t.Error("dialog text is the truncated form that never named the destination directory")
	}
}
