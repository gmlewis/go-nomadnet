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
	"os"
	"runtime/debug"
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
	contentArea       *bodyPages
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
		x, y := event.Position()
		bx, by, bw, bh := md.menuBar.GetRect()
		if x >= bx && x < bx+bw && y >= by && y < by+bh {
			// Activate the clicked item on the completed click only. handleClick
			// must NOT also fire on MouseLeftUp (which immediately precedes
			// MouseLeftClick) or the item's selectMenu runs twice.
			if action == tview.MouseLeftClick {
				md.handleClick(x - bx)
				// MouseConsumed (not 0) marks the event consumed so tview
				// redraws after the click. Returning 0 (MouseMove) left
				// consumed=false, so no redraw fired and the menu appeared
				// frozen until an unrelated async redraw painted it.
				return tview.MouseConsumed, nil
			}
			// Down/Up/Move inside the bar: swallow the event (nil) so the
			// default Box.MouseHandler does not steal focus to the menuBar,
			// but return 0 (not MouseConsumed) so these do not trigger a redraw.
			return 0, nil
		}
		return action, event
	})

	// Create content area (individual displays add their own borders). This is
	// a bodyPages (tview.Pages wrapper) so input dispatch only reaches the
	// VISIBLE page — see body-pages.go for the focus-stealing root cause it fixes.
	md.contentArea = newBodyPages()
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
			SetText(fmt.Sprintf("\n\n%v\n\n[yellow]Content will appear here[-]", item.Label))
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
	rows := max(len(lines), 1)
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
	for seg := range strings.SplitSeq(text, "\n") {
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

	var b strings.Builder
	md.menuWidths = md.menuWidths[:0]

	// Leading menu-indicator glyph column (Main.py:186-188, 226-230).
	indicator := md.glyphs["decoration_menu"]
	if md.unreadIndicator {
		if g := md.glyphs["unread_menu"]; g != "" {
			indicator = g
		}
	}
	indicatorStyled := fmt.Sprintf("[#%06x:#%06x]%v[-:-]",
		int32(fg), int32(bg), indicator)
	b.WriteString(indicatorStyled)

	for _, item := range md.menuItems {
		// "[ Name ]" matches urwid MenuButton (Main.py:35-37): button_left
		// "[" + " "+label+" " + button_right "]".
		//
		// Every button renders the uniform `menubar` style — Python wraps the
		// whole MenuColumns in AttrMap(columns, "menubar") with NO focus_map
		// (Main.py:211), so the active/focused button is NOT recolored or
		// bolded; it is indicated to the user only by the hardware cursor set
		// below. See TestMenuButtonsUniformMenubarStyle.
		label := "[ " + item.Label + " ]"
		styled := fmt.Sprintf("[#%06x:#%06x]%v[-:-]",
			int32(fg), int32(bg), label)
		// dividechars=1: one space between columns.
		b.WriteString(" ")
		b.WriteString(styled)
		w := 1 + runewidth.StringWidth(label)
		md.menuWidths = append(md.menuWidths, w)
	}

	md.menuBar.SetText(b.String())

	if md.focusRegion == "menu" || md.menuBar.HasFocus() {
		indicator := md.glyphs["decoration_menu"]
		if md.unreadIndicator {
			if g := md.glyphs["unread_menu"]; g != "" {
				indicator = g
			}
		}
		cx := runewidth.StringWidth(indicator)
		for i := 0; i < md.activeMenu && i < len(md.menuWidths); i++ {
			cx += md.menuWidths[i]
		}
		md.menuBar.SetDrawFunc(func(screen tcell.Screen, x, y, w, h int) (int, int, int, int) {
			screen.ShowCursor(x+cx+3, y)
			return x, y, w, h
		})
	} else {
		md.menuBar.SetDrawFunc(nil)
	}
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
	diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("FocusMenu activeMenu=%d", md.activeMenu))
	md.mu.Lock()
	md.focusRegion = "menu"
	md.redrawMenuBar()
	md.mu.Unlock()
	if md.app != nil {
		md.app.SetFocus(md.menuBar)
	}
}

