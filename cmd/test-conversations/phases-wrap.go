// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
)

// phases-wrap.go verifies the conversation composer's multiline word wrap on a
// live instance. The regression it guards was reported from the UI: as a long
// draft grew, the composer kept every character on one row, the caret ran off
// the right edge, and the user stopped seeing what they were typing — while the
// RRC channel composer, which uses the same ReadlineEdit multiline behavior,
// wrapped correctly. A wrapped draft therefore has to show its BEGINNING and
// its END at the same time, on different screen rows; a horizontally-scrolled
// single-line field shows only the end.

const (
	// wrapHead and wrapTail bracket the long draft. They are unique to this
	// phase so rowOf cannot match conversation body text.
	wrapHead = "WRAPSTART"
	wrapTail = "WRAPTAIL"
)

// wrapDraftText builds a draft comfortably longer than any pane width the
// harness runs at, bracketed by the two markers.
func wrapDraftText() string {
	return wrapHead + " " + strings.Repeat("alpha bravo ", 18) + wrapTail
}

// draftWrapped reports whether the long draft is live-wrapped: both markers are
// visible and they sit on different screen rows. The row difference is what
// separates wrapping from horizontal scrolling — a single-line field keeps only
// the caret's row of text, so wrapHead is scrolled off the left edge and the
// check fails.
func draftWrapped(v *utils.View) bool {
	head := rowOf(v, wrapHead)
	tail := rowOf(v, wrapTail)
	return head >= 0 && tail >= 0 && head != tail
}

// composerWrap opens a conversation, types a draft longer than the pane width
// and asserts it word-wraps onto several rows with the caret still on screen.
// It is self-contained (it opens its own conversation), so it can run wherever
// the phase list needs it.
func (h *harness) composerWrap() {
	h.logf("=== phase: composer word wrap ===")
	d := h.dB
	d.dismissDialog()
	if !d.openFirstConversation() {
		return
	}
	d.toEditorRegion()
	if !d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 3*time.Second, "editor region focused before the wrap check") {
		return
	}
	d.send("C-l") // readline kill-whole-buffer: start from an empty draft

	d.step("type a draft longer than the pane width")
	text := wrapDraftText()
	d.typeWrapped(text)
	d.snapshot("long draft in composer")
	d.logf("  marker rows: head=%v tail=%v cursor=(%v,%v,ok=%t)",
		rowOf(d.view(), wrapHead), rowOf(d.view(), wrapTail),
		d.view().CursorX, d.view().CursorY, d.view().CursorOK)

	d.assert(draftWrapped, 3*time.Second,
		"long draft is word-wrapped (head and tail visible on different rows)")
	d.assert(func(v *utils.View) bool { return v.CursorOK }, 3*time.Second,
		"caret is visible after typing a long draft")
	d.assert(func(v *utils.View) bool {
		head := rowOf(v, wrapHead)
		return v.CursorOK && head >= 0 && v.CursorY >= head
	}, 3*time.Second, "caret sits within the wrapped draft (not above it or off-screen)")

	// Leave the draft empty so the following phases and any manual inspection
	// start from a clean composer.
	d.send("C-l")
}

// typeWrapped types text into the focused composer and verifies it landed as a
// wrapped draft, retrying like typeIntoComposer (the first literal sent after a
// focus change can be swallowed). Each retry clears the partial draft with C-l
// (readline kill-whole-buffer — the binding Go and Python share).
func (d *driver) typeWrapped(text string) {
	for attempt := 1; attempt <= 3; attempt++ {
		d.logf("  type wrapped draft (attempt %v)", attempt)
		d.sendChunked(text)
		if d.assert(draftWrapped, 3*time.Second, "composer shows the wrapped draft (attempt %v)", attempt) {
			return
		}
		d.send("C-l")
		d.toEditorRegion()
	}
}
