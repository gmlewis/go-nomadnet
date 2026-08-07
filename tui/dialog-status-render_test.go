// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestShowStatusDialogRendersOKButton renders the status dialog to a screen
// and asserts the "OK" button is visible (the dismiss affordance the Python
// original has and the bare-text port lacked).
func TestShowStatusDialogRendersOKButton(t *testing.T) {
	app, _, _ := setupDialogTest(t)
	dm := app.Dialogs

	dm.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	if dm.Count() != 1 {
		t.Fatalf("count=%d want 1", dm.Count())
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(60, 14)
	screen.Init()
	root := dm.Pages()
	root.SetRect(0, 0, 60, 14)
	root.Draw(screen)
	screen.Show()

	var b strings.Builder
	for y := 0; y < 14; y++ {
		for x := 0; x < 60; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		b.WriteByte('\n')
	}
	out := b.String()
	t.Logf("rendered dialog:\n%s", out)

	if !strings.Contains(out, "Saved") {
		t.Errorf("render does not contain 'Saved'")
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("render does not contain the OK button — the dismiss affordance is missing")
	}
}