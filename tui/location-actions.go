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
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/tview"
)

// ShowLocationActions opens the reader's actions for a `L location link: a card
// showing the Plus Code, its coordinate, and — when the client knows its own
// position — the distance and bearing to it, with buttons that copy the code or
// the coordinate.
//
// Copying always yields the code or the coordinate, never the rendered sentence,
// so a location copied out of a page stays something another client can use.
// This is a Go-only enhancement: Python nomadnet has no location construct at
// all, so a Python reader sees the bare code as ordinary text.
func (a *App) ShowLocationActions(code string) {
	if a == nil || a.Dialogs == nil {
		return
	}

	detail := micron.DescribeLocation(code, a.Viewer)
	card := locationCard(detail)
	infoRows := strings.Count(card, "\n") + 1

	info := NewUrwidLeftText(card)
	status := NewUrwidLeftText("")

	// Confirm a copy in place rather than stacking a second dialog: the buttons
	// keep the keyboard, and the reader can still copy the coordinate next.
	copyText := func(text string) {
		if a.clipboard != nil {
			a.clipboard.WriteText(text)
		}
		status.SetText(fmt.Sprintf("Copied %v to the clipboard", text))
	}

	copyCode := NewUrwidButton("Copy code").SetSelectedFunc(func() { copyText(detail.Code) })
	buttons := []*UrwidButton{copyCode}

	if detail.Coordinate != "" {
		copyCoordinate := NewUrwidButton("Copy coordinate").SetSelectedFunc(func() {
			copyText(detail.Coordinate)
		})
		buttons = append(buttons, copyCoordinate)
	}

	closeButton := NewUrwidButton("Close").SetSelectedFunc(func() {
		a.Dialogs.DismissTop()
	})
	buttons = append(buttons, closeButton)

	items := make([]tview.Primitive, 0, len(buttons))
	for _, b := range buttons {
		items = append(items, b)
	}

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, infoRows, 0, false).
		AddItem(status, 1, 0, false).
		AddItem(CreateUrwidButtonRow(buttons...), 1, 0, true)

	a.Dialogs.ShowDialog("Location", layout, 0, infoRows+4, nil)
	// wireDialogNav installs the per-button Tab/Shift-Tab traversal and makes
	// Escape dismiss, and focuses the first action.
	wireDialogNav(a, func() { a.Dialogs.DismissTop() }, items)
}

// locationCard renders the information a location link offers: the code the
// page wrote, its coordinate, and — when the client has a position — the
// distance and bearing to it. A missing position is stated rather than left
// blank, so a reader who expected guidance learns why there is none.
func locationCard(detail micron.LocationDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Code:       %v\n", detail.Code)
	if detail.Coordinate != "" {
		fmt.Fprintf(&b, "Coordinate: %v\n", detail.Coordinate)
	}
	if detail.HasFix {
		fmt.Fprintf(&b, "Distance:   %v\n", detail.Distance)
		fmt.Fprintf(&b, "Bearing:    %v\n", detail.Bearing)
	} else {
		b.WriteString("Distance:   (no position set)\n")
	}
	return b.String()
}
