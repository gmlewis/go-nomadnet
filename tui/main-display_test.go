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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestUrwidSpaceWrap pins the shortcut-bar wrapping against urwid's "space"
// wrap algorithm (urwid/text_layout.py:240-352), which the Python footer uses.
// Expected lines were derived from the urwid algorithm and cross-checked
// against capture.sh output of the Python original:
//
//   - Network bar at 80 cols: urwid fills line 1 to exactly 80 cols
//     ("...[C-f] Forward" — "Forward" ENDS at col 80), then breaks at the space
//     AT the fill column ("perfect space wrap"), dropping that space and
//     starting line 2 with the remaining one of the double-space (" [C-r]…").
//     tview's WordWrap instead breaks at the LAST space before overflow (before
//     "Forward"), which is the bug this replaces.
//   - Conversations list bar at 80 cols: the fill column lands inside "Sort",
//     so urwid walks back to the space after "[C-o]" → line 1 ends "[C-o]",
//     line 2 starts "Sort  [C-p] My LXMF  [C-g] Fullscreen".
func TestUrwidSpaceWrap(t *testing.T) {
	t.Parallel()
	networkBar := "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node"
	listBar := "[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen"
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{
			"network bar at 80 — Forward stays on line 1",
			networkBar, 80,
			[]string{
				"[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward",
				" [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node",
			},
		},
		{
			"conversations list bar at 80 — breaks after [C-o]",
			listBar, 80,
			[]string{
				"[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o]",
				"Sort  [C-p] My LXMF  [C-g] Fullscreen",
			},
		},
		{"network bar at 200 fits one row", networkBar, 200, []string{networkBar}},
		{"empty yields one empty line", "", 80, []string{""}},
		{"short fits one row", "[C-d] Back  [C-f] Forward", 80, []string{"[C-d] Back  [C-f] Forward"}},
		{"zero width yields one line", listBar, 0, []string{listBar}},
		{"embedded newline honored", "a\nbc", 80, []string{"a", "bc"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := urwidSpaceWrap(tt.text, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("urwidSpaceWrap(%q, %v) = %v lines, want %v\n got=%#v", tt.text, tt.width, len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("urwidSpaceWrap(%q, %v) line %v = %q, want %q", tt.text, tt.width, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMenuMouseClick(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode

	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	md.activeMenu = 0
	md.activePage = "conversations"
	md.redrawMenuBar()

	// Column positions (indicator width = 1, dividechar = 1):
	// Col 0: indicator
	// Col 1..18: space + [ Conversations ]
	// Col 19..30: space + [ Network ]
	// Col 31..43: space + [ Channels ]
	// Col 44..51: space + [ Log ]
	// Col 52..66: space + [ Interfaces ]
	// Col 67..77: space + [ Config ]
	// Col 78..87: space + [ Guide ]

	clickTests := []struct {
		x        int
		wantPage string
	}{
		{2, "conversations"}, // inside [ Conversations ]
		{22, "network"},      // inside [ Network ]
		{35, "channels"},     // inside [ Channels ]
		{48, "log"},          // inside [ Log ]
		{58, "interfaces"},   // inside [ Interfaces ]
		{72, "config"},       // inside [ Config ]
		{82, "guide"},        // inside [ Guide ]
	}

	for _, tt := range clickTests {
		md.handleClick(tt.x)
		if md.activePage != tt.wantPage {
			t.Errorf("handleClick(%v) selected page %q, want %q", tt.x, md.activePage, tt.wantPage)
		}
	}
}

func TestMenuMouseClickIgnoresNonZeroY(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	md.menuBar.SetRect(0, 0, 80, 1)
	md.selectMenu(0) // active page = "conversations"

	// Simulate MouseLeftClick on menuBar mouse capture at y=5, x=22
	handler := md.menuBar.GetMouseCapture()
	if handler != nil {
		event := tcell.NewEventMouse(22, 5, tcell.Button1, 0)
		handler(tview.MouseLeftClick, event)
		if md.activePage != "conversations" {
			t.Errorf("click at y=5 changed page to %q, want 'conversations' (mouse click on y=5 should not trigger menu)", md.activePage)
		}
	}
}

func TestMenuClickFocusesBodyAndDoesNotLockup(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	md := app.Main
	md.menuBar.SetRect(0, 0, 135, 1)

	// Simulate MouseLeftClick on menuBar at x=82 (inside [ Guide ])
	handler := md.menuBar.GetMouseCapture()
	if handler != nil {
		event := tcell.NewEventMouse(82, 0, tcell.Button1, 0)
		action, returnedEvent := handler(tview.MouseLeftClick, event)
		if returnedEvent != nil {
			t.Errorf("menu mouse capture returned event %v, want nil (consumed click so focus is not stolen by menuBar)", returnedEvent)
		}
		if action != 0 {
			t.Errorf("menu mouse capture returned action %v, want 0", action)
		}
	}
	if md.activePage != "guide" {
		t.Errorf("active page = %q, want 'guide'", md.activePage)
	}
	if md.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want 'body'", md.focusRegion)
	}
}

func TestMenuBarMouseCaptureConsumesAllActions(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	md := app.Main
	md.menuBar.SetRect(0, 0, 135, 1)

	handler := md.menuBar.GetMouseCapture()
	if handler == nil {
		t.Fatal("menuBar mouse capture handler is nil")
	}

	actionsToTest := []tview.MouseAction{
		tview.MouseLeftDown,
		tview.MouseLeftUp,
		tview.MouseLeftClick,
		tview.MouseRightClick,
		tview.MouseRightDown,
	}

	for _, act := range actionsToTest {
		event := tcell.NewEventMouse(10, 0, tcell.Button1, 0)
		action, returnedEvent := handler(act, event)
		if returnedEvent != nil || action != 0 {
			t.Errorf("action %v inside menuBar returned (%v, %v), want (0, nil)", act, action, returnedEvent)
		}
	}
}
