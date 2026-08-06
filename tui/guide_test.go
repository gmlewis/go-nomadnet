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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNewGuideDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	if gd == nil {
		t.Fatal("NewGuideDisplay returned nil")
	}
	if gd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestGuideKeyUpTopFocusesMenu(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")
	gd.FocusTopics()
	gd.topics.SetCurrentItem(0)

	handler := gd.topicsList.InputHandler()
	if handler != nil {
		handler(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
		if app.Main.focusRegion != "menu" {
			t.Errorf("focusRegion after KeyUp at item 0 = %q, want 'menu'", app.Main.focusRegion)
		}
	}
}

func TestGuideURWIDColumnsLayoutAndClickFocus(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)

	// Verify columns widget type
	cols, ok := gd.Widget().(*urwidColumns)
	if !ok {
		t.Fatalf("gd.Widget() = %T, want *urwidColumns", gd.Widget())
	}

	cols.SetRect(0, 0, 120, 30)
	gd.FocusTopics()
	if !gd.topicsList.HasFocus() {
		t.Errorf("initial focus = %v, want topicsList", app.GetFocus())
	}

	// Click column 1 (reader pane) to move focus
	mouseHandler := cols.MouseHandler()
	if mouseHandler != nil {
		event := tcell.NewEventMouse(80, 10, tcell.Button1, 0)
		mouseHandler(tview.MouseLeftClick, event, func(p tview.Primitive) { app.SetFocus(p) })
		if !gd.scroll.HasFocus() && !gd.reader.HasFocus() {
			t.Errorf("focus after click reader = %v, want scroll/reader focused", app.GetFocus())
		}
	}
}

func TestGuideClickMenuAndDraw(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	guideDisplay := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", guideDisplay.Widget())

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.Root().Draw(screen)

	app.Main.SelectPage("guide")
	app.Main.Root().Draw(screen)
}

func TestGuidePaneFocusSwitching(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	gd.FocusTopics()

	if !gd.topicsList.HasFocus() {
		t.Errorf("initial focus = %v, want topicsList", app.GetFocus())
	}

	handler := gd.Widget().InputHandler()
	if handler != nil {
		handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
		if !gd.scroll.HasFocus() {
			t.Errorf("focus after KeyRight = %v, want scroll (reader)", app.GetFocus())
		}

		handler(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
		if !gd.topicsList.HasFocus() {
			t.Errorf("focus after KeyLeft = %v, want topicsList", app.GetFocus())
		}
	}
}

// TestGuideColumnsWidthsMatchPython pins B7: the Guide two-pane column widths
// must match Python's urwid.Columns with float weights [0.33, 0.67] (Guide.py
// list_width = 0.33). Python computes the topics-pane width as
// int(maxcol*0.33 + 0.5) via urwid's column_widths subtractive rounding. The Go
// port must use the SAME ratio — integer weights [33, 67] replicate 0.33/0.67
// exactly (33/100 == 0.33 in float64), whereas [1, 2] (1/3, 2/3) rounds one
// larger at widths where 1/3 of the width lands just above 0.33 of it
// (e.g. maxcol 200 → [1,2] gives 67, but Python gives 66). At the tmux-suite
// size the live Go topics box was 67 wide vs Python 66, shifting every reader
// wrap point by one column.
func TestGuideColumnsWidthsMatchPython(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	cols, ok := gd.Widget().(*urwidColumns)
	if !ok {
		t.Fatalf("gd.Widget() = %T, want *urwidColumns", gd.Widget())
	}

	for _, maxcol := range []int{80, 120, 134, 199, 200, 250, 300} {
		cols.SetRect(0, 0, maxcol, 40)
		_, _, readerW, _ := gd.readerBox.GetRect()
		// Python: topics width = int(maxcol*0.33 + 0.5) (urwid subtractive
		// rounding with weights [0.33, 0.67] and dividechars 0). The topics
		// LineBox width equals the columns width allocated to column 0.
		wantTopics := int(float64(maxcol)*0.33 + 0.5)
		_, _, topicsW, _ := gd.topicsBox.GetRect()
		if topicsW != wantTopics {
			t.Errorf("maxcol=%v: topics box width = %v, want %v (Python 0.33 ratio)", maxcol, topicsW, wantTopics)
		}
		// Reader pane gets the remainder.
		wantReader := maxcol - wantTopics
		if readerW != wantReader {
			t.Errorf("maxcol=%v: reader box width = %v, want %v", maxcol, readerW, wantReader)
		}
	}
}

func TestListSetCurrentItemInCallback(t *testing.T) {
	t.Parallel()
	list := tview.NewList()
	list.SetRect(0, 0, 40, 10)
	calls := 0
	list.AddItem("Item 0", "", 0, func() {
		calls++
		if calls > 10 {
			t.Fatal("infinite loop detected in List callback")
		}
		list.SetCurrentItem(0)
	})

	handler := list.MouseHandler()
	if handler != nil {
		event := tcell.NewEventMouse(1, 0, tcell.Button1, 0)
		handler(tview.MouseLeftClick, event, func(p tview.Primitive) {})
	}
}

func TestGuideDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	_, ok := gd.Widget().(*urwidColumns)
	if !ok {
		t.Errorf("Widget is %T, want *urwidColumns", gd.Widget())
	}
}

