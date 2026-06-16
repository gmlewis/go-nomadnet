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
	app              *tview.Application
	frame            *tview.Flex
	menuBar          *tview.TextView
	menuItems        []MenuItem
	contentArea      *tview.Pages
	shortcutBar      *tview.TextView
	activeMenu       int
	theme            int
	glyphs           GlyphSet
	onQuit           func()
	onEsc            func() bool // display-specific Esc handler; returns true if consumed
	shortcuts        map[string]string
	shortcutCallback func() string // returns dynamic shortcut text for active display
	quitCh           chan struct{}
	mu               sync.Mutex
	hideGuide        bool
	menuWidths       []int // pixel widths of each menu item for click detection
}

// NewMainDisplay creates the main display with Frame layout:
// header=menu bar, body=content area, footer=shortcut bar.
func NewMainDisplay(app *tview.Application, theme int, glyphSetName string) *MainDisplay {
	glyphs := GetGlyphSet(glyphSetName)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	colors := GetThemeColors(theme)

	md := &MainDisplay{
		app:       app,
		theme:     theme,
		glyphs:    glyphs,
		shortcuts: make(map[string]string),
		quitCh:    make(chan struct{}),
	}

	// Filter menu items based on hideGuide setting
	md.menuItems = make([]MenuItem, 0, len(MenuItems))
	for _, item := range MenuItems {
		if md.hideGuide && item.Key == "guide" {
			continue
		}
		md.menuItems = append(md.menuItems, item)
	}

	// Create menu bar as a single TextView matching Python's
	// urwid.AttrMap(MenuColumns(buttons), "menubar") pattern.
	// This renders menu items as a single row of text with mouse support.
	md.menuBar = tview.NewTextView()
	md.menuBar.SetBackgroundColor(colors["menubar_bg"])
	md.menuBar.SetTextColor(colors["menubar_fg"])
	md.menuBar.SetDynamicColors(true)
	md.menuBar.SetTextAlign(tview.AlignLeft)
	md.menuBar.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action&tview.MouseLeftClick != 0 {
			x, _ := event.Position()
			md.handleClick(x)
		}
		return action, event
	})

	// Create content area (individual displays add their own borders)
	md.contentArea = tview.NewPages()
	md.contentArea.SetBackgroundColor(tcell.ColorDefault)

	// Create shortcut bar (footer)
	md.shortcutBar = tview.NewTextView()
	md.shortcutBar.SetDynamicColors(true)
	md.shortcutBar.SetTextColor(colors["menubar_fg"])
	md.shortcutBar.SetBackgroundColor(colors["menubar_bg"])
	md.shortcutBar.SetTextAlign(tview.AlignLeft)

	// Add placeholder content for each menu item
	for _, item := range md.menuItems {
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
	if len(md.menuItems) > 0 {
		md.selectMenu(0)
	}

	return md
}

// SetDisplay replaces the placeholder for a menu key with a real display widget.
func (md *MainDisplay) SetDisplay(key string, widget tview.Primitive) {
	md.contentArea.AddPage(key, widget, true, false)
	if md.activeMenu >= 0 && md.activeMenu < len(md.menuItems) && key == md.menuItems[md.activeMenu].Key {
		md.contentArea.SwitchToPage(key)
	}
}

// SetShortcut sets the shortcut text for a display key.
func (md *MainDisplay) SetShortcut(key, text string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.shortcuts[key] = text
	md.updateShortcutsLocked()
}

// SetShortcutCallback registers a function that returns the current
// shortcut text for the active display. This enables dynamic shortcut
// bar updates when focus changes within a display.
func (md *MainDisplay) SetShortcutCallback(fn func() string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.shortcutCallback = fn
}

// updateShortcuts refreshes the shortcut bar for the active display.
func (md *MainDisplay) updateShortcuts() {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.updateShortcutsLocked()
}

// updateShortcutsLocked refreshes the shortcut bar. Caller must hold md.mu.
func (md *MainDisplay) updateShortcutsLocked() {
	if md.shortcutCallback != nil {
		if text := md.shortcutCallback(); text != "" {
			md.shortcutBar.SetText(text)
			return
		}
	}
	key := md.menuItems[md.activeMenu].Key
	if text, ok := md.shortcuts[key]; ok {
		md.shortcutBar.SetText(text)
	} else {
		md.shortcutBar.SetText("")
	}
}

