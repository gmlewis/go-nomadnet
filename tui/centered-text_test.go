// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Unit tests for centeredText primitive.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCenteredTextLeftPad(t *testing.T) {
	t.Parallel()

	ct := newCenteredText(tcell.ColorYellow, "Currently, no nodes are saved", "", "Ctrl+L to view the announce stream")
	if got := ct.GetText(); got != "Currently, no nodes are saved\n\nCtrl+L to view the announce stream" {
		t.Errorf("GetText = %q, want joined lines", got)
	}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("failed to init simulation screen: %v", err)
	}
	screen.SetSize(60, 10)

	ct.SetRect(0, 0, 60, 10)
	ct.Draw(screen)

	// Check line 0: "Currently, no nodes are saved" (29 chars) in width 60
	// Ceil-left pad: (60 - 29 + 1) / 2 = 16
	// First non-blank char on row 0 should be 'C' at x=16
	mainc, _, _, _ := screen.GetContent(16, 0)
	if mainc != 'C' {
		t.Errorf("cell (16, 0) = %q, want 'C'", mainc)
	}
}
