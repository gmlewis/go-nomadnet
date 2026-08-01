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
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// defaultFieldWidth is the Micron default text-field width, matching Python's
// field_width = 24 fallback (MicronParser.py:726).
const defaultFieldWidth = 24

// maskCharacter is the masking rune for `!` masked text fields, matching
// Python's mask="*" (MicronParser.py:363).
const maskCharacter = '*'

// RadioGroup is a mutual-exclusion group of radio-button checkboxes sharing one
// Micron field name, mirroring urwid.RadioButton's shared group list
// (MicronParser.py:385-394). Selecting one member unchecks the others.
type RadioGroup struct {
	members []*tview.Checkbox
}

// FieldWidget is an interactive Micron field rendered as a tview primitive —
// the Go equivalent of the ReadlineEdit / CheckBox / RadioButton widgets built
// in parse_line (MicronParser.py:358-394). It records the field metadata (name,
// value, label, width, masked, prechecked) that Python attaches to the widget
// as field_name/field_value for later form submission.
type FieldWidget struct {
	Primitive  tview.Primitive
	Kind       string // "field", "checkbox", "radio"
	Name       string
	Value      string
	Label      string // checkbox/radio label (== Data)
	Data       string // text fields: initial edit text
	Masked     bool
	MaskChar   rune
	Width      int
	Prechecked bool
	Group      *RadioGroup // radio only
}

// NewFieldWidget renders a Micron FieldSpec as an interactive tview primitive.
// groups maps radio field names to their RadioGroup so same-name radios share a
// group; pass nil for checkbox/text fields. Mirrors the per-type widget
// construction in parse_line (MicronParser.py:358-394).
func NewFieldWidget(spec *micron.FieldSpec, groups map[string]*RadioGroup) *FieldWidget {
	fw := &FieldWidget{
		Kind:       spec.Type,
		Name:       spec.Name,
		Value:      spec.Value,
		Label:      spec.Data,
		Data:       spec.Data,
		Masked:     spec.Masked,
		Width:      spec.Width,
		Prechecked: spec.Prechecked,
	}
	if fw.Width == 0 {
		fw.Width = defaultFieldWidth
	}

	switch spec.Type {
	case "checkbox":
		cb := tview.NewCheckbox()
		cb.SetLabel(spec.Data)
		cb.SetChecked(spec.Prechecked)
		fw.Primitive = cb

	case "radio":
		cb := tview.NewCheckbox()
		cb.SetLabel(spec.Data)
		if groups != nil {
			g, ok := groups[spec.Name]
			if !ok {
				g = &RadioGroup{}
				groups[spec.Name] = g
			}
			g.members = append(g.members, cb)
			fw.Group = g
			// Wire mutual exclusion: checking this radio unchecks the others.
			// Ignore the unchecked transition so side-effect unchecks don't
			// cascade (mirrors urwid.RadioButton group semantics).
			cb.SetChangedFunc(func(checked bool) {
				if !checked {
					return
				}
				for _, m := range g.members {
					if m != cb && m.IsChecked() {
						m.SetChecked(false)
					}
				}
			})
			cb.SetChecked(spec.Prechecked)
		}
		fw.Primitive = cb

	default: // "field" — editable text input
		inp := tview.NewInputField()
		inp.SetText(spec.Data)
		inp.SetFieldWidth(fw.Width)
		if spec.Masked {
			inp.SetMaskCharacter(maskCharacter)
			fw.MaskChar = maskCharacter
		}
		fw.Primitive = inp
	}
	return fw
}
