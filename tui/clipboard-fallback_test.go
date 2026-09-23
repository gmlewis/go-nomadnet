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
	"context"
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
//
// Fleet bug #14 (the glenn-mac-mini-m2 SSH session) is the same symptom with a
// WORKING native backend: AppKit is present, so the golang.design write
// succeeds — and fills the REMOTE Mac's pasteboard, which the user never sees.
// The tmux buffer write must therefore run inside tmux even when ready is
// true; tmux drops an application's own OSC 52 under its default
// `set-clipboard external` but always forwards its own `load-buffer -w`.

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

// TestClipboardWriteTextFillsTmuxBufferWithNativeBackend pins fleet bug #14:
// inside tmux the buffer write must happen even when the native clipboard
// backend works (the AppKit-backed remote mac), because the native write only
// reaches the pasteboard of the machine gonomadnet runs on.
func TestClipboardWriteTextFillsTmuxBufferWithNativeBackend(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	sc := &systemClipboard{}
	// Freeze init so the probe never runs (and never succeeds on) the test
	// machine's own pasteboard, then model a node whose backend works.
	sc.initOnce.Do(func() {})
	sc.ready = true

	buffered := make(chan string, 1)
	origLoad := tmuxLoadBuffer
	tmuxLoadBuffer = func(text string) error {
		buffered <- text
		return nil
	}
	t.Cleanup(func() { tmuxLoadBuffer = origLoad })

	native := make(chan string, 1)
	origNative := nativeClipboardWrite
	nativeClipboardWrite = func(_ context.Context, text string) { native <- text }
	t.Cleanup(func() { nativeClipboardWrite = origNative })

	const addr = "da3cc92fff58eb70266d5d6190525bfb"
	sc.WriteText(addr)

	select {
	case got := <-buffered:
		if got != addr {
			t.Errorf("tmux buffer payload = %q, want %q", got, addr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the tmux buffer write did not run with a working native backend — the copy would land only on the remote pasteboard")
	}

	select {
	case got := <-native:
		if got != addr {
			t.Errorf("native payload = %q, want %q", got, addr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the native clipboard write did not run")
	}
}
