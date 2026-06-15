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
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MainDisplay is the top-level layout matching Python's MainFrame:
// header=menu bar, body=content area, footer=shortcut bar.
type MainDisplay struct {
	app          *tview.Application
	frame        *tview.Flex
	menuBar      *tview.Flex
	menuButtons  []*tview.Button
	contentArea  *tview.Pages
	shortcutBar  *tview.TextView
	activeMenu   int
	theme        int
	glyphs       GlyphSet
	onQuit       func()
	shortcuts    map[string]string // display key → shortcut text
	quitCh       chan struct{}
	mu           sync.Mutex
}

// NewMainDisplay creates the main display with Frame layout:
// header=menu bar, body=content area, footer=shortcut bar.
func NewMainDisplay(app *tview.Application, theme int, glyphSetName string) *MainDisplay {
	glyphs := GetGlyphSet(glyphSetName)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	md := &MainDisplay{
		app:       app,
		theme:     theme,
		glyphs:    glyphs,
		shortcuts: make(map[string]string),
		quitCh:    make(chan struct{}),
	}

	// Create menu bar with bracket-wrapped buttons (matching Python style)
	md.menuBar = tview.NewFlex().SetDirection(tview.FlexColumn)
	md.menuButtons = make([]*tview.Button, len(MenuItems))

	for i, item := range MenuItems {
		label := fmt.Sprintf("[%s]", item.Label)
		btn := tview.NewButton(label)
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

	// Create shortcut bar (footer)
	md.shortcutBar = tview.NewTextView()
	md.shortcutBar.SetDynamicColors(true)
	md.shortcutBar.SetTextColor(tcell.NewHexColor(0xdddddd))
	md.shortcutBar.SetBackgroundColor(tcell.NewHexColor(0x444444))
	md.shortcutBar.SetTextAlign(tview.AlignLeft)

	// Add placeholder content for each menu item
	for _, item := range MenuItems {
		placeholder := tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0x999999)).
			SetText(fmt.Sprintf("\n\n%s\n\n[yellow]Content will appear here[-]", item.Label))
		md.contentArea.AddPage(item.Key, placeholder, true, false)
	}

	// Layout: menu bar on top, content in middle, shortcuts at bottom
	md.frame = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(md.menuBar, 1, 0, false).
		AddItem(md.contentArea, 0, 1, true).
		AddItem(md.shortcutBar, 1, 0, false)

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
	if key == MenuItems[md.activeMenu].Key {
		md.contentArea.SwitchToPage(key)
	}
}

// SetShortcut sets the shortcut text for a display key.
func (md *MainDisplay) SetShortcut(key, text string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.shortcuts[key] = text
	md.updateShortcuts()
}

// updateShortcuts refreshes the shortcut bar for the active display.
func (md *MainDisplay) updateShortcuts() {
	md.mu.Lock()
	defer md.mu.Unlock()
	key := MenuItems[md.activeMenu].Key
	if text, ok := md.shortcuts[key]; ok {
		md.shortcutBar.SetText(text)
	} else {
		md.shortcutBar.SetText("")
	}
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
	md.updateShortcuts()
}

// handleInput processes keyboard shortcuts matching Python's MainFrame keypress.
func (md *MainDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// Global shortcuts (Python: unhandled_input)
	switch event.Key() {
	case tcell.KeyCtrlQ, tcell.KeyCtrlD:
		// Python: ctrl-q quits, ctrl-d passes through
		if event.Key() == tcell.KeyCtrlQ {
			if md.onQuit != nil {
				md.onQuit()
			}
			return nil
		}
		// ctrl-d passes through to children
		return event

	case tcell.KeyEscape:
		// Python: Escape quits
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil

	case tcell.KeyTab:
		// Python: Tab from menu bar → focus moves to body
		if md.activeMenu >= 0 && md.activeMenu < len(MenuItems) {
			md.contentArea.SwitchToPage(MenuItems[md.activeMenu].Key)
		}
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

// Root returns the root tview primitive for the application.
func (md *MainDisplay) Root() tview.Primitive {
	return md.frame
}

// SetQuitCallback sets the callback for quit action.
func (md *MainDisplay) SetQuitCallback(fn func()) {
	md.onQuit = fn
}

// SetGlyphs updates the glyph set used for display.
func (md *MainDisplay) SetGlyphs(name string) {
	md.glyphs = GetGlyphSet(name)
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

// StartUnreadBlink starts a goroutine that alternates the unread indicator.
func (md *MainDisplay) StartUnreadBlink() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-md.quitCh:
				return
			case <-ticker.C:
				// Toggle unread indicator in menu bar
				md.mu.Lock()
				// This would toggle an unread glyph on the menu
				md.mu.Unlock()
			}
		}
	}()
}

// StopUnreadBlink stops the unread blink goroutine.
func (md *MainDisplay) StopUnreadBlink() {
	close(md.quitCh)
}
