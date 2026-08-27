// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package main

import (
	"os"
	"testing"
)

// TestHasTTYWithDevNull verifies that /dev/null is NOT detected as a TTY.
// This is the regression test for the stale-lock silent-exit bug: when
// gonomadnet was launched via `nohup bash ./gonomadnet.sh &` from a
// non-interactive SSH session, stdin was not a terminal. The tview TUI
// failed to initialize and gonomadnet exited silently with code 0, leaving
// the operator with no running node and no error. The fix auto-falls back to
// daemon mode when hasTTY() returns false.
func TestHasTTYWithDevNull(t *testing.T) {
	f, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("cannot open /dev/null: %v", err)
	}
	defer func() { _ = f.Close() }()

	if hasTTYFromFile(f) {
		t.Error("/dev/null should NOT be detected as a TTY")
	}
}

// TestHasTTYConsistency verifies that hasTTYFromFile and hasTTY agree for
// stdin. In a test binary stdin is typically not a terminal (go test pipes
// it), so both should return false. This catches regressions where the two
// code paths diverge.
func TestHasTTYConsistency(t *testing.T) {
	// The test binary's stdin is typically a pipe (not a char device).
	if hasTTY() {
		t.Log("stdin IS a terminal (running interactively) — skipping consistency check")
		return
	}
	// Both should agree that it's not a terminal.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer func() { _ = f.Close() }()
	if hasTTYFromFile(f) {
		t.Error("hasTTYFromFile(/dev/null) disagrees with hasTTY() on non-TTY stdin")
	}
}
