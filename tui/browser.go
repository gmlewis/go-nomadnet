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
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// BrowserDisplay provides URL-based page browsing.
type BrowserDisplay struct {
	app     *tview.Application
	widget  tview.Primitive
	layout  *tview.Flex
	urlBar  *ReadlineEdit
	content *tview.TextView
	history []string
	histIdx int
	onLoad  func(url string)

	// Keyboard shortcut callbacks (Python: BrowserFrame.keypress)
	OnDisconnect  func()
	OnBack        func()
	OnForward     func()
	OnReload      func()
	OnURLDialog   func()
	OnSaveNode    func()
	OnToggleFullscreen func()
	OnCopyURL     func()
}

// NewBrowserDisplay creates a new browser display.
func NewBrowserDisplay(app *tview.Application) *BrowserDisplay {
	bd := &BrowserDisplay{app: app}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Browser[-]")

	// URL bar
	bd.urlBar = NewReadlineEdit("URL: ", "Enter RNS address or lxmf:// URI")
	bd.urlBar.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	bd.urlBar.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Content area
	bd.content = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText("[gray]Enter a URL and press Enter to load a page[-]")

	// Navigation bar
	navBar := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0x999999)).
		SetText("[yellow]Enter[-] Load  [yellow]Ctrl-L[-] Back  [yellow]Ctrl-R[-] Forward  [yellow]Esc[-] URL bar")

	// Layout
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(bd.urlBar, 1, 0, false).
		AddItem(navBar, 1, 0, false).
		AddItem(bd.content, 0, 1, true)
	layout.SetBorder(true)
	layout.SetInputCapture(bd.handleInput)

	bd.layout = layout
	bd.widget = layout
	return bd
}

// handleInput processes keyboard shortcuts for the browser display.
// Matches Python's BrowserFrame.keypress() at Browser.py:21.
func (bd *BrowserDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlW:
		if bd.OnDisconnect != nil {
			bd.OnDisconnect()
		}
		return nil
	case tcell.KeyCtrlD:
		if bd.OnBack != nil {
			bd.OnBack()
		}
		return nil
	case tcell.KeyCtrlF:
		if bd.OnForward != nil {
			bd.OnForward()
		}
		return nil
	case tcell.KeyCtrlR:
		if bd.OnReload != nil {
			bd.OnReload()
		}
		return nil
	case tcell.KeyCtrlU:
		if bd.OnURLDialog != nil {
			bd.OnURLDialog()
		}
		return nil
	case tcell.KeyCtrlS:
		if bd.OnSaveNode != nil {
			bd.OnSaveNode()
		}
		return nil
	case tcell.KeyCtrlG:
		if bd.OnToggleFullscreen != nil {
			bd.OnToggleFullscreen()
		}
		return nil
	case tcell.KeyCtrlY:
		if bd.OnCopyURL != nil {
			bd.OnCopyURL()
		}
		return nil
	}

	return event
}

// Widget returns the tview primitive for this display.
func (bd *BrowserDisplay) Widget() tview.Primitive {
	return bd.widget
}

// LoadURL loads a URL and displays the content.
func (bd *BrowserDisplay) LoadURL(url string) {
	if url == "" {
		return
	}

	// If we're not at the end of history, truncate forward
	if bd.histIdx < len(bd.history)-1 {
		bd.history = bd.history[:bd.histIdx+1]
	}
	bd.history = append(bd.history, url)
	bd.histIdx = len(bd.history) - 1

	bd.displayURL(url)
}

// GoBack navigates to the previous URL in history.
func (bd *BrowserDisplay) GoBack() {
	if bd.histIdx > 0 {
		bd.histIdx--
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// GoForward navigates to the next URL in history.
func (bd *BrowserDisplay) GoForward() {
	if bd.histIdx < len(bd.history)-1 {
		bd.histIdx++
		bd.displayURL(bd.history[bd.histIdx])
	}
}

// displayURL shows a URL without modifying history.
func (bd *BrowserDisplay) displayURL(url string) {
	bd.urlBar.SetText(url)
	content := fmt.Sprintf("[::b]Loading: %s[-]\n\n", url)
	content += "[gray]In a full implementation, this would:[-]\n"
	content += "1. Parse the RNS address\n"
	content += "2. Establish a link\n"
	content += "3. Request the page\n"
	content += "4. Render the Micron content\n\n"
	content += fmt.Sprintf("[gray]URL: %s[-]", url)
	bd.content.SetText(content)
}

// renderContent renders Micron content for display.
func renderContent(text string) string {
	if strings.Contains(text, ">>") || strings.Contains(text, "`!") {
		nodes := micron.Parse(text)
		var sb strings.Builder
		for _, node := range nodes {
			switch node.Type {
			case micron.NodeHeading:
				sb.WriteString("[::b]")
				for _, child := range node.Children {
					sb.WriteString(child.Text)
				}
				sb.WriteString("[-]\n")
			case micron.NodeText:
				sb.WriteString(node.Text)
			case micron.NodeBold:
				sb.WriteString("[::b]")
			case micron.NodeReset:
				sb.WriteString("[-]")
			case micron.NodeDivider:
				sb.WriteString(strings.Repeat("─", 30) + "\n")
			default:
				sb.WriteString(node.Text)
			}
		}
		return sb.String()
	}
	return text
}
