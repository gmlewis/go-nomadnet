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
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// messageListView wraps a tview.TextView so the conversation message list can
// extend each header row's colored background to the full pane width. tview's
// TextView only paints a color tag's background onto actual text characters,
// leaving trailing cells with the widget default — but Python's urwid AttrMap
// paints every cell of the row. The Draw override scans each visible row for a
// cell with a non-default background and repaints the trailing cells (from the
// end of the text to the right edge) with that background.
type messageListView struct {
	*tview.TextView

	// headerTitle is the header title string (which embeds the relative
	// timestamp, e.g. "1m ago") this entry was rendered with.
	headerTitle string
}

func newMessageListView() *messageListView {
	return &messageListView{TextView: tview.NewTextView()}
}

// Draw paints the text, then extends each row's colored background to the
// right edge of the pane. For each visible row, it finds the rightmost cell
// whose background is not the widget default (a header cell carrying the
// msg_header_<style> bg) and repaints every cell to its right with a fresh
// truecolor style built from that bg. The bg is reconstructed via NewRGBColor
// (from the cell's RGB components) because the Color returned by
// Style.Decompose may lack the ColorIsRGB flag, which tcell needs to emit a
// truecolor SGR.
func (m *messageListView) Draw(screen tcell.Screen) {
	m.TextView.Draw(screen)
	x, y, w, h := m.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	for row := range h {
		var rowBG tcell.Color
		styleCol := -1
		for col := w - 1; col >= 0; col-- {
			_, style, _ := screen.Get(x+col, y+row)
			_, bg, _ := style.Decompose()
			if bg != tcell.ColorDefault {
				rowBG = bg
				styleCol = col
				break
			}
		}
		if styleCol < 0 {
			continue
		}
		r, g, b := rowBG.RGB()
		// tcell fails to flush space cells whose truecolor bg exactly matches a
		// 256-color cube level (the bg SGR is emitted but the terminal retains
		// the default bg). Nudging the blue component off the nearest cube
		// level (capped to avoid overflow) makes tcell flush the fill. The
		// 1-unit shift is imperceptible on the trailing-space background.
		if bb := b; bb == 0 || bb == 95 || bb == 135 || bb == 175 || bb == 215 {
			b = bb + 1
		} else if bb == 255 {
			b = 254
		}
		fillStyle := tcell.StyleDefault.Background(tcell.NewRGBColor(r, g, b))
		for col := styleCol + 1; col < w; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, fillStyle)
		}
	}
}

// ConversationWidget displays a single conversation's messages,
// peer info header, trust banner, and compose editor.
// Matches Python's ConversationWidget at Conversations.py:1874.
type ConversationWidget struct {
	app    *App
	source string // source hash hex

	// Peer info data (injected by the caller; tui must not import the app
	// package). DisplayName "" → falls back to <full hash> (RNS.prettyhexrep).
	// StampCost nil → omit the "Stamp: N" segment. Hops nil → "unknown".
	TrustLevel  string
	DisplayName string
	StampCost   *int
	Hops        *int

	// OwnHash is this app's LXMF destination hash (app.lxmf_destination.hash),
	// injected by the wiring layer so LXMessageHeader can tell outbound from
	// inbound. OnOwnHash, when set, supersedes the cached OwnHash at every
	// render: Python reads app.lxmf_destination.hash FRESH inside
	// LXMessageWidget.__init__ (Conversations.py:2607), while a widget that
	// caches the value once could keep a nil/stale hash forever — e.g. when
	// the conversation was opened before the LXMF router finished registering
	// (a.LXMFDest still nil) — which classified every message, including
	// own-sent ones, down the inbound branch (green "✓ ←" headers on sent
	// messages, the reported bug). TimeFormat is the configured strftime
	// format (default "%Y-%m-%d %H:%M:%S", Python app.time_format).
	OwnHash    []byte
	OnOwnHash  func() []byte
	TimeFormat string

	// Layout
	frame       *tview.Flex
	headerFlex  *tview.Flex
	messageList *messageListBox
	peerInfoBar *tview.TextView
	// trustBanner is the header banner row (a Flex of the warning text and the
	// three buttons); bannerBtns holds the buttons in row order (Trust, Block,
	// Do nothing) for the keyboard traversal paths.
	trustBanner    *tview.Flex
	bannerBtns     []*tview.Button
	editor         *ReadlineEdit
	titleEditor    *ReadlineEdit
	fullEditorArea *tview.Flex
	footerArea     *tview.Flex
	// attachmentIndicator is the pending-attachments footer line (Python
	// _build_footer, Conversations.py:2167-2175). Lazily created by buildFooter.
	attachmentIndicator *tview.TextView
	widget              tview.Primitive

	fullEditorActive     bool
	sortByTimestamp      bool
	trustBannerDismissed bool

	// renderedTitles maps each rendered message's LXMF hash (hex) to the
	// header title string built at render time. RefreshRelativeTimes compares
	// freshly computed titles against these to detect when a relative-time
	// label ("1m ago") would change.
	renderedTitles map[string]string

	// Callbacks
	OnClose                  func()
	OnPurgeFailed            func()
	OnClearHistory           func()
	OnSend                   func(content, title string, attachments []string)
	OnAttach                 func()
	OnToggleFullscreen       func()
	OnPaperMessage           func(action, content, title string) (path string, ok bool)
	OnPaperMessageSaved      func(path string)
	OnPaperMessageFailed     func()
	OnPaperMessageRequested  func()
	OnAttachFiles            func(paths []string)
	OnSaveAttachments        func(refs []AttachmentRef)
	OnSaveFocusedAttachments func(refs []AttachmentRef)
	// Trust banner button callbacks (Python _on_trust_click/_on_block_click/
	// _on_ignore_click, Conversations.py:1989-2030).
	OnTrust  func()
	OnBlock  func()
	OnIgnore func()
	// OnResolveTrust, when set, returns the peer's CURRENT trust level string
	// ("trusted"/"untrusted"/"unknown"/"warning") read live from the directory
	// by the wiring layer. hasVisibleTrustBanner consults it so the banner
	// reflects the directory's actual trust — not the TrustLevel snapshot
	// captured at DisplayConversation time, which can be stale (e.g. the
	// conversation dir didn't exist yet when the New Conversation dialog
	// created the entry, so the snapshot was ""). This mirrors Python's
	// has_visible_trust_banner reading self.app.directory.trust_level(...)
	// fresh on each render (Conversations.py:1957-1960). Nil falls back to the
	// cached TrustLevel.
	OnResolveTrust func(sourceHex string) string

	// Dialog state
	dialogOpen bool

	// focusPart mirrors Python's ConversationFrame.focus_position ("header",
	// "body" or "footer"): it records which frame region last held focus so
	// the conversations display can restore it when Left/Right column
	// traversal moves focus into this pane (urwid Columns keeps each column's
	// own focus).
	focusPart string

	// OnSwitchToList is fired when Left reaches the right pane's non-consuming
	// widgets (message list, banner buttons) — Python's urwid Columns keypress
	// moves focus back to the conversations list column
	// (Conversations.py:221-229 columns_widget traversal).
	OnSwitchToList func()
	// OnBannerFocus fires when a trust-banner button gains focus (Python's
	// shortcuts() dispatch treats the frame header as the body shortcut bar,
	// Conversations.py:1772-1779).
	OnBannerFocus func()
	// OnBodyFocus fires when the message list gains focus (the body shortcut
	// bar region, Conversations.py:1772-1779).
	OnBodyFocus func()

	// pendingAttachments is the list of staged file paths to attach to the
	// next sent message (Python ConversationWidget.pending_attachments,
	// Conversations.py:1891,2167). Populated by ConfirmAttachFile (the file
	// browser selection); consumed and cleared by sendMessage.
	pendingAttachments []string

	// Message data
	messages []ConversationMessage
}

