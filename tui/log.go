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
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LogDisplay shows the tail of the log file.
type LogDisplay struct {
	app    *tview.Application
	widget tview.Primitive
}

// NewLogDisplay creates a new log display that shows the last N lines.
func NewLogDisplay(app *tview.Application, logPath string, lines int) *LogDisplay {
	ld := &LogDisplay{app: app}

	logView := tview.NewTextView()
	logView.SetDynamicColors(true)
	logView.SetScrollable(true)
	logView.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	logView.SetBackgroundColor(tcell.ColorDefault)

	// Load initial content
	content := tailFile(logPath, lines)
	logView.SetText(content)

	// Title with file path
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("Log Viewer")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(logView, 0, 1, true)
	layout.SetBorder(true)

	ld.widget = layout
	return ld
}

// Widget returns the tview primitive for this display.
func (ld *LogDisplay) Widget() tview.Primitive {
	return ld.widget
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
