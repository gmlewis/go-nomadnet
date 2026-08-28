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
// server), selection still draws and writes become no-ops.
type textClipboard interface {
	// WriteText puts text on the system clipboard. Implementations must be
	// safe to call from any goroutine and must never block the UI loop for
	// long (the underlying clipboard.Write is asynchronous).
	WriteText(text string)
}

// systemClipboard is the real textClipboard backed by
// golang.design/x/clipboard. Init is attempted once per process; a failure
// permanently disables writes (ready stays false).
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
func (s *systemClipboard) WriteText(text string) {
	s.init()
	if !s.ready || text == "" {
		return
	}
	ctx, cancel := clipboardContext()
	go func() {
		defer cancel()
		_, _ = clipboard.Write(ctx, clipboard.FmtText, []byte(text))
	}()
}

// clipboardContext returns a short-lived context for one clipboard write so a
// wedged backend can never hang a goroutine forever.
func clipboardContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
