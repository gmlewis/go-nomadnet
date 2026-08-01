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

// TestInterfaceItemRowTextPythonParity checks the five content rows of an
// interface box against golden values captured from Python's
// SelectableInterfaceItem (Interfaces.py:1125) rendered at width 60 (content
// width 54). It verifies both the meaningful content (TrimRight) and that each
// row is padded to the full content display width, matching urwid's
// left-aligned fill — including the wide emoji icon (🖧, display width 2).
func TestInterfaceItemRowTextPythonParity(t *testing.T) {
	t.Parallel()

	const w = 60
	const cw = w - 6 // 54

	rnodeIcon := GetInterfaceIcon(GlyphUnicode, "RNodeInterface")
	tcpIcon := GetInterfaceIcon(GlyphUnicode, "TCPClientInterface")

	tests := []struct {
		name string
		item *SelectableInterfaceItem
		// wantContent[i] is the meaningful (right-trimmed) content of row i;
		// row index 3 is the divider (checked separately).
		wantContent []string
	}{
		{
			"rnode connected focused",
			func() *SelectableInterfaceItem {
				s := NewSelectableInterfaceItem("RNode Test", "RNodeInterface", true, true, 1234567, 89, rnodeIcon)
				s.SetFocused(true)
				return s
			}(),
			[]string{
				"●   ᚱ  RNode Test",
				"Status:   Enabled    | Connected",
				"Type:     RNodeInterface",
				"",
				"TX:       1.2 MB         RX:       89 bytes",
			},
		},
		{
			"tcpclient disabled unfocused",
			func() *SelectableInterfaceItem {
				s := NewSelectableInterfaceItem("Michmesh", "TCPClientInterface", false, false, 0, 0, tcpIcon)
				return s
			}(),
			[]string{
				"○   🖧  Michmesh",
				"Status:   Disabled   | Disconnected",
				"Type:     TCPClientInterface",
				"",
				"TX:       0 bytes        RX:       0 bytes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rows := InterfaceItemRowText(tt.item, w)
			if len(rows) != 5 {
				t.Fatalf("got %d rows, want 5", len(rows))
			}
			for i, got := range rows {
				if runewidth.StringWidth(got) != cw {
					t.Errorf("row %d display width = %d, want %d (%q)", i, runewidth.StringWidth(got), cw, got)
				}
				if i == 3 {
					want := strings.Repeat("-", cw)
					if got != want {
						t.Errorf("divider row = %q, want %q", got, want)
					}
					continue
				}
				if got := strings.TrimRight(rows[i], " "); got != tt.wantContent[i] {
					t.Errorf("row %d content = %q, want %q", i, got, tt.wantContent[i])
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
	if main, _, _, _ := screen.GetContent(0, 0); main != BorderTopLeftRounded {
		t.Errorf("top-left = %q, want %q", main, BorderTopLeftRounded)
	}
	// Focused selection glyph ● at content start (x=3, y=1).
	if main, _, _, _ := screen.GetContent(3, 1); main != '●' {
		t.Errorf("selection glyph = %q, want ●", main)
	}
	// Icon at x=7 (3 pad + 4-wide sel field).
	if main, _, _, _ := screen.GetContent(7, 1); main != 'ᚱ' {
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
	if main, _, _, _ := screen.GetContent(0, 0); main != BorderTopLeftRounded {
		t.Errorf("top-left = %q, want %q", main, BorderTopLeftRounded)
	}
	// Last visible row (y=4) is the divider content row flanked by verticals,
	// NOT a bottom border.
	if main, _, _, _ := screen.GetContent(0, 4); main != BorderVertical {
		t.Errorf("last row left = %q, want %v (vertical, no bottom border when clipped)", main, BorderVertical)
	}
	if main, _, _, _ := screen.GetContent(59, 4); main != BorderVertical {
		t.Errorf("last row right = %q, want %v", main, BorderVertical)
	}
	// Title row (y=1) content present.
	if main, _, _, _ := screen.GetContent(3, 1); main != '○' {
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
	b := newInterfaceListBox("")
	b.SetItems(items)
	// Height fits 2 full boxes (14) + 3 rows of a partial 3rd.
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
	if main, _, _, _ := screen.GetContent(0, 14); main != BorderTopLeftRounded {
		t.Errorf("partial box C top-left at y=14 = %q, want %q", main, BorderTopLeftRounded)
	}
	// Box C last visible row (y=16) is a vertical (clipped, no bottom border).
	if main, _, _, _ := screen.GetContent(0, 16); main != BorderVertical {
		t.Errorf("partial box C last row y=16 = %q, want %v (clipped, no bottom border)", main, BorderVertical)
	}
	// Box C title row (y=15) shows the name.
	if main, _, _, _ := screen.GetContent(3, 15); main != '○' {
		t.Errorf("partial box C selection glyph y=15 = %q, want ○", main)
	}
}
