// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
)

// phases-impl.go implements the harness phase methods called by runAllPhases.
//
// Navigation reuses the proven patterns from cmd/run-tmux-test-suite
// (escapeToMenu / moveMenuTo) plus a self-adapting focusActionButton loop that
// finds a named "< Button >" by sending Down/Tab until the cursor lands on it.
// Every assertion is non-fatal: a failure logs the observed state and the run
// continues, so a single log records the full sequence even when a step
// diverges. This is the harness's whole point — the log is the artifact for
// analyzing bugs/failures/regressions, so we never abort early.

// appStarted reports whether the menu bar is present (the app has finished its
// intro splash and is showing the main UI).
func appStarted() func(*utils.View) bool {
	return func(v *utils.View) bool {
		return v.Screen != nil && strings.Contains(v.Screen.RowText(0), "Conversations")
	}
}

// escapeToMenu moves focus from wherever it is (a body, the conversation list,
// a dialog) up to the menu bar, capped at maxSteps. Adapted from the proven
// cmd/run-tmux-test-suite escapeToMenu but trimmed to the pages this harness
// touches (Conversations / Network / Guide): Guide escapes via Up, Network via
// Home+Up, everything else via Escape (clears a stray dialog) then Up (climbs
// the pile toward the menu). A bounded loop so a stuck focus still returns.
func (d *driver) escapeToMenu(maxSteps int) {
	homeSent := false
	for range maxSteps {
		v := d.view()
		if _, ok := v.MenuFocusedButton(); ok {
			return
		}
		switch v.ActivePage() {
		case "guide":
			d.send("Up")
			homeSent = false
			continue
		case "network":
			if !homeSent {
				d.send("Home")
				homeSent = true
				continue
			}
			d.send("Up")
			continue
		}
		// Fallback (Conversations body/list, or a stray dialog): Escape
		// dismisses any open dialog (a no-op when none is open), then Up
		// climbs the left-pane pile toward the menu.
		d.send("Escape")
		d.send("Up")
	}
}

// moveMenuTo moves the menu focus to the target button index by sending Right
// (the menu wraps, so Right always advances) until MenuFocusedButton matches,
// capped at cap sends. Returns whether the target was reached.
func (d *driver) moveMenuTo(target, cap int) bool {
	for range cap {
		if idx, ok := d.view().MenuFocusedButton(); ok && idx == target {
			return true
		}
		d.send("Right")
	}
	idx, ok := d.view().MenuFocusedButton()
	return ok && idx == target
}

// focusActionButton sends Down (then Tab) until the named "< Button >" is
// focused (FocusedActionButton matches), capped. tview's flex/pile focus
// traversal between the list and the Local Peer button row is not fully
// deterministic across empty vs populated lists, so this loop adapts: Down
// advances through the list/pile and into the button row; if Down stalls, Tab
// cycles the focusable primitives. Returns whether the button was reached.
func (d *driver) focusActionButton(label string, cap int) bool {
	downBudget := max(cap, 6)
	tabBudget := 6
	for i := 0; i < downBudget+tabBudget; i++ {
		v := d.view()
		if btn, ok := v.FocusedActionButton(); ok && btn == label {
			return true
		}
		if i < downBudget {
			d.send("Down")
		} else {
			d.send("Tab")
		}
	}
	if btn, ok := d.view().FocusedActionButton(); ok && btn == label {
		return true
	}
	return false
}

// toPage escapes to the menu, moves to the given menu index, enters it, and
// asserts the named page is active, then drops focus into the body with Down
// (the menu keeps focus after Enter; Down moves it into the page body so
// page-level shortcuts like C-p/C-n reach the page's input handler). Returns
// whether the page was reached.
func (d *driver) toPage(idx int, page string) bool {
	d.escapeToMenu(40)
	if !d.moveMenuTo(idx, 10) {
		d.logf("  could not reach menu index %d", idx)
		return false
	}
	d.send("Enter")
	ok := d.assert(func(v *utils.View) bool { return v.ActivePage() == page }, 5*time.Second, "%s page active", page)
	if ok {
		d.send("Down") // menu -> body (so page shortcuts are reachable)
	}
	return ok
}

// toConversations navigates to the Conversations page (focus lands on the list,
// the left pane, so list-region shortcuts like C-p are immediately usable).
func (d *driver) toConversations() {
	d.step("navigate to Conversations")
	d.toPage(0, "conversations")
	d.snapshot("Conversations page")
}

