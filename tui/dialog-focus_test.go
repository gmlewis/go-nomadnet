// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestB5WireDialogNavHandlesTabAndDown verifies B5: the New Conversation
// dialog field-advance must support BOTH Tab (gonomadnet enhancement) and
// Down (nomadnet's key), matching the allowed enhancements rule
// ("gonomadnet ⊇ nomadnet's input set"). wireDialogNav should return nil
// (consume) for both KeyTab and KeyDown so focus advances to the next field.
func TestB5WireDialogNavHandlesTabAndDown(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	items := []tview.Primitive{
		tview.NewInputField(),
		tview.NewInputField(),
		tview.NewInputField(),
	}

	wireDialogNav(app, nil, items)

	// Each item should have an InputCapture that consumes Tab and Down.
	for i, item := range items {
		capture := getItemCapture(item)
		if capture == nil {
			t.Errorf("item %d: InputCapture is nil", i)
			continue
		}

		// Tab should be consumed (return nil) so focus advances.
		if got := capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
			t.Errorf("B5: item %d Tab was not consumed (returned non-nil) — Tab should advance fields", i)
		}

		// Down should be consumed (return nil) so focus advances.
		if got := capture(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)); got != nil {
			t.Errorf("B5: item %d Down was not consumed (returned non-nil) — Down should advance fields", i)
		}
	}
}
