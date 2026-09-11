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

// TestChannelsMainDisplayShortcutUsesLiveFocus pins that the MAIN DISPLAY
// footer (what the user actually sees) shows [C-d] Send when the room
// composer has focus. A static SetShortcut("channels", listBar) alone is
// NOT enough — Python's Main.update_active_shortcuts consults the page's
// shortcuts() every draw, so Channels must register a SetShortcutCallback.
func TestChannelsMainDisplayShortcutUsesLiveFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	// Production (cmd/gonomadnet/textui.go) assigns app.Main so focus
	// callbacks can refresh the footer via refreshShortcuts.
	app.Main = md
	cd := NewChannelsDisplay(app, nil)

	// Mirror cmd/gonomadnet/textui.go wiring.
	const listBar = "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove"
	const editorBar = "[C-d] Send  [C-x] Leave  [F8] Collapse  [Tab] Complete Nick"
	md.SetShortcut("channels", listBar)
	md.SetShortcutCallback("channels", cd.GetShortcutText)

	rw := NewRoomWidget(app, "hub", "test")
	rw.OnFocusRegion = cd.setShortcutRegion
	cd.roomWidget = rw

	md.mu.Lock()
	md.activePage = "channels"
	md.updateShortcutsLocked()
	md.mu.Unlock()
	if got := md.GetShortcutText(); got != listBar {
		t.Fatalf("list focus: main bar = %q, want list bar", got)
	}

	app.SetFocus(rw.editor)
	// Editor SetFocusFunc fires OnFocusRegion → setShortcutRegion → refreshShortcuts.
	if got := md.GetShortcutText(); !strings.Contains(got, "[C-d] Send") {
		t.Errorf("composer focused: main bar = %q, want it to contain [C-d] Send", got)
	}
	if got := md.GetShortcutText(); got != editorBar {
		t.Errorf("composer focused: main bar = %q, want %q", got, editorBar)
	}
}
