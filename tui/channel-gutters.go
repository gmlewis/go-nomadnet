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

// GutterDirection indicates which arrow direction a gutter displays.
type GutterDirection int

const (
	GutterRight GutterDirection = iota
	GutterLeft
)

// ExpandGutter is a 1-char-wide clickable gutter that toggles
// panel visibility via mouse left-click. It supports two visual
// directions matching the Python gutters:
//   - GutterRight: matches ChannelsExpandGutter at Channels.py:267
//     which calls delegate.toggle_channel_list()
//   - GutterLeft: matches UsersExpandGutter at Channels.py:290
//     which calls delegate.toggle_users()
type ExpandGutter struct {
	*tview.Box
	expanded  bool
	direction GutterDirection
	onToggle  func()
}

// NewChannelsExpandGutter creates a right-arrow gutter that toggles
// the channel list panel. Matches Python's ChannelsExpandGutter
// at Channels.py:267.
func NewChannelsExpandGutter(onToggle func()) *ExpandGutter {
	return newExpandGutter(GutterRight, onToggle)
}

// NewUsersExpandGutter creates a left-arrow gutter that toggles
// the users panel. Matches Python's UsersExpandGutter
// at Channels.py:290.
func NewUsersExpandGutter(onToggle func()) *ExpandGutter {
	return newExpandGutter(GutterLeft, onToggle)
}

// newExpandGutter creates an ExpandGutter with the given direction.
func newExpandGutter(dir GutterDirection, onToggle func()) *ExpandGutter {
	g := &ExpandGutter{
		Box:       tview.NewBox(),
		expanded:  true,
		direction: dir,
		onToggle:  onToggle,
	}
	g.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action&tview.MouseLeftClick != 0 {
			g.HandleMouseLeftClick()
		}
		return action, event
	})
	return g
}

// HandleMouseLeftClick processes a mouse left-click by toggling
// the expanded state and calling the onToggle callback.
// Matches Python's mouse_event at Channels.py:267 and Channels.py:290.
func (g *ExpandGutter) HandleMouseLeftClick() {
	g.expanded = !g.expanded
	if g.onToggle != nil {
		g.onToggle()
	}
}

// Expanded reports whether the associated panel is currently expanded.
func (g *ExpandGutter) Expanded() bool {
	return g.expanded
}

// SetExpanded sets the expanded state for the gutter indicator.
func (g *ExpandGutter) SetExpanded(expanded bool) {
	g.expanded = expanded
}

// Direction returns the gutter's arrow direction.
func (g *ExpandGutter) Direction() GutterDirection {
	return g.direction
}

// Draw implements tview.Primitive.
func (g *ExpandGutter) Draw(screen tcell.Screen) {
	g.Box.DrawForSubclass(screen, g)
	x, y, _, _ := g.GetInnerRect()
	var glyph string
	switch g.direction {
	case GutterRight:
		glyph = "▸"
		if g.expanded {
			glyph = "▾"
		}
	case GutterLeft:
		glyph = "◂"
		if g.expanded {
			glyph = "▾"
		}
	}
	style := tcell.StyleDefault.Foreground(tcell.NewHexColor(0x666666))
	screen.SetContent(x, y, []rune(glyph)[0], nil, style)
}

// SyncStatusRefresh periodically refreshes the sync status line.
// Matches Python's _refresh_sync_status at Conversations.py:550.
type SyncStatusRefresh struct {
	interval int // seconds between refreshes
}

// NewSyncStatusRefresh creates a sync status refresh helper.
func NewSyncStatusRefresh(intervalSeconds int) *SyncStatusRefresh {
	return &SyncStatusRefresh{interval: intervalSeconds}
}

// Interval returns the refresh interval in seconds.
func (ssr *SyncStatusRefresh) Interval() int {
	return ssr.interval
}