// ConversationMessage is a single message in a conversation view.
type ConversationMessage struct {
	Content     string
	Title       string
	Timestamp   time.Time
	IsSent      bool
	IsDelivered bool
	IsFailed    bool
	HasAttach   bool
	AttachCount int

	// LXMF wire fields used by LXMessageHeader for Python-parity rendering
	// (Conversations.py:2596-2670). When State/SourceHash are zero, the
	// legacy IsSent/IsDelivered/IsFailed bools drive a fallback derivation.
	State                int
	Method               int
	SourceHash           []byte
	Hash                 []byte // LXMF message hash — locates extracted attachments for C-s save
	TransportEncrypted   bool
	SignatureValidated   bool
	SignatureDescription string
	AttachmentTypes      []string
	AttachmentNames      []string
}

// NewConversationWidget creates a conversation view for the given source hash.
// Matches Python's ConversationWidget.__init__().
func NewConversationWidget(app *App, sourceHash string) *ConversationWidget {
	cw := &ConversationWidget{
		app:            app,
		source:         sourceHash,
		renderedTitles: map[string]string{},
	}
	tc := GetThemeColors(app.Theme)

	// Peer info bar — style "msg_header_sent" (ui/TextUI.py:35/88): 3-hex
	// fg #111 / bg #ddd in both themes. urwid cube-quantizes 3-hex even in
	// truecolor (#111→#000000, #ddd→#d7d7d7), so route through the palette
	// (which already cube-quantizes) rather than nibble-doubling to exact
	// #111111/#dddddd.
	cw.peerInfoBar = tview.NewTextView()
	cw.peerInfoBar.SetDynamicColors(true)
	cw.peerInfoBar.SetTextColor(tc["msg_header_sent_fg"])
	cw.peerInfoBar.SetBackgroundColor(tc["msg_header_sent_bg"])
	cw.updatePeerInfo()

	// Trust banner — style "msg_warning_untrusted" (TextUI.py:39/92): true-color
	// fg #111, bg dark red (#800000). Hidden by default; refreshTrustBanner
	// reveals it for non-trusted peers (Python _refresh_trust_banner,
	// Conversations.py:1962).
	cw.trustBanner = tview.NewFlex().SetDirection(tview.FlexColumn)
	cw.trustBanner.SetBackgroundColor(tcell.ColorMaroon)

	// Header: peer info (1 row) + optional trust banner (0 rows when hidden).
	header := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.peerInfoBar, 1, 0, false).
		AddItem(cw.trustBanner, 0, 0, false)
	cw.headerFlex = header
	cw.refreshTrustBanner()

	// Message list — a per-message selectable ListBox (Python messagelist,
	// IndicativeListBox of LXMessageWidget piles, Conversations.py:2286-2287).
	// It exposes the IndicativeListBox top/bottom visibility flags the frame
	// keypress branches on, and its bannerVisible hook lets the main
	// dispatcher's Up-at-top check respect the trust banner (A3/A7).
	cw.messageList = newMessageListBox()
	cw.messageList.bannerVisible = cw.hasVisibleTrustBanner
	cw.messageList.SetFocusFunc(func() {
		cw.focusPart = "body"
		if cw.OnBodyFocus != nil {
			cw.OnBodyFocus()
		}
	})

	// Minimal editor (content only) — Python builds MessageEdit(caption="",
	// edit_text="", multiline=True) wrapped in AttrMap(..., "msg_editor")
	// (Conversations.py:1916): an INVISIBLE empty one-line footer with no
	// caption and no placeholder (B2). msg_editor is 3-hex #111 / #0bb
	// (ui/TextUI.py:32/85), cube-quantized to #000000 / #00afaf.
	cw.editor = NewReadlineEdit(app.killRing, "", "")
	cw.editor.SetFieldBackgroundColor(tc["msg_editor_bg"])
	cw.editor.SetFieldTextColor(tc["msg_editor_fg"])

	// Title editor (hidden by default) — same msg_editor style.
	cw.titleEditor = NewReadlineEdit(app.killRing, "Title: ", "")
	cw.titleEditor.SetFieldBackgroundColor(tc["msg_editor_bg"])
	cw.titleEditor.SetFieldTextColor(tc["msg_editor_fg"])

	// Full editor (title + content)
	cw.fullEditorArea = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.titleEditor, 1, 0, false).
		AddItem(cw.editor, 0, 1, true)

	// Footer area switches between minimal and full editor, optionally
	// prepending the pending-attachments indicator (Python _build_footer,
	// Conversations.py:2160-2177). Populated by buildFooter after the frame
	// exists (it resizes the frame's footer slot).
	cw.footerArea = tview.NewFlex().SetDirection(tview.FlexRow)

	// Main frame: header | messages | editor. The header slot takes the
	// header pile's rendered row count (1 + banner when visible) instead of a
	// fixed 2, so a hidden trust banner leaves no blank row above the list.
	cw.frame = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, cw.headerPileRows(), 0, false).
		AddItem(cw.messageList, 0, 1, false).
		AddItem(cw.footerArea, 1, 0, true)
	cw.frame.SetBorder(true)
	cw.buildFooter()

	// Wire up keyboard shortcuts matching Python's ConversationWidget.keypress()
	cw.frame.SetInputCapture(cw.handleInput)

	cw.widget = cw.frame
	return cw
}

