// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
)

// phases.go implements the 5-phase scripted test as a verified state machine.
// Every keystroke is followed by a Wait on a View condition (parsed screen +
// cursor position) that asserts the intended result, instead of the blind
// sleep+grep the bash predecessor used. Failures are logged but never fatal —
// the run continues through all phases so the log records the full sequence.

// driver holds the test run state and shared helpers.
type driver struct {
	sess         *utils.Session
	logf         func(format string, args ...any) // timestamped line writer
	stepDelay    time.Duration
	announceWait time.Duration
	connectWait  time.Duration
	nodeIters    int

	// visitedLinks is a per-SESSION (non-persistent) cache of every link the
	// suite has already followed during this run, keyed by the browser footer's
	// "Link to <target>" string (View.browserFooterLink). examineMainPage checks
	// it BEFORE pressing Enter so the suite never attempts to visit the same
	// link twice in one session — across nodes, many pages link to the same
	// destination (common hubs, shared indexes), and re-fetching them just to
	// C-d back wastes wall-clock and re-triggers transient fetch errors. The
	// cache lives for the whole test-suite run, not just one examineMainPage
	// call (the per-call `followed` map only dedupes within a single page).
	visitedLinks map[string]bool

	// counters for the end-of-run summary.
	asserts   int
	assertOK  int
	failures  int
	scrollBug int // guide topics where the scroll-reset bug was observed
}

// send sends tmux key names then pauses stepDelay for the app to redraw.
func (d *driver) send(keys ...string) {
	d.logf("send: %s", strings.Join(keys, " "))
	if err := d.sess.SendKeys(keys...); err != nil {
		d.logf("  ERROR send-keys: %v", err)
	}
	time.Sleep(d.stepDelay)
}

// snapshot logs the current pane (plain text) under a label.
func (d *driver) snapshot(label string) {
	out, err := d.sess.Capture()
	if err != nil {
		d.logf("===== SNAPSHOT: %s (capture error: %v) =====", label, err)
		return
	}
	d.logf("===== SNAPSHOT: %s =====\n%s===== END SNAPSHOT =====", label, out)
}

// view captures the current View, logging on error.
func (d *driver) view() *utils.View {
	v, err := d.sess.View()
	if err != nil {
		d.logf("  ERROR capturing view: %v", err)
		return &utils.View{}
	}
	return v
}

// waitFor polls until cond(v) is true or timeout. Returns the last view + ok.
func (d *driver) waitFor(cond func(*utils.View) bool, timeout time.Duration) (*utils.View, bool) {
	return d.sess.Wait(cond, timeout)
}

// assert waits up to timeout for cond(v), then logs PASS/FAIL with the observed
// state. Never fatal. Returns whether the condition held.
func (d *driver) assert(cond func(*utils.View) bool, timeout time.Duration, format string, args ...any) bool {
	d.asserts++
	v, ok := d.waitFor(cond, timeout)
	msg := fmt.Sprintf(format, args...)
	if ok {
		d.assertOK++
		d.logf("  ASSERT PASS: %s", msg)
		return true
	}
	d.failures++
	// Log the observed state to aid reproduction.
	observed := observedState(v)
	d.logf("  ASSERT FAIL: %s | observed: %s", msg, observed)
	return false
}

// observedState summarizes the view for failure logs.
func observedState(v *utils.View) string {
	if v == nil || v.Screen == nil {
		return "<no view>"
	}
	page := v.ActivePage()
	midx, mok := v.MenuFocusedButton()
	cur := ""
	if v.CursorOK {
		cur = fmt.Sprintf("cursor=(%d,%d)", v.CursorX, v.CursorY)
	} else {
		cur = "cursor=?"
	}
	btn := ""
	if b, ok := v.FocusedActionButton(); ok {
		btn = " actionBtn=" + b
	}
	addr := ""
	if a, ok := v.AnnounceInfoAddr(); ok {
		addr = " addr=" + a
	}
	st, url := v.BrowserState()
	return fmt.Sprintf("page=%q menu=%v(%t) %s browser=%s/%q%s%s", page, midx, mok, cur, st, url, btn, addr)
}

// step logs a phase/section header.
func (d *driver) step(msg string) {
	d.logf("\n########## %s ##########", msg)
}

// expectsTitle is a Wait condition for an Announce Stream / Saved Nodes /
// Announce Info / Guide border title.
func hasTitle(title string) func(*utils.View) bool {
	return func(v *utils.View) bool {
		if v.Screen == nil {
			return false
		}
		return utils.HasBorderTitle(v.Screen.FullText(), title)
	}
}

// menuAt is a Wait condition: the menu bar is focused (cursor on row 0) and the
// focused button is the given index.
func menuAt(idx int) func(*utils.View) bool {
	return func(v *utils.View) bool {
		i, ok := v.MenuFocusedButton()
		return ok && i == idx
	}
}

// menuFocused is a Wait condition: any menu button is focused (cursor on row 0).
func menuFocused() func(*utils.View) bool {
	return func(v *utils.View) bool {
		_, ok := v.MenuFocusedButton()
		return ok
	}
}

// enterAnnounceList moves focus from the Announce Stream tab bar / filter bar
// down onto the node list. After Phase 2's Ctrl-L, the Python original leaves
// focus on the tab bar (cursor ~3,2); the Go port is already on the list. This
// sends Down past the tab bar and filter bar (detected by their row text —
// "[ Nodes (N) ]", "Search:") until focus is on the list: 2 Downs for Python,
// 0 for Go (whose cursor is already on a Ⓝ row). It stops as soon as the
// cursor is on a node row OR the cursor has left the tab/filter rows (the ilb
// cursor may be hidden at (0,0) after a re-sort, but focus is on the list and
// Down/Enter still activate the focused node).
func (d *driver) enterAnnounceList(maxDowns int) bool {
	for range maxDowns {
		v := d.view()
		if v.CursorOnAnnounceNodeRow() {
			return true
		}
		if v.CursorOK && v.CursorY >= 0 && v.CursorY < v.Screen.H &&
			utils.IsAnnounceTabOrFilter(v.Screen.RowText(v.CursorY)) {
			d.send("Down")
			continue
		}
		// Cursor is past the tab/filter rows and not visibly on a node row —
		// assume focus is on the list (cursor hidden after a re-sort).
		return true
	}
	return d.view().CursorOnAnnounceNodeRow()
}

