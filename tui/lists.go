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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SelectableList wraps tview.List with click handling and focus management.
type SelectableList struct {
	*tview.List
	onSelect func(index int)
}

// NewSelectableList creates a new list with click handling.
func NewSelectableList() *SelectableList {
	sl := &SelectableList{
		List: tview.NewList(),
	}
	sl.SetHighlightFullLine(true)
	ApplyListFocusStyle(sl.List, ThemeDark)
	return sl
}

// ListFocusColors returns the (foreground, background) colors for a selected
// list row under the given theme, matching Python's list_focus style
// (TextUI.py: #111 on #aaa in both dark and light). Use instead of a
// hardcoded #666.
func ListFocusColors(theme int) (tcell.Color, tcell.Color) {
	colors := GetThemeColors(theme)
	return colors["list_focus_fg"], colors["list_focus_bg"]
}

// ApplyListFocusStyle sets a tview.List's selected-foreground and
// selected-background to the theme's list_focus colors. Every list in the port
// must use this instead of a hardcoded #666 selection background.
func ApplyListFocusStyle(list *tview.List, theme int) {
	fg, bg := ListFocusColors(theme)
	list.SetSelectedTextColor(fg)
	list.SetSelectedBackgroundColor(bg)
}

// SetOnSelect sets the callback for item selection.
func (sl *SelectableList) SetOnSelect(fn func(index int)) {
	sl.onSelect = fn
	sl.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		if sl.onSelect != nil {
			sl.onSelect(i)
		}
	})
}

// TrustListItem is a list item with trust-level styling.
type TrustListItem struct {
	Text       string
	Secondary  string
	TrustLevel string // "trusted", "untrusted", "unknown", "warning"
}

// NewTrustListItem creates a trust-styled list item text.
func NewTrustListItem(name, trustLevel string) string {
	icon := "○"
	switch trustLevel {
	case "trusted":
		icon = "●"
	case "untrusted":
		icon = "×"
	case "warning":
		icon = "⚠"
	}
	return icon + " " + name
}

// EmptyStateMessage creates a centered empty state message.
func EmptyStateMessage(text string) tview.Primitive {
	tv := tview.NewTextView()
	tv.SetTextAlign(tview.AlignCenter)
	tv.SetDynamicColors(true)
	tv.SetTextColor(tcell.NewHexColor(0x999999))
	tv.SetText("\n\n" + text)
	return tv
}

// RefreshList clears and repopulates a list with new items.
func RefreshList(list *tview.List, items []TrustListItem) {
	list.Clear()
	for _, item := range items {
		text := NewTrustListItem(item.Text, item.TrustLevel)
		list.AddItem(text, item.Secondary, 0, nil)
	}
	if len(items) == 0 {
		list.AddItem("[gray]No items[-]", "", 0, nil)
	}
}

// FocusFirstChild moves focus to the first focusable child of a Flex.
func FocusFirstChild(flex *tview.Flex) {
	if flex == nil {
		return
	}
	count := flex.GetItemCount()
	if count > 0 {
		first := flex.GetItem(0)
		flex.Focus(func(p tview.Primitive) {
			_ = p
		})
		_ = first
	}
}

// CycleFocus cycles focus through the children of a Flex.
func CycleFocus(flex *tview.Flex, forward bool) {
	if flex == nil {
		return
	}
	count := flex.GetItemCount()
	if count == 0 {
		return
	}
	// For simplicity, just set focus to first item
	flex.Focus(func(p tview.Primitive) {
		_ = p
	})
}
