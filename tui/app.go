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
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// applyDefaultStylesOnce guards the write to tview's library-global tview.Styles
// (mirroring applyBordersOnce): the values are constants, so applying them
// more than once is a no-op, and the sync.Once makes ApplyDefaultStyles
// idempotent and safe to call from concurrent goroutines (e.g. parallel tests
// each calling NewApp). TestMain fires it once before any parallel test runs.
var applyDefaultStylesOnce sync.Once

// App wraps tview.Application with NomadNet configuration.
type App struct {
	*tview.Application
	Main      *MainDisplay
	Theme     int
	ColorMode int
	Glyphs    GlyphSet
	GlyphSet  string
	Dialogs   *DialogManager
	Styles    *StyleRegistry
	killRing  *killRing

	// updates is a bounded queue of functions to run on the event loop, drained
	// by a single long-lived goroutine (drainUpdates) that calls tview's
	// blocking Application.QueueUpdateDraw serially. See QueueUpdateDraw.
	updates chan func()
	// done is closed by Stop to release drainUpdates and any producer blocked
	// in QueueUpdateDraw's select.
	done     chan struct{}
	stopOnce sync.Once

	// onPanic, when set, is invoked by the recover handlers in GoSafe and
	// drainUpdates for an unrecovered panic in a background goroutine.
	// cmd/gonomadnet installs a handler that restores the terminal and writes
	// the stack to a crash file instead of letting the runtime spray a
	// GOTRACEBACK dump at the (raw, alt-screen) terminal — which forces a manual
	// `reset` and bloats the terminal emulator's scrollback. When nil, a
	// recovered panic is re-panicked so tests still fail loudly.
	onPanic func(any)
}

// NewApp creates a new tview Application with the given theme, color depth,
// and glyph set. colorMode selects the palette variant (mono/16/256/true);
// pass ColorModeTrue for the shipped 24-bit default.
func NewApp(theme int, glyphSet string, colorMode int) *App {
	tviewApp := tview.NewApplication()
	tviewApp.EnableMouse(true)
	glyphs := GetGlyphSet(glyphSet)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	a := &App{
		Application: tviewApp,
		Theme:       theme,
		ColorMode:   colorMode,
		Glyphs:      glyphs,
		GlyphSet:    glyphSet,
		Dialogs:     &DialogManager{},
		Styles:      newStyleRegistry(),
		killRing:    &killRing{},
		updates:     make(chan func(), 128),
		done:        make(chan struct{}),
	}
	go a.drainUpdates()
	a.Styles.Register(theme, colorMode)
	ApplySingleLineBorders()
	ApplyDefaultStyles()

	a.Main = NewMainDisplay(a, theme, glyphSet)

	return a
}

// ApplyDefaultStyles overrides tview's library-global Styles so that pane
// backgrounds, border cells, border titles, and graphics use the TERMINAL
// DEFAULT color (transparent), matching the Python original. The Python
// urwid LineBox and pane backgrounds are emitted with `\x1b[0;39;49m`
// (default fg, default bg) — never a forced black background or white border.
//
// tview's stock Styles default to `PrimitiveBackgroundColor=ColorBlack` and
// `BorderColor=TitleColor=GraphicsColor=ColorWhite`, which box.go combines into
// white-on-black border cells (`\x1b[38;2;255;255;255;48;2;0;0;0`). On a light
// terminal that is a hard black box where Python is transparent.
//
// Idempotent and safe to call repeatedly: this only ever assigns the constant
// ColorDefault to these four fields. Widgets that need a specific fill (the
// menubar/shortcutbar at `menubar_bg`, msg headers, etc.) set their own
// SetBackgroundColor explicitly and are unaffected.
func ApplyDefaultStyles() {
	applyDefaultStylesOnce.Do(func() {
		tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
		tview.Styles.BorderColor = tcell.ColorDefault
		tview.Styles.TitleColor = tcell.ColorDefault
		tview.Styles.GraphicsColor = tcell.ColorDefault
	})
}

// SetRoot sets the root primitive for the application. The main display is
// mounted as the bottom page of a tview.Pages root owned by the dialog
// manager, so modal dialogs can overlay it without destroying the underlying
// screen.
func (a *App) SetRoot() {
	root := a.Dialogs.Init(a.Application, a.Main.Root())
	a.Application.SetRoot(root, true)
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	return a.Application.Run()
}

