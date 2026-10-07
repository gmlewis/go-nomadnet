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

// toRegion drives the conversations shortcut bar to the target region
// ("list"/"editor"/"body") with the keys the app actually binds, capped.
//
// The three regions form one directed cycle, so from any region the target is
// at most two steps away:
//
//	list --Right--> editor --Tab--> body --Left--> list
//
// Right/Left are urwid's Columns focus moves (Conversations.py:221-229); Tab is
// ConversationWidget.keypress "tab" → toggle_focus_area (:2233-2236). Sending
// an unconditional Tab/Left/Tab… sequence cannot work: Left inside the editor
// is a text-cursor move (ReadlineMixin), not a focus move. Best-effort — if the
// region is not reached, the caller's downstream assertion fails and logs the
// observed state.
func (d *driver) toRegion(region string, cap int) {
	for range cap {
		cur := shortcutRegion(d.view())
		if cur == region {
			return
		}
		switch cur {
		case "editor":
			// Tab leaves the composer for the message body.
			d.send("Tab")
		case "body":
			if region == "editor" {
				// Tab returns to the composer.
				d.send("Tab")
			} else {
				// Left is urwid's move to the list column.
				d.send("Left")
			}
		default: // "list"
			// Right moves into the conversation column (a no-op when no
			// conversation is open — the caller's next assert reports it).
			d.send("Right")
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
// snapshot-only).
//
// When no conversation is open (region is "list"), it opens the first row:
// navigate to the list → Home → Enter, which runs the list's open handler and
// lands focus on the composer (DisplayConversation's focusEditor).
func (d *driver) openFirstConversation() bool {
	d.step("open first conversation in list")
	if r := shortcutRegion(d.view()); r == "editor" || r == "body" {
		// A conversation is already open and focused — use it.
		d.snapshot("opened-first-conversation")
		return true
	}
	d.toListRegion()
	d.send("Home")
	d.send("Enter")
	ok := d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 4*time.Second, "first conversation opened (editor region)")
	d.snapshot("opened-first-conversation")
	return ok
}