// escapeToMenu moves focus from wherever it is (browser content, the announce
// list, the tab/filter bar, a dialog) up to the menu bar. The caller asserts.
// Caps total steps so a stuck focus still returns.
//
// ROOT CAUSE (the "Save Node Info loop" bug): the Python AnnounceStream's node
// list is an IndicativeListBox built with modifier_key=MODIFIER_KEY.NONE, so
// MODIFIER_KEY.NONE.prepend_to("up") == "up" — plain Up IS the ilb's handled
// key. The ilb CONSUMES plain Up to move the SELECTION up the list
// (_pass_key_to_contained_listbox -> ListBox.keypress); it only returns "up"
// UNHANDLED when the first item is already selected (first_item_is_selected,
// Network.py:862/1790 + indicative_listbox.py:217). Only then does the
// AnnounceStream's internal Pile climb ilb -> filter -> tab, and the next Up at
// the tab bar sets frame.focus_position="header" (Network.py:448) — 3 Ups total.
// So Up from a MID-LIST selection (where the selection sits after Connect) moves
// the selection (consumed) and never climbs — exactly the 10-Up stuck loop in
// /tmp/nomadnet-tmux-test-suite-1785933157.log (cursor stayed (0,0), browser sig
// unchanged, menu never reached).
//
// FIX: send Home ONCE first. Home -> ilb select_first_item (indicative_listbox
// :217-221) moves the selection to the TOP (consumed when mid-list; a no-op that
// returns "home" unhandled when already at top). After Home the ilb is at the
// top, so the next plain Up returns "up" unhandled and the pile climbs
// ilb -> filter -> tab -> menu in 3 Ups. The homeSent flag prevents looping on
// Home (at the top it does nothing). Home is harmless on the filter/tab bars
// (Edit Home / no-op) and the local-peer panel (no-op), and Up still climbs from
// those, so the same Home+Up sequence escapes from any left-pane focus state.
func (d *driver) escapeToMenu(maxSteps int) {
	// homeSent tracks whether we've sent Home to move the AnnounceStream ilb's
	// selection to the top. Reset on any key that could move focus to a different
	// pile widget (Escape from Announce Info, Left from the browser, Up off the
	// local-peer panel) so Home is re-sent once focus is back on the ilb.
	homeSent := false
	for range maxSteps {
		v := d.view()
		if _, ok := v.MenuFocusedButton(); ok {
			return
		}
		// If an Announce Info is open, the cursor is trapped on its button row
		// (< Back > at ~(3,12)) and Up cycles the buttons without escaping. The
		// probe confirmed Escape closes Announce Info and returns focus to the
		// Announce Stream list (cursor hidden at (0,0)), from which Home+Up×3
		// reaches the menu. Handle this BEFORE the other checks, since the button
		// row is in the LEFT pane at a small cursor_x.
		if v.Screen != nil && utils.HasBorderTitle(v.Screen.FullText(), "Announce Info") {
			d.send("Escape")
			homeSent = false
			continue
		}
		// On the Guide page: the Topics list is a urwid LineBox(ilb). The phase4
		// loop always ends with Left so focus is on the Topics list here; Up
		// climbs the list to the menu (TopicList.keypress sends focus to the
		// header at the first item). Home is not needed (the Guide ilb's Up-at-top
		// already escapes).
		if v.ActivePage() == "guide" {
			d.send("Up")
			continue
		}
		// Local Peer < Save > / < Node Info > button focused (cursor on the
		// button row at ~(3,28)): Up moves the NetworkLeftPile focus from
		// local_peer (pos 1) up to the AnnounceStream (pos 0); then Home+Up
		// climbs to the menu. This is the "Save Node Info loop" exit path.
		if btn, ok := v.FocusedActionButton(); ok && (btn == "Save" || btn == "Node Info") {
			d.send("Up")
			homeSent = false
			continue
		}
		// Cursor genuinely in the right pane (the browser) and NOT on a node
		// row -> Left releases focus back to the left list (urwid Columns moves
		// to the adjacent selectable column on Left when the body returns the
		// key; a LinkableText at position 0 calls micron_released_focus ->
		// focus_lists). The node-row guard keeps the ilb's x≈135 cursor quirk
		// (cursor on the selected Ⓝ row) from being mistaken for the browser.
		if v.CursorOK && v.CursorX >= utils.NetworkLeftWidth && !v.CursorOnAnnounceNodeRow() {
			d.send("Left")
			homeSent = false
			continue
		}
		// On the Network left pane (AnnounceStream ilb with the cursor hidden at
		// (0,0) after Connect/Escape, OR on a node/tab/filter row): Home selects
		// the first item (top of the list), then Up×3 climbs the pile to the menu.
		// See the ROOT CAUSE note above for why Home is required.
		if v.ActivePage() == "network" {
			if !homeSent {
				d.send("Home")
				homeSent = true
				continue
			}
			d.send("Up")
			continue
		}
		// Fallback for any other page: Up climbs toward the menu.
		d.send("Up")
	}
}

// moveMenuTo moves the menu focus to the target button index by sending Right
// (the menu wraps, so Right always advances) until MenuFocusedButton matches,
// capped at `cap` sends. Returns whether the target was reached.
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

// appStarted waits for the menu bar to appear (the "[ Conversations ]" button).
func appStarted() func(*utils.View) bool {
	return func(v *utils.View) bool {
		if v.Screen == nil || len(v.Screen.Rows) == 0 {
			return false
		}
		return strings.Contains(v.Screen.RowText(0), "Conversations")
	}
}

