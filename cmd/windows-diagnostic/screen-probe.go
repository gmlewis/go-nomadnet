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

package main

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gmlewis/tcell/v2"
)

const (
	// traceLimit bounds how much of tcell's output is kept for the report.
	traceLimit = 1 << 16
	// traceQuote bounds how many captured bytes are quoted in the report.
	traceQuote = 400
	// maxFrameHeight bounds the calibration frame so a screen that wrongly
	// believes it is thousands of rows tall does not turn the probe into a
	// scroll test.
	maxFrameHeight = 400
)

// probeScreen builds the same screen tview asks for, initializes it, paints a
// calibration frame sized to what tcell reports, and waits for a key or the
// timeout. It returns only after the screen has been finalized, so the terminal
// is handed back before the report is printed.
//
// The screen probe is the ground truth the rest of the tool defers to: if
// tcell's measured size disagrees with the console window, that is measured,
// not inferred from tcell's source.
func probeScreen(console *ConsoleFacts, timeout time.Duration, trace bool) (facts *ScreenFacts) {
	facts = &ScreenFacts{}

	screen, err := tcell.NewScreen()
	if err != nil {
		facts.Backend = backendNone
		facts.NewScreenError = err.Error()
		return facts
	}
	facts.Backend = backendOf(screen)

	// The terminfo renderer writes its whole escape stream through the tty.
	// Wrapping that tty is the only way to see the bytes without changing
	// what tcell does. The console renderer writes through the Win32 API
	// instead, so there is nothing to capture there.
	var capture *captureTty
	if trace && facts.Backend == backendTerminfo {
		raw, err := tcell.NewDevTty()
		if err != nil {
			facts.TraceNote = fmt.Sprintf("could not open the terminal to trace it: %v", err)
		} else {
			capture = &captureTty{Tty: raw, limit: traceLimit}
			traced, err := tcell.NewTerminfoScreenFromTty(capture)
			if err != nil {
				facts.TraceNote = fmt.Sprintf("could not build the traced screen: %v", err)
				_ = raw.Close()
				capture = nil
			} else {
				screen = traced
			}
		}
	}

	if err := screen.Init(); err != nil {
		facts.InitError = err.Error()
		if capture != nil {
			_ = capture.Close()
		}
		return facts
	}
	facts.InitOK = true

	// One defer does all the teardown: recover from a panic, record where the
	// exit stream starts, finalize the screen (which is what restores the
	// terminal), and collect the trace. Defers registered after Init are
	// guaranteed a screen that can be finalized.
	defer func() {
		if r := recover(); r != nil {
			facts.InitError = fmt.Sprintf("panic during the screen probe: %v", r)
			facts.InitOK = false
		}
		if capture != nil {
			capture.markExit()
		}
		screen.Fini()
		if capture != nil {
			enter, exit := capture.snapshot()
			facts.Traced = true
			facts.BytesOut = capture.bytesWritten()
			facts.EnterQuoted = quotePrefix(enter)
			facts.ExitQuoted = quotePrefix(exit)
			facts.Enter = escapeSequences(enter)
			facts.Exit = escapeSequences(exit)
		}
	}()

	facts.Width, facts.Height = screen.Size()
	facts.Colors = screen.Colors()
	facts.HasMouse = screen.HasMouse()
	facts.Charset = screen.CharacterSet()
	if tty, ok := screen.Tty(); ok && tty != nil {
		facts.TtyPresent = true
		facts.TtyType = ttyType(tty)
	}

	paintCalibration(screen, console, timeout)
	screen.Show()
	waitForKey(screen, timeout)
	return facts
}

// backendOf names the renderer a screen was built from. tcell's terminfo
// screen is the only implementation whose Tty accessor reports ok, and it does
// so even before Init opens the terminal.
func backendOf(screen tcell.Screen) string {
	if screen == nil {
		return backendNone
	}
	if _, ok := screen.Tty(); ok {
		return backendTerminfo
	}
	return backendConsole
}

// ttyType names the terminal implementation tcell is writing to, unwrapping
// the trace wrapper so the report names the real tty (for example
// *tcell.winTty on Windows) rather than this tool's own adapter.
func ttyType(tty tcell.Tty) string {
	if capture, ok := tty.(*captureTty); ok {
		return fmt.Sprintf("%T", capture.Tty)
	}
	return fmt.Sprintf("%T", tty)
}

