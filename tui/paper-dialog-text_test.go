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
// SOT: ui/textui/Conversations.py:2538-2541 (nomadnet 1.2.8). The body must name
// the destination directory, since an empty composer closes the dialog without
// doing anything at all.
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
	// The first sentence alone must not be the whole body.
	if got == "Select the desired paper message output method." {
		t.Error("dialog text is the truncated form that never named the destination directory")
	}
}

// TestAnimationIntervalReachesTheDisplay pins the wiring half of
// [textui] animation_interval: the configured seconds must reach the Network
// page's live widgets, which Python drives from the same key
// (ui/textui/Network.py:399,1030-1231,1542). A configured interval must reach
// both the app and the Node Info ticker.
func TestAnimationIntervalReachesTheDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  []Option
		wantS float64
	}{
		{"configured interval", []Option{WithAnimationInterval(0.25)}, 0.25},
		{"another configured interval", []Option{WithAnimationInterval(3)}, 3},
		{"unset keeps the reference default", nil, 1},
		{"a non-positive value keeps the default", []Option{WithAnimationInterval(0)}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue, tc.opts...)
			got := app.AnimationInterval().Seconds()
			if got != tc.wantS {
				t.Errorf("AnimationInterval = %vs, want %vs", got, tc.wantS)
			}

			// The display's ticker must read the same value, not a constant.
			ni := NewNodeInfoDisplay(app, NodeInfoData{})
			if d := ni.animationInterval(); d != app.AnimationInterval() {
				t.Errorf("the Node Info ticker uses %v, want the app's %v", d, app.AnimationInterval())
			}
		})
	}
}