// ---------- Phase 1: tour every main-menu command except Quit ----------

func (d *driver) phase1() {
	d.step("Phase 1: tour main-menu commands (Conversations..Guide, no Quit)")

	// Wait for the app to boot (go compile + intro splash) — menu bar visible.
	if _, ok := d.waitFor(appStarted(), 150*time.Second); !ok {
		d.logf("FATAL: app did not start within 150s (no menu bar)")
		d.snapshot("app-start-timeout")
		return
	}
	time.Sleep(d.stepDelay)
	d.snapshot("app started (main display)")

	// Boot focus is the body (Conversations list). Go to the top, then Up
	// escapes to the menu (bodyListAtTop -> FocusMenu); active page is index 0.
	d.send("Home")
	d.send("Up")
	d.assert(menuFocused(), 3*time.Second, "menu focused after Home+Up (cursor on row 0)")
	d.snapshot("menu reached (on Conversations, index 0)")

	// Enter activates the focused button and KEEPS focus in the menu, so we
	// can Right/Enter through all pages from here. Menu order:
	// 0 Conversations 1 Network 2 Channels 3 Log 4 Interfaces 5 Config 6 Guide.
	pageByKey := map[int]string{
		0: "conversations", 1: "network", 2: "channels", 3: "log",
		4: "interfaces", 5: "config", 6: "guide",
	}
	for idx := 0; idx <= 6; idx++ {
		if idx > 0 {
			d.send("Right")
			d.assert(menuAt(idx), 3*time.Second, "menu focused on index %d (%s)", idx, utils.MenuLabels[idx])
		}
		d.send("Enter")
		// Focus stays in the menu (cursor on row 0) after Enter.
		d.assert(menuAt(idx), 2*time.Second, "focus stayed in menu on index %d after Enter", idx)
		d.snapshot(fmt.Sprintf("page: %s (menu %d)", utils.MenuLabels[idx], idx))
		// Soft cross-check the rendered page where a reliable title exists.
		if key, has := pageByKey[idx]; has && key != "log" {
			v := d.view()
			if page := v.ActivePage(); page != "" && page != key {
				d.logf("  NOTE: ActivePage=%q, expected %q (soft check)", page, key)
			}
		}
	}
	d.logf("Phase 1 complete (menu toured, ended on Guide index 6)")
}

// ---------- Phase 2: Network, Down, Ctrl-L -> Announce Stream ----------

func (d *driver) phase2() {
	d.step("Phase 2: select Network, Down into body, Ctrl-L -> Announce Stream")

	// From Guide (6) go back to Network (1): five Lefts.
	for range 5 {
		d.send("Left")
	}
	d.assert(menuAt(1), 3*time.Second, "menu on Network (index 1) after 5 Lefts")
	d.send("Enter")
	d.assert(func(v *utils.View) bool { return v.ActivePage() == "network" }, 5*time.Second, "Network page active (Saved Nodes)")
	d.snapshot("Network selected (Saved Nodes)")

	// Down drops focus from the menu to the body.
	d.send("Down")
	d.assert(func(v *utils.View) bool {
		_, ok := v.MenuFocusedButton()
		return !ok // cursor left the menu row
	}, 3*time.Second, "focus left the menu (Down to body)")

	// Ctrl-L toggles Saved Nodes <-> Announce Stream.
	d.send("C-l")
	d.assert(hasTitle("Announce Stream"), 5*time.Second, "Announce Stream showing after Ctrl-L")
	d.snapshot("after Ctrl-L (Announce Stream)")

	// Enter the node list. After C-l the Python original leaves focus on the
	// tab bar (cursor ~3,2); the Go port is already on the list. send Down
	// past the tab/filter bar onto the node list (2 Downs for Python, 0 for
	// Go) so Phase 3 can Down/Enter nodes. The cursor flickers (the stream
	// re-sorts as announces arrive), so this is detected by tab/filter row
	// text, not the cursor.
	d.enterAnnounceList(6)
	d.snapshot("after entering announce list")

	d.logf("waiting up to %v for announcing nodes to populate...", d.announceWait)
	if d.assert(func(v *utils.View) bool { return v.HasAnnounceNode() }, d.announceWait, "announcing nodes present in the stream") {
		if r := d.view().SelectedAnnounceRow(); r != nil {
			d.logf("  announcing node selected: %q at row %d", r.Text, r.Y)
		}
	} else {
		d.logf("  no announcing nodes detected within %v; Phase 3 will record zero connects", d.announceWait)
	}
	d.snapshot("Announce Stream after announce-wait")
}

// ---------- Phase 3: connect to up to N nodes (verified) ----------

