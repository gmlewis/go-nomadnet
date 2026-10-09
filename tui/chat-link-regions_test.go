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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

const (
	// chatLinkLXMF is an lxmf.delivery address, the form a message link carries.
	chatLinkLXMF = "2a6105f57145860441a62fe3b2a1352c"
	// chatLinkHub is an rrc.hub destination, the form a hub link carries.
	chatLinkHub = "a012129c10205c0b9441fcd2b755b2a7"
)

// chatLinkRegionRE matches the numbered region tags the renderer emits around a
// link run: ["N"] opens region N and [""] closes it.
var chatLinkRegionRE = regexp.MustCompile(`\["([0-9]*)"\]`)

// chatLinksOf is the (kind, target) pairs scanChatLinks finds in body order, which
// is the order the renderer numbers its regions in.
func chatLinksOf(body string) []chatLink {
	spans := scanChatLinks(body)
	out := make([]chatLink, 0, len(spans))
	for _, s := range spans {
		out = append(out, chatLink{kind: s.kind, target: s.target})
	}
	return out
}

// TestChatLinkGrammar pins the link forms a chat body may carry. The first, fifth
// and sixth are Python's own _LINK_RE alternatives (Channels.py:60-64); the rrc
// forms are this port's deliberate extension, matching what an rrc:// link carries
// on a Micron page (Browser.py:277-279) so a hub link behaves the same in both
// places, and the "@"-prefixed case is the boundary rule that keeps an identity
// hash from becoming a node link.
func TestChatLinkGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []chatLink
	}{
		{
			"the lxmf sigil marks a message address",
			"ask lxmf@" + chatLinkLXMF,
			[]chatLink{{"lxmf", chatLinkLXMF}},
		},
		{
			"an rrc link opens a hub",
			"join rrc://" + chatLinkHub,
			[]chatLink{{"rrc", chatLinkHub}},
		},
		{
			"an rrc link may name the room",
			"rrc://" + chatLinkHub + "/general",
			[]chatLink{{"rrc", chatLinkHub + "/general"}},
		},
		{
			"an rrc link may name the hub's destination",
			"rrc://" + chatLinkHub + ":gonomadnet/general",
			[]chatLink{{"rrc", chatLinkHub + ":gonomadnet/general"}},
		},
		{
			"a bare hash is a node to browse",
			"node " + chatLinkHub,
			[]chatLink{{"page", chatLinkHub}},
		},
		{
			"a room link",
			"see #general",
			[]chatLink{{"room", "general"}},
		},
		{
			"an identity after @ is not a node link",
			"@" + chatLinkHub,
			nil,
		},
		{
			// The rrc form glued to a word is refused by the boundary rule, and the
			// bare hash inside it then matches the node alternative. Python's own
			// rules do exactly the same, so this is parity rather than a miss.
			"an rrc link glued to a word falls back to the node form",
			"xrrc://" + chatLinkHub,
			[]chatLink{{"page", chatLinkHub}},
		},
		{
			"a short hash is not a link",
			"hash deadbeef",
			nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatLinksOf(tc.body)
			if len(got) != len(tc.want) {
				t.Fatalf("links of %q = %v, want %v", tc.body, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("link %v of %q = %v, want %v", i, tc.body, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestChatLinkURI pins the click target each kind dispatches: Python's chat
// delegate splits the target it is handed on "://" and strips the kind's own sigil
// (Channels.py:1145-1160), so the URI must carry both.
func TestChatLinkURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		link chatLink
		want string
	}{
		{chatLink{"lxmf", chatLinkLXMF}, "lxmf://lxmf@" + chatLinkLXMF},
		{chatLink{"room", "general"}, "room://#general"},
		{chatLink{"page", chatLinkHub}, "page://" + chatLinkHub},
		{chatLink{"rrc", chatLinkHub + "/general"}, "rrc://" + chatLinkHub + "/general"},
	}
	for _, tc := range tests {
		if got := tc.link.uri(); got != tc.want {
			t.Errorf("%v.uri() = %q, want %q", tc.link, got, tc.want)
		}
	}
}

// TestChatLinkRegionsMatchTheirTargets asserts the ids tagged into the rendered
// text are the ids the link table resolves: each opened region's following text is
// the link it stands for, and the table returns that link. This is what makes a
// click safe — the id on screen cannot drift from the link it means.
func TestChatLinkRegionsMatchTheirTargets(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	table := &chatLinkTable{}
	opts.links = table
	tsMs := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local).UnixMilli()

	lines := formatRRCMessageLines(ChannelMessage{
		Room: "general",
		Text: "browse " + chatLinkHub + " and ask lxmf@" + chatLinkLXMF + " in #general",
		TsMs: tsMs,
	}, opts, 200)
	text := strings.Join(lines, "\n")

	want := []chatLink{
		{"page", chatLinkHub},
		{"lxmf", chatLinkLXMF},
		{"room", "general"},
	}
	if len(table.links) != len(want) {
		t.Fatalf("the table holds %v links, want %v: %v", len(table.links), len(want), table.links)
	}
	for i, link := range want {
		if table.links[i] != link {
			t.Errorf("table link %v = %v, want %v", i, table.links[i], link)
		}
	}

	// Walk the tags in pairs: ["N"] opens region N, the link's own text follows, and
	// [""] closes it. The displayed text must be the form the kind is written in.
	tags := chatLinkRegionRE.FindAllStringSubmatchIndex(text, -1)
	if len(tags) != 2*len(want) {
		t.Fatalf("found %v region tags in %q, want %v", len(tags), text, 2*len(want))
	}
	for i := 0; i < len(tags); i += 2 {
		open, close := tags[i], tags[i+1]
		id, err := strconv.Atoi(text[open[2]:open[3]])
		if err != nil {
			t.Fatalf("region tag %q carries no id", text[open[0]:open[1]])
		}
		if close[0] < open[1] {
			t.Fatalf("region %v is not closed after its content: %q", id, text)
		}
		link, ok := table.linkAt(id)
		if !ok {
			t.Fatalf("region id %v does not resolve", id)
		}
		wantShown := map[string]string{
			"page": chatLinkHub,
			"lxmf": "lxmf@" + chatLinkLXMF,
			"room": "#general",
		}[link.kind]
		if shown := text[open[1]:close[0]]; shown != wantShown {
			t.Errorf("region %v shows %q, want %q", id, shown, wantShown)
		}
	}
}

// chatLinkClickAt drives the message view's mouse handler at an absolute screen
// cell the way tview would: a left-down to focus, then the left-click that makes
// the TextView resolve the region under the pointer and hand its id on.
func chatLinkClickAt(rw *RoomWidget, x, y int) {
	handler := rw.messagesArea.MouseHandler()
	setFocus := func(p tview.Primitive) {
		if p != nil {
			p.Focus(func(tview.Primitive) {})
		}
	}
	event := tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)
	handler(tview.MouseLeftDown, event, setFocus)
	handler(tview.MouseLeftClick, event, setFocus)
}

// findChatLinkCell returns the first screen cell where text is drawn.
func findChatLinkCell(t *testing.T, screen tcell.Screen, text string) (int, int) {
	t.Helper()
	width, height := screen.Size()
	for y := range height {
		for x := range width {
			col, match := x, true
			for _, want := range text {
				got, _, _ := screen.Get(col, y)
				if !strings.HasPrefix(got, string(want)) {
					match = false
					break
				}
				col++
			}
			if match {
				return x, y
			}
		}
	}
	t.Fatalf("%q was not drawn on the screen", text)
	return 0, 0
}

// TestRoomWidgetClickDispatchesChatAndNoticeLinks is the end-to-end click: a link
// in a chat row and a link in a NOTICE row are drawn as numbered regions, the
// message view resolves the click, and the room's link handler dispatches each
// one. The notice case is the one that matters for a bot: it is how an address
// gobot printed becomes clickable for a reader, which is what Python's _body_markup
// plus link delegate do (Channels.py:1303-1305).
func TestRoomWidgetClickDispatchesChatAndNoticeLinks(t *testing.T) {
	t.Parallel()

	var opened []string
	handler := NewChatLinkHandler(
		func(room string) { opened = append(opened, "room:"+room) },
		func(hash string) { opened = append(opened, "lxmf:"+hash) },
		func(url string) { opened = append(opened, "page:"+url) },
	)
	handler.OnOpenRRC = func(payload string) { opened = append(opened, "rrc:"+payload) }

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphNerd)
	rw := NewRoomWidget(app, "RNS Community", "general")
	rw.ChatLinks = handler
	tsMs := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local).UnixMilli()
	rw.SetMessages([]ChannelMessage{
		{Room: "general", Text: "addr me: lxmf@" + chatLinkLXMF, IsNotice: true, TsMs: tsMs},
		{Room: "general", Text: "and rrc://" + chatLinkHub + "/general", TsMs: tsMs, Nick: "Alice"},
	})

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(120, 37)
	widget := rw.Widget().(*tview.Flex)
	widget.SetRect(0, 0, 120, 37)
	widget.Draw(screen)

	for _, text := range []string{"lxmf@" + chatLinkLXMF, "rrc://" + chatLinkHub + "/general"} {
		x, y := findChatLinkCell(t, screen, text)
		chatLinkClickAt(rw, x, y)
	}

	want := []string{"lxmf:" + chatLinkLXMF, "rrc:" + chatLinkHub + "/general"}
	if strings.Join(opened, "|") != strings.Join(want, "|") {
		t.Errorf("clicking dispatched %v, want %v", opened, want)
	}
}

