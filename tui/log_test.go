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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestLogDisplayUpAtTopToMenu asserts Up at the top of the log view moves focus
// to the menu bar, matching Python's LogTerminal.keypress (Log.py:55-58) where
// "up" is the escape sequence that returns focus to the header. The log view is
// a scrollable TextView; Up scrolls up through history and, once at the top,
// collapses focus to the menu (the centralized bodyListAtTop dispatcher only
// covers *tview.List, so the log page owns this transition).
func TestLogDisplayUpAtTopToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	ld := NewLogDisplay(app, "", 50)

	// Simulate the user having scrolled to the top of the log.
	ld.logView.ScrollToBeginning()
	app.SetFocus(ld.logView)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := ld.handleInput(up)
	if got != nil {
		t.Errorf("handleInput(Up) = %v, want nil (consumed)", got)
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("focusRegion = %q, want menu", app.Main.focusRegion)
	}
}

// TestLogDisplayUpMidScrollGoesToMenu pins F1 (Python Log.py:55-61): EVERY Up
// collapses focus to the menu — even mid-scroll — because the embedded
// `tail -fn50` terminal never scrolls via keys. The log content is untouched.
// (This reverses the earlier Go behavior of scrolling until the very top.)
func TestLogDisplayUpMidScrollGoesToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	ld := NewLogDisplay(app, "", 50)

	// Force a non-top scroll offset to prove the transition happens regardless
	// of scroll position.
	ld.logView.SetText("line\nline\nline\nline\nline")
	ld.logView.ScrollTo(3, 0)

	app.SetFocus(ld.logView)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if got := ld.handleInput(up); got != nil {
		t.Error("handleInput(Up) not consumed; want the FocusMenu transition (Log.py:58-61)")
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("focusRegion = %q, want menu (first Up goes to the header)", app.Main.focusRegion)
	}
}

// TestLogDisplayPreservesLevelField pins B6: the Log page renders the raw
// logfile verbatim, and the logfile (written by go-reticulum, matching
// Python RNS.format `[timestamp] [Level]   message`) contains `[Info]`,
// `[Error]`, etc. tview's TextView parses `[...]` as a color tag, so an
// unescaped `[Info]` is silently dropped from the visible text (the user sees
// `[timestamp]      message` with the level field blank). Python's Log.py
// embeds urwid.Terminal running `tail -fn50`, which shows the brackets
// literally. The Go substitute must escape tview color tags so the `[Level]`
// field is visible. The log content has NO real tview color tags (it is plain
// log text), so escaping the whole line is safe.
func TestLogDisplayPreservesLevelField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "logfile")
	line := "[2026-08-05 19:00:26] [Notice]  Configuration loaded successfully\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	ld := NewLogDisplay(app, path, 50)

	got := ld.logView.GetText(true)
	// The visible text must retain the literal `[Notice]` level token.
	if !strings.Contains(got, "[Notice]") {
		t.Errorf("Log display stripped the [Level] field (B6: tview parsed [Notice] as a color tag).\ngettext = %q", got)
	}
}

// TestStopTailingIdempotent pins the "close of closed channel" panic regression.
// The application cleanup path that wires a LogDisplay into the running UI
// (cmd/gonomadnet/textui.go wireDisplays cleanup, invoked from the Ctrl-Q /
// Ctrl-C quit handler) can fire more than once when the user issues the quit
// key sequence. The original StopTailing did an unconditional close(stopCh),
// so the second call panicked. StopTailing must be safe to call repeatedly.
func TestStopTailingIdempotent(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	ld := NewLogDisplay(app, "", 50)

	// Calling StopTailing without StartTailing exercises the bare close path:
	// wg.Wait() returns immediately and the only thing that can fail is the
	// double-close. Two sequential calls must both be safe.
	ld.StopTailing()
	ld.StopTailing()
}

// TestStopTailingConcurrentSafe asserts concurrent StopTailing calls (e.g. the
// quit handler racing a teardown path) never panic on close of a closed
// channel. sync.Once serializes exactly one close; the rest see it as a no-op.
func TestStopTailingConcurrentSafe(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	ld := NewLogDisplay(app, "", 50)

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for range n {
		go func() {
			defer wg.Done()
			<-start
			ld.StopTailing() // must not panic, regardless of who wins the close.
		}()
	}
	close(start)
	wg.Wait()
}

// TestStopTailingStopsGoroutine asserts StopTailing actually terminates the live
// tail goroutine and returns, rather than hanging. StartTailing opens the file
// and seeks to the end; because the test never appends new lines, the goroutine
// never calls QueueUpdateDraw (which would otherwise block on the not-running
// test event loop), so the only thing StopTailing waits on is the goroutine
// noticing stopCh and returning.
func TestStopTailingStopsGoroutine(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "logfile")
	// Seed with content so os.Open succeeds; StartTailing seeks past it.
	if err := os.WriteFile(path, []byte("seed line one\nseed line two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	ld := NewLogDisplay(app, path, 50)
	ld.StartTailing()

	done := make(chan struct{})
	go func() {
		ld.StopTailing()
		close(done)
	}()
	select {
	case <-done:
		// StopTailing returned: the tail goroutine observed stopCh and exited.
	case <-time.After(3 * time.Second):
		t.Fatal("StopTailing did not return within 3s — tail goroutine failed to stop")
	}

	// A StopTailing after the goroutine has stopped must still be a cheap no-op
	// (and must not panic), proving the once-guarded close is reusable.
	ld.StopTailing()
}
