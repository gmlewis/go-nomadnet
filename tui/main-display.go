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
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// MainDisplay is the top-level layout matching Python's MainFrame:
// header=menu bar, body=content area, footer=shortcut bar.
type MainDisplay struct {
	app               *App
	frame             *tview.Flex
	menuBar           *tview.TextView
	menuItems         []MenuItem
	contentArea       *tview.Pages
	shortcutBar       *tview.TextView
	shortcutTextRaw   string // raw (unwrapped) shortcut text; the bar renders a pre-wrapped copy
	shortcutWrapSrc   string // raw text last wrapped into the bar (cache invalidation)
	shortcutWrapW     int    // width last wrapped at (-1 ⇒ cache cold)
	activeMenu        int    // focused menu button (highlight); not necessarily the displayed page
	activePage        string // key of the currently displayed body page
	focusRegion       string // "body" (default) or "menu" — mirrors MainFrame.focus_position
	theme             int
	glyphs            GlyphSet
	onQuit            func()
	onEsc             func() bool // display-specific Esc handler; returns true if consumed
	shortcuts         map[string]string
	shortcutCallbacks map[string]func() string // per-page dynamic shortcut providers
	quitCh            chan struct{}
	mu                sync.Mutex
	hideGuide         bool
	unreadIndicator   bool        // true swaps the menu glyph to unread_menu (Main.py:220-230)
	hasUnread         func() bool // unread-conversation probe; nil ⇒ none (app injects via SetUnreadCheck)
	menuWidths        []int       // pixel widths of each menu item for click detection
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
		app:               app,
		theme:             theme,
		glyphs:            glyphs,
		shortcuts:         make(map[string]string),
		shortcutCallbacks: make(map[string]func() string),
		quitCh:            make(chan struct{}),
		focusRegion:       "body",
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
			x, y := event.Position()
			bx, by, bw, bh := md.menuBar.GetRect()
			if x >= bx && x < bx+bw && y >= by && y < by+bh {
				md.handleClick(x - bx)
			}
		}
		return action, event
	})

	// Create content area (individual displays add their own borders)
	md.contentArea = tview.NewPages()
	md.contentArea.SetBackgroundColor(tcell.ColorDefault)

	// Create shortcut bar (footer). It holds plain text (the Python original
	// styles it via the "shortcutbar" attr, not inline color tags), so dynamic
	// colors are OFF — otherwise tview would parse "[Tab]"/"[Enter]" tokens in
	// the shortcut text as color tags and strip them.
	md.shortcutBar = tview.NewTextView()
	md.shortcutBar.SetDynamicColors(false)
	md.shortcutBar.SetTextColor(colors["menubar_fg"])
	md.shortcutBar.SetBackgroundColor(colors["menubar_bg"])
	md.shortcutBar.SetTextAlign(tview.AlignLeft)
	// The Python footer is a wrapping urwid.Text whose height grows to the
	// wrapped row count (e.g. 2 rows for the long Conversations list bar at 80
	// cols). We pre-wrap the text ourselves with urwidSpaceWrap (matching
	// urwid's "space" wrap algorithm — see resizeShortcutBar) and feed the bar
	// newline-broken lines, so WordWrap is OFF: tview's WordWrap breaks at the
	// last space before a line overflows, which drops a word urwid fits exactly
	// (the Network bar's "Forward" at 80 cols). Wrap stays ON as a safety net
	// for any line that somehow exceeds the width.
	md.shortcutBar.SetWrap(true)
	md.shortcutBar.SetWordWrap(false)
	md.shortcutWrapW = -1

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
	// Resize the shortcut bar to its wrapped row count before the Flex lays
	// out its items each frame. The DrawFunc runs inside DrawForSubclass
	// (before Flex.Draw computes item rects), so the new fixed height takes
	// effect for this draw. Returning the rect unchanged leaves the inner
	// rect as-is (the frame has no border/padding).
	md.frame.SetDrawFunc(func(screen tcell.Screen, x, y, w, h int) (int, int, int, int) {
		md.resizeShortcutBar(w)
		return x, y, w, h
	})

	// Set up input handling
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return md.handleInput(event)
	})

	// Select first menu by default
	if len(md.menuItems) > 0 {
		md.selectMenu(0)
	}
	// Boot focus is the body (Python's MainFrame defaults focus_position to
	// "body"). selectMenu no longer drops focus (it stays in the menu on Enter,
	// matching Python's show_*), so establish the initial body focus explicitly.
	md.FocusBody()

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
		// SwitchToPage drives the focus chain, which fires SetFocusFunc
		// callbacks (e.g. ConversationsDisplay.setShortcutRegion). Those
		// callbacks call refreshShortcuts, whose TryLock cannot re-acquire
		// the mu we hold here, so they skip — refresh the cached shortcut
		// text ourselves now that the focus callbacks have run.
		md.updateShortcutsLocked()
	}
}