// readOwnLXMFHash opens the C-p "LXMF Address" dialog, extracts the 32-hex hash
// from it, closes the dialog, and stores it on the driver. Returns the hash.
func (d *driver) readOwnLXMFHash() string {
	d.step("read own LXMF address (C-p)")
	d.send("C-p")
	hash := ""
	if d.assert(func(v *utils.View) bool { return dialogOpen(v, "LXMF Address") }, 5*time.Second, "LXMF Address dialog open") {
		hash = extractLXMFHash(d.view())
		d.assert(func(v *utils.View) bool { return len(hash) == 32 }, 1*time.Second, "own LXMF hash is 32 hex (got %q)", hash)
	}
	d.send("Escape")
	d.snapshot("after reading LXMF hash")
	d.lxmfHash = hash
	return hash
}

// boot waits for both apps to start, then navigates each to the Conversations
// page via the menu (the apps boot to the Guide on first run with a private
// empty config, so they must be moved to Conversations explicitly).
func (h *harness) boot() {
	for _, d := range []*driver{h.dA, h.dB} {
		d.step("boot: wait for app start")
		d.assert(appStarted(), 60*time.Second, "app started (menu bar visible)")
		d.toConversations()
	}
}

// announceAndAddresses reads each instance's own LXMF address off the C-p "My
// LXMF" dialog (the app boots to the Conversations page, so C-p works directly)
// and waits for announce + path propagation. Announcing itself is NOT driven
// via the UI: the app auto-announces at start (App.initRNS -> peer_announce_at
// start -> AnnounceNow after START_ANNOUNCE_DELAY), so both sides announce on
// their own; we just wait for those announces to propagate across the localhost
// TCP link so each side has a path to the other before messaging begins. The
// hashes are stored on the drivers for the cross-fed New Conversation step.
func (h *harness) announceAndAddresses() {
	for _, d := range []*driver{h.dA, h.dB} {
		d.step("read own LXMF address (C-p); auto-announce-at-start handles announce")
		d.readOwnLXMFHash()
	}
	h.logf("addresses: A=%s B=%s", h.dA.lxmfHash, h.dB.lxmfHash)
	// Wait for the announce-at-start packets to propagate across the TCP link
	// so each side has a path to the other before messaging begins.
	h.logf("waiting %s for announce-at-start + path propagation...", h.announceWait.String())
	time.Sleep(h.announceWait)
}

// createConversation opens the C-n "New Conversation" dialog, fills the peer's
// LXMF hash + a display name, and submits Create. After Create the dialog
// auto-dismisses and the conversation opens in the editor region. Returns
// whether the conversation editor is showing.
func (d *driver) createConversation(peerHash, name string) bool {
	d.stepf("new conversation with %s (%s)", peerHash, name)
	d.send("C-n")
	if !d.assert(func(v *utils.View) bool { return dialogOpen(v, "New Conversation") }, 5*time.Second, "New Conversation dialog open") {
		return false
	}
	// Focus starts on the Addr field. Type the hash, Tab to Name, type the name.
	d.sendLiteral(peerHash)
	d.send("Tab")
	d.sendLiteral(name)
	// Select the "Trusted" trust radio before Create. The dialog defaults to
	// "Unknown" trust (matching Python), and a conversation with an untrusted/
	// unknown peer holds inbound messages as "Unknown Origin" behind a
	// "This peer isn't trusted yet" prompt — they are neither ingested to disk nor
	// rendered promptly in the open conversation, which breaks the round-trip
	// receive assertion. Marking the peer Trusted makes inbound messages display
	// and ingest normally so the harness can observe delivery through the TUI.
	//
	// RadioButton.Draw does not ShowCursor, so the focused radio is invisible to
	// cursor-based detection; drive it deterministically by Tab count from the
	// Name field. The dialog's Tab order is fixed (wireDialogNav over
	// [Addr, Name, Untrusted, Unknown, Trusted, Create, Back]), so three Tabs
	// from Name lands on Trusted. Space checks it (urwid RadioButton toggle),
	// unchecking the others. Verify via the "(X) Trusted" glyph on screen.
	d.send("Tab") // Name -> Untrusted
	d.send("Tab") // Untrusted -> Unknown
	d.send("Tab") // Unknown -> Trusted
	d.send("Space")
	if !d.assert(func(v *utils.View) bool { return strings.Contains(v.Screen.FullText(), "(X) Trusted") }, 2*time.Second, "Trusted radio selected") {
		d.snapshot("trusted-radio-not-selected")
	}
	// Land on Create by Tabbing until FocusedActionButton matches.
	if !d.focusActionButton("Create", 8) {
		d.logf("  could not focus Create button; pressing Enter on current focus")
		d.snapshot("new-conv-before-enter")
	}
	d.send("Enter")
	// Create either dismisses the dialog (success) or re-opens it with an error
	// row (invalid hash). Success = the dialog is gone + the editor bar shows.
	ok := d.assert(func(v *utils.View) bool { return !dialogOpen(v, "New Conversation") }, 5*time.Second, "New Conversation dialog dismissed (Create accepted)")
	if ok {
		d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 3*time.Second, "conversation editor region showing")
	}
	d.snapshot("after create conversation")
	return ok
}

