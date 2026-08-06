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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// BrowserPane is the Network page's right pane: the "Remote Node" browser
// display. Matches Python's Browser.display_widget (Browser.py:486): a LineBox
// titled "Remote Node" wrapping a BrowserFrame whose body is a MIDDLE-filled,
// centered "Disconnected\n<arrow_l>  <arrow_r>" while no page is loaded
// (browser_inactive fg #444). URL fetching / page rendering arrive in Phase 5
// (the RNS link); until then the disconnected state is the boot appearance.
type BrowserPane struct {
	app     *App
	widget  *tview.Flex
	body    *centeredText
	display *BrowserDisplay
}

// NewBrowserPane creates a "Remote Node" pane in the disconnected state.
func NewBrowserPane(app *App) *BrowserPane {
	bp := &BrowserPane{app: app, display: NewBrowserDisplay(app)}
	bp.setDisconnected()
	return bp
}

// setDisconnected builds the bordered "Remote Node" pane with a vertically +
// horizontally centered "Disconnected\n<arrow_l>  <arrow_r>" (browser_inactive
// color), matching Python's build_display (Browser.py:472-488). Vertical
// centering uses two equal-weight spacers (tview Flex leftover-to-LAST gives
// top=floor, matching urwid Filler(MIDDLE)); horizontal centering uses
// centeredText (ceil-left, matching urwid Text(align=CENTER)).
func (bp *BrowserPane) setDisconnected() {
	color := GetThemeColors(bp.app.Theme)["browser_inactive"]
	if color == tcell.ColorDefault {
		color = tcell.NewHexColor(0x444444)
	}
	arrowL, arrowR := "<-", "->"
	if g := bp.app.Glyphs; g != nil {
		if gl := g["arrow_l"]; gl != "" {
			arrowL = gl
		}
		if gr := g["arrow_r"]; gr != "" {
			arrowR = gr
		}
	}
	bp.body = newCenteredText(color, "Disconnected", arrowL+"  "+arrowR)
	bp.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(bp.body, 2, 0, false).
		AddItem(tview.NewBox(), 0, 1, false)
	bp.widget.SetBorder(true)
	SetTitledBorder(bp.widget, "Remote Node")
}

// FormatRemoteNodeTitle formats the Remote Node pane title from a URL or hash,
// matching Python's Browser.py:569-576 (simplest_display_str -> <hash>).
func FormatRemoteNodeTitle(url string) string {
	if url == "" {
		return "Remote Node"
	}
	s := url
	for _, prefix := range []string{"nomadnetwork://", "lxmf://", "node://"} {
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			s = s[len(prefix):]
			break
		}
	}
	for i, r := range s {
		if r == '/' || r == ':' {
			s = s[:i]
			break
		}
	}
	if len(s) == 32 {
		return "<" + s + ">"
	}
	return s
}

// Disconnect tears down the Remote Node pane and resets it to the disconnected
// centered view, mirroring Python Browser.disconnect → update_display
// DISCONECTED (Browser.py:862-888, 549-559): the underlying BrowserDisplay's
// in-flight fetch is cancelled and its history/destination hint cleared
// (BrowserDisplay.Disconnect, which also swaps the loading body out via
// showContent), the pane body returns to the MIDDLE-centered
// "Disconnected\n<arrow_l>  <arrow_r>" (browser_inactive), and the LineBox title
// resets to "Remote Node". This is the Network page C-w handler: Python's
// NetworkDisplay.keypress (Network.py:1609-1610) calls
// self.parent.browser.disconnect() directly on ctrl-w.
//
// After resetting the pane, focus is released back to the owning view (the
// Network left list). The Network Columns is self-managing (network.go
// SetSelfManaging): it forwards Left/Right to the pane but does NOT do
// urwid-style column-arrow traversal, so unlike Python — where Left on the
// disconnected body bubbles to the Columns and moves focus to the list — a
// disconnected Go pane (the centered bp.body, with no LinkableText
// Left-at-start handler) would TRAP focus, and the test suite's recovery
// (C-w → Left → Home → Down → Enter) could never reopen Announce Info. The
// BrowserDisplay's OnReleaseFocus is wired to networkDisplay.FocusLists in the
// wiring layer (cmd/gonomadnet/textui.go), the same callback the loaded browser
// fires on Left-at-start; reuse it here so disconnecting returns focus to the
// list. It is released BEFORE Clear so SetFocus lands on the list while the
// (still-mounted) BrowserDisplay is the active focus target, not a dangling
// removed widget.
func (bp *BrowserPane) Disconnect() {
	if bp.display != nil {
		bp.display.Disconnect()
		if bp.display.OnReleaseFocus != nil {
			bp.display.OnReleaseFocus()
		}
	}
	bp.widget.Clear()
	bp.widget.AddItem(tview.NewBox(), 0, 1, false)
	bp.widget.AddItem(bp.body, 2, 0, false)
	bp.widget.AddItem(tview.NewBox(), 0, 1, false)
	SetTitledBorder(bp.widget, "Remote Node")
}

// LoadURL loads a URL and displays the content inside the Remote Node pane.
func (bp *BrowserPane) LoadURL(url string) {
	if bp.display != nil {
		bp.widget.Clear()
		bp.widget.AddItem(bp.display.Widget(), 0, 1, true)
		SetTitledBorder(bp.widget, FormatRemoteNodeTitle(url))
		bp.display.LoadURL(url)
	}
}

// BrowserDisplay returns the underlying BrowserDisplay instance for wiring.
func (bp *BrowserPane) BrowserDisplay() *BrowserDisplay {
	return bp.display
}

// Widget returns the bordered "Remote Node" primitive.
func (bp *BrowserPane) Widget() tview.Primitive { return bp.widget }
