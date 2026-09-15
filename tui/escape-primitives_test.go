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
	"github.com/rivo/tview"
)

// These tests pin the primitive-level bracket escapes. Both sinks are shared by
// many callers, so escaping at the primitive fixes every one of them:
//
//   - tview.Box draws a border title with tview.Print (tag-parsed), so
//     SetTitledBorder escapes (a remote node title/URL can contain brackets).
//   - centeredText draws with tview.Print to emulate urwid.Text, which prints
//     literal text, so it escapes each line.

// TestSetTitledBorderEscapesBrackets covers the border-title sink.
func TestSetTitledBorderEscapesBrackets(t *testing.T) {
	t.Parallel()

	tv := tview.NewTextView()
	SetTitledBorder(tv, "[Remote Node]")
	if got := tv.GetTitle(); !strings.Contains(got, "[Remote Node[]") {
		t.Errorf("border title not escaped: %q", got)
	}
}

// TestCenteredTextEscapesBrackets draws the centeredText primitive on a
// simulation screen and reads the painted cells — the fork's tag parser is the
// only real proof.
func TestCenteredTextEscapesBrackets(t *testing.T) {
	t.Parallel()

	const width, height = 40, 1
	ct := newCenteredText(tcell.ColorDefault, "[loading]")
	ct.SetRect(0, 0, width, height)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	ct.Draw(screen)
	screen.Show()

	var painted strings.Builder
	for col := range width {
		main, _, _ := screen.Get(col, 0)
		painted.WriteString(main)
	}
	if !strings.Contains(painted.String(), "[loading]") {
		t.Errorf("centered text lost brackets: %q", painted.String())
	}
}