// SetShortcut sets the shortcut text for a display key.
func (md *MainDisplay) SetShortcut(key, text string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.shortcuts[key] = text
	md.updateShortcutsLocked()
}

// SetShortcutCallback registers a dynamic shortcut-bar provider for one page
// key. The provider is consulted only when that page is the displayed
// (activePage) one, so a display that switches bars by focus region (e.g.
// Conversations: list/editor/body) can supply the right text without
// overriding other pages' bars. This replaces the former single global
// callback that always returned the Conversations bar.
func (md *MainDisplay) SetShortcutCallback(key string, fn func() string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.shortcutCallbacks[key] = fn
	md.updateShortcutsLocked()
}

// refreshShortcuts updates the cached shortcut bar text without blocking. It
// is intended for focus callbacks (SetFocusFunc) that may fire while md.mu is
// already held by the caller on this goroutine — e.g. SetDisplay holds mu
// across SwitchToPage, which drives the focus chain and fires a display's
// setShortcutRegion callback. A plain updateShortcuts would deadlock trying to
// re-acquire the non-reentrant mu, so refreshShortcuts uses TryLock: when the
// lock is free (the normal event-loop focus change) it refreshes immediately;
// when it is held (the SetDisplay re-entrant case) it skips, and the holder
// refreshes via updateShortcutsLocked before releasing.
func (md *MainDisplay) refreshShortcuts() {
	if !md.mu.TryLock() {
		return
	}
	defer md.mu.Unlock()
	md.updateShortcutsLocked()
}

// updateShortcutsLocked refreshes the shortcut bar. Caller must hold md.mu.
// The footer follows the DISPLAYED page (Python Main.update_active_shortcuts),
// not the focused menu button, so it keys off activePage. A registered
// per-page callback wins over the static SetShortcut text.
func (md *MainDisplay) updateShortcutsLocked() {
	text := ""
	if cb := md.shortcutCallbacks[md.activePage]; cb != nil {
		text = cb()
	}
	if text == "" {
		if t, ok := md.shortcuts[md.activePage]; ok {
			text = t
		}
	}
	if text == md.shortcutTextRaw {
		return // no change; keep the cached wrapped text
	}
	md.shortcutTextRaw = text
	md.shortcutWrapW = -1 // invalidate; resizeShortcutBar re-wraps at next draw
}

// GetShortcutText returns the raw (unwrapped) shortcut bar text currently active.
func (md *MainDisplay) GetShortcutText() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.shortcutTextRaw
}