// paintCalibration draws a frame around the size tcell believes it has and
// states both sizes on screen. When the believed size is the console buffer,
// the note placed just below the real window makes the console scroll: the
// same symptom the client shows.
func paintCalibration(screen tcell.Screen, console *ConsoleFacts, timeout time.Duration) {
	width, height := screen.Size()
	style := tcell.StyleDefault
	screen.SetStyle(style)
	screen.Clear()

	drawFrame(screen, width, height, style)

	putString(screen, 2, 1, width, height, fmt.Sprintf("tcell reports %v x %v", width, height), style)
	if console != nil && console.BufferInfo {
		putString(screen, 2, 2, width, height, fmt.Sprintf(
			"console window %v x %v, screen buffer %v x %v",
			console.WindowWidth, console.WindowHeight, console.BufferWidth, console.BufferHeight), style)
		if console.WindowHeight > 2 && height > console.WindowHeight {
			putString(screen, 2, console.WindowHeight-1, width, height,
				"<- this is the real bottom of the window", style)
			putString(screen, 2, console.WindowHeight, width, height,
				"<- tcell is drawing below the window, so the console scrolls", style)
		}
	}
	putString(screen, 2, 4, width, height,
		fmt.Sprintf("press any key to finish (auto-exits in %v)", timeout), style)
}

// drawFrame draws a box around the screen tcell believes it has. The bottom
// edge is omitted past maxFrameHeight, because drawing it on a screen that
// wrongly believes it is 9001 rows tall would scroll the window thousands of
// lines just to paint one border.
func drawFrame(screen tcell.Screen, width, height int, style tcell.Style) {
	if width < 2 || height < 2 {
		return
	}
	for x := range width {
		screen.SetContent(x, 0, '─', nil, style)
	}
	if height <= maxFrameHeight {
		for x := range width {
			screen.SetContent(x, height-1, '─', nil, style)
		}
	}
	bottom := min(height-1, maxFrameHeight)
	for y := 1; y <= bottom; y++ {
		screen.SetContent(0, y, '│', nil, style)
		screen.SetContent(width-1, y, '│', nil, style)
	}
	screen.SetContent(0, 0, '┌', nil, style)
	screen.SetContent(width-1, 0, '┐', nil, style)
	if height <= maxFrameHeight {
		screen.SetContent(0, height-1, '└', nil, style)
		screen.SetContent(width-1, height-1, '┘', nil, style)
	}
}

// putString writes text at (x, y), clipped to the reported screen.
func putString(screen tcell.Screen, x, y, width, height int, text string, style tcell.Style) {
	if y < 0 || y >= height {
		return
	}
	cell := 0
	for _, r := range text {
		if x+cell >= width {
			return
		}
		screen.SetContent(x+cell, y, r, nil, style)
		cell++
	}
}

// waitForKey returns when a key is pressed or the timeout expires. Mouse and
// resize events are ignored. The polling goroutine exits when Fini stops the
// screen and PollEvent returns nil.
func waitForKey(screen tcell.Screen, timeout time.Duration) {
	pressed := make(chan struct{}, 1)
	go func() {
		for {
			event := screen.PollEvent()
			if event == nil {
				return
			}
			if _, ok := event.(*tcell.EventKey); ok {
				pressed <- struct{}{}
				return
			}
		}
	}()
	select {
	case <-pressed:
	case <-time.After(timeout):
	}
}

// captureTty is a tcell.Tty that records every byte tcell writes and passes it
// through unchanged. Wrapping the tty rather than the screen keeps tcell's own
// rendering path exactly as it is in the client.
type captureTty struct {
	tcell.Tty
	limit int

	mu      sync.Mutex
	buf     []byte
	written int
	mark    int
	marked  bool
}

// Write records the bytes (up to the capture limit) and then writes them to the
// real terminal.
func (c *captureTty) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.written += len(p)
	if room := c.limit - len(c.buf); room > 0 {
		c.buf = append(c.buf, p[:min(len(p), room)]...)
	}
	c.mu.Unlock()
	return c.Tty.Write(p)
}

// markExit records where in the capture the finalization output begins, so the
// enter and exit streams can be reported separately.
func (c *captureTty) markExit() {
	c.mu.Lock()
	c.mark, c.marked = len(c.buf), true
	c.mu.Unlock()
}

// snapshot returns the enter and exit streams captured so far.
func (c *captureTty) snapshot() (enter, exit []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mark := c.mark
	if !c.marked {
		mark = len(c.buf)
	}
	return bytes.Clone(c.buf[:mark]), bytes.Clone(c.buf[mark:])
}

// bytesWritten returns how many bytes tcell has written to the terminal.
func (c *captureTty) bytesWritten() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.written
}

// quotePrefix renders the start of a captured stream as a Go quoted string, so
// every escape byte is visible, capped at traceQuote bytes.
func quotePrefix(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	truncated := false
	if len(buf) > traceQuote {
		buf = buf[:traceQuote]
		truncated = true
	}
	q := strconv.Quote(string(buf))
	if truncated {
		q += " ..."
	}
	return q
}
