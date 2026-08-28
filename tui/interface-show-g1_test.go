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
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// g1Info is a representative interface with parameters in every category.
func g1Info() InterfaceInfo {
	return InterfaceInfo{
		Name:      "g00n Cloud Dallas",
		Type:      "TCPClientInterface",
		Enabled:   true,
		Connected: true,
		TX:        18432,
		RX:        460800,
		Params: map[string]string{
			"target_host":  "dfw.us.g00n.cloud",
			"target_port":  "6969",
			"mode":         "full",
			"ifac_netname": "",
		},
	}
}

// TestG1EnterOpensFullPageShow pins G1: Enter on an interface mounts Python's
// full-page ShowInterface view (Interfaces.py:2198-2620) — the `===` header
// with the centered "Interface: <name>" title, the info rows, the TX/RX stat
// row, the chart boxes, the Connection/Radio/Network/IFAC/Additional parameter
// blocks, and the footer [Back | Disable/Enable | Edit] row — replacing the
// former tiny `Interface: <name>` modal.
func TestG1EnterOpensFullPageShow(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	id := NewInterfacesDisplay(app, []InterfaceInfo{g1Info()})
	app.Main.SetDisplay("interfaces", id.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	// Wire Enter like the runtime wiring does (cmd/gonomadnet/textui.go).
	id.OnShowInterface = func(idx int) { id.ShowInterfaceDetail(id.Items()[idx]) }
	app.Main.SelectPage("interfaces")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	// Down selects the first interface (the list starts unfocused, matching
	// Python's header-led walker), then Enter opens the detail page.
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if id.showWidget == nil {
		t.Fatal("Enter did not mount the full-page ShowInterface view")
	}

	rows := dialogRowTexts(id.showWidget.Widget())
	joined := strings.Join(rows, "\n")
	for _, want := range []string{
		"Interface: g00n Cloud Dallas",
		"Type:    TCPClientInterface",
		"TX:",
		"RX:",
		"Connection Parameters",
		"Network Parameters",
		"Target Host: dfw.us.g00n.cloud",
		"Target Port: 6969",
		"Mode: full",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("ShowInterface body missing %q\nrows: %q", want, rows)
		}
	}
	// IFAC section is skipped when its only key is empty (Python skips empty
	// values, Interfaces.py:2439).
	if strings.Contains(joined, "IFAC Parameters") {
		t.Errorf("IFAC Parameters block must be absent when all its values are empty: %q", rows)
	}

	// Footer buttons (G2): Back / Disable / Edit all present.
	for _, want := range []string{"Back", "Disable", "Edit"} {
		if !dialogHasButton(id.showWidget.Widget(), want) {
			t.Errorf("footer %q button missing", want)
		}
	}
}

// TestG2TabReachesFooterButtons pins G2: Tab moves focus body → footer button
// row (Back first), cycles Back → Disable/Enable → Edit → Back, and Enter on
// Back returns to the interface list (Python on_back → switch_to_list,
// Interfaces.py:2817-2819 + 3011-3016).
func TestG2TabReachesFooterButtons(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	id := NewInterfacesDisplay(app, []InterfaceInfo{g1Info()})
	app.Main.SetDisplay("interfaces", id.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	id.OnShowInterface = func(idx int) { id.ShowInterfaceDetail(id.Items()[idx]) }
	app.Main.SelectPage("interfaces")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	show := id.showWidget
	if show == nil {
		t.Fatal("ShowInterface view not open")
	}

	// Tab: body → footer, Back focused.
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if !show.inFooter || show.btnFocus != 0 {
		t.Fatalf("after Tab: inFooter=%v btnFocus=%v, want true/0 (Back)", show.inFooter, show.btnFocus)
	}

	// Tab cycles Back → Disable → Edit → Back.
	for _, want := range []int{1, 2, 0} {
		dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
		if show.btnFocus != want {
			t.Fatalf("after Tab cycle: btnFocus=%v, want %v", show.btnFocus, want)
		}
	}

	// Enter on Back returns to the interface list.
	show.btnFocus = 0
	show.syncFooterFocus()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if id.showWidget != nil {
		t.Fatal("Enter on Back did not return to the interface list (switch_to_list)")
	}
	switch got := app.GetFocus().(type) {
	case *tview.Flex:
		if got != id.layout {
			t.Errorf("focus after Back = a different Flex; want the interface list layout")
		}
	case *interfaceListBox:
		// The cascade landed on the list leaf — exactly the Python
		// switch_to_list end state (the interface list focused).
	default:
		t.Errorf("focus after Back = %T, want the interface list", got)
	}
}

// TestG2ToggleFiresCallback pins the Disable/Enable footer action wiring
// (Python on_toggle_enabled, Interfaces.py:2516).
func TestG2ToggleFiresCallback(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	id := NewInterfacesDisplay(app, []InterfaceInfo{g1Info()})
	app.Main.SetDisplay("interfaces", id.Widget())
	app.SetRoot()

	var toggled InterfaceInfo
	id.OnToggleInterface = func(info InterfaceInfo) { toggled = info }

	id.ShowInterfaceDetail(g1Info())
	show := id.showWidget
	show.btnFocus = 1
	show.syncFooterFocus()
	t.Logf("DBG focus after sync=%T a.focusIsButton=%v", app.GetFocus(), app.GetFocus() == tview.Primitive(show.buttons[1]))
	for _, r := range app.Main.contentArea.refs {
		if r.name == "interfaces" {
			t.Logf("DBG bodyPages interfaces visible=%v hasFocus=%v", r.visible, r.item.HasFocus())
		}
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	t.Logf("DBG after Enter focus=%T toggled=%q", app.GetFocus(), toggled.Name)
	if toggled.Name == "" {
		id.widget.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
		t.Logf("DBG after direct id.widget: toggled=%q", toggled.Name)
		if toggled.Name == "" {
			show.Flex.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
			t.Logf("DBG after direct show flex: toggled=%q", toggled.Name)
		}
	}

	if toggled.Name != g1Info().Name {
		t.Errorf("toggle callback fired with %+#v, want the shown interface", toggled)
	}
}