// resizeShortcutBar wraps the current shortcut text to width using urwid's
// "space" wrap algorithm (so the breaks land on the same columns as the Python
// footer), feeds the newline-broken lines to the bar, and sizes the Flex item
// to the wrapped row count (minimum 1). Called from the frame DrawFunc each
// draw; the (src,width) cache avoids re-wrapping when nothing changed. The
// DrawFunc runs inside Flex.Draw's DrawForSubclass — before Flex lays out and
// draws its children — so both the new text and the new fixed height take
// effect for this draw.
func (md *MainDisplay) resizeShortcutBar(width int) {
	if md.frame == nil || md.shortcutBar == nil {
		return
	}
	if md.shortcutTextRaw == md.shortcutWrapSrc && width == md.shortcutWrapW {
		return // cached; nothing to do
	}
	lines := urwidSpaceWrap(md.shortcutTextRaw, width)
	rows := len(lines)
	if rows < 1 {
		rows = 1
	}
	md.shortcutBar.SetText(strings.Join(lines, "\n"))
	md.frame.ResizeItem(md.shortcutBar, rows, 0)
	md.shortcutWrapSrc = md.shortcutTextRaw
	md.shortcutWrapW = width
}

// urwidSpaceWrap wraps text to width using urwid's "space" wrap algorithm
// (urwid/text_layout.py:240-352), so the shortcut bar breaks at the SAME
// columns as the Python footer. urwid fills each line to exactly `width`
// columns, then:
//   - if the rune at that column is a space, breaks there ("perfect space
//     wrap") — the break space is dropped and the next line starts after it;
//   - otherwise walks back to the previous space and breaks there (the break
//     space dropped);
//   - otherwise (no space on the line) hard-breaks at the fill column.
//
// This differs from tview's WordWrap, which breaks at the LAST space before a
// line would overflow — that drops a word urwid fits exactly (e.g. the Network
// bar's "Forward" landing at column 80). Embedded newlines are honored. The
// shortcut bar renders brackets literally (dynamic colors off), so there are no
// style tags to skip.
func urwidSpaceWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, seg := range strings.Split(text, "\n") {
		lines = append(lines, urwidWrapSegment(seg, width)...)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// urwidWrapSegment wraps a single newline-free segment per urwid's space wrap.
func urwidWrapSegment(seg string, width int) []string {
	runes := []rune(seg)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	idx := 0
	for idx < len(runes) {
		// Width of the remaining runes.
		remW := 0
		for _, r := range runes[idx:] {
			remW += cellWidth(r)
		}
		if remW <= width {
			lines = append(lines, string(runes[idx:]))
			return lines
		}
		// Position at which `width` columns have been consumed (urwid
		// calc_text_pos): a wide rune that would cross the boundary is left for
		// the next line.
		pos, _ := urwidCalcTextPos(runes, idx, width)
		if pos == idx {
			pos = idx + 1 // pathological: a rune wider than `width`; emit it
		}
		if runes[pos] == ' ' {
			// Perfect space wrap: break here, drop the break space.
			lines = append(lines, string(runes[idx:pos]))
			idx = pos + 1
			continue
		}
		// Walk back to the previous space.
		prev := -1
		for p := pos - 1; p > idx; p-- {
			if runes[p] == ' ' {
				prev = p
				break
			}
		}
		if prev >= 0 {
			lines = append(lines, string(runes[idx:prev]))
			idx = prev + 1
			continue
		}
		// No space on this line: hard-break at the fill column (any-wrap).
		lines = append(lines, string(runes[idx:pos]))
		idx = pos
	}
	return lines
}

// urwidCalcTextPos returns the index pos (into runes) at which `width` screen
// columns have been consumed starting from idx, plus the column count up to
// pos. A wide rune that would cross the boundary stops before it, matching
// urwid calc_text_pos.
func urwidCalcTextPos(runes []rune, idx, width int) (pos, cols int) {
	w := 0
	for p := idx; p < len(runes); p++ {
		cw := cellWidth(runes[p])
		if w+cw > width {
			return p, w
		}
		w += cw
		pos = p + 1
		cols = w
	}
	return pos, cols
}

// cellWidth returns the screen-column width of r, treating zero-width/combining
// runes as 1 so they always occupy a cell in the shortcut bar.
func cellWidth(r rune) int {
	if w := runewidth.RuneWidth(r); w >= 1 {
		return w
	}
	return 1
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
		w := 1 + runewidth.StringWidth(label)
		md.menuWidths = append(md.menuWidths, w)
	}

	md.menuBar.SetText(b.String())
}