// Widget returns the tview primitive.
func (cw *ConversationWidget) Widget() tview.Primitive {
	return cw.widget
}

// SetMessages replaces the message list with the given messages.
func (cw *ConversationWidget) SetMessages(msgs []ConversationMessage) {
	cw.messages = msgs
	cw.renderMessages()
}

// ClearEditor clears the compose editor.
func (cw *ConversationWidget) ClearEditor() {
	cw.editor.SetText("")
	cw.titleEditor.SetText("")
	cw.pendingAttachments = nil
	cw.buildFooter()
}

// handleInput processes keyboard shortcuts for the conversation widget.
// With the compose editor focused the keys are routed through
// handleComposerKey first (Python's bottom-up dispatch: the focused composer
// consumes its keys before the widget shortcuts run); otherwise this behaves
// exactly like Python's ConversationWidget.keypress() at Conversations.py:2222.
func (cw *ConversationWidget) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if cw.composerHasFocus() {
		return cw.handleComposerKey(event)
	}
	return cw.handleWidgetKey(event)
}

// handleWidgetKey is the widget-level shortcut switch consumed by keys that no
// inner widget did.
func (cw *ConversationWidget) handleWidgetKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyUp:
		if !cw.dialogOpen {
			return cw.handleFrameUp(event)
		}
	case tcell.KeyDown:
		if !cw.dialogOpen {
			return cw.handleFrameDown(event)
		}
	case tcell.KeyLeft, tcell.KeyRight:
		if !cw.dialogOpen {
			return cw.handleFrameLeftRight(event)
		}
	case tcell.KeyCtrlW:
		if cw.OnClose != nil {
			cw.OnClose()
		}
		return nil
	case tcell.KeyCtrlU:
		if cw.OnPurgeFailed != nil {
			cw.OnPurgeFailed()
		}
		return nil
	case tcell.KeyCtrlX:
		if cw.OnClearHistory != nil {
			cw.OnClearHistory()
		}
		return nil
	case tcell.KeyCtrlT:
		cw.toggleEditor()
		return nil
	case tcell.KeyCtrlO:
		cw.sortByTimestamp = !cw.sortByTimestamp
		cw.renderMessages()
		return nil
	case tcell.KeyCtrlA:
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlF:
		// C-f → attach_file, same as C-a. Python's MessageEdit.keypress
		// (Conversations.py:1813) binds ctrl f to attach_file; the frame
		// keypress binds ctrl a (Conversations.py:2237). Both reach the same
		// action, so bind both here.
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlS:
		cw.saveFocusedAttachments()
		return nil
	case tcell.KeyCtrlP:
		// C-p → paper_message (Python MessageEdit.keypress,
		// Conversations.py:1811). Opens the paper-message dialog.
		cw.PaperMessageDialog()
		return nil
	case tcell.KeyCtrlD:
		cw.sendMessage()
		return nil
	case tcell.KeyCtrlG:
		if cw.OnToggleFullscreen != nil {
			cw.OnToggleFullscreen()
		}
		return nil
	case tcell.KeyTab:
		// Tab toggles focus between the editor (footer) and the message list
		// (body), matching Python ConversationWidget.keypress "tab" →
		// toggle_focus_area (Conversations.py:2219-2221 + 2206-2216). Tab is
		// CONSUMED (returns nil) so tview's default Flex focus traversal does NOT
		// run — otherwise Tab would cycle through the header (peer-info bar /
		// trust banner) too, never cleanly landing on the body, and the
		// "[Tab] ↑ Messages" / "[Tab] ↓ Editor" shortcut-bar claim would be a lie.
		cw.toggleFocusArea()
		return nil
	}

	return event
}

// composerHasFocus reports whether one of the compose editors currently holds
// focus. Python dispatches keys bottom-up, so with the composer focused its
// MessageEdit/ReadlineMixin keypress runs before the widget-level shortcuts
// ever see the key; tview input captures run top-down, so this check is the
// port's equivalent: it makes the frame capture YIELD the composer's editing
// keys, exactly the keys urwid's inner widgets would have consumed.
func (cw *ConversationWidget) composerHasFocus() bool {
	if cw.app == nil {
		return false
	}
	focused := cw.app.GetFocus()
	return focused == cw.editor || focused == cw.titleEditor
}

// handleComposerKey routes one key while the composer is focused, mirroring
// Python's MessageEdit.keypress (Conversations.py:1807-1825) which consumes
// only ctrl d/p/f/s (send/paper/attach/save) and its special "up" before
// handing everything else to ReadlineMixin. The readline editing keys
// (backspace, ctrl a/e/u/k/w/l/y, ctrl-arrows, plain arrows) fall through to
// the focused editor's own handler — including ctrl a/w/u, which the
// widget-level shortcuts (attach/close/purge) must NOT preempt here.
func (cw *ConversationWidget) handleComposerKey(event *tcell.EventKey) *tcell.EventKey {
	if cw.dialogOpen {
		return event
	}
	switch event.Key() {
	case tcell.KeyCtrlD:
		cw.sendMessage()
		return nil
	case tcell.KeyCtrlP:
		cw.PaperMessageDialog()
		return nil
	case tcell.KeyCtrlF:
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlS:
		cw.saveFocusedAttachments()
		return nil
	case tcell.KeyUp:
		// MessageEdit's special "up": a single-line composer always escapes at
		// cursor line 0 (Conversations.py:1816-1825).
		return cw.handleFrameUp(event)
	case tcell.KeyDown:
		// The full-editor title editor hands Down to the content editor (the
		// full_editor Pile focus moves to the next selectable element).
		return cw.handleFrameDown(event)
	case tcell.KeyCtrlT, tcell.KeyCtrlX, tcell.KeyCtrlG, tcell.KeyCtrlO:
		// Neither MessageEdit nor ReadlineMixin consumes these, so in Python
		// they bubble past the composer to the widget-level shortcuts (toggle
		// editor, clear history, fullscreen, sort) — keep them live while
		// typing.
		return cw.handleWidgetKey(event)
	}
	return event
}

