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

// GuideDisplay shows help content with topic list and scrollable reader.
type GuideDisplay struct {
	app    *tview.Application
	widget *tview.Flex
	topics *tview.List
	reader *tview.TextView
}

// NewGuideDisplay creates a new guide display with two-column layout.
func NewGuideDisplay(app *tview.Application) *GuideDisplay {
	gd := &GuideDisplay{app: app}

	// Title
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]Nomad Network Guide[-]")

	// Topic list (left column)
	gd.topics = tview.NewList()
	gd.topics.SetHighlightFullLine(true)
	gd.topics.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	topics := []struct {
		name    string
		content string
	}{
		{"Introduction", introContent()},
		{"Concepts & Terminology", conceptsContent()},
		{"Channels & RRC", channelsContent()},
		{"Interfaces", interfacesContent()},
		{"Hosting a Node", nodeContent()},
		{"Configuration Options", configContent()},
		{"Keyboard Shortcuts", shortcutsContent()},
		{"Markup", markupContent()},
		{"First Run", firstRunContent()},
		{"Credits & Licenses", creditsContent()},
	}

	for _, topic := range topics {
		t := topic
		gd.topics.AddItem(t.name, "", 0, func() {
			gd.reader.SetText(t.content)
		})
	}

	// Content reader (right column)
	gd.reader = tview.NewTextView()
	gd.reader.SetDynamicColors(true)
	gd.reader.SetScrollable(true)
	gd.reader.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	gd.reader.SetTextAlign(tview.AlignLeft)
	gd.reader.SetText(introContent())

	// Layout: title on top, topics (33%) on left, reader (67%) on right
	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(gd.topics, 0, 1, true)
	content.AddItem(gd.reader, 0, 2, false)

	gd.widget = tview.NewFlex().SetDirection(tview.FlexRow)
	gd.widget.AddItem(title, 2, 0, false)
	gd.widget.AddItem(content, 0, 1, true)

	// Auto-select first topic
	if gd.topics.GetItemCount() > 0 {
		gd.topics.SetCurrentItem(0)
	}

	return gd
}

// Widget returns the tview primitive for this display.
func (gd *GuideDisplay) Widget() tview.Primitive {
	return gd.widget
}

// introContent returns the Introduction topic content.
func introContent() string {
	return `[::b]Welcome to Nomad Network[-]

NomadNet is a peer-to-peer information sharing system built on Reticulum.
It provides encrypted messaging, relay chat, and a decentralized web of
Micron pages hosted by community nodes.

[::b]Getting Started[-]

1. Configure your network interfaces in the Reticulum config
2. Start NomadNet and wait for connection
3. Browse the Network tab to discover peers
4. Send messages via the Conversations tab
5. Join chat rooms via the Channels tab

[::b]Key Concepts[-]

[yellow]Reticulum[-] - The encrypted network transport layer
[yellow]LXMF[-] - Lightweight Extensible Message Format (messaging protocol)
[yellow]RRC[-] - Reticulum Relay Chat (IRC-like chat rooms)
[yellow]Micron[-] - Lightweight markup language for pages
[yellow]Node[-] - A peer hosting pages and files`
}

// conceptsContent returns the Concepts & Terminology topic.
func conceptsContent() string {
	return `[::b]Concepts & Terminology[-]

[::b]Trust Levels[-]
  [green]● Trusted[-] - You have verified this peer's identity
  [yellow]○ Unknown[-] - This peer has not been verified
  [red]× Untrusted[-] - This peer has been flagged
  [yellow]⚠ Warning[-] - Identity collision detected

[::b]Delivery Methods[-]
  [cyan]Direct[-] - Messages sent directly to recipient
  [cyan]Propagated[-] - Messages forwarded through propagation nodes

[::b]Propagation Nodes[-]
  Special nodes that store and forward messages when direct
  delivery is not possible. They help messages reach peers
  that are not currently online.`
}

// channelsContent returns the Channels & RRC topic.
func channelsContent() string {
	return `[::b]Channels & RRC[-]

Reticulum Relay Chat (RRC) provides IRC-like chat rooms
over the Reticulum network.

[::b]Joining a Room[-]
  1. Go to the Channels tab
  2. Press [yellow]Ctrl-A[-] to join a room
  3. Enter the room name (e.g., #general)
  4. Press Enter to connect

[::b]Slash Commands[-]
  [yellow]/help[-]    Show help
  [yellow]/ping[-]    Ping the hub
  [yellow]/list[-]    List available rooms
  [yellow]/join[-]    Join a room
  [yellow]/part[-]    Leave a room
  [yellow]/who[-]     List room members
  [yellow]/nick[-]    Change your nickname
  [yellow]/me[-]      Send an action message`
}

// interfacesContent returns the Interfaces topic.
func interfacesContent() string {
	return `[::b]Interfaces[-]

NomadNet communicates over Reticulum, which supports many
transport types:

  [cyan]TCP/IP[-] - Standard internet connections
  [cyan]Serial[-] - Direct serial/USB connections
  [cyan]RNode[-]  - LoRa radio via RNode hardware
  [cyan]I2P[-]    - Anonymous routing via I2P

Configure interfaces in ~/.reticulum/config`
}

// nodeContent returns the Hosting a Node topic.
func nodeContent() string {
	return `[::b]Hosting a Node[-]

A NomadNet node serves Micron pages and files to the network.

[::b]Setup[-]
  1. Enable node in config: [yellow]enable_node = yes[-]
  2. Create pages in [yellow]~/.nomadnetwork/storage/pages/[-]
  3. Create files in [yellow]~/.nomadnetwork/storage/files/[-]
  4. Restart NomadNet

[::b]Page Files[-]
  Pages use Micron markup (.mu extension)
  Place index.mu for the home page
  Use .allowed files for access control

[::b]File Serving[-]
  Files are served directly to requesting peers
  Supports any file type`
}

