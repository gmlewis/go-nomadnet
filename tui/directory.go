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
	"fmt"
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// DirectoryEntry holds peer information for the directory display.
type DirectoryEntry struct {
	SourceHash  string
	DisplayName string
	TrustLevel  string
	LastSeen    string
	Delivery    string
}

// DirectoryDisplay shows the peer directory with trust levels.
type DirectoryDisplay struct {
	app    *App
	widget *tview.Flex
	list   *tview.List
	detail *tview.TextView
}

// NewDirectoryDisplay creates a new directory display.
func NewDirectoryDisplay(app *App, entries []DirectoryEntry) *DirectoryDisplay {
	dd := &DirectoryDisplay{app: app}

	// Title
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]Directory[-]")

	// Peer list
	dd.list = tview.NewList()
	dd.list.SetHighlightFullLine(true)
	ApplyListFocusStyle(dd.list, app.Theme)

	for _, entry := range entries {
		icon := NewTrustListItem(entry.DisplayName, entry.TrustLevel)
		hashDisplay := entry.SourceHash
		if len(hashDisplay) > 8 {
			hashDisplay = hashDisplay[:8]
		}
		secondary := fmt.Sprintf("%s — %s", hashDisplay, entry.LastMessage())
		dd.list.AddItem(icon, secondary, 0, nil)
	}

	if len(entries) == 0 {
		dd.list.AddItem("[gray]No peers in directory[-]", "", 0, nil)
	}

	// Detail view
	dd.detail = tview.NewTextView()
	dd.detail.SetDynamicColors(true)
	dd.detail.SetScrollable(true)
	dd.detail.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	dd.detail.SetText("[gray]Select a peer to view details[-]")

	// Layout
	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(dd.list, 0, 1, true)
	content.AddItem(dd.detail, 0, 2, false)

	dd.widget = tview.NewFlex().SetDirection(tview.FlexRow)
	dd.widget.SetBorder(true)
	dd.widget.AddItem(title, 2, 0, false)
	dd.widget.AddItem(content, 0, 1, true)

	// Set up selection callback
	dd.list.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		if i < len(entries) {
			dd.showDetail(entries[i])
		}
	})

	return dd
}

// Widget returns the tview primitive for this display.
func (dd *DirectoryDisplay) Widget() tview.Primitive {
	return dd.widget
}

// showDetail shows the peer detail in the right panel.
func (dd *DirectoryDisplay) showDetail(entry DirectoryEntry) {
	trustColor := "[gray]"
	switch entry.TrustLevel {
	case "trusted":
		trustColor = "[green]"
	case "untrusted":
		trustColor = "[red]"
	}

	dd.detail.SetText(fmt.Sprintf(
		"[::b]%s[-]\n\nTrust: %s%s[-]\nHash: %s\nDelivery: %s\nLast seen: %s",
		entry.DisplayName,
		trustColor,
		entry.TrustLevel,
		entry.SourceHash,
		entry.Delivery,
		entry.LastSeen,
	))
}

// LastMessage returns a display string for the last seen time.
func (de *DirectoryEntry) LastMessage() string {
	if de.LastSeen != "" {
		return de.LastSeen
	}
	return "unknown"
}

// SortByTrust sorts directory entries by trust level (trusted first).
func SortByTrust(entries []DirectoryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		order := map[string]int{"trusted": 0, "unknown": 1, "untrusted": 2, "warning": 3}
		return order[entries[i].TrustLevel] < order[entries[j].TrustLevel]
	})
}

// SortByName sorts directory entries alphabetically by display name.
func SortByName(entries []DirectoryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DisplayName < entries[j].DisplayName
	})
}
