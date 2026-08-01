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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// TestFieldWidgetTextField asserts a "field" spec renders as a tview.InputField
// carrying the initial text and width, mirroring Python's
// ReadlineEdit(edit_text=field_data, ...) at MicronParser.py:364.
func TestFieldWidgetTextField(t *testing.T) {
	t.Parallel()
	spec := &micron.FieldSpec{
		Type: "field", Name: "username", Data: "alice", Width: 20,
	}
	fw := NewFieldWidget(spec, nil)

	if fw.Kind != "field" {
		t.Errorf("Kind = %q, want field", fw.Kind)
	}
	if fw.Name != "username" {
		t.Errorf("Name = %q, want username", fw.Name)
	}
	if fw.Data != "alice" {
		t.Errorf("Data = %q, want alice", fw.Data)
	}
	if fw.Masked {
		t.Error("Masked = true, want false")
	}

	inp, ok := fw.Primitive.(*tview.InputField)
	if !ok {
		t.Fatalf("Primitive = %T, want *tview.InputField", fw.Primitive)
	}
	if got := inp.GetText(); got != "alice" {
		t.Errorf("InputField text = %q, want alice", got)
	}
	if got := inp.GetFieldWidth(); got != 20 {
		t.Errorf("InputField width = %v, want 20", got)
	}
}

// TestFieldWidgetMasked asserts the `!` masked flag sets the '*' mask character
// on the InputField, mirroring Python's mask="*" (MicronParser.py:363).
func TestFieldWidgetMasked(t *testing.T) {
	t.Parallel()
	spec := &micron.FieldSpec{
		Type: "field", Name: "password", Data: "secret", Width: 16, Masked: true,
	}
	fw := NewFieldWidget(spec, nil)

	if !fw.Masked {
		t.Error("Masked = false, want true")
	}
	if fw.MaskChar != '*' {
		t.Errorf("MaskChar = %q, want '*'", fw.MaskChar)
	}
}

// TestFieldWidgetDefaultWidth asserts a zero width falls back to Python's
// default field_width of 24 (MicronParser.py:726).
func TestFieldWidgetDefaultWidth(t *testing.T) {
	t.Parallel()
	spec := &micron.FieldSpec{Type: "field", Name: "q", Data: ""}
	fw := NewFieldWidget(spec, nil)
	if fw.Width != 24 {
		t.Errorf("Width = %v, want 24 (default)", fw.Width)
	}
	inp, _ := fw.Primitive.(*tview.InputField)
	if got := inp.GetFieldWidth(); got != 24 {
		t.Errorf("InputField width = %v, want 24", got)
	}
}

// TestFieldWidgetCheckbox asserts a "checkbox" spec renders as a tview.Checkbox
// with the label and prechecked state, mirroring Python's
// urwid.CheckBox(label, state=prechecked) at MicronParser.py:374.
func TestFieldWidgetCheckbox(t *testing.T) {
	t.Parallel()
	spec := &micron.FieldSpec{
		Type: "checkbox", Name: "opt", Data: "Enable notifications",
		Value: "y", Prechecked: true,
	}
	fw := NewFieldWidget(spec, nil)

	if fw.Kind != "checkbox" {
		t.Errorf("Kind = %q, want checkbox", fw.Kind)
	}
	if fw.Value != "y" {
		t.Errorf("Value = %q, want y", fw.Value)
	}

	cb, ok := fw.Primitive.(*tview.Checkbox)
	if !ok {
		t.Fatalf("Primitive = %T, want *tview.Checkbox", fw.Primitive)
	}
	if got := cb.GetLabel(); got != "Enable notifications" {
		t.Errorf("Checkbox label = %q, want Enable notifications", got)
	}
	if !cb.IsChecked() {
		t.Error("Checkbox IsChecked = false, want true (prechecked)")
	}
}

// TestFieldWidgetUncheckedCheckbox asserts a non-prechecked checkbox starts
// unchecked.
func TestFieldWidgetUncheckedCheckbox(t *testing.T) {
	t.Parallel()
	spec := &micron.FieldSpec{Type: "checkbox", Name: "opt", Data: "opt label", Value: "y"}
	fw := NewFieldWidget(spec, nil)
	cb, _ := fw.Primitive.(*tview.Checkbox)
	if cb.IsChecked() {
		t.Error("IsChecked = true, want false (not prechecked)")
	}
}

// TestFieldWidgetRadioGrouping asserts same-name radios share one RadioGroup
// and different names get separate groups, mirroring urwid.RadioButton's
// per-name group list (MicronParser.py:385-394).
func TestFieldWidgetRadioGrouping(t *testing.T) {
	t.Parallel()
	groups := map[string]*RadioGroup{}

	a := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "color", Data: "Red", Value: "red"}, groups)
	b := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "color", Data: "Blue", Value: "blue"}, groups)
	c := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "size", Data: "Big", Value: "big"}, groups)

	if a.Group == nil || b.Group == nil || c.Group == nil {
		t.Fatal("radio Group = nil, want a group")
	}
	if a.Group != b.Group {
		t.Error("same-name radios have different groups, want shared")
	}
	if a.Group == c.Group {
		t.Error("different-name radios share a group, want separate")
	}
	if len(a.Group.members) != 2 {
		t.Errorf("color group has %v members, want 2", len(a.Group.members))
	}
	if len(c.Group.members) != 1 {
		t.Errorf("size group has %v members, want 1", len(c.Group.members))
	}
}

// TestFieldWidgetRadioMutualExclusion asserts checking one radio unchecks the
// others in its group — the core RadioButton invariant.
func TestFieldWidgetRadioMutualExclusion(t *testing.T) {
	t.Parallel()
	groups := map[string]*RadioGroup{}

	red := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "color", Data: "Red", Value: "red", Prechecked: true}, groups)
	blue := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "color", Data: "Blue", Value: "blue"}, groups)

	redCB, _ := red.Primitive.(*tview.Checkbox)
	blueCB, _ := blue.Primitive.(*tview.Checkbox)

	if !redCB.IsChecked() {
		t.Error("prechecked Red should be checked initially")
	}
	if blueCB.IsChecked() {
		t.Error("Blue should be unchecked initially")
	}

	// Select Blue → Red must be automatically unchecked.
	blueCB.SetChecked(true)
	if !blueCB.IsChecked() {
		t.Error("Blue should be checked after selecting it")
	}
	if redCB.IsChecked() {
		t.Error("Red should be unchecked after Blue is selected (mutual exclusion)")
	}
}

// TestFieldWidgetRadioPrecheckedLastWins asserts that when two same-group radios
// are both prechecked, the last one added wins (urwid.RadioButton unchecks prior
// group members when a new one is set True).
func TestFieldWidgetRadioPrecheckedLastWins(t *testing.T) {
	t.Parallel()
	groups := map[string]*RadioGroup{}

	first := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "g", Data: "First", Value: "1", Prechecked: true}, groups)
	second := NewFieldWidget(&micron.FieldSpec{Type: "radio", Name: "g", Data: "Second", Value: "2", Prechecked: true}, groups)

	firstCB, _ := first.Primitive.(*tview.Checkbox)
	secondCB, _ := second.Primitive.(*tview.Checkbox)

	if firstCB.IsChecked() {
		t.Error("first prechecked radio should be unchecked once the second prechecked radio joins the group")
	}
	if !secondCB.IsChecked() {
		t.Error("last prechecked radio should remain checked")
	}
}