// typeAndSend types the message body in the composer editor and sends it with
// C-d, asserting the message body text appears in the conversation (the sent
// message row renders with its content).
func (d *driver) typeAndSend(text string) {
	d.stepf("send message %q", text)
	d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 3*time.Second, "editor region focused before typing")
	d.sendLiteral(text)
	d.send("C-d")
	d.assert(func(v *utils.View) bool { return messageBodyContains(v, text) }, d.msgWait, "sent message %q visible in body", text)
	d.snapshot("after send")
}

// waitForMessage waits up to timeout for the given text to appear in the
// currently-open conversation body (the receiver is already viewing the
// conversation created in the exchange step, so an inbound message lands
// directly in view — no list/tab navigation needed).
func (d *driver) waitForMessage(text string, timeout time.Duration) {
	d.stepf("wait for message %q", text)
	d.assert(func(v *utils.View) bool { return messageBodyContains(v, text) }, timeout, "received message %q visible in body", text)
	d.snapshot("after message received")
}

// firstMessageBtoA proves connectivity: both sides create a conversation with
// the OTHER's hash (so each has a directory entry + an open conversation), then
// B sends a message to A and A receives it in the already-open conversation.
func (h *harness) firstMessageBtoA() {
	h.logf("=== phase: first message B -> A ===")
	// Each creates a conversation with the other's hash. After Create, each is
	// viewing that conversation in the editor region.
	h.dA.createConversation(h.dB.lxmfHash, "Bob")
	h.dB.createConversation(h.dA.lxmfHash, "Alice")
	// B sends; A is already viewing the "Bob" conversation and receives it.
	h.dB.typeAndSend("hello-from-B")
	h.dA.waitForMessage("hello-from-B", h.msgWait)
}

// replyAtoB completes the round trip: A replies to B; B (already viewing the
// "Alice" conversation) receives it.
func (h *harness) replyAtoB() {
	h.logf("=== phase: reply A -> B ===")
	h.dA.typeAndSend("hi-from-A")
	h.dB.waitForMessage("hi-from-A", h.msgWait)
}

// headerStates attempts to exercise a message to a bogus (valid-format but
// unreachable) destination so its header enters the Failed state, then exercises
// C-u Purge from the body region. The whole phase is best-effort and log-only
// (no fail assertions): in the isolated 2-peer segment the only recalled
// identities are the two real peers (each has a path to the other), so a bogus
// all-zeros hash has NO recalled identity. With no recalled identity
// SendConversation returns false without creating/ingesting a message — exactly
// as Python's send_message does (Conversation.py:378-380 "Destination is not
// known, cannot create LXMF Message" → return False). So no failed header can
// be produced here; a real failed-header exercise needs a known-but-unreachable
// destination, which this 2-peer setup cannot construct. The phase still drives
// the New Conversation dialog with a bogus hash, attempts the send, and logs the
// observed state for analysis. Any dialog opened by a stray shortcut (e.g. C-u
// landing in the wrong region) is defensively dismissed so it cannot pollute
// the later in-conversation phase.
func (h *harness) headerStates() {
	h.logf("=== phase: header states (failed + purge) — best-effort, log-only ===")
	d := h.dB
	// Close any open conversation so focus is on the LIST, from which C-n (New
	// Conversation) fires. cd.handleInput gates C-n on shortcutFocus=="list"
	// (matching Python's ConversationsArea being the list column), so C-n from
	// the editor would pass through and never open the dialog. C-w returns focus
	// to the list via OnClose (Conversations.py:1677+1638-1639).
	d.send("C-w")
	d.toListRegion()
	d.step("send to bogus hash to observe failed header")
	// A 32-hex hash that is not either real peer: all 0s is valid format but has
	// no recalled identity, so the send is expected to be a no-op (see above).
	if d.createConversation("00000000000000000000000000000000", "Nobody") {
		// Attempt the send but do NOT assert it appears: with no recalled
		// identity for the bogus hash, Send returns false and nothing is
		// ingested (matching Python). Log the observed body for analysis.
		d.stepf("attempt send %q (expected no-op: bogus hash has no identity)", "are-you-there")
		d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 3*time.Second, "editor region focused before typing")
		d.sendLiteral("are-you-there")
		d.send("C-d")
		time.Sleep(2 * time.Second)
		d.snapshot("bogus-dest-after-send")
		if messageBodyContains(d.view(), "are-you-there") {
			d.logf("  bogus send unexpectedly ingested (are-you-there visible) — waiting for failed header")
			time.Sleep(h.msgWait)
			d.snapshot("bogus-dest-after-wait")
		} else {
			d.logf("  bogus send did not ingest (no identity) — failed-header state cannot be exercised in 2-peer setup; skipping purge assertion")
		}
		// C-u Purge is a body-region shortcut. Only attempt it if we actually
		// have a body (a message was sent); otherwise C-u in the wrong region
		// opens the Ingest URI dialog. Defensively dismiss any dialog afterward
		// so a stray one does not leak into the in-conversation phase.
		if messageBodyContains(d.view(), "are-you-there") {
			d.toBodyRegion()
			d.step("C-u purge failed from body region")
			d.send("C-u")
			d.snapshot("after C-u purge")
		}
		d.dismissDialog()
		d.dismissDialog()
	}
}

