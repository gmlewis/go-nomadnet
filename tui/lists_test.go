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

	"github.com/gmlewis/tview"
)

func TestNewSelectableList(t *testing.T) {
	t.Parallel()

	sl := NewSelectableList()
	if sl == nil {
		t.Fatal("NewSelectableList returned nil")
	}
	if sl.List == nil {
		t.Error("List is nil")
	}
}

func TestNewTrustListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, trust, want string
	}{
		{"Alice", "trusted", "● Alice"},
		{"Bob", "untrusted", "× Bob"},
		{"Charlie", "unknown", "○ Charlie"},
		{"Dave", "warning", "⚠ Dave"},
	}

	for _, tt := range tests {
		got := NewTrustListItem(tt.name, tt.trust)
		if got != tt.want {
			t.Errorf("NewTrustListItem(%q, %q) = %q, want %q", tt.name, tt.trust, got, tt.want)
		}
	}
}

// TestFocusFirstChild asserts the helper's contract: the first child marked
// focusable in a Flex becomes the application's focused primitive, without
// disturbing a focusable child that comes before it.
func TestFocusFirstChild(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	flex := tview.NewFlex()
	first := tview.NewTextView()
	second := tview.NewTextView()
	flex.AddItem(first, 1, 1, true)
	flex.AddItem(second, 1, 1, false)

	// Give focus to something else first, so a no-op implementation cannot
	// pass by accident.
	app.Application.SetRoot(tview.NewTextView(), true)
	FocusFirstChild(app.Application, flex)
	if got := app.Application.GetFocus(); got != first {
		t.Errorf("focus after FocusFirstChild = %T, want the first focusable child", got)
	}
}

func TestFocusFirstChildNil(t *testing.T) {
	t.Parallel()

	// The nil guards are the function's contract for missing app/flex.
	FocusFirstChild(nil, nil)
	FocusFirstChild(newTestApp().Application, nil)
}

// TestListFocusColors asserts the list selection colors come from the theme's
// list_focus_fg/list_focus_bg entries (Python TextUI.py: list_focus is
// #111 on #aaa in both dark and light), NOT the hardcoded #666 the port
// previously used. Both #111 and #aaa are 3-hex, so urwid cube-quantizes them
// even in truecolor: #111→#000000, #aaa→#afafaf.
func TestListFocusColors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		theme  int
		wantFg uint32
		wantBg uint32
	}{
		{ThemeDark, 0x000000, 0xafafaf},
		{ThemeLight, 0x000000, 0xafafaf},
	}
	for _, tc := range cases {
		fg, bg := ListFocusColors(tc.theme)
		if uint32(fg.Hex()) != tc.wantFg {
			t.Errorf("theme %v fg = #%06x, want #%06x", tc.theme, uint32(fg.Hex()), tc.wantFg)
		}
		if uint32(bg.Hex()) != tc.wantBg {
			t.Errorf("theme %v bg = #%06x, want #%06x", tc.theme, uint32(bg.Hex()), tc.wantBg)
		}
	}
}

// TestApplyListFocusStyle asserts ApplyListFocusStyle does not panic and that
// the list focus colors differ from the old hardcoded #666.
func TestApplyListFocusStyle(t *testing.T) {
	t.Parallel()

	list := tview.NewList()
	ApplyListFocusStyle(list, ThemeDark)
	fg, bg := ListFocusColors(ThemeDark)
	if uint32(bg.Hex()) == 0x666666 {
		t.Error("focus bg still hardcoded #666; want theme list_focus_bg #aaa")
	}
	_ = fg
}
