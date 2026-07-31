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
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LogDisplay shows the tail of the log file with optional live tailing.
type LogDisplay struct {
	app     *App
	widget  tview.Primitive
	logView *tview.TextView
	logPath string
	lines   int
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewLogDisplay creates a new log display that shows the last N lines
// and optionally tails the file for live updates.
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
	ld.logView.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	ld.logView.SetBackgroundColor(tcell.ColorDefault)

	// Load initial content
	content := tailFile(logPath, lines)
	ld.logView.SetText(content)

	// Title with file path
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("Log Viewer")

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(title, 1, 0, false)
	layout.AddItem(ld.logView, 0, 1, true)
	layout.SetBorder(true)

	ld.widget = layout
	return ld
}

// Widget returns the tview primitive.
func (ld *LogDisplay) Widget() tview.Primitive {
	return ld.widget
}

// StartTailing begins watching the log file for new lines.
func (ld *LogDisplay) StartTailing() {
	file, err := os.Open(ld.logPath)
	if err != nil {
		return
	}
	// Seek to end to start tailing from here
	_, _ = file.Seek(0, 2)

	ld.wg.Add(1)
	go func() {
		defer ld.wg.Done()
		defer func() { _ = file.Close() }()
		scanner := bufio.NewScanner(file)
		for {
			select {
			case <-ld.stopCh:
				return
			default:
				if scanner.Scan() {
					line := scanner.Text()
					ld.app.QueueUpdateDraw(func() {
						ld.logView.SetText(ld.logView.GetText(false) + "\n" + line)
					})
				}
			}
		}
	}()
}

// StopTailing stops the live tail goroutine.
func (ld *LogDisplay) StopTailing() {
	close(ld.stopCh)
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

	// Return last n lines
	start := len(lines) - n
	return strings.Join(lines[start:], "\n")
}
