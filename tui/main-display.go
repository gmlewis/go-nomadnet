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
	app              *App
	frame            *tview.Flex
	menuBar          *tview.TextView
	menuItems        []MenuItem
	contentArea      *tview.Pages
	shortcutBar      *tview.TextView
	activeMenu       int    // focused menu button (highlight); not necessarily the displayed page
	activePage       string // key of the currently displayed body page
	focusRegion      string // "body" (default) or "menu" — mirrors MainFrame.focus_position
	theme            int
	glyphs           GlyphSet
	onQuit           func()
	onEsc            func() bool // display-specific Esc handler; returns true if consumed
	shortcuts        map[string]string
	shortcutCallback func() string // returns dynamic shortcut text for active display
	quitCh           chan struct{}
	mu               sync.Mutex
	hideGuide        bool
	unreadIndicator  bool  // true swaps the menu glyph to unread_menu (Main.py:220-230)
	menuWidths       []int // pixel widths of each menu item for click detection
}

// NewMainDisplay creates the main display with Frame layout:
// header=menu bar, body=content area, footer=shortcut bar.
func NewMainDisplay(app *App, theme int, glyphSetName string) *MainDisplay {
	glyphs := GetGlyphSet(glyphSetName)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	colors := GetThemeColors(theme)

	md := &MainDisplay{
		app:         app,
		theme:       theme,
		glyphs:      glyphs,
		shortcuts:   make(map[string]string),
		quitCh:      make(chan struct{}),
		focusRegion: "body",
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
// If key is the currently displayed page it is brought to the front.
func (md *MainDisplay) SetDisplay(key string, widget tview.Primitive) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.contentArea.AddPage(key, widget, true, false)
	if key == md.activePage {
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
// The footer follows the DISPLAYED page (Python Main.update_active_shortcuts),
// not the focused menu button, so it keys off activePage.
func (md *MainDisplay) updateShortcutsLocked() {
	if md.shortcutCallback != nil {
		if text := md.shortcutCallback(); text != "" {
			md.shortcutBar.SetText(text)
			return
		}
	}
	if text, ok := md.shortcuts[md.activePage]; ok {
		md.shortcutBar.SetText(text)
	} else {
		md.shortcutBar.SetText("")
	}
}

// redrawMenuBar rebuilds the menu bar text and tracks item widths for
// mouse click detection. It mirrors Python MenuDisplay (Main.py:178-211): a
// leading menu-indicator glyph (decoration_menu / unread_menu) followed by the
// MenuButton columns, each rendered as "[ Name ]" with a single space between
// columns (urwid Columns dividechars=1).
func (md *MainDisplay) redrawMenuBar() {
	colors := GetThemeColors(md.theme)
	fg := colors["menubar_fg"]
	bg := colors["menubar_bg"]
	focusBg := colors["list_focus_bg"]

	var b strings.Builder
	md.menuWidths = md.menuWidths[:0]

	// Leading menu-indicator glyph column (Main.py:186-188, 226-230).
	indicator := md.glyphs["decoration_menu"]
	if md.unreadIndicator {
		if g := md.glyphs["unread_menu"]; g != "" {
			indicator = g
		}
	}
	indicatorStyled := fmt.Sprintf("[#%06x:#%06x]%s[-:-]",
		int32(fg), int32(bg), indicator)
	b.WriteString(indicatorStyled)
	md.menuWidths = append(md.menuWidths, len([]rune(indicator)))

	for i, item := range md.menuItems {
		// "[ Name ]" matches urwid MenuButton (Main.py:35-37): button_left
		// "[" + " "+label+" " + button_right "]".
		label := "[ " + item.Label + " ]"
		var styled string
		if i == md.activeMenu {
			styled = fmt.Sprintf("[#%06x:#%06x:b]%s[-:-:-]",
				int32(fg), int32(focusBg), label)
		} else {
			styled = fmt.Sprintf("[#%06x:#%06x]%s[-:-]",
				int32(fg), int32(bg), label)
		}
		// dividechars=1: one space between columns.
		b.WriteString(" ")
		b.WriteString(styled)
		md.menuWidths = append(md.menuWidths, len([]rune(label)))
	}

	md.menuBar.SetText(b.String())
}

// selectMenu ACTIVATES the given menu item: it becomes the focused button AND
// the displayed body page, then focus drops to the body. This mirrors a urwid
// MenuButton press (Main.py show_* + update_active_sub_display) — used for
// Enter/Space, mouse click, and programmatic page switches.
func (md *MainDisplay) selectMenu(index int) {
	md.mu.Lock()
	md.selectMenuLocked(index)
	md.mu.Unlock()
	md.FocusBody()
}

// selectMenuLocked is the lock-free inner of selectMenu; the caller must hold
// md.mu (used by SetHideGuide, which already holds the lock, to avoid a
// self-deadlock via updateShortcuts). It does NOT drop focus to the body.
func (md *MainDisplay) selectMenuLocked(index int) {
	if index < 0 || index >= len(md.menuItems) {
		return
	}

	md.activeMenu = index
	md.redrawMenuBar()
	md.activePage = md.menuItems[index].Key
	md.contentArea.SwitchToPage(md.activePage)
	md.updateShortcutsLocked()
}

// focusMenuIndex moves the menu highlight to the given button WITHOUT switching
// the body page — this is what Left/Right do in the menu (Python MenuColumns
// only moves Columns focus; the button on_press fires on Enter/Space).
func (md *MainDisplay) focusMenuIndex(index int) {
	md.mu.Lock()
	defer md.mu.Unlock()
	if index < 0 || index >= len(md.menuItems) {
		return
	}
	md.activeMenu = index
	md.redrawMenuBar()
}

// FocusMenu moves focus to the menu bar (MainFrame.focus_position = "header").
// Body pages call this when an Up key reaches the top of their list.
func (md *MainDisplay) FocusMenu() {
	md.mu.Lock()
	md.focusRegion = "menu"
	md.redrawMenuBar()
	md.mu.Unlock()
	if md.app != nil {
		md.app.SetFocus(md.menuBar)
	}
}

// FocusBody moves focus to the content area (MainFrame.focus_position = "body").
func (md *MainDisplay) FocusBody() {
	md.mu.Lock()
	md.focusRegion = "body"
	md.mu.Unlock()
	if md.app != nil {
		md.app.SetFocus(md.contentArea)
	}
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

// handleInput implements the Python focus model. It runs as the app-level
// input capture, so it sees every key before the focused widget.
//
// Global (any region): Ctrl-Q is the only quit (TextUI.py:262-264
// unhandled_input). Esc is NOT a quit — it is forwarded so the DialogManager
// overlay can close the top dialog (Phase 0.5). There are no digit menu
// shortcuts and no 'q' quit.
//
// Menu region (MainFrame.focus_position == "header", Main.py MenuColumns:171-176):
// Left/Right move the button highlight WITHOUT switching the body page;
// Enter/Space activate the focused button (switch page + drop to body);
// Tab/Down drop to the body without switching.
//
// Body region: Left/Right/Up/Tab are forwarded to the page (returned
// unconsumed) so the page can do pane focus and Up-at-top→FocusMenu. The main
// dispatcher never switches pages or quits from the body.
func (md *MainDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return event
	}

	// Global quit — the only key the dispatcher always owns.
	if event.Key() == tcell.KeyCtrlQ {
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil
	}

	if md.focusRegion == "menu" {
		return md.handleMenuInput(event)
	}
	// Body region: forward everything (pane focus, Up-at-top→menu, Esc→dialog,
	// per-page keybindings) to the focused page widget.
	return event
}

// handleMenuInput dispatches keys while the menu bar is focused.
func (md *MainDisplay) handleMenuInput(event *tcell.EventKey) *tcell.EventKey {
	n := len(md.menuItems)
	if n == 0 {
		return event
	}

	switch event.Key() {
	case tcell.KeyLeft:
		prev := md.activeMenu - 1
		if prev < 0 {
			prev = n - 1
		}
		md.focusMenuIndex(prev)
		return nil
	case tcell.KeyRight:
		next := md.activeMenu + 1
		if next >= n {
			next = 0
		}
		md.focusMenuIndex(next)
		return nil
	case tcell.KeyEnter:
		md.selectMenu(md.activeMenu)
		return nil
	case tcell.KeyTab, tcell.KeyDown:
		md.FocusBody()
		return nil
	}

	// Space arrives as a Rune, not a Key.
	if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
		md.selectMenu(md.activeMenu)
		return nil
	}

	// Esc and everything else are forwarded (Esc lets the DialogManager close
	// an open dialog; other keys have no menu action).
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
	md.selectMenuLocked(md.activeMenu)
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
