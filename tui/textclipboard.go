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
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.design/x/clipboard"
)

// textClipboard writes text to the system clipboard so terminal paste
// (Cmd-V / Ctrl-Shift-V / middle-click) works as expected. This is a
// Go-only enhancement: Python nomadnet has no TUI selection support, so
// there is no Python behavior to mirror here.
//
// The implementation wraps golang.design/x/clipboard (pure Go on every
// platform: AppKit via purego on macOS, X11/Wayland on Linux — no cgo).
// When no clipboard backend is available (e.g. a headless box without an X
// server), selection still draws and the writer uses the tmux paste buffer
// when running inside tmux. Inside tmux the tmux path ALWAYS runs, even when
// the native backend works: tmux's own `load-buffer -w` reaches the outer
// terminal under the default `set-clipboard external`, while an application's
// own OSC 52 is dropped by it.
type textClipboard interface {
	// WriteText puts text on the system clipboard. Implementations must be
	// safe to call from any goroutine and must never block the UI loop for
	// long (the underlying clipboard.Write is asynchronous).
	WriteText(text string)
}

// systemClipboard is the real textClipboard backed by
// golang.design/x/clipboard. Init is attempted once per process; a failure
// permanently disables the direct write (ready stays false) and WriteText
// falls back to the tmux paste buffer.
type systemClipboard struct {
	ready    bool
	initOnce sync.Once
}

// newSystemClipboard builds the real clipboard and probes availability once.
func newSystemClipboard() *systemClipboard {
	sc := &systemClipboard{}
	sc.init()
	return sc
}

// init probes the platform clipboard backend exactly once.
func (s *systemClipboard) init() {
	s.initOnce.Do(func() {
		s.ready = clipboard.Init() == nil
	})
}

// WriteText puts text on the system clipboard. clipboard.Write is
// asynchronous: it returns a done channel that (on macOS) may not close until
// an unrelated pasteboard access — the data is on the pasteboard as soon as
// the call returns (verified: a second process reads it back, and osascript
// sees it), so this never blocks the UI loop on the done channel.
//
// Inside tmux the text is ALSO loaded into the running tmux server's paste
// buffer (`load-buffer -w`), which is what makes the copy reach the machine
// the user is typing on. tmux's own clipboard write is forwarded to the outer
// terminal under the default `set-clipboard external`, but an application's
// own OSC 52 escape is NOT (verified against tmux 3.7c: with `external` the
// app's OSC 52 never leaves tmux, while `load-buffer -w` does). That matters
// for a remote node with a WORKING native backend — the AppKit-backed
// glenn-mac-mini-m2 over SSH — where ready is true and the native write only
// fills the remote pasteboard; without the tmux write nothing would reach the
// local clipboard. On a headless Linux node over SSH (glenn-OMEN-875) there
// is no native backend at all, so the tmux write is the only path.
func (s *systemClipboard) WriteText(text string) {
	s.init()
	if text == "" {
		return
	}
	go s.tmuxFallback(text)
	if !s.ready {
		return
	}
	ctx, cancel := clipboardContext()
	go func() {
		defer cancel()
		nativeClipboardWrite(ctx, text)
	}()
}

// nativeClipboardWrite posts text to the platform clipboard of the machine
// gonomadnet runs on. A package variable so tests can observe the native path
// without touching the real pasteboard.
var nativeClipboardWrite = func(ctx context.Context, text string) {
	_, _ = clipboard.Write(ctx, clipboard.FmtText, []byte(text))
}

// tmuxFallback stores text in the running tmux server's paste buffer via
// `tmux load-buffer -w`. The -w flag makes tmux itself emit the OSC 52 escape
// toward its outer terminal (forwarding through nested tmux layers), so the
// copy reaches the local clipboard even though the intermediate tmux drops the
// application's own OSC 52 under its default `set-clipboard external`. Best
// effort: any failure is silent (the selection highlight already confirms the
// gesture).
func (s *systemClipboard) tmuxFallback(text string) {
	if os.Getenv("TMUX") == "" {
		return
	}
	_ = tmuxLoadBuffer(text)
}

// tmuxLoadBuffer runs `tmux load-buffer -w` with text on stdin. A package
// variable so tests can capture the payload without touching the user's
// running tmux server.
var tmuxLoadBuffer = func(text string) error {
	cmd := exec.Command("tmux", "load-buffer", "-w", "-")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// clipboardContext returns a short-lived context for one clipboard write so a
// wedged backend can never hang a goroutine forever.
func clipboardContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