// handleFrameUp implements the Python "up" focus path of an open conversation:
//
//   - minimal (content) editor at cursor y==0 → frame body (message list)
//     (Python MessageEdit.keypress, Conversations.py:1816-1825);
//   - full-editor content editor Up escapes to the title editor and the full
//     editor title editor Up → frame body (Python: urwid Edit returns "up" at
//     y==0 and the full_editor Pile moves focus to the previous selectable —
//     Conversations.py:1918-1930 — and MessageEdit "up" for the title editor
//     with the full editor active sets frame.focus_position = "body");
//   - message-list top: the trust banner's Trust button when a banner is
//     visible (ConversationFrame.keypress → _header_pile.focus_position = 1 +
//     focus_position = "header", Conversations.py:1854-1862), otherwise the
//     menu bar (main_display.frame.focus_position = "header");
//   - banner buttons Up → menu bar (Python: Frame header "up" result →
//     main_display.frame.focus_position = "header").
func (cw *ConversationWidget) handleFrameUp(event *tcell.EventKey) *tcell.EventKey {
	if cw.app == nil {
		return event
	}
	focused := cw.app.GetFocus()
	switch {
	case focused == cw.editor && !cw.fullEditorActive:
		cw.app.SetFocus(cw.messageList)
		return nil
	case focused == cw.titleEditor && cw.fullEditorActive:
		cw.app.SetFocus(cw.messageList)
		return nil
	case focused == cw.editor && cw.fullEditorActive:
		// Single-line Go editor: Up always escapes (Python does at y==0).
		cw.app.SetFocus(cw.titleEditor)
		return nil
	case cw.isBannerButton(focused):
		if cw.app.Main != nil {
			cw.app.Main.FocusMenu()
		}
		return nil
	case focused == cw.messageList && cw.messageList.TopIsVisible():
		if buttons := cw.bannerButtons(); cw.hasVisibleTrustBanner() && len(buttons) > 0 {
			cw.app.SetFocus(buttons[0])
			return nil
		}
		if cw.app.Main != nil {
			cw.app.Main.FocusMenu()
		}
		return nil
	}
	return event
}

// handleFrameDown implements the Python "down" focus path of an open
// conversation:
//
//   - message-list bottom → frame footer composer (ConversationFrame.keypress
//     "down" + bottom_is_visible → focus_position = "footer",
//     Conversations.py:1866-1867); with the full editor active Python's footer
//     is the full_editor Pile whose focus lands on the title editor;
//   - banner buttons Down → frame body (Python: Frame header "down" result →
//     focus_position = "body");
//   - full-editor title editor Down → content editor (Python: urwid Edit
//     returns "down" at the last line and the full_editor Pile moves focus to
//     the next selectable).
func (cw *ConversationWidget) handleFrameDown(event *tcell.EventKey) *tcell.EventKey {
	if cw.app == nil {
		return event
	}
	focused := cw.app.GetFocus()
	switch {
	case focused == cw.messageList && cw.messageList.BottomIsVisible():
		if cw.fullEditorActive {
			cw.app.SetFocus(cw.titleEditor)
		} else {
			cw.app.SetFocus(cw.editor)
		}
		return nil
	case cw.isBannerButton(focused):
		cw.app.SetFocus(cw.messageList)
		return nil
	case focused == cw.titleEditor && cw.fullEditorActive:
		cw.app.SetFocus(cw.editor)
		return nil
	}
	return event
}

// handleFrameLeftRight implements the Python urwid Columns traversal for the
// conversation column (columns_widget, Conversations.py:221-229): the message
// list and the banner buttons do not consume Left/Right, so they bubble to the
// Columns. Left from the conversation column moves focus back to the
// conversations list; within the banner row Left/Right move between the
// Trust/Block/Do nothing buttons, and at the row's edges the key bubbles
// (leftmost Left → the list column, rightmost Right → dies at the last
// column). The editors consume Left/Right as cursor movement, so they fall
// through unchanged.
func (cw *ConversationWidget) handleFrameLeftRight(event *tcell.EventKey) *tcell.EventKey {
	if cw.app == nil {
		return event
	}
	focused := cw.app.GetFocus()
	if event.Key() == tcell.KeyLeft {
		if idx, ok := cw.bannerButtonIndex(focused); ok {
			if idx == 0 {
				if cw.OnSwitchToList != nil {
					cw.OnSwitchToList()
					return nil
				}
				return event
			}
			cw.app.SetFocus(cw.bannerButtons()[idx-1])
			return nil
		}
		if focused == cw.messageList && cw.OnSwitchToList != nil {
			cw.OnSwitchToList()
			return nil
		}
		return event
	}
	// Right
	if idx, ok := cw.bannerButtonIndex(focused); ok {
		if buttons := cw.bannerButtons(); idx < len(buttons)-1 {
			cw.app.SetFocus(buttons[idx+1])
		}
		// Rightmost button: the key bubbles to the Columns, which has no
		// further column — Python drops it.
		return nil
	}
	return event
}

// bannerButtons returns the current trust-banner buttons (empty when the
// banner is hidden).
func (cw *ConversationWidget) bannerButtons() []*tview.Button {
	return cw.bannerBtns
}

// BannerButtons is the exported view of the current trust-banner buttons
// (Trust, Block, Do nothing) for focus-path tests.
func (cw *ConversationWidget) BannerButtons() []*tview.Button {
	return cw.bannerButtons()
}

// isBannerButton reports whether focused is one of the trust-banner buttons.
func (cw *ConversationWidget) isBannerButton(focused tview.Primitive) bool {
	_, ok := cw.bannerButtonIndex(focused)
	return ok
}

// bannerButtonIndex returns the index of focused within the banner buttons.
func (cw *ConversationWidget) bannerButtonIndex(focused tview.Primitive) (int, bool) {
	for i, b := range cw.bannerBtns {
		if b == focused {
			return i, true
		}
	}
	return 0, false
}

// toggleFocusArea swaps focus between the composer (editor, frame footer) and
// the message list (body), matching Python's toggle_focus_area
// (Conversations.py:2206-2216): focused on the message list → focus the editor;
// focused on the editor → focus the message list. No-op when focus is elsewhere
// (e.g. on the trust banner) so Tab from the banner does not jump to the body.
func (cw *ConversationWidget) toggleFocusArea() {
	if cw.app == nil {
		return
	}
	focused := cw.app.GetFocus()
	switch focused {
	case cw.messageList:
		cw.app.SetFocus(cw.editor)
	case cw.editor, cw.titleEditor:
		cw.app.SetFocus(cw.messageList)
	}
}

// toggleEditor switches between minimal and full editor modes.
func (cw *ConversationWidget) toggleEditor() {
	cw.fullEditorActive = !cw.fullEditorActive
	cw.buildFooter()
}

