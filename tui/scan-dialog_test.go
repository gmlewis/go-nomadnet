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

	"github.com/gmlewis/tview"
)

// dialogField is the part of a dialog's text field these tests read back. The
// two field types a dialog may use — ReadlineEdit and tview.InputField — both
// have GetText, so this covers either.
type dialogField interface {
	GetText() string
}

// findField walks a dialog's tree for the field carrying the given label.
func findField(p tview.Primitive, label string) dialogField {
	var found dialogField
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
		case *ReadlineEdit:
			if v.GetLabel() == label {
				found = v
			}
		case *tview.InputField:
			if v.GetLabel() == label {
				found = v
			}
		}
	}
	walk(p)
	return found
}

// firstRowIndex returns the index of the first row containing want, or -1.
func firstRowIndex(rows []string, want string) int {
	for i, row := range rows {
		if strings.Contains(row, want) {
			return i
		}
	}
	return -1
}

// TestNewConversationDialogOmitsScanButtonWithoutAScanner is the parity
// guarantee for this feature: on a client with no camera — a desktop terminal,
// the tmux sweeps, the render-diff harnesses — the New Conversation dialog must
// be exactly the one Python draws, with no scan affordance anywhere in it.
func TestNewConversationDialogOmitsScanButtonWithoutAScanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	if cd.listSlotOverlay == nil {
		t.Fatal("the New Conversation dialog did not open")
	}

	dialog := cd.listSlotOverlay.Dialog()
	if btn := findButton(dialog, "Scan"); btn != nil {
		t.Errorf("a client with no camera still offered a %q button", btn.Label())
	}
	for _, row := range contentRows(t, cd, 80, 24) {
		if strings.Contains(row, "Scan") {
			t.Errorf("a client with no camera still rendered a scan row: %q", row)
		}
	}
}

// TestNewConversationDialogScanButtonReadsAnAddress pins the feature itself:
// with an appliance that has a camera, the dialog offers "< Scan >" under the
// Addr field, and pressing it puts the address the camera read into that field.
func TestNewConversationDialogScanButtonReadsAnAddress(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner("lxmf@" + strings.ToUpper(validAddr)))
	app.updates = make(chan func(), 8)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	if cd.listSlotOverlay == nil {
		t.Fatal("the New Conversation dialog did not open")
	}

	dialog := cd.listSlotOverlay.Dialog()
	scanBtn := findButton(dialog, "Scan")
	if scanBtn == nil {
		t.Fatal("an appliance with a camera offered no scan button")
	}
	addr := findField(dialog, "Addr : ")
	if addr == nil {
		t.Fatal("the dialog has no Addr field")
	}

	// The scan row sits directly beneath the field it fills, which is what
	// makes it clear on a touch screen which field the camera will fill.
	rows := contentRows(t, cd, 80, 24)
	// The button pads its label out to the full row width, so the rendered row
	// begins "< Scan" and the bracket sits at the far end.
	addrRow, scanRow := firstRowIndex(rows, "Addr : "), firstRowIndex(rows, "< Scan")
	if addrRow < 0 || scanRow < 0 {
		t.Fatalf("addr row = %v, scan row = %v (rows: %v)", addrRow, scanRow, rows)
	}
	if scanRow != addrRow+1 {
		t.Errorf("the scan row is at %v and the Addr row at %v, want them adjacent", scanRow, addrRow)
	}

	pressScan(scanBtn)
	drainUpdate(t, app)()

	if got := addr.GetText(); got != validAddr {
		t.Errorf("the Addr field holds %q after a scan, want %q", got, validAddr)
	}
}

// TestNewConversationDialogScanRowCostsOneRow pins that turning the scanner on
// grows the dialog by exactly the scan row and nothing else: the rest of the
// dialog is the same dialog.
func TestNewConversationDialogScanRowCostsOneRow(t *testing.T) {
	t.Parallel()

	rows := func(withScanner bool) []string {
		app := newTestApp()
		if withScanner {
			app.SetScanner(newFakeScanner(validAddr))
		}
		cd := NewConversationsDisplay(app, nil)
		cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
		if cd.listSlotOverlay == nil {
			t.Fatal("the New Conversation dialog did not open")
		}
		return dialogRowTexts(cd.listSlotOverlay.Dialog())
	}

	without, with := rows(false), rows(true)
	if len(with) != len(without)+1 {
		t.Errorf("the dialog has %v rows with a scanner and %v without, want exactly one more", len(with), len(without))
	}
	// Everything but the scan row is unchanged, in the same order.
	stripped := make([]string, 0, len(with))
	for _, row := range with {
		if row == "< Scan >" {
			continue
		}
		stripped = append(stripped, row)
	}
	if strings.Join(stripped, "\n") != strings.Join(without, "\n") {
		t.Errorf("the dialog changed beyond the scan row:\n with: %v\nwithout: %v", stripped, without)
	}
}

// TestNewHubDialogScanButtonReadsAnAddress pins the same offer on the New Hub
// dialog, whose field takes a hub hash rather than a destination hash.
func TestNewHubDialogScanButtonReadsAnAddress(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner(validAddr))
	app.updates = make(chan func(), 8)
	cd := NewChannelsDisplay(app, nil)
	cd.NewHubDialog()
	if cd.dialogOverlay == nil {
		t.Fatal("the New Hub dialog did not open")
	}

	dialog := cd.dialogOverlay.Dialog()
	scanBtn := findButton(dialog, "Scan")
	if scanBtn == nil {
		t.Fatal("an appliance with a camera offered no scan button on the New Hub dialog")
	}
	hubField := findField(dialog, "Hub address : ")
	if hubField == nil {
		t.Fatal("the New Hub dialog has no Hub address field")
	}

	pressScan(scanBtn)
	drainUpdate(t, app)()

	if got := hubField.GetText(); got != validAddr {
		t.Errorf("the Hub address field holds %q after a scan, want %q", got, validAddr)
	}
}

// TestNewHubDialogOmitsScanButtonWithoutAScanner is the parity half of the
// above: Python's new_hub_dialog has four rows and an Add/Back row, and with no
// camera that is exactly what this dialog must render.
func TestNewHubDialogOmitsScanButtonWithoutAScanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.NewHubDialog()
	if cd.dialogOverlay == nil {
		t.Fatal("the New Hub dialog did not open")
	}

	dialog := cd.dialogOverlay.Dialog()
	if btn := findButton(dialog, "Scan"); btn != nil {
		t.Errorf("a client with no camera still offered a %q button", btn.Label())
	}
	for _, row := range dialogRowTexts(dialog) {
		if strings.Contains(row, "Scan") {
			t.Errorf("a client with no camera still rendered a scan row: %q", row)
		}
	}
}