func (d *driver) phase3() {
	d.step(fmt.Sprintf("Phase 3: connect to nodes, render index.mu, follow links (x%d)", d.nodeIters))

	// tried is keyed by the node's Announce Info ADDRESS HASH — the only
	// robust identifier. The Python announce stream re-sorts as new announces
	// arrive and signals selection via the hardware cursor, which flickers
	// (the ilb cursor is sometimes hidden at (0,0) even while the list has
	// focus), so neither the row text nor the cursor reliably identifies the
	// selected node. The Announce Info address is plain text (content-based),
	// so keying on it avoids reconnects even when the list reorders. tried
	// covers both successes and failures so the suite never revisits a node.
	tried := map[string]bool{}
	success := 0
	attempts := 0
	maxAttempts := d.nodeIters * 5

	// Phase 2 already called enterAnnounceList, so focus is on the node list
	// (cursor may flicker, but Down/Enter activate the focused node regardless).
	for success < d.nodeIters && attempts < maxAttempts {
		attempts++
		d.logf("iteration: success=%d/%d attempts=%d/%d", success, d.nodeIters, attempts, maxAttempts)
		if !d.sess.HasSession() {
			d.logf("session died mid-Phase-3; aborting Phase 3")
			break
		}

		// 1. Advance to the next node. On iteration 1 we are on the top node;
		//    on later iterations the previous Left released the browser back to
		//    the list on the node we just connected, so Down moves to the next.
		if attempts > 1 {
			d.send("Down")
		}

		// 2. Enter -> Announce Info for the focused node. Focus is on the list,
		//    so Enter opens the Announce Info even when the cursor is hidden.
		d.send("Enter")
		opened := d.assert(func(v *utils.View) bool {
			_, ok := v.AnnounceInfoAddr()
			return ok && utils.HasBorderTitle(v.Screen.FullText(), "Announce Info")
		}, 6*time.Second, "Announce Info opened (attempt %d)", attempts)
		if !opened {
			d.logf("  no Announce Info (focus not on a node, or peer/pn entry); skipping")
			d.snapshot("phase3: no announce info")
			// Recover focus back to the NODE LIST. The most common cause is that
			// the previous Down overshot PAST the last list node into the Local
			// Peer panel (Enter there opens the "Save Node Info" dialog), or focus
			// is still in the browser. Reset robustly from any state: Escape
			// closes any open dialog (Save Node Info / Announce Info); Up climbs
			// Local Peer -> AnnounceStream list (or scrolls the browser up, a
			// no-op at its top); Home moves the list selection to the TOP so the
			// next Down moves to the 2nd node instead of overshooting again; Left
			// releases the browser -> list (a no-op if already on the list).
			d.send("Escape")
			d.send("Up")
			d.send("Home")
			d.send("Left")
			continue
		}
		targetHash := ""
		if hv := d.view(); hv != nil {
			targetHash, _ = hv.AnnounceInfoAddr()
		}
		d.snapshot(fmt.Sprintf("phase3: announce info opened (addr=%s)", targetHash))

		// Skip nodes we already tried this session — the list re-sorts, so a
		// Down can land on a previously-connected node.
		if targetHash != "" && tried[targetHash] {
			d.logf("  node %s already tried this session; skipping", targetHash)
			d.send("Escape")
			continue
		}
		if targetHash != "" {
			tried[targetHash] = true
		}

		// 3. Focus the Connect button (color-neutral; detected via cursor_x on
		//    the button row). Right moves Back->Connect. Only press Enter if
		//    Connect is actually focused, else we would activate Back.
		for range 4 {
			if bv := d.view(); bv != nil {
				if b, ok := bv.FocusedActionButton(); ok && b == "Connect" {
					break
				}
			}
			d.send("Right")
		}
		connectFocused := d.assert(func(v *utils.View) bool {
			b, ok := v.FocusedActionButton()
			return ok && b == "Connect"
		}, 3*time.Second, "Connect button focused (addr=%s)", targetHash)
		if !connectFocused {
			d.logf("  could not focus Connect; skipping")
			d.send("Escape")
			continue
		}
		d.send("Enter")

		// 4. VERIFY the connect — wait for a TERMINAL state and ONLY then
		//    proceed: either the target node's index.mu rendered (state ==
		//    rendered AND url == targetHash, proving navigation to THIS node
		//    and not a stale page), OR a connect error (link establishment
		//    failure / no path / request failed / timed out). Content-based, so
		//    it works for both ports regardless of cursor visibility.
		d.logf("  Connect pressed (target %q); waiting for render or failure...", targetHash)
		_, _ = d.waitFor(func(v *utils.View) bool {
			st, url := v.BrowserState()
			if strings.HasPrefix(st, "error:") {
				return true
			}
			return st == utils.BSRendered && targetHash != "" && url == targetHash
		}, d.connectWait+6*time.Second)
		st, url := d.view().BrowserState()

		if !(st == utils.BSRendered && targetHash != "" && url == targetHash) {
			switch {
			case strings.HasPrefix(st, "error:"):
				d.logf("  connect FAILED: %s; skipping node", st)
			case st == utils.BSRendered:
				d.logf("  connect FAILED: rendered page url=%q != target %q (stale page?); skipping", url, targetHash)
			default:
				d.logf("  connect TIMEOUT after %v (state=%q); skipping node", d.connectWait+6*time.Second, st)
			}
			d.snapshot("phase3: connect failed")
			// C-w disconnects the browser pane. BrowserPane.Disconnect (tui)
			// cancels the in-flight fetch, resets the pane to the centered
			// "Disconnected" view, AND releases focus back to the left list (it
			// fires OnReleaseFocus → networkDisplay.FocusLists). The release is
			// necessary because the Network Columns is self-managing and does
			// NOT do urwid-style column-arrow traversal, so a disconnected pane
			// (centered text, no LinkableText Left-at-start handler) would
			// otherwise trap focus and this recovery could never reopen the
			// next Announce Info. This must match the success-path recovery
			// below: C-w alone already lands focus on the left list, so send
			// ONLY C-w + Home — NOT a follow-up Left. Left is NOT a no-op here:
			// the left list column is not self-managing, so urwidColumns.moveFocus
			// wraps Left to the RIGHT (disconnected) pane, re-trapping focus and
			// freezing every later attempt at cursor=(<right pane>,<row>)
			// browser=disconnected (the "0 connects after one timeout" bug).
			d.send("C-w") // Disconnect to reset the browser pane before the next node.
			time.Sleep(d.stepDelay)
			d.send("Home") // List selection -> top so the next Down can't overshoot into the Local Peer panel.
			continue
		}

		d.logf("  page RENDERED OK (url=%q)", url)
		d.snapshot("phase3: node index.mu rendered")
		success++

		// 5. Thoroughly examine the node's main page. After Connect the Python
		//    reference returns focus to the Announce Stream LIST (probe-confirmed:
		//    a stray Enter on the list re-opens the Announce Info), NOT to the
		//    browser. So we MUST send Right to move focus list->browser before
		//    walking the page, else the first Enter re-opens the Announce Info and
		//    traps the cursor on its button row (the Go port has the same
		//    list-focus behavior after Connect — R-NET-FOCUS-TRAP). Right from the
		//    list lands in the browser (cursor hidden at (0,0); the browser is a
		//    urwid Text/Filler with no visible cursor).
		d.assert(func(v *utils.View) bool {
			// The Announce Info dialog closed and the browser is active: the
			// Announce Info border title is gone.
			return !utils.HasBorderTitle(v.Screen.FullText(), "Announce Info")
		}, 3*time.Second, "Announce Info closed after Connect (browser active)")
		d.snapshot("phase3: browser pane (before thorough walk)")
		d.send("Right") // list -> browser (cursor at the top of the main page)

		// 5. Thoroughly examine the node's main page in ONE pass: walk Down
		//    through the whole page (snapshotting each screenful for full-page
		//    regression coverage) and follow every position-0 hyperlink as it
		//    comes under the cursor. Link detection is by the browser footer
		//    ("Link to <target>"), so linkless pages get a single clean walk with
		//    no Enters — the efficient one-pass the user asked for (the old
		//    two-pass walk-then-rejog was O(N²) and scanned the page twice).
		d.examineMainPage(targetHash, 160)

		// 6. Release the browser back to the list for the next iteration. C-w
		//    disconnects the browser pane, which (via BrowserPane.Disconnect ->
		//    OnReleaseFocus -> networkDisplay.FocusLists) reliably returns focus
		//    to the left list REGARDLESS of where the walk left the browser cursor.
		//    The previous recovery sent a single Left, but Left only releases
		//    focus at a line's START (Python delegate.micron_released_focus fires on
		//    Left-at-start, MicronParser.py:972-974) — and the walk frequently ends
		//    mid-line (e.g. cursor x=52 resting on a link), so Left just moved
		//    within the line and focus stayed trapped in the browser. The next
		//    iteration's Down+Enter then acted on the stale browser page instead
		//    of the list, so no Announce Info opened, no Connect happened, and the
		//    browser stayed on the prior node's stale page (looked "blank" with no
		//    "Retrieving" on the next Connect). C-w is the same reliable release
		//    the connect-FAILED path uses below. Home then moves the list
		//    selection to the TOP so the next Down advances to the 2nd node —
		//    preventing the overshoot into the Local Peer panel (Enter on < Save >
		//    opens "Save Node Info") that happened when a connected node sat at the
		//    BOTTOM of the list and Down moved past the last node.
		d.send("C-w")  // disconnect -> releases focus to the left list (cursor-position independent)
		d.send("Home") // list selection -> top so the next Down can't overshoot into Local Peer
	}
	d.logf("Phase 3 complete: %d/%d successful connects in %d attempts (%d distinct nodes tried)",
		success, d.nodeIters, attempts, len(tried))
}