// buildFooter rebuilds the footer area, mirroring Python's _build_footer
// (Conversations.py:2160-2177): a pending-attachments indicator line
// ("{file-glyph} N file(s): {basenames}") sits above the editor when
// pendingAttachments is non-empty; otherwise only the editor shows. The frame's
// footer slot is resized to the total row count.
func (cw *ConversationWidget) buildFooter() {
	cw.footerArea.Clear()
	if cw.attachmentIndicator != nil {
		cw.attachmentIndicator.Clear()
	}
	rows := 0
	if len(cw.pendingAttachments) > 0 {
		if cw.attachmentIndicator == nil {
			cw.attachmentIndicator = tview.NewTextView()
		}
		cw.attachmentIndicator.Clear()
		g := cw.glyphs()
		names := make([]string, len(cw.pendingAttachments))
		for i, p := range cw.pendingAttachments {
			names[i] = filepath.Base(p)
		}
		_, _ = fmt.Fprintf(cw.attachmentIndicator, "%v %v file(s): %v", g["file"], len(cw.pendingAttachments), strings.Join(names, ", "))
		cw.footerArea.AddItem(cw.attachmentIndicator, 1, 0, false)
		rows++
	}
	if cw.fullEditorActive {
		cw.footerArea.AddItem(cw.fullEditorArea, 2, 0, true)
		rows += 2
	} else {
		cw.footerArea.AddItem(cw.editor, 1, 0, true)
		rows++
	}
	if cw.frame != nil {
		cw.frame.ResizeItem(cw.footerArea, rows, 0)
	}
}

// footerIndicatorText returns the current pending-attachments indicator text
// (empty when no attachments are staged), for testing/parity verification.
func (cw *ConversationWidget) footerIndicatorText() string {
	if cw.attachmentIndicator == nil {
		return ""
	}
	return cw.attachmentIndicator.GetText(true)
}

// sendMessage sends the current editor content.
func (cw *ConversationWidget) sendMessage() {
	content := cw.editor.GetText()
	if content == "" {
		return
	}
	title := ""
	if cw.fullEditorActive {
		title = cw.titleEditor.GetText()
	}
	// Hand off the staged attachments (file paths) to the wiring layer so
	// it can build the LXMF FIELD_FILE_ATTACHMENTS field, then clear the
	// staging list (Python send_message + clear_editor,
	// Conversations.py:2412-2436,2294-2298).
	attachments := cw.pendingAttachments
	cw.pendingAttachments = nil
	if cw.OnSend != nil {
		cw.OnSend(content, title, attachments)
	}
	cw.ClearEditor()
}

// updatePeerInfo refreshes the peer info header bar, mirroring Python's
// _update_peer_info (Conversations.py:2084-2120): " {name} | {right} " where
// name is the display name or <full hash> (RNS.prettyhexrep), and right joins
// "Stamp: N" (when a stamp cost is known) and "{speed}{hops}" by two spaces.
// hops is "N hop"/"N hops"/"unknown". RNS-dependent fields (stamp cost, hop
// count, app-data-derived name) are injected by the caller via StampCost/Hops/
// DisplayName; nil values yield the same fallbacks Python uses when RNS data
// is unavailable.
func (cw *ConversationWidget) updatePeerInfo() {
	if cw.source == "" {
		cw.peerInfoBar.SetText(" No conversation selected")
		return
	}
	name := cw.DisplayName
	if name == "" {
		name = "<" + cw.source + ">" // RNS.prettyhexrep
	}

	g := cw.app.Glyphs
	if g == nil {
		g = glyphsUnicode
	}
	speed := g["speed"]

	var hopsStr string
	switch {
	case cw.Hops == nil:
		hopsStr = "unknown"
	case *cw.Hops == 1:
		hopsStr = "1 hop"
	default:
		hopsStr = fmt.Sprintf("%v hops", *cw.Hops)
	}

	var rightParts []string
	if cw.StampCost != nil {
		rightParts = append(rightParts, fmt.Sprintf("Stamp: %v", *cw.StampCost))
	}
	rightParts = append(rightParts, speed+hopsStr)

	cw.peerInfoBar.SetText(" " + name + " | " + strings.Join(rightParts, "  ") + " ")
}

// hasVisibleTrustBanner reports whether the trust banner should show,
// mirroring Python's has_visible_trust_banner (Conversations.py:1953-1960),
// which reads the directory trust level fresh each render. We prefer the live
// OnResolveTrust lookup (so a peer trusted via the New Conversation / Peer Info
// dialog is reflected immediately, even before the conversation list catches
// up) and fall back to the cached TrustLevel snapshot when no resolver is wired.
func (cw *ConversationWidget) hasVisibleTrustBanner() bool {
	if cw.trustBannerDismissed {
		return false
	}
	level := cw.TrustLevel
	if cw.OnResolveTrust != nil {
		if resolved := cw.OnResolveTrust(cw.source); resolved != "" {
			level = resolved
		}
	}
	return level != "trusted"
}

// refreshTrustBanner shows or hides the trust banner in the header pile,
// mirroring Python's _refresh_trust_banner (Conversations.py:1962-1973). The
// widget frame's header slot is resized along with it so a hidden banner
// collapses the header to a single row — Python's _header_pile is handed to
// urwid.Frame as the header (Conversations.py:1924), which always takes exactly
// the pile's rendered rows, leaving no blank row between the peer info bar and
// the message list.
func (cw *ConversationWidget) refreshTrustBanner() {
	if cw.hasVisibleTrustBanner() {
		cw.buildTrustBanner()
		cw.headerFlex.ResizeItem(cw.trustBanner, 1, 0)
	} else {
		cw.trustBanner.Clear()
		cw.bannerBtns = nil
		cw.headerFlex.ResizeItem(cw.trustBanner, 0, 0)
	}
	// The frame is built after the first refreshTrustBanner call in the
	// constructor; the initial AddItem below already uses the correct height.
	if cw.frame != nil {
		cw.frame.ResizeItem(cw.headerFlex, cw.headerPileRows(), 0)
	}
}

// headerPileRows returns the row count the header pile should occupy in the
// widget frame: the peer info bar (1 row) plus the trust banner (1 row) when
// it is visible.
func (cw *ConversationWidget) headerPileRows() int {
	rows := 1
	if cw.hasVisibleTrustBanner() {
		rows++
	}
	return rows
}