// selectMenu ACTIVATES the given menu item: it becomes the highlighted button
// and the displayed body page, but focus is NOT moved to the body. This mirrors
// a urwid MenuButton press (Main.py show_* + update_active_sub_display), which
// swaps the body content but never touches MainFrame.focus_position — focus
// stays in the menu (header) until the user presses Tab/Down (Main.py
// MenuColumns:172-176). Used for Enter/Space, mouse click, and programmatic
// page switches; when focus is already in the body (e.g. a body action calling
// SelectPage) it remains in the body, pointing at the new page.
//
// The "quit" item is the exception: Python's handler.quit (Main.py:158-168)
// raises urwid.ExitMainLoop so the atexit-registered exit_handler runs the
// graceful shutdown (save directory, tear down RRC). It shows no page. So
// activating "quit" invokes the quit callback (which performs App.Shutdown +
// stops the UI) instead of switching the body. The callback runs OUTSIDE md.mu
// because it drives the full shutdown (Stop, which may re-enter the app).
func (md *MainDisplay) selectMenu(index int) {
	md.mu.Lock()
	if index < 0 || index >= len(md.menuItems) {
		md.mu.Unlock()
		return
	}
	key := md.menuItems[index].Key
	if key == "quit" {
		onQuit := md.onQuit
		md.mu.Unlock()
		if onQuit != nil {
			onQuit()
		}
		return
	}
	md.selectMenuLocked(index)
	md.mu.Unlock()
}

// SelectPage switches the body to the menu page with the given key (e.g.
// "conversations", "network") and drops focus to the body. It is the
// programmatic equivalent of clicking that menu button, for cross-page actions
// like Network's "Converse" opening the Conversations page. Unknown keys are a
// no-op.
func (md *MainDisplay) SelectPage(key string) {
	md.mu.Lock()
	idx := -1
	for i, item := range md.menuItems {
		if item.Key == key {
			idx = i
			break
		}
	}
	md.mu.Unlock()
	if idx >= 0 {
		md.selectMenu(idx)
	}
}

// selectMenuLocked is the lock-free inner of selectMenu; the caller must hold
// md.mu (used by SetHideGuide, which already holds the lock, to avoid a
// self-deadlock re-acquiring it). It does NOT drop focus to the body.
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
	indicator := md.glyphs["decoration_menu"]
	if md.unreadIndicator {
		if g := md.glyphs["unread_menu"]; g != "" {
			indicator = g
		}
	}
	offset := runewidth.StringWidth(indicator)
	if offset < 1 {
		offset = 1
	}
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
// Enter/Space activate the focused button (switch page, focus STAYS in the
// menu — Python's show_* does not move focus_position);
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
	// Body region. Up at the top of the focused list collapses focus to the
	// menu bar (MainFrame.focus_position = "header"), matching Python's
	// MainFrame where Up at the top of the body pile moves focus to the header
	// (Main.py MainFrame:80-86). tview.List clamps silently at the top — it
	// does NOT fire SetDoneFunc on Up-at-top (only on Escape) — so no page can
	// detect this on its own; the dispatcher must own the transition. Anything
	// else is forwarded to the page (pane focus, Esc→dialog, per-page keys).
	if event.Key() == tcell.KeyUp && md.bodyListAtTop() {
		md.FocusMenu()
		return nil
	}
	return event
}

