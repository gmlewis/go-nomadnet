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

// ChannelGutter is a 1-char-wide clickable gutter that toggles
// panel visibility. Matches Python's ChannelsExpandGutter at
// Channels.py:267 and UsersExpandGutter at Channels.py:290.
type ChannelGutter struct {
	*tview.Box
	expanded bool
	onToggle func()
}

// NewChannelGutter creates a 1-char-wide expand/collapse gutter.
func NewChannelGutter(onToggle func()) *ChannelGutter {
	cg := &ChannelGutter{
		Box:      tview.NewBox(),
		expanded: true,
		onToggle: onToggle,
	}
	cg.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action&tview.MouseLeftClick != 0 {
			cg.expanded = !cg.expanded
			if cg.onToggle != nil {
				cg.onToggle()
			}
		}
		return action, event
	})
	return cg
}

// SetExpanded sets the expanded state for the gutter indicator.
func (cg *ChannelGutter) SetExpanded(expanded bool) {
	cg.expanded = expanded
}

// Draw implements tview.Primitive.
func (cg *ChannelGutter) Draw(screen tcell.Screen) {
	cg.Box.DrawForSubclass(screen, cg)
	x, y, _, _ := cg.GetInnerRect()
	glyph := "▸"
	if cg.expanded {
		glyph = "▾"
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