// buildTrustBanner populates the trust banner row, mirroring Python's
// _build_trust_banner (Conversations.py:1975-1987): a warning message on the
// left followed by Trust / Block / Do nothing buttons, on the dark-red
// "msg_warning_untrusted" background.
func (cw *ConversationWidget) buildTrustBanner() {
	cw.trustBanner.Clear()
	g := cw.app.Glyphs
	if g == nil {
		g = glyphsUnicode
	}
	fg := cubeHex3("#111")
	bg := tcell.ColorMaroon

	msg := tview.NewTextView()
	msg.SetDynamicColors(true)
	msg.SetText(" " + g["warning"] + " This peer isn't trusted yet.")
	msg.SetTextColor(fg)
	msg.SetBackgroundColor(bg)

	button := func(label string, fn func()) *tview.Button {
		b := tview.NewButton(label).SetSelectedFunc(fn)
		b.SetBackgroundColor(bg)
		b.SetLabelColor(fg)
		b.SetLabelColorActivated(tcell.ColorMaroon)
		b.SetBackgroundColorActivated(cubeHex3("#111"))
		// Focus part + shortcut bar: the banner lives in the frame header, so
		// Python's shortcuts() dispatches the body shortcut bar while a banner
		// button holds focus (Conversations.py:1772-1779 shortcuts():
		// frame.focus_position != "footer" → body_shortcuts).
		b.SetFocusFunc(func() {
			cw.focusPart = "header"
			if cw.OnBannerFocus != nil {
				cw.OnBannerFocus()
			}
		})
		return b
	}
	btnTrust := button("Trust", cw.trustClick)
	btnBlock := button("Block", cw.blockClick)
	btnNothing := button("Do nothing", cw.ignoreClick)
	cw.bannerBtns = []*tview.Button{btnTrust, btnBlock, btnNothing}
	spacer := func() *tview.TextView {
		s := tview.NewTextView()
		s.SetBackgroundColor(bg)
		return s
	}

	cw.trustBanner.SetDirection(tview.FlexColumn).
		AddItem(msg, 0, 1, false).
		AddItem(btnTrust, 8, 0, true).
		AddItem(spacer(), 1, 0, false).
		AddItem(btnBlock, 8, 0, false).
		AddItem(spacer(), 1, 0, false).
		AddItem(btnNothing, 13, 0, false).
		AddItem(spacer(), 1, 0, false)
	cw.trustBanner.SetBackgroundColor(bg)
}

// SetTrustLevel sets the peer's trust level and refreshes the trust banner
// (so trusting/blocking/dismissing updates the header immediately).
func (cw *ConversationWidget) SetTrustLevel(level string) {
	cw.TrustLevel = level
	cw.refreshTrustBanner()
}

// trustClick fires the OnTrust callback (Python _on_trust_click,
// Conversations.py:1989).
func (cw *ConversationWidget) trustClick() {
	if cw.OnTrust != nil {
		cw.OnTrust()
	}
}

// blockClick fires the OnBlock callback (Python _on_block_click).
func (cw *ConversationWidget) blockClick() {
	if cw.OnBlock != nil {
		cw.OnBlock()
	}
}

// ignoreClick dismisses the trust banner and fires OnIgnore (Python
// _on_ignore_click).
func (cw *ConversationWidget) ignoreClick() {
	cw.trustBannerDismissed = true
	cw.refreshTrustBanner()
	if cw.OnIgnore != nil {
		cw.OnIgnore()
	}
}

// renderMessages renders the message list into per-message entries (Python
// update_message_widgets builds one LXMessageWidget Pile per message and puts
// them in the messagelist IndicativeListBox, Conversations.py:2254-2304). Each
// entry carries the header line(s) FIRST (title string + style), then the
// indented content lines, then the trailing blank row — the LXMessageWidget
// Pile order [title, content, ""] (Conversations.py:2670-2762).
func (cw *ConversationWidget) renderMessages() {
	entries := make([]*messageListView, 0, len(cw.messages))
	rendered := make(map[string]string, len(cw.messages))
	for _, msg := range cw.messages {
		entry := cw.renderMessageEntry(msg)
		entries = append(entries, entry)
		rendered[hex.EncodeToString(msg.Hash)] = entry.headerTitle
	}
	cw.renderedTitles = rendered
	cw.messageList.SetEntries(entries)
}

// relativeTimesChanged reports whether any rendered message's header title
// (which embeds the relative timestamp, e.g. "1m ago") differs from what the
// same message would render with the current wall clock. Message headers are
// computed at render time; without a periodic refresh an open conversation's
// relative-time labels would freeze until the next event-driven reload.
func (cw *ConversationWidget) relativeTimesChanged() bool {
	if cw == nil || len(cw.messages) == 0 || cw.renderedTitles == nil {
		return false
	}
	for _, msg := range cw.messages {
		title, _ := LXMessageHeader(cw.headerInputs(msg))
		if cw.renderedTitles[hex.EncodeToString(msg.Hash)] != title {
			return true
		}
	}
	return false
}

// renderedMessageText returns the concatenated per-message entry text (tags
// stripped when strip is true) — the diagnostics view of what the message
// list renders, mirroring reading the old flat TextView.
func (cw *ConversationWidget) renderedMessageText(strip bool) string {
	var sb strings.Builder
	for _, e := range cw.messageList.entries {
		sb.WriteString(e.GetText(strip))
	}
	return sb.String()
}

