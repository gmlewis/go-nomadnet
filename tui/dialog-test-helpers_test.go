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

	"github.com/rivo/tview"
)

// dialogRowTexts walks a dialog's content tree in visual order (Flex rows,
// then each leaf) and returns one string per leaf row — the same lines a
// reader sees inside the dialog border. Multi-line TextViews are split.
// Custom-drawn rows (dividers) yield "".
func dialogRowTexts(p tview.Primitive) []string {
	var out []string
	var walk func(p tview.Primitive)
	walk = func(p tview.Primitive) {
		switch v := p.(type) {
		case *DialogLineBox:
			walk(v.content)
		case *centeredText:
			out = append(out, v.GetText())
		case *pileFiller:
			for _, it := range v.items {
				walk(it.widget)
			}
		case *urwidColumns:
			for _, child := range v.children {
				walk(child)
			}
		case *tview.Flex:
			for i := 0; i < v.GetItemCount(); i++ {
				walk(v.GetItem(i))
			}
		case *tview.TextView:
			for line := range strings.SplitSeq(v.GetText(true), "\n") {
				out = append(out, line)
			}
		case *ReadlineEdit:
			out = append(out, v.GetLabel()+v.GetText())
		case *tview.InputField:
			out = append(out, v.GetLabel()+v.GetText())
		case *RadioButton:
			marker := "( ) "
			if v.Checked() {
				marker = "(X) "
			}
			out = append(out, marker+v.Label())
		case *UrwidButton:
			out = append(out, "< "+v.Label()+" >")
		case *UrwidCheckBox:
			marker := "[ ] "
			if v.IsChecked() {
				marker = "[X] "
			}
			out = append(out, marker+v.label)
		case *tview.Checkbox:
			marker := "[ ] "
			if v.IsChecked() {
				marker = "[X] "
			}
			out = append(out, marker+v.GetLabel())
		case *tview.List:
			for i := 0; i < v.GetItemCount(); i++ {
				main, secondary := v.GetItemText(i)
				out = append(out, main)
				if secondary != "" {
					out = append(out, secondary)
				}
			}
		case *tview.Grid:
			// Grid has no item accessor in this fork; skip.
		default:
			out = append(out, "")
		}
	}
	walk(p)
	return out
}

// findCheckbox walks the dialog tree for an UrwidCheckBox with the given
// label, returning nil when absent.
func findCheckbox(p tview.Primitive, label string) *UrwidCheckBox {
	var found *UrwidCheckBox
	var walk func(p tview.Primitive)
	walk = func(p tview.Primitive) {
		if found != nil {
			return
		}
		switch v := p.(type) {
		case *DialogLineBox:
			walk(v.content)
		case *pileFiller:
			for _, it := range v.items {
				walk(it.widget)
			}
		case *urwidColumns:
			for _, child := range v.children {
				walk(child)
			}
		case *tview.Flex:
			for i := 0; i < v.GetItemCount(); i++ {
				walk(v.GetItem(i))
			}
		case *UrwidCheckBox:
			if v.label == label {
				found = v
			}
		}
	}
	walk(p)
	return found
}

// dialogContainsCheckbox reports whether the dialog tree contains a checkbox
// with the given label.
func dialogContainsCheckbox(p tview.Primitive, label string) bool {
	return findCheckbox(p, label) != nil
}

// pressDialogButton walks the dialog tree for an UrwidButton with the given
// label and fires its selected function (the Enter/Space action).
func pressDialogButton(p tview.Primitive, label string) bool {
	var fired bool
	var walk func(p tview.Primitive)
	walk = func(p tview.Primitive) {
		if fired {
			return
		}
		switch v := p.(type) {
		case *DialogLineBox:
			walk(v.content)
		case *pileFiller:
			for _, it := range v.items {
				walk(it.widget)
			}
		case *urwidColumns:
			for _, child := range v.children {
				walk(child)
			}
		case *tview.Flex:
			for i := 0; i < v.GetItemCount(); i++ {
				walk(v.GetItem(i))
			}
		case *UrwidButton:
			if v.Label() == label && v.selected != nil {
				v.selected()
				fired = true
			}
		}
	}
	walk(p)
	return fired
}

// dialogHasButton reports whether a button with the given label exists in the
// dialog tree.
func dialogHasButton(p tview.Primitive, label string) bool {
	return findButton(p, label) != nil
}

// findButton walks a primitive tree for an UrwidButton with the given label.
func findButton(p tview.Primitive, label string) *UrwidButton {
	var found *UrwidButton
	var walk func(p tview.Primitive)
	walk = func(p tview.Primitive) {
		if found != nil {
			return
		}
		switch v := p.(type) {
		case *DialogLineBox:
			walk(v.content)
		case *pileFiller:
			for _, it := range v.items {
				walk(it.widget)
			}
		case *urwidColumns:
			for _, child := range v.children {
				walk(child)
			}
		case *tview.Flex:
			for i := 0; i < v.GetItemCount(); i++ {
				walk(v.GetItem(i))
			}
		case *UrwidButton:
			if v.Label() == label {
				found = v
			}
		}
	}
	walk(p)
	return found
}
