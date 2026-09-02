// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"errors"
	"testing"
	"time"
)

// Regression tests for fleet bug #13 (the glenn-OMEN-875 SSH session): on a
// headless Linux box there is no X11/Wayland clipboard backend, so the
// golang.design write is a no-op and the copy previously vanished entirely.
// The tmux `load-buffer -w` fallback both fills the running tmux server's
// paste buffer and forwards OSC 52 toward the outer terminal, so the Peer
// Info address copy reaches the machine the user types on.

func TestClipboardTmuxFallbackWhenNoBackend(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-0/default,123,0")
	sc := &systemClipboard{} // Init never succeeded → ready stays false

	var got string
	orig := tmuxLoadBuffer
	tmuxLoadBuffer = func(text string) error {
		got = text
		return nil
	}
	t.Cleanup(func() { tmuxLoadBuffer = orig })

	sc.tmuxFallback("d8b6dad35315fdc93ddc21ec2785fd40")
	if got != "d8b6dad35315fdc93ddc21ec2785fd40" {
		t.Errorf("tmux load-buffer payload = %q, want the selected address", got)
	}
}

func TestClipboardTmuxFallbackSkippedOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	sc := &systemClipboard{}

	fired := false
	orig := tmuxLoadBuffer
	tmuxLoadBuffer = func(text string) error {
		fired = true
		return errors.New("must not run outside tmux")
	}
	t.Cleanup(func() { tmuxLoadBuffer = orig })

	sc.tmuxFallback("d8b6dad35315fdc93ddc21ec2785fd40")
	if fired {
		t.Errorf("tmux fallback ran with no TMUX env — it would target the wrong server or fail silently")
	}
}

func TestClipboardWriteTextFallsBackWhenInitFailed(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-0/default,123,1234567890")
	sc := &systemClipboard{}
	// Simulate a backend-less host (headless Linux over SSH): consume the
	// initOnce so WriteText's init() cannot probe (and succeed on) the
	// macOS clipboard of the test machine, leaving ready=false.
	sc.initOnce.Do(func() {})
	sc.ready = false

	done := make(chan string, 1)
	orig := tmuxLoadBuffer
	tmuxLoadBuffer = func(text string) error {
		done <- text
		return nil
	}
	t.Cleanup(func() { tmuxLoadBuffer = orig })

	sc.WriteText("2a6105f57145860441a62fe3b2a1352c")
	select {
	case got := <-done:
		if got != "2a6105f57145860441a62fe3b2a1352c" {
			t.Errorf("fallback payload = %q, want the selected address", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the tmux fallback did not run — the copy would be a silent no-op without a system clipboard")
	}
}