// TestGuideShortcutBarEmpty pins A2: the Guide page footer shortcut bar must be
// EMPTY, matching Python's GuideDisplayShortcuts (Guide.py:8-13) which wraps an
// empty urwid.Text("") in the "shortcutbar" attr — the footer row is just the
// shortcutbar background fill with no text. The Go port previously returned a
// "[C-q] Quit  [Tab] Move Focus  [Enter] Select Topic / Follow Link" bar
// (go_session-002.cast confirms the extra text; python_session.cast has none).
func TestGuideShortcutBarEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	if got := gd.Shortcuts(); got != "" {
		t.Errorf("GuideDisplay.Shortcuts() = %q, want \"\" (Python Guide.py:13 urwid.Text(\"\") empty footer)", got)
	}
}

// TestGuideReaderBorderUntitled pins A3: the Guide reader (right pane) border
// must carry NO title. Golden (Python Guide.py:207): the reader is
// `urwid.LineBox(urwid.Filler(topic_text, urwid.TOP))` with no title= argument
// — the topic title is a content `>` heading INSIDE the pane, not a border
// title. The Topics list (left pane) IS titled "Topics" (Guide.py:188). The Go
// port previously titled the reader border with the topic label (showTopic) or
// "?" (showPlaceholder); both must be removed.
func TestGuideReaderBorderUntitled(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	// After construction the placeholder is shown; the reader border must be
	// untitled (not "?").
	if got := gd.readerBox.GetTitle(); got != "" {
		t.Errorf("readerBox title after placeholder = %q, want \"\" (Python reader LineBox is untitled)", got)
	}

	// After showing a real topic the reader border must STILL be untitled
	// (not the topic label).
	gd.showTopic(0)
	if got := gd.readerBox.GetTitle(); got != "" {
		t.Errorf("readerBox title after showTopic = %q, want \"\" (topic title is content, not a border title)", got)
	}
}

func TestGuideContent(t *testing.T) {
	t.Parallel()

	// The Introduction topic is embedded from the Python Guide (with a
	// deliberate, user-authorized grammar / US-spelling cleanup pass; see the
	// guide-grammar-review-findings memory) and rendered through the micron
	// styled-line renderer. Its rendered plain text must mention Nomad Network
	// and Reticulum (the original introContent checked the same substrings).
	content := guideTopicPlainText(guideIntroduction, ThemeDark)
	if len(content) == 0 {
		t.Error("introduction rendered text is empty")
	}
	if !containsStr(content, "Nomad Network") {
		t.Error("introduction missing 'Nomad Network'")
	}
	if !containsStr(content, "Reticulum") {
		t.Error("introduction missing 'Reticulum'")
	}
}

// TestGuideTopicsCount verifies all 12 embedded topics are registered in the
// canonical order from Python's TopicList (Guide.py:167-180), with "First Run"
// at index 8.
func TestGuideTopicsCount(t *testing.T) {
	t.Parallel()

	if len(guideTopics) != 12 {
		t.Fatalf("guideTopics has %v entries, want 12", len(guideTopics))
	}
	if guideTopics[firstRunTopicIndex].label != "First Run" {
		t.Errorf("index %v = %q, want First Run", firstRunTopicIndex, guideTopics[firstRunTopicIndex].label)
	}
	want := []string{
		"Introduction", "Concepts & Terminology", "Channels & RRC", "Interfaces",
		"Hosting a Node", "Configuration Options", "Keyboard Shortcuts", "Markup",
		"First Run", "Network Configuration", "Display Test", "Credits & Licenses",
	}
	for i, w := range want {
		if guideTopics[i].label != w {
			t.Errorf("guideTopics[%v].label = %q, want %q", i, guideTopics[i].label, w)
		}
	}
}

func TestNewInterfacesDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	if id == nil {
		t.Fatal("NewInterfacesDisplay returned nil")
	}
	if id.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestInterfacesDisplayWithInterfaces(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	interfaces := []InterfaceInfo{
		{Name: "TCP", Type: "TCPClientInterface", Status: "connected", Connected: true, Enabled: true, Target: "192.168.1.1:4173"},
		{Name: "LoRa", Type: "RNodeInterface", Status: "disconnected", Connected: false, Enabled: true, Target: "/dev/ttyUSB0"},
	}

	id := NewInterfacesDisplay(app, interfaces)
	if id == nil {
		t.Fatal("NewInterfacesDisplay returned nil")
	}
	if id.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestFormatBandwidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input float64
		want  string
	}{
		{0, "0 B/s"},
		{512, "512 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"},
		{1048576, "1.0 MB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := formatBandwidth(tt.input)
			if got != tt.want {
				t.Errorf("formatBandwidth(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatInterfaces(t *testing.T) {
	t.Parallel()

	result := formatInterfaces(nil)
	if !containsStr(result, "No interfaces") {
		t.Error("formatInterfaces(nil) missing empty message")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
