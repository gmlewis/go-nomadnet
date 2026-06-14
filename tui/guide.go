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

// GuideDisplay shows help content rendered as Micron pages.
type GuideDisplay struct {
	app    *tview.Application
	widget tview.Primitive
}

// NewGuideDisplay creates a new guide display with help content.
func NewGuideDisplay(app *tview.Application) *GuideDisplay {
	gd := &GuideDisplay{app: app}

	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Nomad Network Guide[-]")

	content := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(guideContent())

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)

	gd.widget = layout
	return gd
}

// Widget returns the tview primitive for this display.
func (gd *GuideDisplay) Widget() tview.Primitive {
	return gd.widget
}

// guideContent returns the help text content.
func guideContent() string {
	return `[::b]Navigation[-]

Use [yellow]Tab[-] and [yellow]Shift-Tab[-] to switch between menu items.
Press [yellow]1[-] through [yellow]9[-] or [yellow]0[-] to jump to a menu item.
Press [yellow]q[-] or [yellow]Esc[-] to quit.

[::b]Keyboard Shortcuts[-]

  [yellow]Ctrl-A[-]  Beginning of line (in text fields)
  [yellow]Ctrl-E[-]  End of line
  [yellow]Ctrl-U[-]  Kill to beginning of line
  [yellow]Ctrl-K[-]  Kill to end of line
  [yellow]Ctrl-W[-]  Kill previous word
  [yellow]Ctrl-Y[-]  Yank (paste killed text)

[::b]Menu Items[-]

  [green]Network[-]     View announces, nodes, and propagation status
  [green]Conversations[-]  Send and receive messages
  [green]Channels[-]    Join RRC chat rooms
  [green]Directory[-]   Manage peer directory
  [green]Config[-]      View and edit configuration
  [green]Log[-]         View application logs
  [green]Guide[-]       This help guide
  [green]Interfaces[-]  View network interface status

[::b]Configuration[-]

Edit the config file at ~/.nomadnetwork/config
Restart the application for changes to take effect.

[::b]Micron Markup[-]

NomadNet uses Micron markup for page rendering:
  [yellow]>>[-]  Heading level 1
  [yellow]>>>[-]  Heading level 2
  [yellow]![/bold]bold text[/!][-]  Bold
  [yellow]_[/italic]italic text[/_][-]  Italic
  [yellow]/underline/[-]  Underline
  [yellow]"link":http://example.com[-]  Link
  [yellow]---[-]  Horizontal divider`
}
