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
	app    *App
	widget *tview.Flex
	body   *centeredText
}

// NewBrowserPane creates a "Remote Node" pane in the disconnected state.
func NewBrowserPane(app *App) *BrowserPane {
	bp := &BrowserPane{app: app}
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

// Widget returns the bordered "Remote Node" primitive.
func (bp *BrowserPane) Widget() tview.Primitive { return bp.widget }