// TestRoomWidgetUsesALinkHandlerInstalledAfterItWasBuilt covers the wiring order
// in cmd/gonomadnet: the RRC handlers that can open a room are registered before
// the chat-link handler is installed on the channels display, so a WELCOME landing
// in that window builds a room widget whose handler is not there yet. The click
// must still dispatch — which is why the widget resolves the handler when the link
// is clicked rather than when the widget is built.
func TestRoomWidgetUsesALinkHandlerInstalledAfterItWasBuilt(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphNerd)
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{
		&fakeHub{name: "Live Hub", addressHex: "bbbb", status: hubStatusConnected, joined: []string{"general"}},
	})

	// The room is opened before the wiring layer supplies the handler.
	cd.ShowRoom(0, "general", nil)
	rw := cd.roomWidget
	if rw == nil {
		t.Fatal("ShowRoom did not create a room widget")
	}

	var opened []string
	cd.ChatLinks = NewChatLinkHandler(
		func(room string) { opened = append(opened, "room:"+room) },
		func(hash string) { opened = append(opened, "lxmf:"+hash) },
		func(url string) { opened = append(opened, "page:"+url) },
	)

	tsMs := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local).UnixMilli()
	rw.SetMessages([]ChannelMessage{
		{Room: "general", Text: "addr me: lxmf@" + chatLinkLXMF, IsNotice: true, TsMs: tsMs},
	})

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(120, 37)
	widget := rw.Widget().(*tview.Flex)
	widget.SetRect(0, 0, 120, 37)
	widget.Draw(screen)

	x, y := findChatLinkCell(t, screen, "lxmf@"+chatLinkLXMF)
	chatLinkClickAt(rw, x, y)

	want := "lxmf:" + chatLinkLXMF
	if strings.Join(opened, "|") != want {
		t.Errorf("clicking dispatched %v, want %v", opened, want)
	}
}