// listShortcuts exercises the list-region shortcuts (the conversations display
// InputCapture catches these regardless of the focused region, but we drive
// them from the list region for parity with user flow). Each shortcut is sent
// + snapshotted + loosely asserted where a clear signal exists; the log is the
// artifact for analysis.
func (h *harness) listShortcuts() {
	h.logf("=== phase: list-region shortcuts ===")
	d := h.dA
	// Close any open conversation so focus returns to the LIST (the conversation
	// widget's OnClose sets focus to the list, matching Python's
	// columns_widget.focus_position = 0, Conversations.py:1677+1638-1639). List-
	// region shortcuts (C-e Peer Info, C-p My LXMF, C-u Ingest URI, C-r Sync,
	// C-x Delete, C-o Sort) must be driven from the list pane: cd.handleInput
	// gates them on shortcutFocus=="list" (mirroring Python's ConversationsArea
	// being the list column), and the dual-meaning keys (C-p/C-u/C-x/C-o) only
	// deliver their list meaning when the list — not the editor/body — is focused.
	// Driving them from the editor would instead fire the editor/body meaning
	// (C-p Paper Msg, C-u Purge, C-x Clear History, C-o sort-by-timestamp). C-w
	// is a no-op when no conversation is open.
	d.send("C-w")
	d.toListRegion()

	d.step("C-e Peer Info")
	d.send("C-e")
	d.assert(func(v *utils.View) bool { return dialogOpen(v, "Peer Info") }, 5*time.Second, "Peer Info dialog open")
	d.snapshot("peer-info")
	d.dismissDialog()

	d.step("C-o Sort (toggle)")
	d.send("C-o")
	d.snapshot("after sort toggle")

	d.step("C-p My LXMF")
	d.send("C-p")
	d.assert(func(v *utils.View) bool { return dialogOpen(v, "LXMF Address") }, 5*time.Second, "LXMF Address dialog open")
	d.snapshot("my-lxmf")
	d.dismissDialog()

	d.step("C-g Fullscreen toggle")
	d.send("C-g")
	d.snapshot("fullscreen-on")
	d.send("C-g")
	d.snapshot("fullscreen-off")

	d.step("C-u Ingest URI (invalid)")
	d.send("C-u")
	d.assert(func(v *utils.View) bool { return dialogOpen(v, "Ingest URI") || dialogOpen(v, "URI") }, 5*time.Second, "Ingest URI dialog open")
	d.snapshot("ingest-uri-dialog")
	// Submit an invalid URI to exercise the error-result path.
	d.sendLiteral("not-a-valid-uri")
	d.send("Enter")
	d.snapshot("after invalid ingest submit")
	d.dismissDialog()

	d.step("C-r Sync")
	d.send("C-r")
	d.snapshot("sync-dialog")
	d.dismissDialog()

	d.step("C-x Delete (cancel)")
	// Cancel the delete dialog so we keep the conversation for later phases.
	d.send("C-x")
	d.snapshot("delete-dialog")
	d.dismissDialog()
}

