// Copyright 2026 Glenn Lewis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/utils"
)

// phases.go implements the conversations test as a verified state machine over
// TWO tmux-driven gonomadnet instances (A and B). Each keystroke is followed by
// a Wait on a View condition (parsed screen + cursor). Failures are logged but
// never fatal — the run continues through all phases so the log records the
// full sequence, exactly like cmd/run-tmux-test-suite.
//
// driver holds per-session state; harness holds the pair + cross-coordination
// (the LXMF address exchange + round-trip waits). Counters aggregate across
// both drivers for the end-of-run summary.

// driver holds one instance's test-run state and shared helpers.
type driver struct {
	sess         *utils.Session
	tag          string                           // "[A]" / "[B]" prefix for log lines
	logf         func(format string, args ...any) // shared timestamped line writer
	stepDelay    time.Duration
	announceWait time.Duration
	msgWait      time.Duration

	// peer is the OTHER driver (A.peer = B, B.peer = A) so a driver can drive
	// its counterpart when a step is symmetric (e.g. "on the other side, wait
	// for the message I just sent").
	peer *driver

	// lxmfHash is this instance's own LXMF address (read off the screen via the
	// C-p "My LXMF" dialog), fed to the OTHER instance's New Conversation dialog.
	lxmfHash string

	// counters for the end-of-run summary.
	asserts  int
	assertOK int
	failures int
}

// newDriver builds a driver for one tmux session. logf is the shared logger;
// each driver tags its own lines with tag ("[A]"/"[B]").
func newDriver(sess *utils.Session, tag string, logf func(string, ...any), cfg *config) *driver {
	return &driver{
		sess:         sess,
		tag:          tag,
		logf:         func(f string, a ...any) { logf(tag+" "+f, a...) },
		stepDelay:    cfg.stepDelay,
		announceWait: cfg.announceWait,
		msgWait:      cfg.msgWait,
	}
}

// summary returns this driver's assert/pass/fail counts.
func (d *driver) summary() (asserts, pass, fail int) {
	return d.asserts, d.assertOK, d.failures
}

// send sends tmux key names then pauses stepDelay for the app to redraw.
func (d *driver) send(keys ...string) {
	d.logf("send: %s", strings.Join(keys, " "))
	if err := d.sess.SendKeys(keys...); err != nil {
		d.logf("  ERROR send-keys: %v", err)
	}
	time.Sleep(d.stepDelay)
}

// sendLiteral types literal text (no tmux key-name interpretation).
func (d *driver) sendLiteral(text string) {
	d.logf("send-literal: %q", text)
	if err := d.sess.SendLiteral(text); err != nil {
		d.logf("  ERROR send-literal: %v", err)
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
	observed := observedState(v)
	d.logf("  ASSERT FAIL: %s | observed: %s", msg, observed)
	return false
}

// step logs a phase step header.
func (d *driver) step(msg string) {
	d.logf("--- %s ---", msg)
}

// observedState summarizes the view for failure logs.
func observedState(v *utils.View) string {
	if v == nil || v.Screen == nil {
		return "<no view>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "page=%q cursor=(%d,%d,ok=%t)", v.ActivePage(), v.CursorX, v.CursorY, v.CursorOK)
	if st, url := v.BrowserState(); st != "" || url != "" {
		fmt.Fprintf(&b, " browser=%q url=%q", st, url)
	}
	if mb, ok := v.MenuFocusedButton(); ok {
		fmt.Fprintf(&b, " menu=%d", mb)
	}
	if region := shortcutRegion(v); region != "" {
		fmt.Fprintf(&b, " region=%q", region)
	}
	if onConversationsPage(v) {
		fmt.Fprintf(&b, " convTabUnread=%d convUnreadRows=%d convFailedRows=%d menuUnread=%t",
			tabUnreadCount(v), unreadRowCount(v), failedRowCount(v), menuUnreadShown(v))
	}
	if onNetworkPage(v) {
		b.WriteString(" on-network")
	}
	return b.String()
}

// harness holds the two drivers and cross-coordination state.
type harness struct {
	dA           *driver
	dB           *driver
	logf         func(string, ...any)
	announceWait time.Duration
	msgWait      time.Duration
}

// runAllPhases runs the conversations phases in order. Each phase is guarded so
// a session death does not abort the logging of later phases' state.
func runAllPhases(h *harness, logf func(string, ...any)) {
	logf("waiting for both apps to start (go compile + intro splash)...")
	h.boot()
	h.announceAndAddresses()
	h.firstMessageBtoA()
	h.replyAtoB()
	// inConversationShortcuts (dB) runs BEFORE headerStates so it exercises the
	// REAL Alice conversation B is left viewing after the round-trip (editor
	// focused, with hi-from-A in the body) — giving meaningful coverage of the
	// titled-send + body-scroll steps. headerStates would replace it with the
	// bogus "Nobody" conversation (no recalled identity → send is a no-op,
	// empty body), so running it after would leave those steps snapshot-only on
	// an empty conversation. openFirstConversation uses the already-open
	// conversation, so no re-open (and its focus gap) is needed here.
	h.listShortcuts()
	h.inConversationShortcuts()
	h.headerStates()
	h.cleanup()
}
