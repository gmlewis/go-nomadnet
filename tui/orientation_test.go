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
)

func TestConversationsDisplaySetPortrait(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	if cd.Fullscreen() {
		t.Fatal("expected conversations display to not be fullscreen in landscape")
	}

	// Switch to portrait: side panel should auto-close.
	cd.SetPortrait(true)
	if !cd.Fullscreen() {
		t.Fatal("expected conversations display to be fullscreen in portrait")
	}

	// Switch back to landscape: side panel should reappear.
	cd.SetPortrait(false)
	if cd.Fullscreen() {
		t.Fatal("expected conversations display to restore side panel in landscape")
	}
}

func TestNetworkDisplaySetPortrait(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if nd.Fullscreen() {
		t.Fatal("expected network display to not be fullscreen in landscape")
	}

	// Switch to portrait: side panel should auto-close.
	nd.SetPortrait(true)
	if !nd.Fullscreen() {
		t.Fatal("expected network display to be fullscreen in portrait")
	}

	// Switch back to landscape: side panel should reappear.
	nd.SetPortrait(false)
	if nd.Fullscreen() {
		t.Fatal("expected network display to restore side panel in landscape")
	}
}

func TestChannelsDisplaySetPortrait(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})
	cd.ShowRoom(0, "lobby", nil)

	if !cd.ChannelListVisible() {
		t.Fatal("expected channel list to be visible in landscape")
	}
	if cd.roomWidget == nil || !cd.roomWidget.UsersVisible() {
		t.Fatal("expected room users to be visible in landscape")
	}

	// Switch to portrait: side panels (channel list & users) should auto-close.
	cd.SetPortrait(true)
	if cd.ChannelListVisible() {
		t.Fatal("expected channel list to be closed in portrait")
	}
	if cd.roomWidget.UsersVisible() {
		t.Fatal("expected room users list to be closed in portrait")
	}

	// Switch back to landscape: side panels should reappear.
	cd.SetPortrait(false)
	if !cd.ChannelListVisible() {
		t.Fatal("expected channel list to reappear in landscape")
	}
	if !cd.roomWidget.UsersVisible() {
		t.Fatal("expected room users list to reappear in landscape")
	}
}

func TestGuideDisplaySetPortrait(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	gd := NewGuideDisplay(app)

	if !gd.TopicsVisible() {
		t.Fatal("expected guide topics to be visible in landscape")
	}

	// Switch to portrait: topics side panel should auto-close.
	gd.SetPortrait(true)
	if gd.TopicsVisible() {
		t.Fatal("expected guide topics to be closed in portrait")
	}

	// Switch back to landscape: topics side panel should reappear.
	gd.SetPortrait(false)
	if !gd.TopicsVisible() {
		t.Fatal("expected guide topics to reappear in landscape")
	}
}

func TestMainDisplayOrientationAutoClosesSidePanels(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	main := NewMainDisplay(app, ThemeDark, "unicode")

	cd := NewConversationsDisplay(app, nil)
	nd := NewNetworkDisplay(app, nil, nil)
	ch := NewChannelsDisplay(app, nil)
	ch.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})
	ch.ShowRoom(0, "lobby", nil)
	gd := NewGuideDisplay(app)

	main.RegisterSidePanelController(cd)
	main.RegisterSidePanelController(nd)
	main.RegisterSidePanelController(ch)
	main.RegisterSidePanelController(gd)

	// Simulate draw at landscape dimensions: 80x24.
	main.checkOrientation(80, 24)
	if cd.Fullscreen() || nd.Fullscreen() || !ch.ChannelListVisible() || !ch.roomWidget.UsersVisible() || !gd.TopicsVisible() {
		t.Fatal("expected all side panels open in landscape (80x24)")
	}

	// Simulate draw at portrait dimensions: 40x50 (w < h).
	main.checkOrientation(40, 50)
	if !cd.Fullscreen() {
		t.Error("expected conversations list to be closed in portrait")
	}
	if !nd.Fullscreen() {
		t.Error("expected network list to be closed in portrait")
	}
	if ch.ChannelListVisible() {
		t.Error("expected channels list to be closed in portrait")
	}
	if ch.roomWidget.UsersVisible() {
		t.Error("expected channels users list to be closed in portrait")
	}
	if gd.TopicsVisible() {
		t.Error("expected guide topics list to be closed in portrait")
	}

	// Rotate back to landscape (80x24).
	main.checkOrientation(80, 24)
	if cd.Fullscreen() {
		t.Error("expected conversations list restored in landscape")
	}
	if nd.Fullscreen() {
		t.Error("expected network list restored in landscape")
	}
	if !ch.ChannelListVisible() {
		t.Error("expected channels list restored in landscape")
	}
	if !ch.roomWidget.UsersVisible() {
		t.Error("expected channels users list restored in landscape")
	}
	if !gd.TopicsVisible() {
		t.Error("expected guide topics list restored in landscape")
	}
}
