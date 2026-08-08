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

// Package tui implements the NomadNet terminal user interface.
//
// Log display: ports Python's Log.py. Python embeds an urwid.Terminal running
// `tail -fn50`; tview has no embedded terminal widget, so this substitute tails
// the file into a scrollable TextView. The "up" escape sequence that returns
// focus to the menu (Log.py:55-58) is handled here.

package tui

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LogDisplay shows the tail of the log file with optional live tailing.
type LogDisplay struct {
	app      *App
	widget   tview.Primitive
	logView  *tview.TextView
	logPath  string
	lines    int
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewLogDisplay creates a new log display that shows the last N lines and tails
// the file for live updates. The view starts following the end (latest lines),
// matching `tail -f`. Call StartTailing to begin watching.
func NewLogDisplay(app *App, logPath string, lines int) *LogDisplay {
	ld := &LogDisplay{
		app:     app,
		logPath: logPath,
		lines:   lines,
		stopCh:  make(chan struct{}),
	}

	ld.logView = tview.NewTextView()
	ld.logView.SetDynamicColors(true)
	ld.logView.SetScrollable(true)
	ld.logView.SetWrap(true)
	// Python's LogTerminal embeds urwid.Terminal (tail -f) wrapped only in a
	// LineBox with no AttrMap (Log.py:44-51), so the log text uses the terminal's
	// own default colors — there is no palette text fg.
	ld.logView.SetTextColor(tcell.ColorDefault)
	ld.logView.SetBackgroundColor(tcell.ColorDefault)

	// Load initial content and follow the end (Python `tail -f` shows latest).
	// The logfile (go-reticulum, matching Python RNS.format
	// `[timestamp] [Level]   message`) contains `[Info]` / `[Notice]` / `[Error]`
	// tokens. tview's TextView parses `[...]` as a color tag, so an unescaped
	// `[Notice]` is silently dropped from the visible text (B6: the Log page showed
	// `[timestamp]      message` with the level field blank). Python's Log.py
	// embeds urwid.Terminal running `tail -fn50`, which shows the brackets
	// literally. Escape tview color tags so the `[Level]` field is visible. The
	// log content has NO real tview color tags (plain log text), so escaping the
	// whole content is safe.
	content := tview.Escape(tailFile(logPath, lines))
	ld.logView.SetText(content)
	ld.logView.ScrollToEnd()

	// Python LogTerminal = urwid.LineBox(log_term) — a border, NO title row.
	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(ld.logView, 0, 1, true)
	layout.SetBorder(true)
	layout.SetInputCapture(ld.handleInput)

	ld.widget = layout
	return ld
}

// Widget returns the tview primitive.
func (ld *LogDisplay) Widget() tview.Primitive {
	return ld.widget
}

// handleInput implements the Python LogTerminal.keypress "up" escape
// (Log.py:55-58): Up at the top of the log returns focus to the menu bar. The
// centralized MainDisplay.bodyListAtTop only covers *tview.List, so this
// TextView-based page owns the transition. Up elsewhere is forwarded so the
// view scrolls up through history.
func (ld *LogDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return event
	}
	if event.Key() == tcell.KeyUp && ld.logAtTop() {
		if ld.app != nil && ld.app.Main != nil {
			ld.app.Main.FocusMenu()
		}
		return nil
	}
	return event
}

// logAtTop reports whether the log view is scrolled to its top (so Up should
// collapse focus to the menu). It is false while a modal dialog is open.
func (ld *LogDisplay) logAtTop() bool {
	if ld.app != nil && ld.app.Dialogs != nil && ld.app.Dialogs.Open() {
		return false
	}
	row, _ := ld.logView.GetScrollOffset()
	return row <= 0
}

// StartTailing begins watching the log file for new lines, appending them to the
// view. It polls the file at a fixed interval (avoiding the CPU spin of a
// busy-loop) and marshals each append onto the application event loop.
func (ld *LogDisplay) StartTailing() {
	file, err := os.Open(ld.logPath)
	if err != nil {
		return
	}
	// Seek to end to start tailing from here.
	_, _ = file.Seek(0, 2)

	ld.wg.Go(func() {
		defer func() { _ = file.Close() }()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		reader := bufio.NewReader(file)
		for {
			select {
			case <-ld.stopCh:
				return
			case <-ticker.C:
				// Drain any lines available since the last poll.
				for {
					line, err := reader.ReadString('\n')
					if line != "" {
						chunk := strings.TrimSuffix(line, "\n")
						ld.app.QueueUpdateDraw(func() {
							_, _ = ld.logView.Write([]byte(tview.Escape(chunk) + "\n"))
						})
					}
					if err != nil {
						break
					}
				}
			}
		}
	})
}

// StopTailing stops the live tail goroutine. It is idempotent and safe to call
// any number of times, including concurrently: the cleanup path that wires the
// log display into the application (cmd/gonomadnet/textui.go) can be invoked
// more than once when the user issues the quit key sequence, and an
// unconditional close here previously panicked with "close of closed channel".
func (ld *LogDisplay) StopTailing() {
	ld.stopOnce.Do(func() {
		close(ld.stopCh)
	})
	ld.wg.Wait()
}

// tailFile reads the last n lines from a file.
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "[red]Error reading log file: " + err.Error() + "[-]"
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) <= n {
		return string(data)
	}

	// Return last n lines.
	start := len(lines) - n
	return strings.Join(lines[start:], "\n")
}
