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
	"github.com/mattn/go-runewidth"
)

func TestSelectableInterfaceItemStatusText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{"enabled", true, "Enabled"},
		{"disabled", false, "Disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &SelectableInterfaceItem{IsEnabled: tt.enabled}
			if got := s.StatusText(); got != tt.want {
				t.Errorf("StatusText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectableInterfaceItemConnectedText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		connected bool
		want      string
	}{
		{"connected", true, "Connected"},
		{"disconnected", false, "Disconnected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &SelectableInterfaceItem{IsConnected: tt.connected}
			if got := s.ConnectedText(); got != tt.want {
				t.Errorf("ConnectedText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectableInterfaceItemTitleText(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{Icon: "↗", Name: "TCPClient"}
	want := "↗  TCPClient"
	if got := s.TitleText(); got != want {
		t.Errorf("TitleText() = %q, want %q", got, want)
	}
}

func TestSelectableInterfaceItemUpdateStats(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{TX: 100, RX: 200}
	s.UpdateStats(500, 1000)
	if s.TX != 500 {
		t.Errorf("TX = %v, want 500", s.TX)
	}
	if s.RX != 1000 {
		t.Errorf("RX = %v, want 1000", s.RX)
	}
}

func TestSelectableInterfaceItemByteFormatting(t *testing.T) {
	t.Parallel()

	s := &SelectableInterfaceItem{TX: 1536, RX: 1048576}
	txText := s.TXText()
	rxText := s.RXText()
	if txText != "1.5 KB" {
		t.Errorf("TXText() = %q, want %q", txText, "1.5 KB")
	}
	if rxText != "1.0 MB" {
		t.Errorf("RXText() = %q, want %q", rxText, "1.0 MB")
	}
}

// TestInterfaceItemRowTextPythonParity is a LIVE cross-implementation check:
// it stubs only gi (GLib is broken on this host) but uses the REAL urwid
// (4.0.3, whose text render engine does not need GLib), instantiates Python's
// real SelectableInterfaceItem (Interfaces.py:1125) with the real
// _get_interface_icon, calls its render at width 60 (which sets the focus
// selection glyph ●/○ and title style), then renders the inner Pile at the
// 54-wide content width and captures the five content rows freshly on every
// run. Go's InterfaceItemRowText is compared to those rows exactly (full
// padded strings, display width 54), which subsumes the content, the
// left-aligned padding, the divider row, and the wide emoji icon (🖧, display
// width 2). The test SKIPs, not fails, when the Python reference is not
// importable.
func TestInterfaceItemRowTextPythonParity(t *testing.T) {
	t.Parallel()

	const w = 60
	const cw = w - 6 // 54

	rnodeIcon := GetInterfaceIcon(GlyphUnicode, "RNodeInterface")
	tcpIcon := GetInterfaceIcon(GlyphUnicode, "TCPClientInterface")

	tests := []struct {
		name      string
		item      *SelectableInterfaceItem
		focused   bool
		ifaceType string
	}{
		{
			"rnode connected focused",
			func() *SelectableInterfaceItem {
				s := NewSelectableInterfaceItem("RNode Test", "RNodeInterface", true, true, 1234567, 89, rnodeIcon)
				s.SetFocused(true)
				return s
			}(),
			true, "RNodeInterface",
		},
		{
			"tcpclient disabled unfocused",
			func() *SelectableInterfaceItem {
				s := NewSelectableInterfaceItem("Michmesh", "TCPClientInterface", false, false, 0, 0, tcpIcon)
				return s
			}(),
			false, "TCPClientInterface",
		},
	}

	type ifaceInput struct {
		Name      string `json:"name"`
		Connected bool   `json:"connected"`
		Enabled   bool   `json:"enabled"`
		IfaceType string `json:"iface_type"`
		TX        int64  `json:"tx"`
		RX        int64  `json:"rx"`
		Focus     bool   `json:"focus"`
	}
	inputs := []ifaceInput{
		{Name: "RNode Test", Connected: true, Enabled: true, IfaceType: "RNodeInterface", TX: 1234567, RX: 89, Focus: true},
		{Name: "Michmesh", Connected: false, Enabled: false, IfaceType: "TCPClientInterface", TX: 0, RX: 0, Focus: false},
	}

	const script = `
import sys, json, types
gi=types.ModuleType("gi"); girepo=types.ModuleType("gi.repository")
glib=types.ModuleType("gi.repository.GLib")
for n in ["MainLoop","MainContext"]: setattr(glib,n,object)
glib.PRIORITY_HIGH=glib.PRIORITY_DEFAULT=0
glib.timeout_add_seconds=glib.timeout_add=glib.idle_add=glib.source_remove=lambda *a,**k:0
glib.IOCondition=object; glib.IO_IN=1
gio=types.ModuleType("gi.repository.Gio")
for n in ["UnixInputStream","Socket"]: setattr(gio,n,object)
girepo.GLib=glib; girepo.Gio=gio; gi.repository=girepo
gi.require_version=lambda *a,**k:None
sys.modules["gi"]=gi; sys.modules["gi.repository"]=girepo
sys.modules["gi.repository.GLib"]=glib; sys.modules["gi.repository.Gio"]=gio

import urwid
from nomadnet.ui import TextUI as T
from nomadnet.ui.textui import Interfaces as I

def glyph_dict(gs):
    g={}; idx=T.GLYPHSETS[gs]
    for tup in T.GLYPHS: g[tup[0]]=tup[idx]
    return g
G=glyph_dict("unicode")
class Parent:
    def __init__(self): self.g=G; self.glyphset="unicode"
    def switch_to_show_interface(self, n): pass

cases=json.load(sys.stdin)
out=[]
for c in cases:
    p=Parent()
    icon=I._get_interface_icon("unicode", c["iface_type"])
    item=I.SelectableInterfaceItem(p, c["name"], c["connected"], c["enabled"], c["iface_type"], c["tx"], c["rx"], icon=icon)
    item.render((60,), focus=c["focus"])  # set selection glyph + title style
    pile=item._w.original_widget.original_widget  # LineBox -> Padding -> Pile
    canv=pile.render((54,), focus=c["focus"])
    out.append([r.decode("utf-8") if isinstance(r,bytes) else r for r in canv.text])
json.dump(out, sys.stdout, ensure_ascii=False)
`

	var want [][]string
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rows := InterfaceItemRowText(tt.item, w)
			if len(rows) != 5 {
				t.Fatalf("got %v rows, want 5", len(rows))
			}
			wRows := want[i]
			if len(wRows) != 5 {
				t.Fatalf("python rows = %v, want 5", len(wRows))
			}
			for j, got := range rows {
				if runewidth.StringWidth(got) != cw {
					t.Errorf("row %v display width = %v, want %v (%q)", j, runewidth.StringWidth(got), cw, got)
				}
				if j == 3 {
					divWant := strings.Repeat("-", cw)
					if got != divWant {
						t.Errorf("divider row = %q, want %q", got, divWant)
					}
					continue
				}
				if got != wRows[j] {
					t.Errorf("row %v = %q\nwant       %q (Python)", j, got, wRows[j])
				}
			}
		})
	}
}

// TestSelectableInterfaceItemDraw renders an item to a simulation screen and
// checks the rounded border + selection glyph appear at the expected cells.
func TestSelectableInterfaceItemDraw(t *testing.T) {
	t.Parallel()

	s := NewSelectableInterfaceItem("RNode Test", "RNodeInterface", true, true, 100, 200, "ᚱ")
	s.SetFocused(true)
	s.SetRect(0, 0, 60, InterfaceItemHeight)

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, InterfaceItemHeight)
	s.Draw(screen)

	// Rounded top-left corner.
	if main, _, _, _ := cellContent(screen, 0, 0); main != BorderTopLeftRounded {
		t.Errorf("top-left = %q, want %q", main, BorderTopLeftRounded)
	}
	// Focused selection glyph ● at content start (x=3, y=1).
	if main, _, _, _ := cellContent(screen, 3, 1); main != '●' {
		t.Errorf("selection glyph = %q, want ●", main)
	}
	// Icon at x=7 (3 pad + 4-wide sel field).
	if main, _, _, _ := cellContent(screen, 7, 1); main != 'ᚱ' {
		t.Errorf("icon = %q, want ᚱ", main)
	}
}

// TestSelectableInterfaceItemPartialDraw verifies a bottom-clipped item (height
// < InterfaceItemHeight, as the list renders for the last visible item) draws
// its top rows flanked by verticals with NO bottom border — matching urwid's
// ListBox, which renders the visible top portion and leaves the off-screen
// bottom border undrawn.
func TestSelectableInterfaceItemPartialDraw(t *testing.T) {
	t.Parallel()

	s := NewSelectableInterfaceItem("Iface", "TCPClientInterface", true, true, 10, 20, "󰈀")
	s.SetRect(0, 0, 60, 5) // 5 of 7 rows: top border + 4 content rows, bottom clipped
	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 5)
	s.Draw(screen)

	// Top border present.
	if main, _, _, _ := cellContent(screen, 0, 0); main != BorderTopLeftRounded {
		t.Errorf("top-left = %q, want %q", main, BorderTopLeftRounded)
	}
	// Last visible row (y=4) is the divider content row flanked by verticals,
	// NOT a bottom border.
	if main, _, _, _ := cellContent(screen, 0, 4); main != BorderVertical {
		t.Errorf("last row left = %q, want %v (vertical, no bottom border when clipped)", main, BorderVertical)
	}
	if main, _, _, _ := cellContent(screen, 59, 4); main != BorderVertical {
		t.Errorf("last row right = %q, want %v", main, BorderVertical)
	}
	// Title row (y=1) content present.
	if main, _, _, _ := cellContent(screen, 3, 1); main != '○' {
		t.Errorf("selection glyph = %q, want ○", main)
	}
}