// walkToBottom presses Down through a Scrollable page (the browser main page
// or the Guide reader), snapshotting each time the pane content scrolls (a new
// screenful becomes visible), until it reaches the bottom — so every screenful
// is rendered and snapshotted, exposing rendering regressions across the full
// page. Returns the number of Downs pressed. See phase3 step 5 and phase4.
//
// Bottom detection: urwid's Scrollable (force_forward_keypress) first moves the
// Pile focus line by line WITHIN the visible area — the cursor y advances but
// the content does NOT scroll — and only scrolls once the focus reaches the
// visible-area bottom. So content-signature stability alone is NOT a reliable
// bottom signal (it fires while the cursor is still moving within the visible
// area). We therefore require BOTH the pane signature AND the cursor y to be
// stable for K consecutive Downs. The LinkableText cursor shows within ~2s of
// the last keypress, so `step` must stay well under 2s. maxSteps is the safety
// net.
//
// Some pages have NO selectable widgets (e.g. the Introduction Guide topic is
// plain text) — there is no cursor to track and no Phase 1 focus movement, so
// Down either scrolls immediately or does nothing. For those pages the cursor
// is never seen (cursorEverSeen stays false), and sigStuck >= K alone is a
// valid bottom (a short page: Down does nothing from the first step; a long
// page: Down scrolls each step so sigStuck only accumulates at the real
// bottom). When the cursor IS seen we additionally require cursorStuck >= K so
// Phase 1 focus movement (sig stable, cursor advancing) does not false-bottom.
func (d *driver) walkToBottom(sig func(v *utils.View) string, maxSteps int, step time.Duration, label string) int {
	const K = 4
	v := d.view()
	// hardStuck is the sig-stuck fallback threshold: if the content signature
	// is unchanged for more than a full screen of consecutive Downs, the page
	// is at its bottom regardless of the cursor. This CANNOT false-bottom
	// during Phase-1 focus movement (before the first scroll): urwid moves
	// Pile focus through the selectable widgets currently in view, and there
	// are at most (visible rows) < H of those before the next Down scrolls —
	// so sigStuck can never reach H while there is still content to scroll.
	// It guarantees a bottom for pages whose final screenful is dense with
	// selectable links/fields (e.g. Guide topic 7, "Markup"): there the cursor
	// keeps stepping between widgets so cursorStuck never reaches K, but Down
	// stops changing the content, so sigStuck climbs past H and bottoms out.
	hardStuck := max(v.Screen.H, K+1)
	prevSig := sig(v)
	prevCY := -1
	sigStuck, cursorStuck := 0, 0
	cursorEverSeen := false
	screenfuls := 0
	for i := range maxSteps {
		if err := d.sess.SendKeys("Down"); err != nil {
			d.logf("  ERROR walk send-keys: %v", err)
		}
		time.Sleep(step)
		v = d.view()
		s := sig(v)
		if s != prevSig {
			sigStuck = 0
			screenfuls++
			// Snapshot every 5th screenful (and the first). The reader/browser
			// visible window is ~25 rows and scroll steps 1 row per Down, so a
			// 5-row stride still captures every line in some snapshot (with
			// overlap) — full regression coverage without flooding the log.
			if screenfuls == 1 || screenfuls%5 == 0 {
				d.snapshot(fmt.Sprintf("%s: screenful %d scrolled into view", label, screenfuls))
			}
		} else {
			sigStuck++
		}
		prevSig = s
		if v.CursorOK && v.CursorY > 0 {
			cursorEverSeen = true
			if prevCY > 0 && v.CursorY == prevCY {
				cursorStuck++
			} else {
				cursorStuck = 0
			}
			prevCY = v.CursorY
		}
		if sigStuck >= K && (cursorStuck >= K || !cursorEverSeen) {
			d.logf("  %s: bottom reached after %d downs (%d screenfuls)", label, i+1, screenfuls)
			return i + 1
		}
		if sigStuck >= hardStuck {
			d.logf("  %s: bottom reached after %d downs (%d screenfuls, sig-stuck fallback)",
				label, i+1, screenfuls)
			return i + 1
		}
	}
	d.logf("  %s: max %d downs without a clean bottom signal (%d screenfuls)",
		label, maxSteps, screenfuls)
	return maxSteps
}

