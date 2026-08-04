// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"time"
)

// phases.go implements the 5-phase scripted test as a verified state machine.
// Every keystroke is followed by a Wait on a View condition (parsed screen +
// cursor position) that asserts the intended result, instead of the blind
// sleep+grep the bash predecessor used. Failures are logged but never fatal —
// the run continues through all phases so the log records the full sequence.

// driver holds the test run state and shared helpers.
type driver struct {
	sess         *Session
	logf         func(format string, args ...any) // timestamped line writer
	stepDelay    time.Duration
	announceWait time.Duration
	connectWait  time.Duration
	nodeIters    int

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
func (d *driver) view() *View {
	v, err := d.sess.View()
	if err != nil {
		d.logf("  ERROR capturing view: %v", err)
		return &View{}
	}
	return v
}

// waitFor polls until cond(v) is true or timeout. Returns the last view + ok.
func (d *driver) waitFor(cond func(*View) bool, timeout time.Duration) (*View, bool) {
	return d.sess.Wait(cond, timeout)
}

// assert waits up to timeout for cond(v), then logs PASS/FAIL with the observed
// state. Never fatal. Returns whether the condition held.
func (d *driver) assert(cond func(*View) bool, timeout time.Duration, format string, args ...any) bool {
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
func observedState(v *View) string {
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
func hasTitle(title string) func(*View) bool {
	return func(v *View) bool {
		if v.Screen == nil {
			return false
		}
		return hasBorderTitle(v.Screen.fullText(), title)
	}
}

// menuAt is a Wait condition: the menu bar is focused (cursor on row 0) and the
// focused button is the given index.
func menuAt(idx int) func(*View) bool {
	return func(v *View) bool {
		i, ok := v.MenuFocusedButton()
		return ok && i == idx
	}
}

// menuFocused is a Wait condition: any menu button is focused (cursor on row 0).
func menuFocused() func(*View) bool {
	return func(v *View) bool {
		_, ok := v.MenuFocusedButton()
		return ok
	}
}

// onAList reports whether focus currently sits on a selectable list (the
// Network Announce/Saved list, or the Guide Topics list). Used by escapeToMenu
// to decide between Up (escape a list to the menu) and Left (release browser
// content focus back to a list). Up while in the browser content only scrolls
// the browser and never reaches the menu — the exact failure that stranded
// Phase 4 on the Network page.
func (v *View) onAList() bool {
	if v == nil || v.Screen == nil {
		return false
	}
	if v.SelectedAnnounceRow() != nil {
		return true
	}
	if idx, ok := v.GuideSelectedTopic(); ok && idx >= 0 {
		return true
	}
	return false
}

// releaseToList sends Left until focus is back on a selectable list (the
// Network Announce/Saved list or the Guide Topics list), capped at `cap`
// sends. Returns whether a list is focused. It does nothing if focus is
// already on a list. This is the inverse of escapeToMenu: Left in the browser
// content releases browser focus back to the left list; Left on a list would
// move INTO the browser, so it is guarded by onAList and never sent when
// already on a list.
func (d *driver) releaseToList(cap int) bool {
	for i := 0; i < cap; i++ {
		if d.view().onAList() {
			return true
		}
		d.send("Left")
	}
	return d.view().onAList()
}

// escapeToMenu releases focus from wherever it is (browser content, a list,
// a dialog) up to the menu bar, asserting nothing — the caller asserts. From
// the browser content it sends Left (which at a line's start releases browser
// focus back to the left list); once on a list it sends Up (bodyListAtTop ->
// FocusMenu). It never sends Up while in the browser (that only scrolls).
// Caps total steps so a stuck focus still returns.
func (d *driver) escapeToMenu(maxSteps int) {
	for i := 0; i < maxSteps; i++ {
		v := d.view()
		if _, ok := v.MenuFocusedButton(); ok {
			return
		}
		if v.onAList() {
			d.send("Up")
		} else {
			d.send("Left")
		}
	}
}

// moveMenuTo moves the menu focus to the target button index by sending Right
// (the menu wraps, so Right always advances) until MenuFocusedButton matches,
// capped at `cap` sends. Returns whether the target was reached.
func (d *driver) moveMenuTo(target, cap int) bool {
	for i := 0; i < cap; i++ {
		if idx, ok := d.view().MenuFocusedButton(); ok && idx == target {
			return true
		}
		d.send("Right")
	}
	idx, ok := d.view().MenuFocusedButton()
	return ok && idx == target
}

// appStarted waits for the menu bar to appear (the "[ Conversations ]" button).
func appStarted() func(*View) bool {
	return func(v *View) bool {
		if v.Screen == nil || len(v.Screen.Rows) == 0 {
			return false
		}
		return strings.Contains(v.Screen.rowText(0), "Conversations")
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
			d.assert(menuAt(idx), 3*time.Second, "menu focused on index %d (%s)", idx, menuLabels[idx])
		}
		d.send("Enter")
		// Focus stays in the menu (cursor on row 0) after Enter.
		d.assert(menuAt(idx), 2*time.Second, "focus stayed in menu on index %d after Enter", idx)
		d.snapshot(fmt.Sprintf("page: %s (menu %d)", menuLabels[idx], idx))
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
	for i := 0; i < 5; i++ {
		d.send("Left")
	}
	d.assert(menuAt(1), 3*time.Second, "menu on Network (index 1) after 5 Lefts")
	d.send("Enter")
	d.assert(func(v *View) bool { return v.ActivePage() == "network" }, 5*time.Second, "Network page active (Saved Nodes)")
	d.snapshot("Network selected (Saved Nodes)")

	// Down drops focus from the menu to the body.
	d.send("Down")
	d.assert(func(v *View) bool {
		_, ok := v.MenuFocusedButton()
		return !ok // cursor left the menu row
	}, 3*time.Second, "focus left the menu (Down to body)")

	// Ctrl-L toggles Saved Nodes <-> Announce Stream.
	d.send("C-l")
	d.assert(hasTitle("Announce Stream"), 5*time.Second, "Announce Stream showing after Ctrl-L")
	d.snapshot("after Ctrl-L (Announce Stream)")

	d.logf("waiting up to %v for announcing nodes to populate...", d.announceWait)
	nodesSeen := false
	deadline := time.Now().Add(d.announceWait)
	for time.Now().Before(deadline) {
		v := d.view()
		if r := v.SelectedAnnounceRow(); r != nil && r.Text != "" {
			nodesSeen = true
			d.logf("  announcing node present: %q at row %d", r.Text, r.Y)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !nodesSeen {
		d.logf("  no announcing nodes detected within %v; Phase 3 will record zero connects", d.announceWait)
	}
	d.snapshot("Announce Stream after announce-wait")
}

// ---------- Phase 3: connect to up to N nodes (verified) ----------

func (d *driver) phase3() {
	d.step(fmt.Sprintf("Phase 3: connect to nodes, render index.mu, follow links (x%d)", d.nodeIters))

	// tried is keyed by the node's STABLE display name (announceNodeName), not
	// the full announce row text. The announce stream re-sorts and re-emits
	// nodes with a fresh timestamp on every re-announce, so the row text
	// ("16:32:23 Ⓝ Rostov") changes while the name ("Rostov") does not —
	// keying on row text (the old behavior) made the suite reconnect the same
	// node repeatedly. tried covers both successes and failures so the suite
	// never revisits a node this session (forward progress; no reconnects).
	tried := map[string]bool{}
	success := 0
	attempts := 0
	maxAttempts := d.nodeIters * 4

	for success < d.nodeIters && attempts < maxAttempts {
		attempts++
		d.logf("iteration: success=%d/%d attempts=%d/%d", success, d.nodeIters, attempts, maxAttempts)
		if !d.sess.HasSession() {
			d.logf("session died mid-Phase-3; aborting Phase 3")
			break
		}

		// 0. Ensure focus is back on the Announce Stream list. A failed
		//    connect (C-w + continue) can leave focus in the (now-disconnected)
		//    browser pane; seekUntriedNode would then send Down into the browser
		//    (scrolling it) and never find a node. releaseToList Lefts out of the
		//    browser back to the list, and is a no-op when already on the list.
		d.releaseToList(4)

		// 1. Seek an untried node. The cursor may be on a node we already
		//    connected/attempted (the list reorders dynamically); walk Down
		//    until we find a name not in `tried`, or cycle through all visible
		//    nodes (then nothing new to do this round).
		name, row := d.seekUntriedNode(tried)
		if name == "" || row == nil {
			d.logf("  no untried node visible (all %d known names attempted); ending Phase 3", len(tried))
			break
		}
		d.logf("  selected node %q (row %q)", name, row.Text)

		// 2. Enter -> Announce Info. Verify it opened (border title + Addr
		//    hash) — the check the blind script faked with a stale "Connect"
		//    substring match.
		d.send("Enter")
		opened := d.assert(func(v *View) bool {
			_, ok := v.AnnounceInfoAddr()
			return ok && hasBorderTitle(v.Screen.fullText(), "Announce Info")
		}, 6*time.Second, "Announce Info opened for %q", name)
		if !opened {
			d.logf("  no node-type Announce Info (peer/pn entry or empty); marking tried and skipping")
			tried[name] = true
			d.send("Escape")
			d.snapshot("phase3 skip (no Announce Info)")
			continue
		}
		targetHash := ""
		if hv := d.view(); hv != nil {
			targetHash, _ = hv.AnnounceInfoAddr()
		}
		d.snapshot(fmt.Sprintf("phase3: announce info opened (name=%q addr=%s)", name, targetHash))

		// 3. Focus the Connect button (color-neutral; detected via cursor_x on
		//    the button row). Right moves Back->Connect. Only press Enter if
		//    Connect is actually focused, else we would activate Back.
		for tries := 0; tries < 4; tries++ {
			if bv := d.view(); bv != nil {
				if b, ok := bv.FocusedActionButton(); ok && b == "Connect" {
					break
				}
			}
			d.send("Right")
		}
		connectFocused := d.assert(func(v *View) bool {
			b, ok := v.FocusedActionButton()
			return ok && b == "Connect"
		}, 3*time.Second, "Connect button focused for %q", name)
		if !connectFocused {
			d.logf("  could not focus Connect; marking tried and skipping")
			tried[name] = true
			d.send("Escape")
			continue
		}
		d.send("Enter")

		// 4. VERIFY the connect — wait for a TERMINAL state and ONLY then
		//    proceed: either the target node's index.mu rendered (state ==
		//    rendered AND url == targetHash, proving navigation to THIS node
		//    and not a stale page), OR a connect error (link establishment
		//    failure / no path / request failed / timed out). The blind script
		//    declared success the instant "Disconnected" disappeared, matching
		//    a still-"Retrieving" frame; this single wait blocks through
		//    "Retrieving" until one of the two outcomes actually occurs.
		d.logf("  Connect pressed (target %q); waiting for render or failure...", targetHash)
		_, _ = d.waitFor(func(v *View) bool {
			st, url := v.BrowserState()
			if strings.HasPrefix(st, "error:") {
				return true
			}
			return st == bsRendered && targetHash != "" && url == targetHash
		}, d.connectWait+6*time.Second)
		st, url := d.view().BrowserState()
		tried[name] = true

		if !(st == bsRendered && targetHash != "" && url == targetHash) {
			switch {
			case strings.HasPrefix(st, "error:"):
				d.logf("  connect FAILED: %s; skipping node", st)
			case st == bsRendered:
				d.logf("  connect FAILED: rendered page url=%q != target %q (stale page?); skipping", url, targetHash)
			default:
				d.logf("  connect TIMEOUT after %v (state=%q); skipping node", d.connectWait+6*time.Second, st)
			}
			d.snapshot("phase3: connect failed")
			d.send("C-w") // Disconnect to reset the browser pane before the next node.
			time.Sleep(d.stepDelay)
			continue
		}

		d.logf("  page RENDERED OK (url=%q)", url)
		d.snapshot("phase3: node index.mu rendered")
		success++

		// 5. Move into the browser page and follow EVERY link on the front
		//    page, returning to the main page (Back = C-d) after each. The
		//    browser resets focus to the first selectable line on every render
		//    (initNavState), so link i is reached with i Downs. Links render
		//    in the inherited text color (no distinct link color), so we
		//    detect "tried all links" by URL reuse: once a Down+Enter
		//    re-navigates to an already-followed URL (or fails to leave the
		//    main page), we stop.
		d.send("Right") // list -> browser content
		d.assert(func(v *View) bool {
			_, ok := v.MenuFocusedButton()
			return !ok // focus moved into the body/browser
		}, 3*time.Second, "focus moved into browser pane")
		d.snapshot("phase3: browser pane (before link nav)")
		d.followAllLinks(targetHash)

		// 6. Release focus back to the Announce Stream list (Left at a line's
		//    start releases browser focus) so the next iteration can seek the
		//    next node. releaseToList is guarded so it is a no-op if focus is
		//    already back on the list.
		d.releaseToList(4)
		d.assert(func(v *View) bool {
			return v.SelectedAnnounceRow() != nil
		}, 3*time.Second, "focus back on the announce list")
		d.snapshot("phase3: back at node list")
	}
	d.logf("Phase 3 complete: %d/%d successful connects in %d attempts (%d distinct nodes tried)",
		success, d.nodeIters, attempts, len(tried))
}

// seekUntriedNode walks the Announce Stream list (Down, wrapping) until the
// selected row's stable node name is not in `tried`, returning the name + row.
// Returns ("", nil) if every visible node is already tried (detected when a
// Down lands on a name already seen this seek — i.e. we cycled through all).
func (d *driver) seekUntriedNode(tried map[string]bool) (string, *ListRow) {
	visited := map[string]bool{}
	nilStreak := 0 // consecutive steps with no selected list row (focus not on list)
	for steps := 0; steps < 60; steps++ {
		row := d.view().SelectedAnnounceRow()
		name := ""
		if row != nil {
			name = announceNodeName(row.Text)
		}
		if name != "" && !tried[name] {
			return name, row
		}
		if name != "" {
			nilStreak = 0
			// A repeat means we've cycled through every visible node and they
			// are all tried — stop seeking.
			if visited[name] {
				return "", nil
			}
			visited[name] = true
		} else {
			// No selected list row: focus is not on the list (e.g. stuck in a
			// linkless browser page). A few Downs should release/advance; if it
			// persists, stop rather than burn the whole budget scrolling.
			nilStreak++
			if nilStreak >= 5 {
				return "", nil
			}
		}
		d.send("Down")
	}
	return "", nil
}

// followAllLinks follows every link on the front (index.mu) page, going Back
// (C-d) to the main page after each. mainHash is the main page's URL (the node
// hash) used to confirm we returned and to detect exhaustion. See phase3 step 5.
func (d *driver) followAllLinks(mainHash string) {
	followed := map[string]bool{}
	const maxLinks = 20
	for i := 0; i < maxLinks; i++ {
		// Reach link i from the top (focus resets to the first selectable line
		// after each Back, so i Downs lands on the i-th link). Send them in one
		// tmux call with a short settle — we only verify state after Enter.
		if i > 0 {
			keys := make([]string, i)
			for j := range keys {
				keys[j] = "Down"
			}
			d.jog(keys...)
		}
		d.send("Enter")

		_, ok := d.waitFor(func(v *View) bool {
			st, _ := v.BrowserState()
			return st == bsRendered || strings.HasPrefix(st, "error:")
		}, d.connectWait)
		if !ok {
			d.logf("  link %d: no render/error within %v; stopping link nav", i, d.connectWait)
			d.send("C-d")
			d.settleMain(mainHash)
			break
		}
		st, url := d.view().BrowserState()
		if strings.HasPrefix(st, "error:") {
			d.logf("  link %d: error %q; back to main, trying next link", i, st)
			d.snapshot(fmt.Sprintf("phase3: link %d error", i))
			d.send("C-d")
			d.settleMain(mainHash)
			continue
		}
		// Exhaustion: Enter did not navigate away from the main page (no link
		// at this position — we're past the last link, Down had scrolled), OR
		// it re-followed a link we already tried (Down past the last link
		// landed on the last link again). Either way we've tried them all.
		if url == "" || url == mainHash || followed[url] {
			d.logf("  link %d: url=%q (main page or repeat) — exhausted front-page links after %d", i, url, len(followed))
			if url != "" && url != mainHash {
				d.send("C-d")
				d.settleMain(mainHash)
			}
			break
		}
		followed[url] = true
		d.logf("  followed link %d -> %q", i, url)
		d.snapshot(fmt.Sprintf("phase3: link %d -> %s", i, url))
		d.send("C-d") // Back to the main page.
		d.settleMain(mainHash)
	}
	d.logf("  followed %d distinct front-page links", len(followed))
}

// settleMain waits for the main page to come back after a Back (C-d), confirmed
// by the browser URL bar showing the main page hash again.
func (d *driver) settleMain(mainHash string) {
	d.waitFor(func(v *View) bool {
		st, url := v.BrowserState()
		return st == bsRendered && mainHash != "" && url == mainHash
	}, d.connectWait)
}

// jog sends keys in one tmux send-keys call with a short settle. Used for the
// repetitive Down-jogging between links (we only verify state after Enter, not
// after each Down), keeping link navigation responsive.
func (d *driver) jog(keys ...string) {
	if len(keys) == 0 {
		return
	}
	d.logf("jog: %d x %s", len(keys), keys[0])
	if err := d.sess.SendKeys(keys...); err != nil {
		d.logf("  ERROR jog send-keys: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
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
	d.assert(func(v *View) bool { return v.ActivePage() == "guide" }, 5*time.Second, "Guide page active (Topics title present)")
	d.snapshot("Guide selected")
	d.send("Down")
	d.assert(func(v *View) bool {
		return hasBorderTitle(v.Screen.fullText(), "Topics")
	}, 3*time.Second, "Guide topic list visible (Topics box)")
	d.snapshot("Guide topic list (item 0, Introduction)")

	// Select topic 0 via Enter (AddItem selected callback -> showTopic).
	d.send("Enter")
	d.snapshot("Guide topic 0 (Introduction) selected")

	// For each topic: move to the reader, capture the FIRST visible line. If it
	// is the topic title, scroll is at row 0 (correct). If not, the scroll-reset
	// bug is present (showTopic has no ScrollTo(0,0), so the offset leaked from
	// the previous topic). Then Home (which DOES ScrollToBeginning) and re-check
	// to contrast buggy vs correct; End to scroll to bottom; Left back to list;
	// Down to the next topic (auto-renders it).
	for topic := 0; topic <= 11; topic++ {
		expected := guideTopicTitles[topic]
		d.send("Right")
		time.Sleep(300 * time.Millisecond)
		v := d.view()
		rendered := v.GuideTopicRendered()
		d.snapshot(fmt.Sprintf("Guide topic %d reader right after selection (BUG CHECK: top should be %q)", topic, expected))
		if strings.TrimSpace(rendered) == expected {
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
		d.send("End")
		time.Sleep(400 * time.Millisecond)
		d.snapshot(fmt.Sprintf("Guide topic %d reader scrolled to bottom", topic))
		// Release focus back to the Topics list. Left at a line's start in the
		// reader releases focus; after End the cursor may not be at position 0,
		// so send Left a few times until the Topics list is selected again.
		for tries := 0; tries < 4; tries++ {
			if idx, ok := d.view().GuideSelectedTopic(); ok && idx >= 0 {
				break
			}
			d.send("Left")
		}

		if topic < 11 {
			d.send("Down")
			time.Sleep(300 * time.Millisecond)
			d.assert(func(v *View) bool {
				idx, ok := v.GuideSelectedTopic()
				return ok && idx == topic+1
			}, 3*time.Second, "Guide topic advanced to %d", topic+1)
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
