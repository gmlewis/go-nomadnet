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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// decideAndApply runs the two steps production runs one frame apart: a draw
// DECIDES the orientation for the grid it was given (checkOrientation), and the
// event loop then tells the displays (applyOrientation). A draw may not do the
// second itself — see checkOrientation — so a test that wants the outcome has to
// run both, which is what the live client does between two frames.
func (md *MainDisplay) decideAndApply(w, h int) {
	md.checkOrientation(w, h)
	md.applyOrientation()
}

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

// TestChannelsDisplayPortraitCollapsesTheHubListWithNothingOpened is the
// appliance's promise in its simplest state: standing the console up hides the
// hub-list pane, whether or not a hub has been opened yet.
//
// Python's Ctrl-Y toggle refuses to hide the list while nothing is opened (the
// right pane still shows the placeholder, so hiding the list would leave a
// screen with nothing to act on). That guard belongs to the user's toggle, not
// to the rotation: on a phone-sized screen the list must go, and Ctrl-Y — which
// the guard never blocks in the showing direction — brings it back.
func TestChannelsDisplayPortraitCollapsesTheHubListWithNothingOpened(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})

	if !cd.ChannelListVisible() {
		t.Fatal("expected the hub list to be visible in landscape")
	}

	cd.SetPortrait(true)
	if cd.ChannelListVisible() {
		t.Error("the hub list stayed open in portrait with nothing opened")
	}

	// Ctrl-Y must still bring it back, or the pane would be unreachable until
	// the device is rotated again.
	cd.ToggleChannelListState()
	if !cd.ChannelListVisible() {
		t.Error("Ctrl-Y could not bring the hub list back in portrait")
	}

	cd.SetPortrait(false)
	if !cd.ChannelListVisible() {
		t.Error("expected the hub list back in landscape")
	}
}

// TestChannelsDisplayOpeningAHubInPortraitCollapsesTheHubList covers the
// rotation that never happens: the console is already standing up when the hub
// is opened, so there is no orientation change left to collapse the pane the hub
// was picked from. Opening must collapse it for itself.
func TestChannelsDisplayOpeningAHubInPortraitCollapsesTheHubList(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})

	cd.SetPortrait(true)
	// The list is hidden by the rotation; the user calls it back to pick a hub.
	cd.ToggleChannelListState()
	if !cd.ChannelListVisible() {
		t.Fatal("expected the hub list to be shown again by Ctrl-Y")
	}

	cd.ShowHubInfo(0)
	if cd.ChannelListVisible() {
		t.Error("opening a hub in portrait left the hub list open")
	}
}

// TestChannelsDisplayHiddenPaneGivesUpFocus pins the focus handoff when a pane
// collapses. The hub list draws the terminal's cursor on its focused row — at
// the pane's left border column — and a collapse that leaves the keyboard on
// that list leaves the cursor lit at coordinates belonging to a pane
// that is no longer in the layout: tview shows the cursor where the last-drawn
// primitive put it, and nothing hides it once that primitive stops drawing.
func TestChannelsDisplayHiddenPaneGivesUpFocus(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})
	cd.ShowHubInfo(0)
	app.SetFocus(cd.ilb)
	if !cd.ilb.HasFocus() {
		t.Fatal("expected the hub list to hold the keyboard")
	}

	cd.SetPortrait(true)
	if cd.ilb.HasFocus() {
		t.Error("the collapsed hub list kept the keyboard, leaving the cursor where the pane used to be")
	}
	if app.GetFocus() == nil {
		t.Error("focus was dropped instead of moved")
	}
}

