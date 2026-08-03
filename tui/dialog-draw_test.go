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

// boundsAssertScreen wraps a tcell.Screen and fails the test if any
// SetContent call targets a negative coordinate (x<0 or y<0). tcell's real
// Screen silently clips such writes, so this wrapper makes negative-position
// draws observable. Writes past the right/bottom edge are NOT flagged: tview
// content widgets routinely draw their full allocated rect and rely on tcell
// to clip the overflow, so only negative coordinates are a genuine defect.
type boundsAssertScreen struct {
	tcell.Screen
	t *testing.T
}

func (s *boundsAssertScreen) SetContent(x, y int, mainc rune, combc []rune, st tcell.Style) {
	if x < 0 || y < 0 {
		w, h := s.Screen.Size()
		s.t.Errorf("SetContent at negative coordinate (%v,%v) on a %vx%v screen: rune %q", x, y, w, h, mainc)
		return
	}
	s.Screen.SetContent(x, y, mainc, combc, st)
}

// TestDialogLineBoxDrawClampsToScreen verifies DialogLineBox.Draw never writes
// at negative coordinates. When the dialog's inner rect sits at the top-left
// edge (x=0, y=0) and is narrower than its title, the previous implementation
// drew the left/top border at x-1=-1 / y-1=-1 and centered the title at a
// negative x, relying on tcell to clip. On resize to a terminal smaller than
// the dialog this is the fragile spot in the resize path. The draw must clamp
// every cell write to non-negative coordinates.
func TestDialogLineBoxDrawClampsToScreen(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(10, 5)

	bounds := &boundsAssertScreen{Screen: screen, t: t}

	d := NewDialogLineBox("A Rather Long Title", tview.NewTextView().SetText("body"), nil)
	// Inner rect at the top-left edge, narrower than the title.
	d.SetRect(0, 0, 5, 3)
	d.Draw(bounds)
}

// TestDialogLineBoxDrawClampsToSmallScreen resizes the screen smaller than the
// dialog's rect in both dimensions and asserts no negative-coordinate writes
// occur (left/top border at x-1/y-1).
func TestDialogLineBoxDrawClampsToSmallScreen(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(3, 2)

	bounds := &boundsAssertScreen{Screen: screen, t: t}

	d := NewDialogLineBox("Title", tview.NewTextView().SetText("x"), nil)
	d.SetRect(0, 0, 8, 4) // larger than the 3x2 screen in both dimensions
	d.Draw(bounds)
}
