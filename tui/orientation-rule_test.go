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
	"os"
	"testing"

	"github.com/creack/pty/v2"
)

// TestPortraitGrid is the rule, asserted for every combination of a cell grid
// and a reported physical size.
//
// The tablet cases are the ones this exists for: its console font draws a cell
// 12 pixels wide and 24 tall, so a 1200-by-1920 display in portrait holds 100
// columns and 75 rows — more columns than rows — and only the physical size can
// tell that it is standing up. The desktop cases are the ones that must not
// change: a terminal that reports no physical size is the grid it always was.
func TestPortraitGrid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		cols, rows int
		width      int
		height     int
		havePixels bool
		want       bool
	}{
		{"tablet portrait: 100x75 cells of 12x24 on a 1200x1920 display", 100, 75, 1200, 1920, true, true},
		{"tablet landscape: 160x45 cells of 12x24 is 1920x1080", 160, 45, 1920, 1080, true, false},
		{"phone portrait: 90x95 cells of 12x24 is 1080x2280", 90, 95, 1080, 2280, true, true},
		{"phone landscape: 190x45 cells of 12x24 is 2280x1080", 190, 45, 2280, 1080, true, false},
		{"a square console is not portrait", 100, 50, 1200, 1200, true, false},

		{"no physical size: a wide short terminal is landscape", 100, 76, 0, 0, false, false},
		{"no physical size: a tall narrow terminal is portrait", 60, 80, 0, 0, false, true},

		{"a cell grid taller than wide is portrait without pixels to say otherwise", 40, 50, 0, 0, false, true},
		{"pixels that cover the grid are believed even against the grid", 40, 50, 800, 600, true, false},

		{"a size smaller than the grid is refused: the grid decides", 100, 76, 999, 1, true, false},
		{"a size one column short is refused", 100, 76, 99, 99999, true, false},
		{"a size of exactly the grid is believed", 100, 76, 100, 76, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := portraitGrid(tc.cols, tc.rows, tc.width, tc.height, tc.havePixels)
			if got != tc.want {
				t.Errorf("portraitGrid(%v, %v, %v, %v, %v) = %v, want %v",
					tc.cols, tc.rows, tc.width, tc.height, tc.havePixels, got, tc.want)
			}
		})
	}
}

// TestSetTerminalPixelsOverridesTheBuiltInProbe asserts the seam an appliance
// installs: the answer the host gives wins over anything the client could ask
// its own terminal, and clearing the seam puts the built-in probe back.
//
// It does not run in parallel, because the seam is one value for the package and
// this test is the only one that writes it. Every parallel test in the package
// runs after the sequential ones have finished, so none of them can observe the
// probe this installs.
func TestSetTerminalPixelsOverridesTheBuiltInProbe(t *testing.T) {
	SetTerminalPixels(func() (int, int, bool) { return 1200, 1920, true })
	t.Cleanup(func() { SetTerminalPixels(nil) })

	if got := terminalIsPortrait(100, 76); !got {
		t.Error("expected 100x75 to be portrait while the host reports a 1200x1920 display")
	}
	if got := terminalIsPortrait(2000, 1000); got {
		t.Error("expected a grid the host's pixels cannot cover to fall back to the grid")
	}
}

// TestStdinTerminalPixelsReadsARealPty asserts the whole chain the appliance
// relies on, on the build machine and with no device: a pty is given a physical
// size, and the probe reading its own terminal is told it.
//
// The size is set on the master and read back through the probe's own ioctl, so
// what is asserted is the kernel's round trip and not a cached value — the same
// round trip that carries the console's size from the Android host to the client.
//
// It does not run in parallel: the probe reads os.Stdin, and the only way to give
// a test a terminal is to point the process's stdin at the pty's slave. Every
// parallel test in the package runs after the sequential ones have finished, so
// none of them can see the terminal this installs.
func TestStdinTerminalPixelsReadsARealPty(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("no pty on this machine: %v", err)
	}
	defer func() { _ = master.Close() }()

	if err := pty.Setsize(master, &pty.Winsize{Rows: 75, Cols: 100, X: 1200, Y: 1920}); err != nil {
		t.Fatalf("setting the pty's size: %v", err)
	}

	saved := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = saved })

	width, height, ok := stdinTerminalPixels()
	if !ok {
		t.Fatal("expected a pty with a physical size to report one")
	}
	if width != 1200 || height != 1920 {
		t.Errorf("the pty reported %vx%v pixels, want 1200x1920", width, height)
	}
	if !terminalIsPortrait(100, 75) {
		t.Error("expected the console the pty describes to be portrait")
	}
}

// TestStdinTerminalPixelsWithoutAPhysicalSize reports that a pty whose pixel
// fields were never set answers so, which is the fallback rather than a failure:
// the ioctl succeeds and both fields are zero, exactly as they are on a terminal
// that keeps no font metrics.
func TestStdinTerminalPixelsWithoutAPhysicalSize(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("no pty on this machine: %v", err)
	}
	defer func() { _ = master.Close() }()

	if err := pty.Setsize(master, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("setting the pty's size: %v", err)
	}

	saved := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = saved })

	if _, _, ok := stdinTerminalPixels(); ok {
		t.Error("expected a pty with no pixel fields to report no physical size")
	}
	if !terminalIsPortrait(60, 80) {
		t.Error("expected a tall terminal with no physical size to fall back to its cells")
	}
}
