package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestEscProbeRealMainDisplay reproduces the reported bug: with the REAL
// MainDisplay as the root main page, opening a TextView-content status dialog
// (as the Local Peer "Saved" dialog does) and dispatching Esc through the root
// Pages InputHandler (the path tview.Application takes) — does it dismiss?
func TestEscProbeRealMainDisplay(t *testing.T) {
	app := newTestApp()
	md := NewMainDisplay(app, 0, "default")
	main := md.frame // *tview.Flex is the root Primitive
	pages := app.Dialogs.Init(app.Application, main)
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(main)

	// Mimic the "Saved" dialog the Local Peer Save handler shows: a bare
	// centered TextView, no OK button.
	body := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("\n\n\nSaved\n\n")
	app.Dialogs.ShowDialog("Saved", body, 40, 9, nil)
	if app.Dialogs.Count() != 1 {
		t.Fatalf("count=%d want 1", app.Dialogs.Count())
	}

	mainHas := main.HasFocus()
	top := app.Dialogs.stack[len(app.Dialogs.stack)-1]
	dlgHas := top.overlay.HasFocus()
	t.Logf("main.HasFocus=%v dialog-overlay.HasFocus=%v app.GetFocus=%T", mainHas, dlgHas, app.Application.GetFocus())

	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	if h := pages.InputHandler(); h != nil {
		h(esc, func(p tview.Primitive) { app.Application.SetFocus(p) })
	}

	if app.Dialogs.Count() != 0 {
		t.Errorf("REAL MainDisplay: Esc did NOT dismiss the dialog (count=%d) — main page steals the key", app.Dialogs.Count())
	} else {
		t.Logf("REAL MainDisplay: Esc dismissed OK")
	}
}
