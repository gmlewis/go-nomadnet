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
	"github.com/rivo/tview"
)

// MainDisplay is the top-level layout with a menu bar and content area.
type MainDisplay struct {
	app         *tview.Application
	pages       *tview.Pages
	menuBar     *tview.Flex
	menuButtons []*tview.Button
	contentArea *tview.Pages
	activeMenu  int
	theme       int
	glyphs      GlyphSet
	onQuit      func()
}

// NewMainDisplay creates the main display with menu bar and content area.
func NewMainDisplay(app *tview.Application, theme int, glyphSetName string) *MainDisplay {
	glyphs := GetGlyphSet(glyphSetName)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	md := &MainDisplay{
		app:    app,
		pages:  tview.NewPages(),
		theme:  theme,
		glyphs: glyphs,
	}

	// Create menu bar
	md.menuBar = tview.NewFlex().SetDirection(tview.FlexColumn)
	md.menuButtons = make([]*tview.Button, len(MenuItems))

	for i, item := range MenuItems {
		btn := tview.NewButton(item.Label)
		btn.SetBackgroundColor(tcell.ColorDefault)
		idx := i
		btn.SetSelectedFunc(func() {
			md.selectMenu(idx)
		})
		md.menuButtons[i] = btn
		md.menuBar.AddItem(btn, 0, 1, false)
	}

	// Create content area
	md.contentArea = tview.NewPages()
	md.contentArea.SetBackgroundColor(tcell.ColorDefault)

	// Add placeholder content for each menu item
	for _, item := range MenuItems {
		placeholder := tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetTextColor(tcell.NewHexColor(0x999999)).
			SetText(fmt.Sprintf("\n\n%s\n\n[yellow]Content will appear here[-]", item.Label))
		md.contentArea.AddPage(item.Key, placeholder, true, false)
	}

	// Layout: menu bar on top, content below
	md.pages.AddPage("main",
		tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(md.menuBar, 1, 0, false).
			AddItem(md.contentArea, 0, 1, true),
		true, true)

	// Set up input handling
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return md.handleInput(event)
	})

	// Select first menu by default
	if len(MenuItems) > 0 {
		md.selectMenu(0)
	}

	return md
}

// SetDisplay replaces the placeholder for a menu key with a real display widget.
func (md *MainDisplay) SetDisplay(key string, widget tview.Primitive) {
	md.contentArea.AddPage(key, widget, true, false)
}

// selectMenu highlights the given menu item and switches content.
func (md *MainDisplay) selectMenu(index int) {
	if index < 0 || index >= len(MenuItems) {
		return
	}

	// Update button highlights
	for i, btn := range md.menuButtons {
		if i == index {
			btn.SetBackgroundColor(tcell.NewHexColor(0x666666))
		} else {
			btn.SetBackgroundColor(tcell.ColorDefault)
		}
	}

	md.activeMenu = index
	key := MenuItems[index].Key
	md.contentArea.SwitchToPage(key)
}

// handleInput processes keyboard shortcuts.
func (md *MainDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil
	case tcell.KeyTab:
		next := md.activeMenu + 1
		if next >= len(MenuItems) {
			next = 0
		}
		md.selectMenu(next)
		return nil
	case tcell.KeyBacktab:
		prev := md.activeMenu - 1
		if prev < 0 {
			prev = len(MenuItems) - 1
		}
		md.selectMenu(prev)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q', 'Q':
			if md.onQuit != nil {
				md.onQuit()
			}
			return nil
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			idx := int(event.Rune() - '1')
			if idx < len(MenuItems) {
				md.selectMenu(idx)
			}
			return nil
		case '0':
			if len(MenuItems) >= 10 {
				md.selectMenu(9)
			}
			return nil
		}
	}
	return event
}

// AddContentPage adds a page to the content area.
func (md *MainDisplay) AddContentPage(name string, item tview.Primitive) {
	md.contentArea.AddPage(name, item, true, false)
}

// SetQuitCallback sets the callback for quit action.
func (md *MainDisplay) SetQuitCallback(fn func()) {
	md.onQuit = fn
}

// SetGlyphs updates the glyph set used for display.
func (md *MainDisplay) SetGlyphs(name string) {
	md.glyphs = GetGlyphSet(name)
}

// Root returns the root tview primitive for the application.
func (md *MainDisplay) Root() tview.Primitive {
	return md.pages
}

// BuildMenuBarText creates a formatted menu bar string for display.
func BuildMenuBarText(activeIndex int) string {
	var parts []string
	for i, item := range MenuItems {
		if i == activeIndex {
			parts = append(parts, fmt.Sprintf("[::b]%s[::-]", item.Label))
		} else {
			parts = append(parts, item.Label)
		}
	}
	return strings.Join(parts, " | ")
}