// bodyListAtTop reports whether the currently-focused primitive is a list
// sitting at item 0 — the condition under which Up collapses focus to the
// menu. It recognizes both a bare *tview.List and an *IndicativeListBox (which
// wraps a List and is what the Conversations page actually focuses, since
// FocusBody chains SetFocus down through the Flex/Pages to the wrapped list).
// It is false for non-list primitives (TextView/Form/InputField), for lists
// not at the top, and whenever a modal dialog overlay is open (the dispatcher
// must not steal focus from an open dialog). An empty list reports item 0, so
// Up on an empty Conversations list still reaches the menu (matching Python).
func (md *MainDisplay) bodyListAtTop() bool {
	if md.app == nil {
		return false
	}
	if md.app.Dialogs != nil && md.app.Dialogs.Open() {
		return false
	}
	var list *tview.List
	switch v := md.app.GetFocus().(type) {
	case *tview.List:
		list = v
	case *IndicativeListBox:
		list = v.List
	default:
		return false
	}
	if list == nil {
		return false
	}
	return list.GetCurrentItem() == 0
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

// SetUnreadCheck installs the probe used by the unread-indicator blink. The
// tui package must not import the app package (that would be a cycle), so the
// app injects a callback wrapping app.HasUnreadConversations. nil ⇒ no unread.
func (md *MainDisplay) SetUnreadCheck(fn func() bool) {
	md.mu.Lock()
	md.hasUnread = fn
	md.mu.Unlock()
}

// updateUnreadIndicator is the synchronous core of the Python
// MenuDisplay.update_display job (Main.py:216-230): probe for unread
// conversations and, when the result differs from the current indicator, swap
// the leading menu glyph (decoration_menu ⇄ unread_menu) and redraw the bar.
// The probe runs OUTSIDE md.mu to avoid holding the lock across app work; only
// the read/swap/redraw is under the lock. It does no UI-thread marshalling, so
// it is safe to call directly from tests — production wraps it in
// QueueUpdateDraw via startUnreadBlink(marshal=true).
func (md *MainDisplay) updateUnreadIndicator() {
	unread := false
	if fn := md.hasUnread; fn != nil {
		unread = fn()
	}
	md.mu.Lock()
	if md.unreadIndicator == unread {
		md.mu.Unlock()
		return
	}
	md.unreadIndicator = unread
	md.redrawMenuBar()
	md.mu.Unlock()
}

// StartUnreadBlink starts a background goroutine that probes for unread
// conversations every 2 s (Python UPDATE_INTERVAL, Main.py:194,216) and
// refreshes the menu indicator. Updates are marshalled onto the application
// event loop via QueueUpdateDraw because tview primitives must not be mutated
// concurrently with Draw.
func (md *MainDisplay) StartUnreadBlink() {
	md.startUnreadBlink(time.NewTicker(2*time.Second), true)
}

// startUnreadBlink runs the indicator loop on the given ticker. When marshal is
// true each tick's update is queued onto the application event loop
// (production); when false it runs synchronously (tests, where no event loop is
// running and QueueUpdateDraw would block forever on an undrained channel).
// The goroutine captures the quit channel at start so StopUnreadBlink can nil
// the field without leaving a live goroutine reading a nil channel; this also
// makes Start/Stop restartable (Stop closes the captured channel, the next
// Start mints a fresh one).
func (md *MainDisplay) startUnreadBlink(ticker *time.Ticker, marshal bool) {
	md.mu.Lock()
	if md.quitCh == nil {
		md.quitCh = make(chan struct{})
	}
	quit := md.quitCh
	md.mu.Unlock()
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				if marshal && md.app != nil {
					md.app.QueueUpdateDraw(md.updateUnreadIndicator)
				} else {
					md.updateUnreadIndicator()
				}
			}
		}
	}()
}

// StopUnreadBlink stops the unread blink goroutine. Idempotent: a second call
// (or a call after the channel was already closed and replaced) is a no-op.
func (md *MainDisplay) StopUnreadBlink() {
	md.mu.Lock()
	ch := md.quitCh
	md.quitCh = nil
	md.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// RequestRedraw forces a redraw after a short delay.
func (md *MainDisplay) RequestRedraw() {
	go func() {
		time.Sleep(250 * time.Millisecond)
		md.app.QueueUpdateDraw(func() {})
	}()
}