// renderMessageEntry renders one message into its list entry view: the header
// line(s) first (LXMessageHeader title + msg_header style), then the indented
// content lines (Python "  "+line), an attachment count fallback row when the
// wire attachment names are absent, and the trailing empty row. Python wraps
// the title in AttrMap(..., "msg_header_<style>") (Conversations.py:2596-2670
// + TextUI.py palette); the messageListView.Draw override extends that
// background to the full row width (tview's TextView only paints a color tag's
// bg onto actual text characters).
func (cw *ConversationWidget) renderMessageEntry(msg ConversationMessage) *messageListView {
	var sb strings.Builder
	in := cw.headerInputs(msg)
	title, style := LXMessageHeader(in)
	// styleHeader appends the newline after the header row(s); the content
	// rows follow IMMEDIATELY (Python's LXMessageWidget Pile is
	// [title, content, ""] — no blank between title and content,
	// Conversations.py:2757-2762).
	sb.WriteString(cw.styleHeader(title, style))

	// Body: indent every content line two columns (Python LXMessageWidget
	// "  "+line for non-markdown content).
	for line := range strings.SplitSeq(msg.Content, "\n") {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if msg.HasAttach && len(msg.AttachmentNames) == 0 {
		fmt.Fprintf(&sb, "  [gray]%v %v attachment(s)[-]\n", cw.glyphs()["file"], msg.AttachCount)
	}
	sb.WriteString("\n")

	v := newMessageListView()
	v.SetDynamicColors(true)
	v.SetScrollable(true)
	// Python's messagelist is a bare IndicativeListBox with NO AttrMap
	// (Conversations.py:2287), so its base color is the terminal default;
	// message widgets carry their own styling. Do not impose a #bbbbbb
	// base (Python never does).
	v.SetTextColor(tcell.ColorDefault)
	v.SetBackgroundColor(tcell.ColorDefault)
	v.SetTextAlign(tview.AlignLeft)
	v.headerTitle = title
	v.SetText(sb.String())
	return v
}

// headerInputs builds MessageHeaderInputs for a message, deriving the LXMF
// wire fields from the legacy Is* bools when State/SourceHash are unset so
// older callers still render a sensible header.
func (cw *ConversationWidget) headerInputs(msg ConversationMessage) MessageHeaderInputs {
	// Python reads app.lxmf_destination.hash fresh on every widget build, so
	// resolve the own hash live here too: a one-shot snapshot taken while the
	// LXMF router was still initializing kept OwnHash nil for the widget's
	// whole lifetime and classified every message (sent included) as inbound.
	ownHash := cw.OwnHash
	if cw.OnOwnHash != nil {
		if h := cw.OnOwnHash(); len(h) > 0 {
			ownHash = h
		}
	}
	in := MessageHeaderInputs{
		Timestamp:            msg.Timestamp,
		Now:                  time.Now(),
		State:                msg.State,
		Method:               msg.Method,
		SourceHash:           msg.SourceHash,
		OwnHash:              ownHash,
		TransportEncrypted:   msg.TransportEncrypted,
		Title:                msg.Title,
		SignatureValidated:   msg.SignatureValidated,
		SignatureDescription: msg.SignatureDescription,
		AttachmentTypes:      msg.AttachmentTypes,
		AttachmentNames:      msg.AttachmentNames,
		TimeFormat:           cw.timeFormat(),
		Glyphs:               cw.glyphs(),
	}

	// Legacy fallback when the LXMF wire fields are unset.
	if in.State == 0 && in.SourceHash == nil && in.Method == 0 && !in.TransportEncrypted {
		switch {
		case msg.IsSent:
			own := in.OwnHash
			if own == nil {
				own = []byte{0x00}
			}
			in.OwnHash = own
			in.SourceHash = own
			in.TransportEncrypted = true
			switch {
			case msg.IsFailed:
				in.State = lxmfStateFailed
			case msg.IsDelivered:
				in.State = lxmfStateDelivered
			default:
				in.State = lxmfStateSent
				in.Method = lxmfMethodPropagated
			}
		case msg.IsFailed:
			in.State = lxmfStateFailed // failed_no_source
		default:
			// A message with no wire fields is one whose envelope could not
			// be loaded (Message.Load fell back to mtime-only metadata).
			// Python's ConversationMessage keeps _cached_source_hash = None in
			// that case, so LXMessageWidget's FIRST branch renders
			// "msg_header_failed" with the warning glyph and no arrow
			// (Conversations.py:2607-2609). Fabricating an inbound source with
			// a validated signature here instead rendered own-sent messages
			// with the green "✓ ←" inbound header — the reported direction
			// bug. Leave SourceHash nil so the header takes the same
			// no-source → msg_header_failed branch Python takes.
		}
	}
	return in
}

// styleHeader renders a header title with the urwid header style's tview
// foreground AND background colors, mirroring Python's
// AttrMap(..., "msg_header_<style>") (Conversations.py:2596-2670 + TextUI.py
// palette lines 33-38): each header style is a dark foreground (#111/#000,
// cube-quantized to #000000) on a colored background (e.g. msg_header_sent
// bg #ddd cube→#d7d7d7, msg_header_delivered bg #28b cube→#0087af). The
// messageListView.Draw override extends the bg to the full row width (tview's
// TextView only paints a color tag's bg onto actual text characters, not
// trailing cells).
func (cw *ConversationWidget) styleHeader(title, style string) string {
	theme := ThemeDark
	if cw.app != nil {
		theme = cw.app.Theme
	}
	tc := GetThemeColors(theme)
	fg, haveFG := tc[style+"_fg"]
	bg, haveBG := tc[style+"_bg"]
	if !haveFG && !haveBG {
		return title + "\n"
	}
	tag := buildColorTag(fg, bg)
	if tag == "" {
		return title + "\n"
	}
	return tag + title + "[-:-]\n"
}

// buildColorTag builds a tview color-tag prefix "[fg:bg]" from two tcell
// colors. A color whose Hex() is -1 (ColorDefault / invalid) is omitted from
// its position so tview leaves that channel at the widget default. Returns ""
// when both are default.
func buildColorTag(fg, bg tcell.Color) string {
	var fb strings.Builder
	fb.WriteByte('[')
	if h := fg.Hex(); h >= 0 {
		fmt.Fprintf(&fb, "#%06x", uint32(h)&0xffffff)
	}
	fb.WriteByte(':')
	if h := bg.Hex(); h >= 0 {
		fmt.Fprintf(&fb, "#%06x", uint32(h)&0xffffff)
	}
	fb.WriteByte(']')
	return fb.String()
}

// glyphs returns the glyph set for this widget, falling back to the unicode
// set when no app/glyphs are available.
func (cw *ConversationWidget) glyphs() GlyphSet {
	if cw.app != nil && cw.app.Glyphs != nil {
		return cw.app.Glyphs
	}
	return glyphsUnicode
}

// timeFormat returns the configured strftime format, defaulting to the Python
// app.time_format default.
func (cw *ConversationWidget) timeFormat() string {
	if cw.TimeFormat != "" {
		return cw.TimeFormat
	}
	return "%Y-%m-%d %H:%M:%S"
}

// FormatQRText creates a text-based QR-like display for an address.
// Matches Python's show_qr_dialog at Conversations.py:641.
func FormatQRText(data string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "┌%v┐\n", strings.Repeat("─", len(data)+4))
	fmt.Fprintf(&sb, "│  %v  │\n", data)
	fmt.Fprintf(&sb, "└%v┘\n", strings.Repeat("─", len(data)+4))
	return sb.String()
}

// ClearHistoryDialog shows a confirmation dialog before clearing
// conversation history. Matches Python's clear_history_dialog()
// at Conversations.py:2122.
func (cw *ConversationWidget) ClearHistoryDialog() {
	cw.dialogOpen = true
	cw.app.Dialogs.ShowConfirmDialog("Clear conversation history?", func() {
		cw.ConfirmClearHistory()
	}, func() {
		cw.DismissClearHistoryDialog()
	})
}

// ConfirmClearHistory confirms the clear history action, fires
// the OnClearHistory callback, and closes the dialog.
func (cw *ConversationWidget) ConfirmClearHistory() {
	cw.dialogOpen = false
	if cw.OnClearHistory != nil {
		cw.OnClearHistory()
	}
}

