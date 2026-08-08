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

	"github.com/gdamore/tcell/v2"
)

// TestTabBarWidgetWidthDistribution pins the urwid-style width split: at an odd
// inner width the LEFT button gets the extra column (urwid Columns distributes
// leftover left-to-right), so the trusted tab is one wider than the untrusted
// tab — matching the Python capture `[ Trusted (0)           ] [ Untrusted (0)
//
//	]` (25 + 1 divider + 24 = 50).
func TestTabBarWidgetWidthDistribution(t *testing.T) {
	t.Parallel()
	left := NewTabButton("Trusted (0)")
	right := NewTabButton("Untrusted (0)")
	bar := newTabBarWidget(left, right)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(50, 1)
	bar.SetRect(0, 0, 50, 1)
	bar.Draw(screen)
	screen.Sync()

	var row strings.Builder
	for x := range 50 {
		c, _, _, _ := screen.GetContent(x, 0)
		row.WriteRune(c)
	}
	got := row.String()
	// Left tab 25 wide (cols 0..24), divider at col 25, right tab 24 wide
	// (cols 26..49). The trusted "]" lands at col 24, untrusted "[" at 26.
	want := "[ Trusted (0)           ]" + " " + "[ Untrusted (0)        ]"
	if got != want {
		t.Errorf("tab bar width split = %q, want %q", got, want)
	}
}

// TestTabBarWidgetEvenWidth pins the even-width split: both tabs equal.
func TestTabBarWidgetEvenWidth(t *testing.T) {
	t.Parallel()
	left := NewTabButton("ABC")
	right := NewTabButton("DEFG")
	bar := newTabBarWidget(left, right)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(21, 1) // 21 = 10 + 1 divider + 10
	bar.SetRect(0, 0, 21, 1)
	bar.Draw(screen)
	screen.Sync()

	var row strings.Builder
	for x := range 21 {
		c, _, _, _ := screen.GetContent(x, 0)
		row.WriteRune(c)
	}
	want := "[ ABC    ]" + " " + "[ DEFG   ]"
	if got := row.String(); got != want {
		t.Errorf("even tab bar split = %q, want %q", got, want)
	}
}
