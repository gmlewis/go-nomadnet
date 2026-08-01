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

// drawRadioButtonAt renders rb on a fresh simulation screen of width w and
// returns the joined cell text of its single row.
func drawRadioButtonAt(t *testing.T, rb *RadioButton, w int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, 1)
	rb.SetRect(0, 0, w, 1)
	rb.Draw(screen)
	screen.Sync()
	var b strings.Builder
	for x := 0; x < w; x++ {
		c, _, _, _ := screen.GetContent(x, 0)
		b.WriteRune(c)
	}
	return b.String()
}

// TestRadioButtonConstructionQuirk pins urwid's RadioButton construction
// semantics: the first radio in a group defaults to checked ("first True"), and
// creating a second radio with an explicit checked=True does NOT uncheck the
// first (RadioButton.__init__ sets _state without calling set_state). The New
// Conversation dialog opens in exactly this two-checked state.
func TestRadioButtonConstructionQuirk(t *testing.T) {
	t.Parallel()
	g := &DialogRadioGroup{}
	rUntrusted := NewRadioButton(g, "Untrusted", false, true)
	rUnknown := NewRadioButton(g, "Unknown", true, false)
	rTrusted := NewRadioButton(g, "Trusted", false, true)

	if !rUntrusted.Checked() {
		t.Errorf("Untrusted (first in group) = unchecked, want checked (first True)")
	}
	if !rUnknown.Checked() {
		t.Errorf("Unknown = unchecked, want checked (explicit True)")
	}
	if rTrusted.Checked() {
		t.Errorf("Trusted = checked, want unchecked (group non-empty → first True=false)")
	}
	// Both Untrusted and Unknown render "(X)", matching the Python capture.
	if got := strings.TrimSpace(drawRadioButtonAt(t, rUntrusted, 40)); got != "(X) Untrusted" {
		t.Errorf("Untrusted render = %q, want %q", got, "(X) Untrusted")
	}
	if got := strings.TrimSpace(drawRadioButtonAt(t, rUnknown, 40)); got != "(X) Unknown" {
		t.Errorf("Unknown render = %q, want %q", got, "(X) Unknown")
	}
	if got := strings.TrimSpace(drawRadioButtonAt(t, rTrusted, 40)); got != "( ) Trusted" {
		t.Errorf("Trusted render = %q, want %q", got, "( ) Trusted")
	}
}

// TestRadioButtonSetCheckedUnchecksGroup verifies that checking a radio
// (SetState(True) semantics) unchecks the other group members — the behavior
// that DOES happen on a user toggle, as opposed to construction.
func TestRadioButtonSetCheckedUnchecksGroup(t *testing.T) {
	t.Parallel()
	g := &DialogRadioGroup{}
	a := NewRadioButton(g, "A", false, true) // first → checked
	b := NewRadioButton(g, "B", false, false)
	if !a.Checked() || b.Checked() {
		t.Fatalf("initial: a=%v b=%v, want a=true b=false", a.Checked(), b.Checked())
	}
	b.SetChecked(true)
	if a.Checked() {
		t.Errorf("after checking B: A still checked, want unchecked (group uncheck)")
	}
	if !b.Checked() {
		t.Errorf("after checking B: B unchecked, want checked")
	}
}

// TestRadioButtonSpaceToggle verifies Space/Enter checks an unchecked radio and
// unchecks the rest of the group, but Space on an already-checked radio leaves
// the group unchanged (urwid radios cannot all be unset by toggling one).
func TestRadioButtonSpaceToggle(t *testing.T) {
	t.Parallel()
	g := &DialogRadioGroup{}
	a := NewRadioButton(g, "A", false, true) // checked
	b := NewRadioButton(g, "B", false, false)
	h := b.InputHandler()
	h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	if !b.Checked() || a.Checked() {
		t.Errorf("Space on B: a=%v b=%v, want a=false b=true", a.Checked(), b.Checked())
	}
	// Space on the now-checked B must not uncheck it.
	h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	if !b.Checked() {
		t.Errorf("Space on checked B: B became unchecked, want to stay checked")
	}
	// Enter on A re-checks it and unchecks B.
	ha := a.InputHandler()
	ha(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if !a.Checked() || b.Checked() {
		t.Errorf("Enter on A: a=%v b=%v, want a=true b=false", a.Checked(), b.Checked())
	}
	// A non-space rune is ignored.
	ha(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone), func(p tview.Primitive) {})
	if !a.Checked() {
		t.Errorf("rune 'x' unchecked A, want ignored")
	}
}

// TestRadioButtonChangedFunc verifies the change callback fires for the toggled
// radio and for the sibling unchecked by the group uncheck.
func TestRadioButtonChangedFunc(t *testing.T) {
	t.Parallel()
	g := &DialogRadioGroup{}
	a := NewRadioButton(g, "A", false, true)
	b := NewRadioButton(g, "B", false, false)
	var aEvents, bEvents []bool
	a.SetChangedFunc(func(c bool) { aEvents = append(aEvents, c) })
	b.SetChangedFunc(func(c bool) { bEvents = append(bEvents, c) })
	b.SetChecked(true)
	if aEvents[0] != false {
		t.Errorf("A changed event = %v, want false (unchecked by group)", aEvents[0])
	}
	if bEvents[0] != true {
		t.Errorf("B changed event = %v, want true (checked)", bEvents[0])
	}
}