// TestChannelsPaneWalkIgnoresAHiddenHubList covers the other half of the focus
// rule: focus may not be moved INTO a pane that the rotation has collapsed,
// either. Left from the hub info panel walks back to the list — but in portrait
// there is no list column left to walk to, and focusing the hidden one would
// leave the keyboard somewhere that is not drawn.
func TestChannelsPaneWalkIgnoresAHiddenHubList(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "TestHub", status: hubStatusConnected, joined: []string{"lobby"}}})

	cd.SetPortrait(true)
	cd.ToggleChannelListState() // the user calls the list back to pick a hub
	if !cd.ChannelListVisible() {
		t.Fatal("expected the hub list to be shown again by Ctrl-Y")
	}
	cd.ShowHubInfo(0)
	if cd.ChannelListVisible() {
		t.Fatal("expected the hub list collapsed once the hub was opened in portrait")
	}
	app.SetFocus(cd.hubInfo.Widget())

	cd.handleInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if cd.ilb.HasFocus() {
		t.Error("Left moved the keyboard into the hidden hub list")
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
	main.decideAndApply(80, 24)
	if cd.Fullscreen() || nd.Fullscreen() || !ch.ChannelListVisible() || !ch.roomWidget.UsersVisible() || !gd.TopicsVisible() {
		t.Fatal("expected all side panels open in landscape (80x24)")
	}

	// Simulate draw at portrait dimensions: 40x50 (w < h).
	main.decideAndApply(40, 50)
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
	main.decideAndApply(80, 24)
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

// TestMainDisplayOrientationFollowsTheConsolePhysicalSize is the appliance's own
// rotation, asserted end to end: a tablet whose display is 1200 by 1920 pixels
// holds 100 columns of 12 pixels and 75 rows of 24, so its grid has more columns
// than rows and only the physical size its host reports can say that the console
// is standing up.
//
// It does not run in parallel, because it installs the host's probe for the
// package while it runs; every parallel test runs after the sequential ones have
// finished.
func TestMainDisplayOrientationFollowsTheConsolePhysicalSize(t *testing.T) {
	width, height := 1200, 1920
	SetTerminalPixels(func() (int, int, bool) { return width, height, true })
	t.Cleanup(func() { SetTerminalPixels(nil) })

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

	// The tablet held in portrait: 100x75 cells on a 1200x1920 display.
	main.decideAndApply(100, 75)
	if !cd.Fullscreen() {
		t.Error("expected the conversations list to be closed in portrait")
	}
	if !nd.Fullscreen() {
		t.Error("expected the network list to be closed in portrait")
	}
	if ch.ChannelListVisible() {
		t.Error("expected the channels list to be closed in portrait")
	}
	if ch.roomWidget.UsersVisible() {
		t.Error("expected the channels users list to be closed in portrait")
	}
	if gd.TopicsVisible() {
		t.Error("expected the guide topics list to be closed in portrait")
	}

	// Rotated to landscape: the console is now 1920x1080 pixels and 160x45 cells,
	// which is what the app sends with the rotation.
	width, height = 1920, 1080
	main.decideAndApply(160, 45)
	if cd.Fullscreen() {
		t.Error("expected the conversations list restored in landscape")
	}
	if nd.Fullscreen() {
		t.Error("expected the network list restored in landscape")
	}
	if !ch.ChannelListVisible() {
		t.Error("expected the channels list restored in landscape")
	}
	if !ch.roomWidget.UsersVisible() {
		t.Error("expected the channels users list restored in landscape")
	}
	if !gd.TopicsVisible() {
		t.Error("expected the guide topics list restored in landscape")
	}
}

// TestPortraitCollapseSurvivesADraw asserts that a panel the orientation
// collapsed is still collapsed after the interface draws, which is the whole of
// what the appliance does between a rotation and the next frame: the panes are
// collapsed for the shape of the terminal, and every draw after that has to
// leave them that way.
//
// A display that rebuilds its columns on a draw — a width table applied again, a
// widget replaced — would put the pane back while the terminal is still standing
// up, which is a panel that reappears on a rotation and never goes away again.
func TestPortraitCollapseSurvivesADraw(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)

	guide := NewGuideDisplay(app)
	guide.SetPortrait(true)
	if guide.TopicsVisible() {
		t.Fatal("SetPortrait(true) left the guide's topics pane visible")
	}

	network := NewNetworkDisplay(app, nil, nil)
	network.SetPortrait(true)
	if !network.Fullscreen() {
		t.Fatal("SetPortrait(true) left the network's node list visible")
	}

	for _, display := range []tview.Primitive{guide.Widget(), network.Widget()} {
		_ = renderPrimitive(t, display, 100, 40)
	}

	if guide.TopicsVisible() {
		t.Error("the guide's topics pane came back after a draw")
	}
	if !network.Fullscreen() {
		t.Error("the network's node list came back after a draw")
	}
}
