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
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

// TestGuideFieldCursorFollowsRenderedRow pins that the Guide reader's
// within-line cursor and part boundaries count the RENDERED span text, the same
// measure its reader is drawn from (StyledLinesToTviewParts → spanText). The
// Guide's "Markup" topic documents input fields and checkboxes
// (guidetopics/markup.mu:381-447), so a field's padded box (or a checkbox's icon
// column) sits between the visible label and the link on those lines; measuring
// the raw span text dropped the hardware cursor ~24 columns left of the glyph it
// marks and made the links after a field unreachable.
func TestGuideFieldCursorFollowsRenderedRow(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)

	const w, h = 80, 6
	screen.SetSize(w, h)
	gd.reader.SetRect(0, 0, w, h)
	gd.showMarkupForTest("Field line: `B444`<name`>`b tail `[Go`/go]")
	gd.reader.Draw(screen)

	idx := gd.selectable[0]
	row := gd.lineRows[idx]
	if row != 0 {
		t.Fatalf("focused line row = %d, want 0", row)
	}
	drawn := drawnRowText(t, screen, 0, row, w)
	at := strings.LastIndex(drawn, "Go")
	if at < 0 {
		t.Fatalf("drawn row does not show the link: %q", drawn)
	}

	// The cursor's within-line offset is the link's position in the drawn row, so
	// the hardware cursor must land on the link's first cell.
	gd.focusCol = utf8.RuneCountInString(drawn[:at])
	x, _, ok := gd.cursorScreenXY()
	if !ok {
		t.Fatal("cursorScreenXY reported no cursor position")
	}
	if x != at {
		t.Errorf("cursor x on the link = %d, but the link is drawn at column %d (row %q)",
			x, at, strings.TrimRight(drawn, " "))
	}

	// The same offset must resolve the link for activation.
	var gotTarget string
	gd.OnHandleLink = func(target, fields string) { gotTarget = target }
	gd.focusActivate()
	if gotTarget != "/go" {
		t.Errorf("activating the link at the cursor = %q, want /go", gotTarget)
	}

	// Part stepping walks the same rendered boundaries: the field's box occupies
	// one part, so Right from the start reaches the field, not a raw-text offset
	// inside it.
	if got, want := gd.focusedPartPositions(), []int{0, 12, 36, 42, 44}; !slices.Equal(got, want) {
		t.Errorf("part positions = %v, want %v", got, want)
	}
}
