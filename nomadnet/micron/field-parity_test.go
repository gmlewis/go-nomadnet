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

package micron

import "testing"

// Expected values captured from Python MicronParser.make_output's '<' branch
// (MicronParser.py:669-758). In micron, fields are entered from formatting
// mode with the `< prefix (the backtick enters formatting mode, then '<'
// triggers field parsing) — bare "<" in text mode is a literal character.
// The Python field model keeps field_value (3rd pipe component) and
// field_data (text after the inner backtick) separate: for text fields only
// field_data is emitted, while checkbox/radio emit value (field_value or
// label) and label, with no width.
func TestParseFieldParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		markup      string
		wantType    NodeType
		wantFType   string
		wantName    string
		wantWidth   int
		wantMask    bool
		wantData    string
		wantValue   string
		wantChecked bool
		wantNoField bool
	}{
		{
			name: "plain", markup: "`<fieldname`default>",
			wantType: NodeField, wantFType: "field", wantName: "fieldname",
			wantWidth: 24, wantData: "default",
		},
		{
			name: "width_and_name", markup: "`<20|username`actual>",
			wantType: NodeField, wantFType: "field", wantName: "username",
			wantWidth: 20, wantData: "actual",
		},
		{
			name: "value_does_not_overwrite_data", markup: "`<20|username|preset`actual>",
			wantType: NodeField, wantFType: "field", wantName: "username",
			wantWidth: 20, wantData: "actual",
		},
		{
			name: "masked", markup: "`<!|password`secret>",
			wantType: NodeField, wantFType: "field", wantName: "password",
			wantWidth: 24, wantMask: true, wantData: "secret",
		},
		{
			name: "checkbox", markup: "`<?|agree|yes`I agree>",
			wantType: NodeField, wantFType: "checkbox", wantName: "agree",
			wantValue: "yes", wantData: "I agree",
		},
		{
			name: "radio", markup: "`<^|color|red`Pick red>",
			wantType: NodeField, wantFType: "radio", wantName: "color",
			wantValue: "red", wantData: "Pick red",
		},
		{
			name: "width_clamped_to_256", markup: "`<300|toobig`x>",
			wantType: NodeField, wantFType: "field", wantName: "toobig",
			wantWidth: 256, wantData: "x",
		},
		{
			name: "checkbox_prechecked", markup: "`<?|opt|y|*`Label>",
			wantType: NodeField, wantFType: "checkbox", wantName: "opt",
			wantValue: "y", wantData: "Label", wantChecked: true,
		},
		{
			name: "checkbox_value_defaults_to_label", markup: "`<?|opt`Label>",
			wantType: NodeField, wantFType: "checkbox", wantName: "opt",
			wantValue: "Label", wantData: "Label",
		},
		{
			name: "empty_data", markup: "`<30|name`>",
			wantType: NodeField, wantFType: "field", wantName: "name",
			wantWidth: 30, wantData: "",
		},
		{
			// No inner backtick: invalid field. Python passes (no field
			// emitted) and the rest of the line becomes text.
			name: "no_backtick_invalid", markup: "`<nobtick>", wantNoField: true,
		},
		{
			// No closing >: invalid field. Python passes; rest is text.
			name: "no_close_invalid", markup: "`<no_close`data", wantNoField: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nodes := Parse(tc.markup)
			if tc.wantNoField {
				for _, n := range nodes {
					if n.Type == NodeField {
						t.Fatalf("Parse(%q) produced a field node, want none", tc.markup)
					}
				}
				return
			}
			var n *Node
			for _, cand := range nodes {
				if cand.Type == NodeField {
					n = cand
					break
				}
			}
			if n == nil {
				t.Fatalf("Parse(%q) produced no field node", tc.markup)
			}
			if n.Type != tc.wantType {
				t.Errorf("Type = %d, want %d", n.Type, tc.wantType)
			}
			if n.FieldType != tc.wantFType {
				t.Errorf("FieldType = %q, want %q", n.FieldType, tc.wantFType)
			}
			if n.FieldName != tc.wantName {
				t.Errorf("FieldName = %q, want %q", n.FieldName, tc.wantName)
			}
			if n.FieldWidth != tc.wantWidth {
				t.Errorf("FieldWidth = %d, want %d", n.FieldWidth, tc.wantWidth)
			}
			if n.FieldMask != tc.wantMask {
				t.Errorf("FieldMask = %v, want %v", n.FieldMask, tc.wantMask)
			}
			if n.FieldData != tc.wantData {
				t.Errorf("FieldData = %q, want %q", n.FieldData, tc.wantData)
			}
			if n.FieldValue != tc.wantValue {
				t.Errorf("FieldValue = %q, want %q", n.FieldValue, tc.wantValue)
			}
			if n.FieldPrechecked != tc.wantChecked {
				t.Errorf("FieldPrechecked = %v, want %v", n.FieldPrechecked, tc.wantChecked)
			}
		})
	}
}
