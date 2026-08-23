// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"
)

// TestChannelsShortcutBar verifies that ChannelsDisplay.GetShortcutText
// returns the correct per-focus-region shortcut bar text, matching Python's
// Channels.py:217-229 (three regions: list / editor / body).
func TestChannelsShortcutBar(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)

	for _, tc := range []struct {
		name   string
		region string
		want   string
	}{
		{
			"list",
			"list",
			"[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove",
		},
		{
			"editor",
			"editor",
			"[C-d] Send  [C-x] Leave  [F8] Collapse  [Tab] Complete Nick",
		},
		{
			"body",
			"body",
			"[C-x] Leave  [C-u] Users  [C-y] Channels  [F8] Collapse Joins  [Tab] ↓ Editor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Each subtest gets its own ChannelsDisplay to avoid
			// racing on shortcutFocus across parallel subtests.
			cd := NewChannelsDisplay(app, nil)
			cd.SetShortcutFocus(tc.region)
			got := cd.GetShortcutText()
			if got != tc.want {
				t.Errorf("GetShortcutText() = %q, want %q", got, tc.want)
			}
		})
	}
}
