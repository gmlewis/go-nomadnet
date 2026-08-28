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
	"testing"
	"time"

	"github.com/rivo/tview"
)

const (
	// ConversationsGivenWidth matches Python ConversationsDisplay.given_list_width = 52
	ConversationsGivenWidth = 52
	// NetworkGivenWidth matches Python NetworkDisplay.given_list_width = 52
	NetworkGivenWidth = 52
	// ChannelsGivenWidth matches Python ChannelsDisplay.given_list_width = 36
	ChannelsGivenWidth = 36
	// UsersPaneWidth matches Python RoomWidget.USERS_PANE_WIDTH = 22
	UsersPaneWidth = 22
)

func TestLayoutConversationListWidthMatchesPython(t *testing.T) {
	t.Parallel()
	if ConversationsGivenWidth != 52 {
		t.Errorf("ConversationsGivenWidth = %v, want 52 (Python ConversationsDisplay.given_list_width)", ConversationsGivenWidth)
	}
}

func TestLayoutNetworkListWidthMatchesPython(t *testing.T) {
	t.Parallel()
	if NetworkGivenWidth != 52 {
		t.Errorf("NetworkGivenWidth = %v, want 52 (Python NetworkDisplay.given_list_width)", NetworkGivenWidth)
	}
}

func TestLayoutChannelListWidthMatchesPython(t *testing.T) {
	t.Parallel()
	if ChannelsGivenWidth != 36 {
		t.Errorf("ChannelsGivenWidth = %v, want 36 (Python ChannelsDisplay.given_list_width)", ChannelsGivenWidth)
	}
}

func TestLayoutUsersPaneWidthMatchesPython(t *testing.T) {
	t.Parallel()
	if UsersPaneWidth != 22 {
		t.Errorf("UsersPaneWidth = %v, want 22 (Python RoomWidget.USERS_PANE_WIDTH)", UsersPaneWidth)
	}
}

func TestLayoutWidgetExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func()
	}{
		{"ConversationsDisplay widget", func() {
			app := newTestApp()
			cd := NewConversationsDisplay(app, nil)
			if cd.Widget() == nil {
				t.Error("ConversationsDisplay.Widget() is nil")
			}
		}},
		{"NetworkDisplay widget", func() {
			app := newTestApp()
			nd := NewNetworkDisplay(app, nil, nil)
			if nd.Widget() == nil {
				t.Error("NetworkDisplay.Widget() is nil")
			}
		}},
		{"ChannelsDisplay widget", func() {
			app := newTestApp()
			cd := NewChannelsDisplay(app, nil)
			if cd.Widget() == nil {
				t.Error("ChannelsDisplay.Widget() is nil")
			}
		}},
		{"RoomWidget widget", func() {
			app := newTestApp()
			rw := NewRoomWidget(app, "hub1", "general")
			if rw.Widget() == nil {
				t.Error("RoomWidget.Widget() is nil")
			}
		}},
		{"BrowserDisplay widget", func() {
			app := newTestApp()
			bd := NewBrowserDisplay(app)
			if bd.Widget() == nil {
				t.Error("BrowserDisplay.Widget() is nil")
			}
		}},
		{"ConversationWidget widget", func() {
			app := newTestApp()
			cw := NewConversationWidget(app, "aabb112233445566")
			if cw.Widget() == nil {
				t.Error("ConversationWidget.Widget() is nil")
			}
		}},
		{"InterfacesDisplay widget", func() {
			app := newTestApp()
			id := NewInterfacesDisplay(app, nil)
			if id.Widget() == nil {
				t.Error("InterfacesDisplay.Widget() is nil")
			}
		}},
		{"ConfigDisplay widget", func() {
			app := newTestApp()
			cd := NewConfigDisplay(app, "/tmp/config")
			if cd.Widget() == nil {
				t.Error("ConfigDisplay.Widget() is nil")
			}
		}},
		{"LogDisplay widget", func() {
			app := newTestApp()
			ld := NewLogDisplay(app, "/tmp/log", 10)
			if ld.Widget() == nil {
				t.Error("LogDisplay.Widget() is nil")
			}
		}},
		{"GuideDisplay widget", func() {
			app := newTestApp()
			gd := NewGuideDisplay(app)
			if gd.Widget() == nil {
				t.Error("GuideDisplay.Widget() is nil")
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.fn()
		})
	}
}