// DismissClearHistoryDialog closes the clear history dialog
// without taking action.
func (cw *ConversationWidget) DismissClearHistoryDialog() {
	cw.dialogOpen = false
}

// DialogOpen reports whether a dialog is currently open.
func (cw *ConversationWidget) DialogOpen() bool {
	return cw.dialogOpen
}

// PaperMessageDialog shows a dialog for choosing how to output a
// paper message: Print QR, Save QR, Save URI, or Cancel.
// Matches Python's paper_message() at Conversations.py:2505. The widget marks
// its dialog state open and delegates the actual dialog rendering to the
// wiring layer via OnPaperMessageRequested (the display's PaperMessageDialog
// shows the real button overlay).
func (cw *ConversationWidget) PaperMessageDialog() {
	cw.dialogOpen = true
	if cw.OnPaperMessageRequested != nil {
		cw.OnPaperMessageRequested()
	}
}

// paperAction runs a paper-message output action, mirroring Python's
// print_paper_message_qr / save_paper_message_qr / save_paper_message_uri
// (Conversations.py:2474-2503): read the editor content+title, short-circuit on
// empty content, fire OnPaperMessage(action, content, title) which returns the
// saved path and ok, then on success clear the editor (and for the save modes
// fire OnPaperMessageSaved with the path) or on failure fire
// OnPaperMessageFailed and leave the editor intact.
func (cw *ConversationWidget) paperAction(action string, saved bool) {
	cw.dialogOpen = false
	content := cw.editor.GetText()
	if content == "" {
		return
	}
	title := cw.titleEditor.GetText()
	if cw.OnPaperMessage == nil {
		return
	}
	path, ok := cw.OnPaperMessage(action, content, title)
	if !ok {
		if cw.OnPaperMessageFailed != nil {
			cw.OnPaperMessageFailed()
		}
		return
	}
	cw.ClearEditor()
	if saved && cw.OnPaperMessageSaved != nil {
		cw.OnPaperMessageSaved(path)
	}
}

// PaperMessagePrintQR fires OnPaperMessage with "PrintQR" and closes the dialog.
func (cw *ConversationWidget) PaperMessagePrintQR() { cw.paperAction("PrintQR", false) }

// PaperMessageSaveQR fires OnPaperMessage with "SaveQR" and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveQR() { cw.paperAction("SaveQR", true) }

// PaperMessageSaveURI fires OnPaperMessage with "SaveURI" and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveURI() { cw.paperAction("SaveURI", true) }

// DismissPaperMessageDialog closes the paper message dialog
// without taking action.
func (cw *ConversationWidget) DismissPaperMessageDialog() {
	cw.dialogOpen = false
}

// OpenAttachFileDialog opens a file browser dialog for selecting
// files to attach. Matches Python's attach_file() at
// Conversations.py:2438.
func (cw *ConversationWidget) OpenAttachFileDialog(startDir string) {
	cw.dialogOpen = true
}

// ConfirmAttachFile confirms the file selection, fires the
// OnAttachFiles callback, and closes the dialog.
func (cw *ConversationWidget) ConfirmAttachFile(paths []string) {
	cw.dialogOpen = false
	// Stage the selected file paths for the next send (Python appends to
	// pending_attachments in the file-browser selection handler,
	// Conversations.py:3057-3060).
	cw.pendingAttachments = append(cw.pendingAttachments, paths...)
	cw.buildFooter()
	if cw.OnAttachFiles != nil {
		cw.OnAttachFiles(paths)
	}
}

// DismissAttachFileDialog closes the attach file dialog
// without selecting any files.
func (cw *ConversationWidget) DismissAttachFileDialog() {
	cw.dialogOpen = false
}

// AttachmentRef represents a single attachment in a conversation.
// Matches Python's _collect_attachment_refs() at Conversations.py:2300: each
// ref carries the attachment's display name, field type, the owning message's
// LXMF hash (to locate the extracted attachment directory), and the field index
// within that message (to select the extracted file_ N).
type AttachmentRef struct {
	Name        string
	Type        string
	MessageHash []byte
	FieldIndex  int
}

// collectAttachmentRefs gathers every attachment across all messages, ordered
// by sort_timestamp descending (newest message first). Matches Python's
// _collect_attachment_refs() at Conversations.py:2300-2322: it iterates the
// conversation's messages sorted by sort_timestamp reverse and emits one ref
// per attachment (file or otherwise) on every message that has attachments.
func (cw *ConversationWidget) collectAttachmentRefs() []AttachmentRef {
	sorted := make([]ConversationMessage, len(cw.messages))
	copy(sorted, cw.messages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	var refs []AttachmentRef
	for _, msg := range sorted {
		if !msg.HasAttach || len(msg.AttachmentNames) == 0 {
			continue
		}
		for i, name := range msg.AttachmentNames {
			atype := "file"
			if i < len(msg.AttachmentTypes) && msg.AttachmentTypes[i] != "" {
				atype = msg.AttachmentTypes[i]
			}
			refs = append(refs, AttachmentRef{
				Name:        name,
				Type:        atype,
				MessageHash: msg.Hash,
				FieldIndex:  i,
			})
		}
	}
	return refs
}

// saveFocusedAttachments opens the save-attachments flow for the current
// conversation. Matches Python's save_focused_attachments() at
// Conversations.py:2324: it sets dialog_active, collects the attachment refs,
// and hands them to the delegate (wiring layer) to render the save dialog.
// Distinct from C-a/attach_file.
func (cw *ConversationWidget) saveFocusedAttachments() {
	cw.dialogOpen = true
	refs := cw.collectAttachmentRefs()
	if cw.OnSaveFocusedAttachments != nil {
		cw.OnSaveFocusedAttachments(refs)
	}
}

// SaveAttachmentsDialog shows a dialog with checkboxes for each
// attachment in the conversation, allowing the user to select
// which to save. Matches Python's save_focused_attachments()
// at Conversations.py:2324.
func (cw *ConversationWidget) SaveAttachmentsDialog(attachments []AttachmentRef) {
	cw.dialogOpen = true
}

// ConfirmSaveAttachments saves the selected attachments, fires
// the OnSaveAttachments callback with the selected refs, and closes the dialog.
func (cw *ConversationWidget) ConfirmSaveAttachments(refs []AttachmentRef) {
	cw.dialogOpen = false
	if cw.OnSaveAttachments != nil {
		cw.OnSaveAttachments(refs)
	}
}

// DismissSaveAttachmentsDialog closes the save attachments
// dialog without saving.
func (cw *ConversationWidget) DismissSaveAttachmentsDialog() {
	cw.dialogOpen = false
}