// inConversationShortcuts exercises the editor + body region shortcuts while a
// conversation is open. Tab switches editor <-> body; C-t toggles the title
// editor; C-w closes the conversation; C-p opens the paper-message dialog;
// arrow/PgUp/PgDn/Home/End scroll the message list from body focus. Each is
// sent + snapshotted; assertions are loose where the signal is unambiguous.
func (h *harness) inConversationShortcuts() {
	h.logf("=== phase: in-conversation shortcuts ===")
	d := h.dB
	// Defensively dismiss any dialog left open by a prior phase (e.g. a stray
	// Ingest URI dialog from headerStates' C-u landing in the wrong region) so
	// it does not intercept the list navigation below.
	d.dismissDialog()
	// Re-open a conversation (it may have been closed/replaced by the headerStates
	// bogus conversation). openFirstConversation sends C-w to close any open
	// conversation → focus returns to the list → Home+Enter opens the first row,
	// so there is no need to toListRegion first (Tab from the editor only cycles
	// editor↔body and never reaches the list).
	d.openFirstConversation()

	d.step("C-t Title toggle")
	d.toEditorRegion()
	d.send("C-t")
	d.snapshot("title-editor-on")
	d.sendLiteral("Title1")
	d.send("C-d") // send with a title
	d.snapshot("after titled send")
	d.send("C-t") // toggle title editor off
	d.snapshot("title-editor-off")

	d.step("C-p Paper Msg (editor)")
	d.toEditorRegion()
	d.send("C-p")
	// Assert the Paper Message dialog opened, NOT the list-level "My LXMF"
	// (LXMF Address) dialog. C-p is region-aware in Python: list-focused →
	// show_my_qr (My LXMF, Conversations.py:103-104), editor-focused →
	// paper_message (Conversations.py:1811-1812). The Go display-level capture
	// must pass C-p through to the conversation widget when the editor is focused
	// (tui/conversations.go handleInput gates on shortcutFocus=="list"); without
	// that gate C-p from the editor wrongly opens "LXMF Address". Assert BOTH the
	// right dialog present and the wrong one absent so a regression is caught.
	d.assert(func(v *utils.View) bool { return dialogOpen(v, "Create Paper Message") }, 3*time.Second, "C-p from editor opened Paper Message dialog (not My LXMF)")
	d.assert(func(v *utils.View) bool { return !dialogOpen(v, "LXMF Address") }, 3*time.Second, "C-p from editor did not open My LXMF (LXMF Address) dialog")
	d.snapshot("paper-msg-dialog")
	d.dismissDialog()

	d.step("Tab editor <-> body")
	d.toEditorRegion()
	d.send("Tab")
	d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "body" }, 3*time.Second, "Tab moved focus to body region")
	d.snapshot("body-region")
	d.send("Tab")
	d.assert(func(v *utils.View) bool { return shortcutRegion(v) == "editor" }, 3*time.Second, "Tab moved focus back to editor region")

	d.step("body scrolling keys")
	d.toBodyRegion()
	for _, k := range []string{"Up", "Down", "PageUp", "PageDown", "Home", "End"} {
		d.send(k)
	}
	d.snapshot("after body scroll keys")

	d.step("C-w Close from body")
	d.toBodyRegion()
	d.send("C-w")
	d.snapshot("after C-w close")
}

// cleanup deletes the test conversations on both instances (best-effort). It is
// purely cosmetic: main.go's defers remove the temp config dirs (and every
// conversation stored in them) regardless, so leftover rows do not outlive the
// run. The editor→list navigation gap (Tab cycles editor↔body within the
// conversation frame and never escapes to the left list pane; Escape+Up from
// the editor does not reach the menu bar in tview the way urwid's Up-at-top
// does in Python) means we often cannot reach the list to C-x a conversation
// from here. So this makes ONE short, bounded probe for the list and, if it is
// not reached, gives up immediately rather than flailing Escape+Up for minutes
// — and crucially does NOT send C-x from the editor/body, where it would
// misfire as Clear History (the conversation-widget frame capture) instead of
// Delete Conversation.
func (h *harness) cleanup() {
	h.logf("=== phase: cleanup ===")
	for _, d := range []*driver{h.dA, h.dB} {
		d.step("cleanup: delete test conversations")
		d.toListRegion() // short Tab/Left probe; no-op if stuck in editor/body
		if shortcutRegion(d.view()) != "list" {
			d.logf("  could not reach list for cleanup (editor->list nav gap); temp dirs removed by defer")
			continue
		}
		for range 3 {
			d.send("Home")
			d.send("C-x")
			d.snapshot("delete-confirm")
			// Confirm delete if a dialog appeared: Tab to the confirm button + Enter.
			d.send("Tab")
			d.send("Enter")
			d.snapshot("after delete")
		}
	}
}
