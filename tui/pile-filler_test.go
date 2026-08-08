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

// pfRender renders a pileFiller at w×h after laying it out, returning the rows.
func pfRender(t *testing.T, p *pileFiller, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Sync()
	rows := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = strings.TrimRight(b.String(), " ")
	}
	return rows
}

// pfKey sends a key event to the pileFiller's input handler.
func pfKey(p *pileFiller, key tcell.Key) {
	p.InputHandler()(tcell.NewEventKey(key, 0, tcell.ModNone), func(tview.Primitive) {})
}

// pfField builds a selectable 1-row InputField with the given label.
func pfField(label string) *tview.InputField {
	f := tview.NewInputField()
	f.SetLabel(label)
	return f
}

// TestPileFillerFocusCyclingTab verifies Tab/Down move focus forward (wrapping)
// and BackTab/Up move it backward, with the old widget blurred and the new one
// focused.
func TestPileFillerFocusCyclingTab(t *testing.T) {
	t.Parallel()
	p := newPileFiller()
	a := pfField("A:")
	b := pfField("B:")
	c := pfField("C:")
	p.AddItem(a, 1, true)
	p.AddItem(b, 1, true)
	p.AddItem(c, 1, true)

	if p.FocusIndex() != 0 {
		t.Fatalf("initial focusIndex = %v, want 0", p.FocusIndex())
	}
	p.Focus(func(tview.Primitive) {})
	if !a.HasFocus() {
		t.Fatal("initial focus should be on item A")
	}

	// Tab → B.
	pfKey(p, tcell.KeyTab)
	if p.FocusIndex() != 1 {
		t.Errorf("after Tab focusIndex = %v, want 1", p.FocusIndex())
	}
	if !b.HasFocus() {
		t.Error("B should be focused after Tab")
	}
	if a.HasFocus() {
		t.Error("A should be blurred after Tab moves focus to B")
	}

	// Tab → C.
	pfKey(p, tcell.KeyTab)
	if p.FocusIndex() != 2 {
		t.Errorf("after 2x Tab focusIndex = %v, want 2", p.FocusIndex())
	}
	if !c.HasFocus() {
		t.Error("C should be focused after 2x Tab")
	}

	// Tab wraps → A.
	pfKey(p, tcell.KeyTab)
	if p.FocusIndex() != 0 {
		t.Errorf("after 3x Tab focusIndex = %v, want 0 (wrap)", p.FocusIndex())
	}
	if !a.HasFocus() {
		t.Error("A should be focused after wrap")
	}

	// Backtab from A wraps → C.
	pfKey(p, tcell.KeyBacktab)
	if p.FocusIndex() != 2 {
		t.Errorf("after Backtab from A focusIndex = %v, want 2 (wrap)", p.FocusIndex())
	}
	if !c.HasFocus() {
		t.Error("C should be focused after Backtab wrap")
	}

	// Tab moves forward to wrap → A.
	pfKey(p, tcell.KeyTab)
	if p.FocusIndex() != 0 {
		t.Errorf("after Tab from C focusIndex = %v, want 0 (wrap)", p.FocusIndex())
	}
}

// TestPileFillerTrimFollowsFocus verifies the top-trim tracks the current focus:
// with 3 one-row items in a 2-row slot, focusing the last item shows items 1+2
// (item 0 scrolled off the top), while focusing the first shows items 0+1.
func TestPileFillerTrimFollowsFocus(t *testing.T) {
	t.Parallel()
	p := newPileFiller()
	a := pfField("AAA:")
	b := pfField("BBB:")
	c := pfField("CCC:")
	p.AddItem(a, 1, true)
	p.AddItem(b, 1, true)
	p.AddItem(c, 1, true)

	p.Focus(func(tview.Primitive) {})

	// Focus on A (index 0): topTrim 0 → rows 0,1 = A, B.
	rows := pfRender(t, p, 10, 2)
	if !strings.Contains(rows[0], "AAA") {
		t.Errorf("focus A row0 = %q, want AAA", rows[0])
	}
	if !strings.Contains(rows[1], "BBB") {
		t.Errorf("focus A row1 = %q, want BBB", rows[1])
	}

	// Tab twice → focus C (index 2): topTrim = 2-2+1 = 1 → rows show B, C.
	pfKey(p, tcell.KeyTab)
	pfKey(p, tcell.KeyTab)
	rows = pfRender(t, p, 10, 2)
	if !strings.Contains(rows[0], "BBB") {
		t.Errorf("focus C row0 = %q, want BBB (scrolled)", rows[0])
	}
	if !strings.Contains(rows[1], "CCC") {
		t.Errorf("focus C row1 = %q, want CCC", rows[1])
	}
	if strings.Contains(rows[0], "AAA") {
		t.Errorf("focus C row0 = %q, AAA should be scrolled off", rows[0])
	}
}

// TestPileFillerEscHandler verifies Esc invokes the dismiss handler.
func TestPileFillerEscHandler(t *testing.T) {
	t.Parallel()
	p := newPileFiller()
	p.AddItem(pfField("X:"), 1, true)
	called := false
	p.SetEscHandler(func() { called = true })
	pfKey(p, tcell.KeyEscape)
	if !called {
		t.Fatal("Esc did not invoke the dismiss handler")
	}
}

// TestPileFillerSetFocusIndex verifies overriding the default first-selectable
// focus (KnownNodeInfo focuses the last item — the button row).
func TestPileFillerSetFocusIndex(t *testing.T) {
	t.Parallel()
	p := newPileFiller()
	p.AddItem(pfField("A:"), 1, true)
	p.AddItem(pfField("B:"), 1, true)
	p.AddItem(pfField("C:"), 1, true)
	p.SetFocusIndex(2) // last
	p.Focus(func(tview.Primitive) {})
	if p.FocusIndex() != 2 {
		t.Errorf("focusIndex = %v, want 2", p.FocusIndex())
	}
	if !c_hasFocus(p) {
		t.Error("last item should be focused after SetFocusIndex(2)")
	}
}

// c_hasFocus checks whether the 3rd item of the test pile is focused.
func c_hasFocus(p *pileFiller) bool {
	return p.items[p.selectable[2]].widget.HasFocus()
}