// configContent returns the Configuration Options topic.
func configContent() string {
	return `[::b]Configuration Options[-]

Config file: [yellow]~/.nomadnetwork/config[-]

[::b][logging][-]
  loglevel = 4        (0-7, higher = more verbose)
  destination = file  (file or stdout)

[::b][client][-]
  enable_client = yes
  user_interface = text
  downloads_path = ~/Downloads
  announce_interval = 360  (minutes)
  lxmf_sync_interval = 360 (minutes)

[::b][textui][-]
  theme = dark     (dark or light)
  glyphs = unicode (plain, unicode, nerdfont)
  editor = nano

[::b][rrc][-]
  history_per_room_cap = 500
  nick_colors = yes
  render_micron = yes

[::b][node][-]
  enable_node = no
  announce_interval = 360`
}

// shortcutsContent returns the Keyboard Shortcuts topic.
func shortcutsContent() string {
	return `[::b]Keyboard Shortcuts[-]

[::b]Global[-]
  [yellow]q[-] or [yellow]Esc[-]  Quit
  [yellow]Tab[-]          Switch menu items
  [yellow]1-9[-], [yellow]0[-]     Jump to menu item

[::b]Text Editing[-]
  [yellow]Ctrl-A[-]  Beginning of line
  [yellow]Ctrl-E[-]  End of line
  [yellow]Ctrl-U[-]  Kill to start
  [yellow]Ctrl-K[-]  Kill to end
  [yellow]Ctrl-W[-]  Delete word
  [yellow]Ctrl-Y[-]  Yank (paste)

[::b]Network[-]
  [yellow]Ctrl-L[-]  Toggle Nodes/Announces
  [yellow]Ctrl-G[-]  Fullscreen toggle
  [yellow]Ctrl-X[-]  Delete selected
  [yellow]Ctrl-E[-]  Edit selected

[::b]Conversations[-]
  [yellow]Ctrl-N[-]  New conversation
  [yellow]Ctrl-O[-]  Toggle sort
  [yellow]Ctrl-R[-]  Sync conversations
  [yellow]Ctrl-X[-]  Delete conversation

[::b]Channels[-]
  [yellow]Ctrl-N[-]  New hub
  [yellow]Ctrl-A[-]  Join room
  [yellow]Ctrl-R[-]  Connect hub
  [yellow]Ctrl-D[-]  Send message`
}

// markupContent returns the Micron Markup topic.
func markupContent() string {
	return `[::b]Micron Markup[-]

NomadNet uses Micron, a lightweight markup language.

[::b]Headings[-]
  [yellow]>>[-]  Heading level 1
  [yellow]>>>[-]  Heading level 2
  [yellow]>>>>[-] Heading level 3

[::b]Formatting[-]
  [yellow]` + "`!" + `bold text` + "`!" + `[-]    Bold
  [yellow]` + "`_" + `underline` + "`_" + `[-]  Underline
  [yellow]` + "`*" + `italic` + "`*" + `[-]    Italic
  [yellow]` + "``" + `[-]           Reset all formatting

[::b]Colors[-]
  [yellow]` + "`F" + `RRGGBB` + "`" + `[-]  Set foreground color
  [yellow]` + "`B" + `RRGGBB` + "`" + `[-]  Set background color
  [yellow]` + "`f" + `[-]            Reset foreground
  [yellow]` + "`b" + `[-]            Reset background

[::b]Links[-]
  [yellow]["link text":url]` + `[-]  Hyperlink
  [yellow][#anchor-name]` + `[-]  Internal anchor

[::b]Other[-]
  [yellow]` + "---" + `[-]       Horizontal divider
  [yellow]` + "`<" + `[-]         Begin field
  [yellow]` + "`>" + `[-]         End field
  [yellow]` + "`{" + `partial_url` + "}`" + `[-]  Include partial page`
}

// firstRunContent returns the First Run topic.
func firstRunContent() string {
	return `[::b]First Run[-]

Welcome to NomadNet! This is your first time running the application.

[::b]Initial Setup[-]

1. NomadNet will create configuration files in [yellow]~/.nomadnetwork/[-]
2. A Reticulum identity will be generated automatically
3. Network interfaces will be configured from ~/.reticulum/config

[::b]Next Steps[-]

  1. Check the [yellow]Interfaces[-] tab to verify network connectivity
  2. Browse the [yellow]Network[-] tab to discover peers
  3. Send your first message via [yellow]Conversations[-]
  4. Join a chat room via [yellow]Channels[-]

[::b]Getting Help[-]

  - This Guide tab contains detailed documentation
  - Press [yellow]?[-] in any input field for context help
  - Visit the NomadNet community for support`
}

// creditsContent returns the Credits & Licenses topic.
func creditsContent() string {
	return `[::b]Credits & Licenses[-]

[::b]NomadNet[-]
  Created by Mark Qvist
  https://github.com/markqvist/nomadnet
  Licensed under the Reticulum License

[::b]Go Port[-]
  Ported by Glenn Lewis
  https://github.com/gmlewis/go-nomadnet
  Licensed under GPL-3.0

[::b]Reticulum[-]
  https://reticulum.network
  Reticulum License

[::b]Go Libraries[-]
  tview - Terminal UI (MIT License)
  tcell - Terminal cells (Apache 2.0)
  cbor  - CBOR codec (MIT License)
  msgpack - Msgpack codec (MIT License)`
}