// ShowIntro displays the intro/splash widget as the root for the given number
// of seconds, then swaps to the main display root (SetRoot), matching Python's
// TextUI.py:223-232 (intro_display shown for intro_time, then display_main).
// A non-positive seconds shows the main display immediately.
func (a *App) ShowIntro(intro tview.Primitive, seconds float64) {
	if seconds <= 0 {
		a.SetRoot()
		return
	}
	a.Application.SetRoot(intro, true)
	duration := time.Duration(seconds * float64(time.Second))
	time.AfterFunc(duration, func() {
		a.QueueUpdateDraw(func() {
			a.SetRoot()
		})
	})
}

// Stop stops the tview application event loop.
func (a *App) Stop() {
	a.stopOnce.Do(func() { close(a.done) })
	a.Application.Stop()
}

// SetOnPanic installs the crash handler invoked by GoSafe and drainUpdates when
// a background goroutine panics. See the onPanic field docs.
func (a *App) SetOnPanic(fn func(any)) { a.onPanic = fn }

// GoSafe launches fn on a new goroutine whose panic is routed to onPanic (or
// re-panicked when no handler is set, preserving test behavior). Long-lived
// gonomadnet goroutines (tickers, the draw drainer, the unread-blink loop)
// should be launched through GoSafe so a panic in any of them restores the
// terminal and writes a crash file instead of killing the process mid-draw and
// leaving the tty in raw mode.
func (a *App) GoSafe(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if a.onPanic != nil {
					a.onPanic(r)
				} else {
					panic(r)
				}
			}
		}()
		fn()
	}()
}

// RestoreTerminal best-effort finalizes the tcell screen — exiting the
// alternate screen and restoring cooked termios — so a crash doesn't leave the
// terminal in raw mode spewing escape-sequence garbage (the state that forces a
// manual `reset`). Safe to call from any goroutine; never propagates a panic.
// The launcher's EXIT trap is the reliable backstop; this covers users running
// the binary directly.
func (a *App) RestoreTerminal() {
	if a == nil || a.Application == nil {
		return
	}
	defer func() { _ = recover() }()
	a.Application.Stop()
}

// drainUpdates is the single long-lived goroutine that serializes queued
// functions onto tview's blocking Application.QueueUpdateDraw. It exits when
// Stop closes done. At most one of these exists per App, replacing the prior
// one-goroutine-per-QueueUpdateDraw-call pattern.
func (a *App) drainUpdates() {
	defer func() {
		if r := recover(); r != nil {
			if a.onPanic != nil {
				a.onPanic(r)
			} else {
				panic(r)
			}
		}
	}()
	for {
		select {
		case f := <-a.updates:
			a.Application.QueueUpdateDraw(f)
		case <-a.done:
			return
		}
	}
}

// QueueUpdateDraw queues f to be executed on the application event loop.
//
// It is non-blocking and safe to call from any goroutine, including the event
// loop itself and during shutdown — the same deadlock-safety the prior
// go-spawned wrapper provided, but without spawning a goroutine per call.
//
// Why the old `go a.Application.QueueUpdateDraw(f)` is gone: tview's
// QueueUpdateDraw blocks on a buffered updates channel and then waits for the
// draw to finish. When the event loop cannot keep up (e.g. a slow terminal on a
// memory-constrained device), that channel fills and every per-call goroutine
// blocks forever, accumulating at the producer rate for the life of the process
// — unbounded memory growth. A live profile of a long-running node showed ~85%
// of allocated objects flowing through that wrapper's gowrap1. Handing f to a
// single drainer caps goroutines at one and bounds queued work to the channel
// buffer (no per-call allocation).
//
// If the buffer is full (the event loop is behind), the oldest pending update is
// dropped to make room for the newest, which reflects the most current state.
// Every caller here is a background refresh (tickers, network/fetch callbacks)
// that rebuilds UI state from the underlying model, so a dropped call only
// delays a redraw until the next producer tick (1-5s); no model state is lost.
func (a *App) QueueUpdateDraw(f func()) {
	if a == nil || a.Application == nil || f == nil {
		return
	}
	if a.updates == nil {
		// Not constructed via NewApp (e.g. a minimal test fixture): fall back to
		// the direct non-blocking spawn to preserve prior behavior.
		go a.Application.QueueUpdateDraw(f)
		return
	}
	select {
	case a.updates <- f:
		return
	case <-a.done:
		return
	default:
	}
	// Buffer full: drop the oldest pending update and try once more. The newest
	// update wins because it reflects the most current model state.
	select {
	case <-a.updates:
	default:
	}
	select {
	case a.updates <- f:
	case <-a.done:
	default:
	}
}

// SetQuitCallback sets the callback invoked when the user quits.
func (a *App) SetQuitCallback(fn func()) {
	if a.Main != nil {
		a.Main.SetQuitCallback(fn)
	}
}
