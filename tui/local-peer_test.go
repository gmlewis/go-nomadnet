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
	"time"

	"github.com/gdamore/tcell/v2"
)

// renderLocalPeer draws the LocalPeerDisplay widget on a 50x12 simulation
// screen (the panel is 10 rows tall; 12 gives a little vertical margin) and
// returns the joined cell text per row. Mirrors the headless parity capture so
// the layout can be pinned against Python's LocalPeer (Network.py:1259-1355).
func renderLocalPeer(t *testing.T, lp *LocalPeerDisplay, width, height int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	lp.Widget().SetRect(0, 0, width, height)
	lp.Widget().Draw(screen)
	screen.Sync()

	rows := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// TestLocalPeerLayout pins the Local Peer Info panel structure against the
// Python ground truth (Network.py:1259-1355, captured at 135x32): a bordered
// "Local Peer Info" LineBox wrapping a Pile of the LXMF address, identity
// hash, a Name edit, a divider, the announce time, an "Announce Now" button,
// another divider, and a Save | Node Info button row.
func TestLocalPeerLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	lp := NewLocalPeerDisplay(app, "<146d83fa6b82b5d0b5e4da5828cff967>", "<c57cb580abe0efa0f0dabcd9b95802e5>", "Anonymous Peer", time.Time{})

	rows := renderLocalPeer(t, lp, 50, lp.Height())

	// Row 0: top border with centered title "Local Peer Info".
	if !strings.Contains(rows[0], "Local Peer Info") {
		t.Errorf("row 0 = %q, want it to contain the title %q", rows[0], "Local Peer Info")
	}
	if r := []rune(rows[0]); r[0] != '┌' || r[len(r)-1] != '┐' {
		t.Errorf("row 0 corners = %q…%q, want ┌…┐", string(r[0]), string(r[len(r)-1]))
	}

	// LXMF Addr line.
	if got := strings.TrimRight(rows[1], " "); !strings.HasPrefix(got, "│LXMF Addr : <146d83fa") {
		t.Errorf("row 1 = %q, want LXMF Addr line starting with │LXMF Addr : <146d83fa…", rows[1])
	}
	// Identity line.
	if got := strings.TrimRight(rows[2], " "); !strings.HasPrefix(got, "│Identity  : <c57cb580") {
		t.Errorf("row 2 = %q, want Identity line starting with │Identity  : <c57cb580…", rows[2])
	}
	// Name edit line (label "Name      : " + the display name).
	if got := rows[3]; !strings.Contains(got, "Name      : ") || !strings.Contains(got, "Anonymous Peer") {
		t.Errorf("row 3 = %q, want it to contain \"Name      : \" and \"Anonymous Peer\"", got)
	}
	// Divider row (row 4): full-width divider1 glyph (┄ in unicode set) inside borders.
	if r := []rune(rows[4]); r[0] != '│' || r[len(r)-1] != '│' {
		t.Errorf("row 4 corners = %q…%q, want │…│ (divider row)", string(r[0]), string(r[len(r)-1]))
	}
	if !strings.Contains(rows[4], "┄") {
		t.Errorf("row 4 = %q, want a divider row of ┄ glyphs", rows[4])
	}
	// Announce line: zero time → "Never".
	if got := rows[5]; !strings.Contains(got, "Announced : Never") {
		t.Errorf("row 5 = %q, want \"Announced : Never\"", got)
	}
	// Announce Now button.
	if got := rows[6]; !strings.Contains(got, "Announce Now") {
		t.Errorf("row 6 = %q, want the Announce Now button", got)
	}
	// Second divider row (row 7).
	if r := []rune(rows[7]); r[0] != '│' || r[len(r)-1] != '│' {
		t.Errorf("row 7 corners = %q…%q, want │…│ (divider row)", string(r[0]), string(r[len(r)-1]))
	}
	if !strings.Contains(rows[7], "┄") {
		t.Errorf("row 7 = %q, want a divider row of ┄ glyphs", rows[7])
	}
	// Save | Node Info button row.
	if got := rows[8]; !strings.Contains(got, "Save") || !strings.Contains(got, "Node Info") {
		t.Errorf("row 8 = %q, want Save and Node Info buttons", got)
	}
	// Bottom border.
	if r := []rune(rows[9]); r[0] != '└' || r[len(r)-1] != '┘' {
		t.Errorf("row 9 corners = %q…%q, want └…┘", string(r[0]), string(r[len(r)-1]))
	}
}

// TestLocalPeerHeight pins the fixed PACK height: 8 content rows + 2 border.
func TestLocalPeerHeight(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	lp := NewLocalPeerDisplay(app, "<a>", "<b>", "", time.Time{})
	if got := lp.Height(); got != 10 {
		t.Errorf("Height = %d, want 10", got)
	}
}

// TestLocalPeerAnnounceLine pins the "Announced : …" formatting: "Never" for a
// zero time, and PrettyDate for a recent stamp.
func TestLocalPeerAnnounceLine(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	lp := NewLocalPeerDisplay(app, "<a>", "<b>", "", time.Time{})
	if got := lp.announceLine(time.Time{}); got != "Announced : Never" {
		t.Errorf("announceLine(zero) = %q, want \"Announced : Never\"", got)
	}
	recent := time.Now().Add(-5 * time.Second)
	if got := lp.announceLine(recent); got != "Announced : just now" {
		t.Errorf("announceLine(5s ago) = %q, want \"Announced : just now\"", got)
	}
}

// TestLocalPeerSetData verifies SetData updates all four fields.
func TestLocalPeerSetData(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	lp := NewLocalPeerDisplay(app, "", "", "", time.Time{})
	lp.SetData("<deadbeef>", "<cafef00d>", "My Name", time.Now().Add(-time.Hour))

	if got := lp.lxmfAddr.GetText(true); got != "LXMF Addr : <deadbeef>" {
		t.Errorf("lxmfAddr = %q, want \"LXMF Addr : <deadbeef>\"", got)
	}
	if got := lp.identity.GetText(true); got != "Identity  : <cafef00d>" {
		t.Errorf("identity = %q, want \"Identity  : <cafef00d>\"", got)
	}
	if got := lp.Name(); got != "My Name" {
		t.Errorf("Name = %q, want \"My Name\"", got)
	}
	if got := lp.announce.GetText(true); !strings.Contains(got, "Announced : ") {
		t.Errorf("announce = %q, want it to start with \"Announced : \"", got)
	}
}