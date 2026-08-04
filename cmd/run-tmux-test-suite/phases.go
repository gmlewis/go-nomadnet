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

	connected := map[string]bool{} // dedup by announce row text (the list reorders)
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

		// 1. Find the highlighted node (the #aaaaaa cursor row in the left list).
		v := d.view()
		row := v.SelectedAnnounceRow()
		if row == nil || row.Text == "" {
			d.logf("  no node row selected (empty list?); advancing")
			d.snapshot("phase3 no-node-row")
			d.send("Down")
			continue
		}
		if connected[row.Text] {
			d.logf("  node %q already connected; advancing to next", row.Text)
			d.send("Down")
			continue
		}

		// 2. Enter -> Announce Info. Verify it actually opened (border title +
		//    an Addr hash), the check the blind script's `wait_for "Connect"`
		//    faked by matching stale "Connect" text.
		d.send("Enter")
		opened := d.assert(func(v *View) bool {
			_, ok := v.AnnounceInfoAddr()
			return ok && hasBorderTitle(v.Screen.fullText(), "Announce Info")
		}, 6*time.Second, "Announce Info opened for %q", row.Text)
		if !opened {
			d.logf("  no node-type Announce Info opened (peer/pn entry or empty); skip")
			d.send("Escape")
			d.snapshot("phase3 skip (no Announce Info)")
			d.send("Down")
			continue
		}
		targetHash := ""
		if hv := d.view(); hv != nil {
			targetHash, _ = hv.AnnounceInfoAddr()
		}
		d.snapshot(fmt.Sprintf("phase3: announce info opened (addr=%s)", targetHash))

		// 3. Focus the Connect button (color-neutral; detected via cursor_x on
		//    the button row). Right moves Back->Connect.
		for tries := 0; tries < 4; tries++ {
			if bv := d.view(); bv != nil {
				if b, ok := bv.FocusedActionButton(); ok && b == "Connect" {
					break
				}
			}
			d.send("Right")
		}
		d.assert(func(v *View) bool {
			b, ok := v.FocusedActionButton()
			return ok && b == "Connect"
		}, 3*time.Second, "Connect button focused")
		d.send("Enter")

		// 4. VERIFY the connect (the core fix). Wait for the browser URL bar to
		//    show the target node's hash (proves navigation to THIS node, not a
		//    stale page), then wait for rendered or error. The blind script
		//    instead fired `wait_gone "Disconnected"` after 0s against a
		//    "Retrieving" frame and declared success on a still-loading page.
		d.logf("  Connect pressed (target %q); waiting for navigation...", targetHash)
		navOK := d.assert(func(v *View) bool {
			_, url := v.BrowserState()
			return targetHash != "" && url == targetHash
		}, 6*time.Second, "browser navigated to target hash %q", targetHash)

		state, _ := d.view().BrowserState()
		d.logf("  browser state after connect: %q", state)
		rendered := false
		if navOK {
			v, ok := d.waitFor(func(v *View) bool {
				st, _ := v.BrowserState()
				return st == bsRendered || strings.HasPrefix(st, "error:")
			}, d.connectWait)
			state, _ = v.BrowserState()
			if ok && state == bsRendered {
				rendered = true
			} else if ok && strings.HasPrefix(state, "error:") {
				d.logf("  connect FAILED: %s; skipping node", state)
			} else {
				d.logf("  connect TIMEOUT after %v (state=%q); skipping node", d.connectWait, state)
			}
		} else {
			d.logf("  connect FAILED: browser never navigated to %q; skipping", targetHash)
		}

		if !rendered {
			d.snapshot("phase3: connect failed")
			d.send("C-w") // Disconnect to reset the browser pane.
			time.Sleep(d.stepDelay)
			d.send("Down")
			continue
		}

		d.logf("  page RENDERED OK")
		d.snapshot("phase3: node index.mu rendered")
		success++
		connected[row.Text] = true

		// 5. Move into the browser pane and follow a couple of links.
		d.send("Right")
		d.assert(func(v *View) bool {
			_, ok := v.MenuFocusedButton()
			return !ok // focus moved into the body/browser
		}, 3*time.Second, "focus moved into browser pane")
		d.snapshot("phase3: browser pane (before link nav)")
		for link := 1; link <= 2; link++ {
			d.send("Down")
			d.send("Enter")
			d.logf("  followed link %d", link)
			d.waitFor(func(v *View) bool {
				st, _ := v.BrowserState()
				return st == bsRendered || strings.HasPrefix(st, "error:")
			}, 5*time.Second)
			d.snapshot(fmt.Sprintf("phase3: after following link %d", link))
		}

		// Release focus back to the Announce Stream list (Left at a line's
		// start releases the browser focus), then advance to the next node.
		for i := 0; i < 3; i++ {
			d.send("Left")
		}
		d.assert(func(v *View) bool {
			return v.SelectedAnnounceRow() != nil
		}, 3*time.Second, "focus back on the announce list")
		d.snapshot("phase3: back at node list")
		d.send("Down")
	}
	d.logf("Phase 3 complete: %d/%d successful connects in %d attempts", success, d.nodeIters, attempts)
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

	// From the Announce Stream list, escape up to the menu: list->filter bar->
	// tab bar->menu (pileFiller onUpEscape, three Ups). Then assert the menu is
	// focused — the check the blind script never made, so it walked 12 "topics"
	// on the Network page without realizing it.
	for i := 0; i < 3; i++ {
		d.send("Up")
	}
	// Retry up to a few extra Ups in case focus was elsewhere (e.g. the browser).
	for tries := 0; tries < 4; tries++ {
		if _, ok := d.view().MenuFocusedButton(); ok {
			break
		}
		d.send("Up")
	}
	d.assert(menuFocused(), 3*time.Second, "menu reached from Announce Stream (cursor on row 0)")
	d.snapshot("menu reached from Announce Stream")

	// Active page is Network (index 1). Right 5 to Guide (index 6), Enter, then
	// Down drops to the body (topic list, focused on item 0).
	for i := 0; i < 5; i++ {
		d.send("Right")
	}
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
		d.send("Left")

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
	// From the topic list at item 11, escape to the menu (Up-at-0 -> FocusMenu),
	// then Right to Quit (index 7), Enter to activate -> graceful shutdown.
	for i := 0; i < 13; i++ {
		d.send("Up")
	}
	d.assert(menuFocused(), 3*time.Second, "menu reached from Guide (cursor on row 0)")
	d.snapshot("menu reached from Guide")
	d.send("Right")
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