// examineMainPage walks Down through the node's whole main page in ONE pass,
// snapshotting each screenful as it scrolls into view (full-page regression
// coverage) AND following every position-0 hyperlink as it comes under the
// cursor. This replaces the old two-pass walk-then-rejog (Pass 1 walkToBottom +
// Pass 2 followLinksFromTop) the user found unbearably slow: that was O(N²)
// (jog from the top to every line and back) and scanned the page twice. This is
// a single O(N) walk.
//
// Link detection is by the browser footer, not by pressing Enter on every line:
// the Python browser renders "Link to <target>" in its footer when the cursor
// rests on a link (LinkableText.peek_link -> marked_link_job, ~100ms after a
// keypress — see browserFooterLink). So we only press Enter on link-bearing
// lines. Linkless pages (the common case — e.g. PARTEI, MKS, Santino) get a
// single clean walk with NO Enters at all.
//
// Following a link navigates to the destination; Python's Browser.back (C-d)
// RE-FETCHES the page with a fresh Scrollable at offset 0, so after a link we
// C-d back to the TOP and jog Down `curLine` to resume the walk at the line we
// left. The jog-back is outside this loop, so it does not perturb the
// bottom-detection counters. Dedup is by the destination content signature
// (same-node /page/x.mu links return url == mainHash, so the URL is not a usable
// dedup key — see followLinksFromTop's note).
//
// Bottom detection is the walkToBottom rule, applied on every forward Down: sig
// stable for K consecutive Downs AND (cursor stable for K, OR cursor never
// seen), with the hard sigStuck >= Screen.H fallback for pages whose final
// screenful is dense with selectable links/fields.
func (d *driver) examineMainPage(mainHash string, maxSteps int) {
	const K = 4
	followed := map[string]bool{}
	v := d.view()
	hardStuck := max(v.Screen.H, K+1)
	prevSig := v.BrowserPaneSig()
	prevCY := -1
	sigStuck, cursorStuck := 0, 0
	cursorEverSeen := false
	screenfuls := 0
	curLine := 0
	linksFollowed := 0
	for i := range maxSteps {
		if err := d.sess.SendKeys("Down"); err != nil {
			d.logf("  ERROR walk send-keys: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
		curLine++
		v = d.view()
		s := v.BrowserPaneSig()
		if s != prevSig {
			sigStuck = 0
			screenfuls++
			if screenfuls == 1 || screenfuls%5 == 0 {
				d.snapshot(fmt.Sprintf("main page: screenful %d scrolled into view", screenfuls))
			}
		} else {
			sigStuck++
		}
		prevSig = s
		if v.CursorOK && v.CursorY > 0 {
			cursorEverSeen = true
			if prevCY > 0 && v.CursorY == prevCY {
				cursorStuck++
			} else {
				cursorStuck = 0
			}
			prevCY = v.CursorY
		}
		if sigStuck >= K && (cursorStuck >= K || !cursorEverSeen) {
			d.logf("  main page: bottom reached after %d downs (%d screenfuls)", i+1, screenfuls)
			break
		}
		if sigStuck >= hardStuck {
			d.logf("  main page: bottom reached after %d downs (%d screenfuls, sig-stuck fallback)", i+1, screenfuls)
			break
		}

		// Follow a position-0 link if the cursor is resting on one (browser
		// footer shows "Link to"). Linkless lines get no Enter — the walk just
		// continues Down.
		if lt := v.BrowserFooterLink(); lt != "" {
			// Per-session visited-links cache (the user-requested non-persistent
			// cache): never attempt the same link twice during one suite run. Many
			// nodes link to the same shared destinations (common hubs, shared
			// indexes); re-fetching a destination only to C-d back wastes wall-clock
			// and re-triggers transient fetch errors. Keyed by the footer "Link to
			// <target>" string, checked BEFORE Enter so we don't even attempt a
			// re-visit; recorded on attempt so a link that errored is not retried.
			// (The per-call `followed` map below still dedupes within one page.)
			if d.visitedLinks[lt] {
				d.logf("  link line %d (%q): already visited this session; skipping", curLine, lt)
				continue
			}
			d.visitedLinks[lt] = true
			sigBefore := s
			d.send("Enter")
			_, ok := d.waitFor(func(vv *utils.View) bool {
				st, _ := vv.BrowserState()
				return st == utils.BSRendered || strings.HasPrefix(st, "error:")
			}, d.connectWait)
			if !ok {
				d.logf("  link line %d (%q): no render/error within %v; back to main", curLine, lt, d.connectWait)
				// Only navigate back if Enter actually left the main page (sig
				// changed). A no-op Enter on a non-link line leaves sig unchanged,
				// so C-d would needlessly navigate AWAY from the main page.
				if d.view().BrowserPaneSig() != sigBefore {
					d.send("C-d")
					d.settleMain(mainHash)
					d.jogKeys(curLine, "Down")
				}
				prevSig = d.view().BrowserPaneSig()
				continue
			}
			lv := d.view()
			st, url := lv.BrowserState()
			if strings.HasPrefix(st, "error:") {
				d.logf("  link line %d (%q): error %q; back to main", curLine, lt, st)
				d.snapshot(fmt.Sprintf("phase3: link line %d error", curLine))
				if lv.BrowserPaneSig() != sigBefore {
					d.send("C-d")
					d.settleMain(mainHash)
					d.jogKeys(curLine, "Down")
				}
				prevSig = d.view().BrowserPaneSig()
				continue
			}
			sigAfter := lv.BrowserPaneSig()
			// Only count + return if Enter actually navigated (sig changed). A
			// false-positive footer (e.g. body text "Link to ") leaves sig
			// unchanged — Enter was a no-op, so we must NOT C-d back (that would
			// navigate away from the main page and leave the browser mid-retrieval
			// for the next Phase-3 iteration).
			if sigAfter != sigBefore {
				if !followed[sigAfter] {
					followed[sigAfter] = true
					linksFollowed++
					d.logf("  link line %d: followed link -> %q", curLine, url)
					d.snapshot(fmt.Sprintf("phase3: link -> %s", url))
				}
				// Return to the main page and jog Down curLine to resume the walk.
				// (C-d re-fetches at offset 0, so we must jog back from the top.)
				d.send("C-d")
				d.settleMain(mainHash)
				d.jogKeys(curLine, "Down")
			}
			prevSig = d.view().BrowserPaneSig()
		}
	}
	d.logf("  examined main page: %d downs, %d screenfuls, %d distinct links followed (%d links in session cache)",
		curLine, screenfuls, linksFollowed, len(d.visitedLinks))
}

// followLinksFromTop followed every hyperlink on the main page by jogging from
// the top to each line, pressing Enter, and jogging back — an O(N²) two-pass
// scan the user found unbearably slow. It has been replaced by the one-pass
// examineMainPage (link detection via the browser footer, so linkless pages get
// a single clean walk with no Enters). The navigation/dedup notes below still
// apply to examineMainPage: navigation is detected by a CHANGE in the browser
// pane content signature after Enter (NOT the URL — browserURL strips the path,
// so a same-node /page/x.mu link returns url == mainHash), and dedup is by the
// destination signature (same-node pages share url == mainHash). Python's
// Browser.back (C-d) RE-FETCHES the page at offset 0, so after a link we jog
// Down curLine to resume the walk.

// settleMain waits for the main page to come back after a Back (C-d), confirmed
// by the browser URL bar showing the main page hash again.
func (d *driver) settleMain(mainHash string) {
	d.waitFor(func(v *utils.View) bool {
		st, url := v.BrowserState()
		return st == utils.BSRendered && mainHash != "" && url == mainHash
	}, d.connectWait)
}

// jogKeys sends `n` copies of `key` in one tmux send-keys call, then settles.
// Used by examineMainPage to jog Down curLine to resume the walk after a link
// follow's C-d reset the page to offset 0 (we only verify state after Enter,
// not after each Down).
func (d *driver) jogKeys(n int, key string) {
	if n <= 0 {
		return
	}
	d.logf("jog: %d x %s", n, key)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = key
	}
	if err := d.sess.SendKeys(keys...); err != nil {
		d.logf("  ERROR jog send-keys: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
}

// guideTopicTitles is the first rendered line (the `>Title` heading) of each
// Guide topic in guideTopics order (tui/guide.go:83-95), used to verify each
// topic actually rendered and to detect the scroll-reset bug.
var guideTopicTitles = []string{
	"Nomad Network",                         // 0 Introduction
	"Concepts and Terminology",              // 1 Concepts & Terminology
	"Channels & RRC",                        // 2 Channels & RRC
	"Interfaces",                            // 3 Interfaces
	"Hosting a Node",                        // 4 Hosting a Node
	"Configuration Options",                 // 5 Configuration Options
	"Keyboard Shortcuts",                    // 6 Keyboard Shortcuts
	"Outputting Formatted Text",             // 7 Markup
	"First Time Information",                // 8 First Run
	"Network Configuration",                 // 9 Network Configuration
	"Markup & Color Display Test",           // 10 Display Test
	"Thanks, Acknowledgements and Licenses", // 11 Credits & Licenses
}

// ---------- Phase 4: Guide — walk all 12 topics, exercise the scroll bug ----------

func (d *driver) phase4() {
	d.step("Phase 4: Guide — select each topic, move right, scroll to bottom")

	if !d.sess.HasSession() {
		d.logf("session died before Phase 4")
		return
	}

	// From wherever Phase 3 left focus (the Announce Stream list, or the browser
	// content if the last connect failed and released focus via C-w), escape up
	// to the menu. The blind script sent a fixed number of Ups — which only
	// scrolled the browser when focus was stuck there, so it never reached the
	// menu and walked 12 "topics" on the Network page. escapeToMenu sends Left
	// to release browser focus back to a list FIRST, then Up to escape the list
	// to the menu.
	d.escapeToMenu(15)
	d.assert(menuFocused(), 3*time.Second, "menu reached from Announce Stream (cursor on row 0)")
	d.snapshot("menu reached from Announce Stream")

	// The active page is Network (index 1). Right to Guide (index 6), Enter,
	// then Down drops to the body (topic list, focused on item 0). moveMenuTo is
	// robust to the menu landing on any index after the escape.
	d.moveMenuTo(6, 8)
	d.assert(menuAt(6), 3*time.Second, "menu on Guide (index 6)")
	d.send("Enter")
	d.assert(func(v *utils.View) bool { return v.ActivePage() == "guide" }, 5*time.Second, "Guide page active (Topics title present)")
	d.snapshot("Guide selected")
	d.send("Down")
	d.assert(func(v *utils.View) bool {
		return utils.HasBorderTitle(v.Screen.FullText(), "Topics")
	}, 3*time.Second, "Guide topic list visible (Topics box)")
	d.snapshot("Guide topic list (item 0, Introduction)")

	// Select topic 0 via Enter. In the Python reference, a topic ListEntry
	// emits "click" on Enter -> display_topic -> set_content_widgets builds a
	// FRESH Scrollable (scroll offset reset to 0) AND focus_reader() moves
	// focus to the reader column. So after Enter, focus is in the reader and
	// the topic is at the top. Down alone does NOT render (no click signal),
	// so each topic needs its own Enter.
	d.send("Enter")
	d.snapshot("Guide topic 0 (Introduction) selected")

	// For each topic: focus is in the reader (from this iteration's Enter, or
	// the pre-loop Enter for topic 0), showing the topic at the top (Python
	// resets the offset; the Go port's known bug leaks it, so the first
	// visible line is mid-content instead of the title). Capture the FIRST
	// visible content line (GuideTopicRendered skips the reader's border) and
	// compare to the expected title. Then Home (ScrollToBeginning, correct) to
	// contrast buggy-vs-correct; walkToBottom scrolls through the FULL topic
	// content (every screenful, for regression coverage) and leaves the reader
	// at the bottom (sets up the next topic's leak test); Left to release
	// focus back to the Topics list; Down to the next topic + Enter to render.
	for topic := 0; topic <= 11; topic++ {
		expected := guideTopicTitles[topic]
		time.Sleep(300 * time.Millisecond)
		v := d.view()
		rendered := v.GuideTopicRendered()
		d.snapshot(fmt.Sprintf("Guide topic %d reader right after selection (BUG CHECK: top should be %q)", topic, expected))
		// Only count a scroll-reset bug when we are ACTUALLY on the Guide page.
		// The blind script counted bogus hits because it never verified it was
		// on the Guide (it read the browser border title as the Guide reader
		// while stranded on the Network page). If focus never reached the
		// Guide, log a NOTE instead of incrementing the bug counter.
		if v.ActivePage() != "guide" {
			d.logf("  topic %d: NOT on Guide page (active=%q) — not counting a scroll-reset bug; top is %q",
				topic, v.ActivePage(), rendered)
		} else if strings.TrimSpace(rendered) == expected {
			d.logf("  topic %d: scroll at row 0 (title %q at top) — no leak", topic, expected)
		} else {
			d.scrollBug++
			d.logf("  topic %d: SCROLL-RESET BUG — top is %q, expected %q (offset leaked from previous topic)", topic, rendered, expected)
		}

		// Home resets to the top (correct behavior); re-verify the title.
		d.send("Home")
		time.Sleep(300 * time.Millisecond)
		v = d.view()
		if home := v.GuideTopicRendered(); strings.TrimSpace(home) == expected {
			d.logf("  topic %d: after Home, title %q at top (correct)", topic, expected)
		} else {
			d.logf("  topic %d: after Home, top is %q (expected %q)", topic, home, expected)
		}
		// Walk Down through the FULL topic content, snapshotting every screenful
		// as it scrolls into view, until the bottom — so every screenful of every
		// topic is rendered (several topics span multiple screenfuls), catching
		// any rendering regressions across the whole page. This also leaves the
		// reader scrolled to the bottom, which sets up the next topic's leak test
		// (the Go port's scroll-reset bug).
		// Topic 7 (Markup) is ~490+ lines of micron examples — the longest topic
		// by far. walkToBottom bottoms out at (page length) + H downs in the
		// worst case (the sig-stuck fallback fires H downs after the last
		// scroll), so the cap must clear ~490 + H with margin.
		d.walkToBottom(func(v *utils.View) string { return v.ReaderPaneSig() },
			700, 120*time.Millisecond, fmt.Sprintf("guide topic %d reader", topic))
		// Release focus back to the Topics list: Left moves reader->topics in
		// the urwid Columns (the vertical Scrollable does not consume Left).
		d.send("Left")
		time.Sleep(200 * time.Millisecond)

		if topic < 11 {
			// Down selects the next topic in the list (no render); Enter fires
			// display_topic -> renders it and moves focus back to the reader.
			d.send("Down")
			d.send("Enter")
			// Soft cross-check only: GuideSelectedTopic is unreliable in the
			// Python reference (the ilb cursor sits at x=135 and the list_focus
			// bg only shows while the list has focus, which it now does not),
			// so an assert here would noise up the Python run. The real
			// verification is the next iteration's BUG CHECK (the reader shows
			// the expected title).
			d.logf("  advanced to topic %d", topic+1)
		}
	}
	d.logf("Phase 4 complete (12 topics walked, scroll-reset bug observed on %d)", d.scrollBug)
}

// ---------- Phase 5: Quit ----------

func (d *driver) phase5() {
	d.step("Phase 5: navigate to Quit and select it")

	if !d.sess.HasSession() {
		d.logf("session already gone before Phase 5")
		return
	}
	// From the topic list at item 11, escape to the menu (escapeToMenu: Left
	// releases the reader if focus is there, then Up escapes the list to the
	// menu — robust to wherever Phase 4 left focus), then Right to Quit (index 7).
	// The active page is Guide (index 6), so Right 1 lands on Quit (7); moveMenuTo
	// handles the wrap and is robust to the menu landing on any index.
	d.escapeToMenu(15)
	d.assert(menuFocused(), 3*time.Second, "menu reached from Guide (cursor on row 0)")
	d.snapshot("menu reached from Guide")
	d.moveMenuTo(7, 8)
	d.assert(menuAt(7), 3*time.Second, "cursor on Quit (index 7)")
	d.snapshot("cursor on Quit (index 7)")
	d.send("Enter")
	d.logf("Quit activated; waiting for the app to exit...")

	elapsed := 0 * time.Second
	for elapsed < 15*time.Second {
		if !d.sess.HasSession() {
			d.logf("app exited cleanly after %v", elapsed)
			break
		}
		time.Sleep(1 * time.Second)
		elapsed += 1 * time.Second
	}
	if d.sess.HasSession() {
		d.logf("WARN: app did not exit within 15s of Quit; sending Ctrl-Q as a fallback")
		d.send("C-q")
		time.Sleep(3 * time.Second)
	}
	d.snapshot("final state")
}