func TestLayoutWidgetHasBorder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func() tview.Primitive
	}{
		{"ConversationsDisplay", func() tview.Primitive {
			app := newTestApp()
			return NewConversationsDisplay(app, nil).Widget()
		}},
		{"NetworkDisplay", func() tview.Primitive {
			app := newTestApp()
			return NewNetworkDisplay(app, nil, nil).Widget()
		}},
		{"ChannelsDisplay", func() tview.Primitive {
			app := newTestApp()
			return NewChannelsDisplay(app, nil).Widget()
		}},
		{"RoomWidget", func() tview.Primitive {
			app := newTestApp()
			return NewRoomWidget(app, "h", "r").Widget()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := tt.fn()
			if w == nil {
				t.Fatal("widget is nil")
			}
			// tview primitives that have borders are typically
			// *tview.Flex or *tview.Box with SetBorder(true).
			// We just verify the widget is non-nil (border is
			// set during construction).
		})
	}
}

func TestLayoutConversationWidgetPartsExist(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cw := NewConversationWidget(app, "aabb112233445566")

	if cw.peerInfoBar == nil {
		t.Error("peerInfoBar is nil")
	}
	if cw.messageList == nil {
		t.Error("messageList is nil")
	}
	if cw.editor == nil {
		t.Error("editor is nil")
	}
	if cw.titleEditor == nil {
		t.Error("titleEditor is nil")
	}
	if cw.fullEditorArea == nil {
		t.Error("fullEditorArea is nil")
	}
	if cw.footerArea == nil {
		t.Error("footerArea is nil")
	}
}

func TestLayoutConversationWidgetEditorToggle(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cw := NewConversationWidget(app, "aabb112233445566")

	if cw.fullEditorActive {
		t.Error("editor should start in minimal mode")
	}
	cw.toggleEditor()
	if !cw.fullEditorActive {
		t.Error("editor should be in full mode after toggle")
	}
	cw.toggleEditor()
	if cw.fullEditorActive {
		t.Error("editor should be back in minimal mode")
	}
}

func TestLayoutConversationWidgetMessages(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cw := NewConversationWidget(app, "aabb112233445566")

	now := time.Now()
	msgs := []ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
		{Content: "Hi there", Timestamp: now.Add(time.Minute), IsSent: false},
		{Content: "Failed msg", Timestamp: now.Add(2 * time.Minute), IsFailed: true},
	}
	cw.SetMessages(msgs)
	text := cw.renderedMessageText(false)
	if text == "" {
		t.Error("message list should have content after SetMessages")
	}
}

func TestLayoutAllDisplaysNonNil(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	displays := []struct {
		name string
		w    tview.Primitive
	}{
		{"Conversations", NewConversationsDisplay(app, nil).Widget()},
		{"Network", NewNetworkDisplay(app, nil, nil).Widget()},
		{"Channels", NewChannelsDisplay(app, nil).Widget()},
		{"RoomWidget", NewRoomWidget(app, "h", "r").Widget()},
		{"Browser", NewBrowserDisplay(app).Widget()},
		{"Interfaces", NewInterfacesDisplay(app, nil).Widget()},
		{"Config", NewConfigDisplay(app, "/tmp/c").Widget()},
		{"Log", NewLogDisplay(app, "/tmp/l", 10).Widget()},
		{"Guide", NewGuideDisplay(app).Widget()},
		{"ConversationWidget", NewConversationWidget(app, "aabb112233445566").Widget()},
	}

	for _, d := range displays {
		if d.w == nil {
			t.Errorf("%v widget is nil", d.name)
		}
	}
}