// FocusBody moves focus to the content area (MainFrame.focus_position = "body").
//
// Focus is moved to the content area BEFORE redrawing the menu bar so that
// menuBar.HasFocus() is already false when redrawMenuBar runs — its else
// branch then clears the hardware-cursor DrawFunc, removing the solid green
// menu cursor. Without this redraw, the DrawFunc installed while the menu was
// focused keeps painting the cursor onto the menu bar on every frame, so the
// cursor appears to stay in the menu after Down/Tab even though tview focus
// has moved to the body (mirrors FocusMenu, which redraws to install it).
func (md *MainDisplay) FocusBody() {
	diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("FocusBody focusRegion=%q focus=%T", md.focusRegion, md.app.GetFocus()))
	if md.app != nil {
		md.app.SetFocus(md.contentArea)
	}
	md.mu.Lock()
	md.focusRegion = "body"
	md.redrawMenuBar()
	md.mu.Unlock()
}

// handleClick determines which menu item was clicked based on x position.
func (md *MainDisplay) handleClick(x int) {
	indicator := md.glyphs["decoration_menu"]
	if md.unreadIndicator {
		if g := md.glyphs["unread_menu"]; g != "" {
			indicator = g
		}
	}
	offset := max(runewidth.StringWidth(indicator), 1)
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
// Global (any region): Ctrl-Q is the documented quit (TextUI.py:262-264
// unhandled_input); Ctrl-C is also routed to quit (Python KeyboardInterrupt →
// clean exit + atexit save, NomadNetworkApp.py:38-42). Esc is NOT a quit — it
// is forwarded so the DialogManager overlay can close the top dialog (Phase
// 0.5). There are no digit menu shortcuts and no 'q' quit.
//
// Menu region (MainFrame.focus_position == "header", Main.py MenuColumns:171-176):
// Left/Right move the button highlight WITHOUT switching the body page;
// Enter/Space activate the focused button (switch page, focus STAYS in the
// menu — Python's show_* does not move focus_position);
// Tab/Down drop to the body without switching the page, and FORWARD the key
// to the body (Python MenuColumns.keypress sets focus_position="body" and
// urwid.Frame re-dispatches the key to the body, so one Down also advances
// the body list — e.g. the Interfaces list focuses item 0).
//
// Body region: Left/Right/Up/Tab are forwarded to the page (returned
// unconsumed) so the page can do pane focus and Up-at-top→FocusMenu. The main
// dispatcher never switches pages or quits from the body.
func (md *MainDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return event
	}

	// Invariant: a running app always has a focused primitive whose
	// HasFocus() returns true. Two failure modes are caught here:
	//
	//  1. nil focus: a.focus == nil (a SetFocus(nil), a container
	//     Focus delegate(nil), or a dialog dismiss that skipped
	//     restoration).
	//  2. zombie focus: a.focus is non-nil but root.HasFocus() returns
	//     false — e.g. a.focus points to a container (pileFiller,
	//     urwidColumns) whose Focus() didn't cascade to any child and
	//     didn't set its own hasFocus flag, or to a widget removed from
	//     the tree while a dialog was open. tview's event loop gate at
	//     application.go:439 silently drops ALL key events when
	//     root.HasFocus() is false, so the UI appears frozen (arrow keys
	//     do nothing, cursor disappears) with no stack trace because
	//     a.focus is non-nil.
	//
	// Recover BEFORE any dispatch so no key is ever lost, and dump a stack
	// so the violation surfaces immediately instead of festering for hours.
	// FocusBody cascades contentArea→page→…→a real primitive; if that still
	// fails, FocusMenu targets menuBar, which is always a valid non-nil
	// primitive.
	if md.app != nil {
		focus := md.app.GetFocus()
		root := md.app.GetRoot()
		if focus == nil || (root != nil && !root.HasFocus()) {
			dumpFocusInvariantViolation(fmt.Sprintf(
				"nil/zombie focus on key=%v focusRegion=%q focus=%T rootHasFocus=%v",
				event.Key(), md.focusRegion, focus, root != nil && root.HasFocus()))
			md.FocusBody()
			focus = md.app.GetFocus()
			root = md.app.GetRoot()
			if focus == nil || (root != nil && !root.HasFocus()) {
				md.FocusMenu()
			}
		}
	}

	if p := md.app.GetFocus(); p != nil {
		diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("HANDLE key=%v focusRegion=%q focus=%T menuBarHasFocus=%v", event.Key(), md.focusRegion, p, md.menuBar.HasFocus()))
	}

	// When an embedded terminal (the in-body editor) has focus, it owns ALL
	// keys — including Ctrl-C/Ctrl-Q, which editors like vim use — so the
	// global quit/menu logic must not intercept them. This mirrors Python's
	// urwid.Terminal, which grabs every key while focused (the editor is quit
	// via its own keys, e.g. nano Ctrl-X; the 'closed' signal then restores the
	// interfaces display). Forward the event unchanged so tview dispatches it
	// to the embedded terminal's InputHandler.
	if _, ok := md.app.GetFocus().(*EmbeddedTerminal); ok {
		return event
	}

	// Global quit — the only keys the dispatcher always owns. Ctrl-Q is the
	// documented quit (TextUI.py:262-264 unhandled_input). Ctrl-C is also
	// routed here: in Python, Ctrl-C raises KeyboardInterrupt which the urwid
	// loop catches to exit cleanly, and the atexit handler then saves the
	// directory (NomadNetworkApp.py:38-42). tcell runs the terminal in raw
	// mode so Ctrl-C arrives as a key event (KeyCtrlC) rather than SIGINT —
	// without this it would be a no-op and the user would lose all discovered
	// nodes on Ctrl-C, since the shutdown path (which saves the directory) is
	// the only one that persists it. Routing it through onQuit makes Shutdown
	// save the directory before stopping, mirroring Python's graceful-exit save.
	if event.Key() == tcell.KeyCtrlQ || event.Key() == tcell.KeyCtrlC {
		if md.onQuit != nil {
			md.onQuit()
		}
		return nil
	}

	// When a modal dialog is open, route Esc straight to DismissTop and pass
	// every other key through unchanged. The DialogLineBox already dismisses
	// on Esc (dialog.go), but tview dispatches a key only to the single
	// focused primitive (no bubbling), and after a mouse click opens a dialog
	// tview can leave focus on the dialog's content TextView rather than the
	// DialogLineBox — so Esc reaches the TextView (which ignores it) instead
	// of the dismiss handler, and the dialog appears stuck. Handling Esc here
	// (at the app-level capture, which runs before any focused primitive)
	// guarantees dismissal for every dialog regardless of where focus landed.
	// Other keys are returned so the dialog's own fields/buttons receive them
	// via the normal tview dispatch.
	if md.app != nil && md.app.Dialogs != nil && md.app.Dialogs.Open() {
		if event.Key() == tcell.KeyEscape {
			md.app.Dialogs.DismissTop()
			return nil
		}
		return event
	}

	if md.focusRegion == "menu" {
		return md.handleMenuInput(event)
	}
	if md.app != nil && md.app.GetFocus() == md.menuBar {
		md.FocusBody()
	}
	// Body region. Up at the top of the focused list collapses focus to the
	// menu bar (MainFrame.focus_position = "header"), matching Python's
	// MainFrame where Up at the top of the body pile moves focus to the header
	// (Main.py MainFrame:80-86). tview.List clamps silently at the top — it
	// does NOT fire SetDoneFunc on Up-at-top (only on Escape) — so no page can
	// detect this on its own; the dispatcher must own the transition. Anything
	// else is forwarded to the page (pane focus, Esc→dialog, per-page keys).
	if event.Key() == tcell.KeyUp {
		top := md.bodyListAtTop()
		diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("Up focusRegion=%q bodyListAtTop=%v focus=%T", md.focusRegion, top, md.app.GetFocus()))
		if top {
			md.FocusMenu()
			return nil
		}
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
	case *centeredText:
		// The network left pane swaps the saved-nodes IndicativeListBox for a
		// centeredText empty-state placeholder when no nodes are saved (Python
		// KnownNodes empty-state, Network.py:833-882). In Python the empty-state
		// widget is still the top of the body Pile, so MainFrame collapses focus
		// to the header on Up-at-top regardless of whether the top widget is a
		// ListBox or the placeholder. Treat the non-scrollable placeholder as an
		// empty list at item 0 so the dispatcher's Up-at-top→FocusMenu transition
		// still fires (without this, escapeToMenu on an empty left pane strands
		// focus — Up does nothing and the menu is never reached). Other
		// centeredText body placeholders (config explainer, node-info, empty
		// peers) get the same correct body-top→header parity.
		return true
	default:
		diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("bodyListAtTop default focus=%T", v))
		return false
	}
	if list == nil {
		return false
	}
	cur := list.GetCurrentItem()
	diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("bodyListAtTop list cur=%d", cur))
	return cur == 0
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
		// Mirror Python's MenuColumns.keypress (Main.py:172-176): it sets
		// frame.focus_position = "body" for Tab/Down and then returns
		// super().keypress, leaving the key unhandled. urwid.Frame.keypress then
		// re-dispatches that same key to the now-focused body, so a single Down
		// both enters the body AND advances its list (e.g. the Interfaces list
		// focuses item 0). FocusBody moves focus to the body; returning the
		// event (instead of nil) lets tview forward it to the body's focused
		// widget, matching that cascade. Consuming it (the old behavior) dropped
		// the key, so the first Down from the menu did nothing visible and the
		// user had to press Down a second time to move the cursor.
		md.FocusBody()
		return event
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
			parts = append(parts, fmt.Sprintf("[::b]%v[::-]", item.Label))
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
// The hasUnread field is read under md.mu (it is written by SetUnreadCheck
// under the same lock), but the probe callback runs OUTSIDE md.mu to avoid
// holding the lock across app work; only the snapshot/swap/redraw is under the
// lock. It does no UI-thread marshalling, so it is safe to call directly from
// tests — production wraps it in QueueUpdateDraw via startUnreadBlink(marshal=true).
func (md *MainDisplay) updateUnreadIndicator() {
	md.mu.Lock()
	fn := md.hasUnread
	md.mu.Unlock()
	unread := false
	if fn != nil {
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
	run := func() {
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
	}
	// Production (marshal, md.app set): route through App.GoSafe so a panic in
	// the blink loop restores the terminal + writes a crash file instead of
	// killing the process mid-draw. Tests (no app): a plain goroutine, so a
	// panic still fails the test.
	if marshal && md.app != nil {
		md.app.GoSafe(run)
	} else {
		go run()
	}
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

// diagFileMD appends a diagnostic line to a file (temporary debug).
func diagFileMD(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(line + "\n")
}

// focusInvariantDump is the sink for focus-invariant violations (a nil
// a.focus). It defaults to appending the offending stack to the diagnostic log
// so the culprit surfaces immediately; tests swap it to capture the dump. The
// program keeps running regardless — the caller recovers focus after dumping.
var focusInvariantDump = func(msg string, stack []byte) {
	diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("FOCUS INVARIANT VIOLATION: %v\n%s", msg, stack))
}

// SetFocusInvariantSink redirects the focus-invariant violation sink so dumps
// land in the application's real log (the file the "[ Log ]" menu tails)
// instead of the default /tmp scratch file. The wiring layer calls this once
// startup has created the app logger. Passing nil restores the default sink.
func SetFocusInvariantSink(fn func(msg string, stack []byte)) {
	if fn == nil {
		focusInvariantDump = func(msg string, stack []byte) {
			diagFileMD("/tmp/quit-diag.log", fmt.Sprintf("FOCUS INVARIANT VIOLATION: %v\n%s", msg, stack))
		}
		return
	}
	focusInvariantDump = fn
}

// dumpFocusInvariantViolation captures the current goroutine stack and reports
// a focus-invariant violation through focusInvariantDump.
func dumpFocusInvariantViolation(msg string) {
	focusInvariantDump(msg, debug.Stack())
}