// redrawMenuBar rebuilds the menu bar text and tracks item widths for
// mouse click detection.
func (md *MainDisplay) redrawMenuBar() {
	colors := GetThemeColors(md.theme)
	fg := colors["menubar_fg"]
	bg := colors["menubar_bg"]
	focusBg := colors["list_focus_bg"]

	var parts []string
	md.menuWidths = md.menuWidths[:0]

	for i, item := range md.menuItems {
		label := " " + item.Label + " "
		var styled string
		if i == md.activeMenu {
			styled = fmt.Sprintf("[#%06x:#%06x:b]%s[-:-:-]",
				int32(fg), int32(focusBg), label)
		} else {
			styled = fmt.Sprintf("[#%06x:#%06x]%s[-:-]",
				int32(fg), int32(bg), label)
		}
		parts = append(parts, styled)
		md.menuWidths = append(md.menuWidths, len([]rune(label)))
	}

	text := strings.Join(parts, "")
	md.menuBar.SetText(text)
}

// selectMenu highlights the given menu item and switches content.
func (md *MainDisplay) selectMenu(index int) {
	if index < 0 || index >= len(md.menuItems) {
		return
	}

	md.activeMenu = index
	md.redrawMenuBar()
	key := md.menuItems[index].Key
	md.contentArea.SwitchToPage(key)
	md.updateShortcuts()
}

// handleClick determines which menu item was clicked based on x position.
func (md *MainDisplay) handleClick(x int) {
	offset := 0
	for i, w := range md.menuWidths {
		if x >= offset && x < offset+w {
			md.selectMenu(i)
			return
		}
		offset += w
	}
}

// handleInput processes keyboard shortcuts matching Python's MainFrame keypress.
func (md *MainDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlQ:
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil

	case tcell.KeyCtrlD:
		return event

	case tcell.KeyEscape:
		// Let the active display handle Esc first (e.g., leaving
		// AnnounceInfo). Only quit if the display didn't consume it.
		if md.onEsc != nil && md.onEsc() {
			return nil
		}
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil

	case tcell.KeyLeft:
		prev := md.activeMenu - 1
		if prev < 0 {
			prev = len(md.menuItems) - 1
		}
		md.selectMenu(prev)
		return nil

	case tcell.KeyRight:
		next := md.activeMenu + 1
		if next >= len(md.menuItems) {
			next = 0
		}
		md.selectMenu(next)
		return nil

	case tcell.KeyTab:
		return event

	case tcell.KeyBacktab:
		prev := md.activeMenu - 1
		if prev < 0 {
			prev = len(md.menuItems) - 1
		}
		md.selectMenu(prev)
		return nil

	case tcell.KeyF8:
		return event

	case tcell.KeyRune:
		switch event.Rune() {
		case 'q', 'Q':
			if md.onQuit != nil {
				md.onQuit()
			}
			return nil
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			idx := int(event.Rune() - '1')
			if idx < len(md.menuItems) {
				md.selectMenu(idx)
			}
			return nil
		case '0':
			if len(md.menuItems) >= 10 {
				md.selectMenu(9)
			}
			return nil
		case 'g':
			return event
		}
	}

	// Refresh shortcut bar after any key event to capture focus changes
	md.updateShortcuts()
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

// SetEscCallback sets the display-specific Escape handler.
// Returns true if the event was consumed (should not quit).
func (md *MainDisplay) SetEscCallback(fn func() bool) {
	md.onEsc = fn
}

// SetGlyphs updates the glyph set used for display.
func (md *MainDisplay) SetGlyphs(name string) {
	md.glyphs = GetGlyphSet(name)
}

// SetHideGuide hides or shows the Guide menu button.
func (md *MainDisplay) SetHideGuide(hideGuide bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.hideGuide = hideGuide

	md.menuItems = make([]MenuItem, 0, len(MenuItems))
	for _, item := range MenuItems {
		if hideGuide && item.Key == "guide" {
			continue
		}
		md.menuItems = append(md.menuItems, item)
	}
	if md.activeMenu >= len(md.menuItems) {
		md.activeMenu = 0
	}
	md.selectMenu(md.activeMenu)
}

// BuildMenuBarText creates a formatted menu bar string for display.
func (md *MainDisplay) BuildMenuBarText() string {
	var parts []string
	for i, item := range md.menuItems {
		if i == md.activeMenu {
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
				md.mu.Lock()
				md.mu.Unlock()
			}
		}
	}()
}

// StopUnreadBlink stops the unread blink goroutine.
func (md *MainDisplay) StopUnreadBlink() {
	close(md.quitCh)
}

// RequestRedraw forces a redraw after a short delay.
func (md *MainDisplay) RequestRedraw() {
	go func() {
		time.Sleep(250 * time.Millisecond)
		md.app.QueueUpdateDraw(func() {})
	}()
}
