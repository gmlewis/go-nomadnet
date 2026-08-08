// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
)

// phases-nav.go holds the small navigation/focus helpers shared across phases:
// moving between the conversations list / editor / body regions, dismissing a
// dialog, and opening the first conversation in the list. Each is best-effort
// and bounded — the conversations layout's focus traversal (left list vs right
// detail, editor vs body) is driven by Tab/Left, and a bounded loop adapts to
// whichever region we happen to start in.

// stepf is step with formatting.
func (d *driver) stepf(format string, args ...any) {
	d.step(fmt.Sprintf(format, args...))
}

// toRegion sends Tab (and Left/Right as nudge) until the conversations shortcut
// bar reports the target region ("list"/"editor"/"body"), capped. Best-effort:
// if the region is not reached, the caller's downstream assertion will fail and
// log the observed state.
func (d *driver) toRegion(region string, cap int) {
	for i := range cap {
		if shortcutRegion(d.view()) == region {
			return
		}
		switch i % 3 {
		case 0, 1:
			d.send("Tab")
		case 2:
			// Nudge focus between the left list and right detail pane. Left is
			// harmless in the editor (moves the text cursor) but the Tab cycle
			// is the primary mechanism; Left is a fallback for body->list.
			d.send("Left")
		}
	}
}

func (d *driver) toListRegion()   { d.toRegion("list", 9) }
func (d *driver) toEditorRegion() { d.toRegion("editor", 9) }
func (d *driver) toBodyRegion()   { d.toRegion("body", 9) }

// dismissDialog closes any open dialog by sending Escape (tview dialogs dismiss
// on Esc). A couple of Escapes are sent to cover a nested/confirm dialog. Safe
// when no dialog is open (Esc is a no-op in the body).
func (d *driver) dismissDialog() {
	d.send("Escape")
	d.send("Escape")
}

// openFirstConversation ensures a conversation is open and returns whether one
// is. If a conversation is already open (editor or body region), it is used
// as-is — the in-conversation editor/body shortcut tests (C-p Paper Msg, Tab
// editor↔body, C-w Close, C-t Title) work on ANY open conversation and do not
// require a specific peer or a non-empty message list (body scrolling is
// snapshot-only). Re-opening from the list via Home+Enter is avoided when a
// conversation is already open: closing it first with C-w (whose OnClose
// returns focus to the list) and re-opening via list-Enter renders the
// conversation but the editor focus does not reliably stick
// (DisplayConversation.focusEditor's SetFocus(editor) does not take hold when
// the list just relinquished focus via C-w), leaving the shortcut bar on "list"
// and the editor/body assertions failing. Using the already-open conversation
// matches the proven path (the editor is already focused from the prior phase)
// and avoids that focus-timing gap.
//
// When no conversation is open (region is "list"), it opens the first row:
// C-w (no-op when none open) → list → Home → Enter.
func (d *driver) openFirstConversation() bool {
	d.step("open first conversation in list")
	if r := shortcutRegion(d.view()); r == "editor" || r == "body" {
		// A conversation is already open and focused — use it.
		d.snapshot("opened-first-conversation")
		return true
	}
	d.send("C-w") // no-op when no conversation is open
	d.toListRegion()
	d.send("Home")
	d.send("Enter")
	ok := d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 4*time.Second, "first conversation opened (editor region)")
	d.snapshot("opened-first-conversation")
	return ok
}