// TestInterfaceListBoxPartialLastItem verifies the list draws a bottom-clipped
// partial last item into the remaining height rather than skipping it, matching
// urwid's ListBox render of a partially-fit final item.
func TestInterfaceListBoxPartialLastItem(t *testing.T) {
	t.Parallel()

	items := []*SelectableInterfaceItem{
		NewSelectableInterfaceItem("A", "TCPClientInterface", true, true, 0, 0, "󰈀"),
		NewSelectableInterfaceItem("B", "TCPClientInterface", true, true, 0, 0, "󰈀"),
		NewSelectableInterfaceItem("C", "TCPClientInterface", true, true, 0, 0, "󰈀"),
	}
	b := newInterfaceListBox(nil, "")
	b.SetItems(items)
	// Height fits 2 full boxes (14) + 2 rows of a partial 3rd (the 17th row
	// is a blank buffer, matching Python's iface_row_offset behavior).
	b.SetRect(0, 0, 60, 17)
	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 17)
	b.Draw(screen)

	// Box A top border at y=0, Box B top border at y=7, Box C top border at y=14.
	if main, _, _, _ := cellContent(screen, 0, 14); main != BorderTopLeftRounded {
		t.Errorf("partial box C top-left at y=14 = %q, want %q", main, BorderTopLeftRounded)
	}
	// Box C last visible row (y=15) is the title row (clipped, no bottom
	// border). The 17th row (y=16) is a blank buffer row.
	if main, _, _, _ := cellContent(screen, 0, 16); main != 0 && main != ' ' {
		t.Errorf("buffer row y=16 = %q, want blank (Python iface_row_offset buffer)", main)
	}
	// Box C title row (y=15) shows the name.
	if main, _, _, _ := cellContent(screen, 3, 15); main != '○' {
		t.Errorf("partial box C selection glyph y=15 = %q, want ○", main)
	}
}
