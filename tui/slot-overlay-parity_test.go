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

// screenRows renders p at (x,y,w,h) on a fresh simulation screen of size sw x sh
// and returns the per-row text (runes joined) for rows [0,rows).
func screenRows(t *testing.T, p tview.Primitive, x, y, w, h, sw, sh, rows int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(sw, sh)
	p.SetRect(x, y, w, h)
	screen.Clear()
	p.Draw(screen)
	screen.Sync()
	out := make([]string, rows)
	for ry := range rows {
		var b strings.Builder
		for rx := range sw {
			b.WriteString(cellString(screen, rx, ry))
		}
		out[ry] = b.String()
	}
	return out
}

// cellString returns the rendered string of one simulation-screen cell via the
// non-deprecated Screen.Get (GetContent is deprecated).
func cellString(screen tcell.Screen, x, y int) string {
	s, _, _ := screen.Get(x, y)
	return s
}

// TestStatusDialogMatchesPython renders the "Saved" status dialog (Python
// LocalPeer.save_query, Network.py:1282-1295: a LineBox titled with the info
// glyph, body "\n\n\nSaved\n\n", an OK button, swapped into the LocalPeer slot
// at the slot width) and compares it to the nomadnet canvas. The dialog is 52
// wide (left-pane width), 9 rows (PACK: 6 message + 1 OK + 2 border), titled "ℹ".
func TestStatusDialogMatchesPython(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.ShowLocalPeerStatus("\n\n\nSaved\n\n", 6)
	dialog := nd.statusInPeerSlot
	if dialog == nil {
		t.Fatal("ShowLocalPeerStatus did not install a status dialog")
	}
	rows := screenRows(t, dialog, 0, 0, 52, 9, 52, 9, 9)

	want := []string{
		"┌──────────────────────── ℹ ───────────────────────┐",
		"│                                                  │",
		"│                                                  │",
		"│                                                  │",
		"│                       Saved                      │",
		"│                                                  │",
		"│                                                  │",
		"│< OK                                             >│",
		"└──────────────────────────────────────────────────┘",
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d:\n  got  %q\n  want %q", i, rows[i], want[i])
		}
	}
}

// TestSlotOverlayConnectDialogMatchesPython renders a connect-style dialog as a
// SlotOverlay over a list slot (Python Network.connect_node, Network.py:881-919:
// left_pile.contents[0] = urwid.Overlay(dialog, bottom=list, width=RELATIVE_100,
// valign=MIDDLE, height=PACK, left=2, right=2), title "?"). At a 52-wide slot
// the dialog is 48 wide (RELATIVE_100 minus left/right=2), centered vertically,
// with the list showing through above and below.
func TestSlotOverlayConnectDialogMatchesPython(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, []AnnounceEntry{nodeAnnounce("N")}, nil)
	_ = app
	// Put a visible row in the list so the show-through is observable.
	nd.toggleList() // Announce Stream
	bottom := nd.listBox

	msg := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorDefault).
		SetText("Connect to node\n<node>\n")
	row := CreateUrwidButtonRow(NewUrwidButton("Yes"), NewUrwidButton("No"), NewUrwidButton("Info"))
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(msg, 3, 0, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("?", layout, nil)
	ov := NewSlotOverlay(bottom, dialog, 100, 6) // msg 3 + buttons 1 + border 2

	const sw, sh = 52, 20
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(sw, sh)
	ov.SetRect(0, 0, sw, sh)
	screen.Clear()
	ov.Draw(screen)
	screen.Sync()

	rowText := func(y int) string {
		var b strings.Builder
		for x := range sw {
			b.WriteString(cellString(screen, x, y))
		}
		return b.String()
	}

	// The dialog is 48 wide (RELATIVE_100 minus left/right=2), inset 2 from each
	// edge, and 6 rows tall, centered vertically in the 20-row slot → rows 7-12.
	// The bottom (nd.listBox) is bordered, so its │ border shows through at
	// x=0/x=51 and the dialog sits at x=2..49.
	top := rowText(7)
	rr := []rune(top)
	if string(rr[0]) != "│" || string(rr[51]) != "│" {
		t.Errorf("list box border │ should show through at x=0/x=51:\n  %q", top)
	}
	if string(rr[2]) != "┌" || string(rr[49]) != "┐" {
		t.Errorf("dialog corners not at x=2 and x=49 (48 wide, inset 2):\n  %q", top)
	}
	if !strings.Contains(top, " ? ") {
		t.Errorf("dialog top border missing title \" ? \":\n  %q", top)
	}
	// Show-through: the slot rows above (row 0) and below (row 19) show the
	// bottom (list), not the dialog title.
	if strings.Contains(rowText(0), "?") {
		t.Errorf("row 0 should show the list (show-through), not the dialog title")
	}
	if strings.Contains(rowText(19), "?") {
		t.Errorf("row 19 should show the list (show-through), not the dialog")
	}
}

// TestSlotOverlayURLDialog65Percent renders a URL dialog as a SlotOverlay over a
// browser pane at 65% width (Python Browser.url_dialog, Browser.py:1169: width=
// ("relative", 65)). At a 28-wide browser pane (80-col terminal: 80-52) the
// dialog is floor(28*0.65)=18 wide, centered.
func TestSlotOverlayURLDialog65Percent(t *testing.T) {
	t.Parallel()
	bottom := tview.NewTextView().SetText("browser pane content")
	input := tview.NewInputField()
	input.SetLabel("URL : ")
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)
	row := CreateUrwidButtonRow(NewUrwidButton("Cancel"), NewUrwidButton("Go"))
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(row, 1, 0, false)
	dialog := NewDialogLineBox("Enter URL", layout, nil)
	ov := NewSlotOverlay(bottom, dialog, 65, 4)

	const sw, sh = 28, 12
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(sw, sh)
	ov.SetRect(0, 0, sw, sh)
	screen.Clear()
	ov.Draw(screen)
	screen.Sync()

	// The dialog width matches urwid's calculate_left_right_padding:
	// maxwidth = max(28-2-2, 0) = 24; width = int(24*65/100+0.5) = 16,
	// centered in 28 → x=6..21, valign MIDDLE in 12 → rows 4-7 (4 tall).
	// Find the top border row.
	var top string
	topY := -1
	for y := range sh {
		var b strings.Builder
		for x := range sw {
			b.WriteString(cellString(screen, x, y))
		}
		s := b.String()
		if strings.Contains(s, "┌") {
			top = s
			topY = y
			break
		}
	}
	if topY < 0 {
		t.Fatalf("no dialog top border found in:\n%s", strings.Join(func() []string {
			out := []string{}
			for y := range sh {
				var b strings.Builder
				for x := range sw {
					b.WriteString(cellString(screen, x, y))
				}
				out = append(out, b.String())
			}
			return out
		}(), "\n"))
	}
	rr := []rune(top)
	left := -1
	right := -1
	for i, r := range rr {
		if r == '┌' && left < 0 {
			left = i
		}
		if r == '┐' {
			right = i
		}
	}
	width := right - left + 1
	if width != 16 {
		t.Errorf("URL dialog width = %d (x=%d..%d), want 16 (urwid: int(24*65/100+0.5))", width, left, right)
	}
	if !strings.Contains(top, " Enter URL ") {
		t.Errorf("URL dialog top border missing title \" Enter URL \":\n  %q", top)
	}
	// Centered: left margin ≈ right margin.
	if left < 5 || left > 7 {
		t.Errorf("URL dialog left margin = %d, want ~6 (centered in 28)", left)
	}
}

// TestDeleteNodeDialogMatchesPython renders the delete-node confirm dialog
// (Python KnownNodes.delete_selected_entry, Network.py:921-961: title "?",
// message "Delete Node\n<display>\n", Yes/No) at the 48-wide list-slot width
// (RELATIVE_100 of the 52-wide left pane minus left/right=2) and compares it to
// the nomadnet canvas. This pins the 2-button list-slot confirm rendering.
func TestDeleteNodeDialogMatchesPython(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.ShowListSlotConfirm("?", "Delete Node\n<node>\n", nil, nil)
	dialog := nd.listSlotOverlay.Dialog()
	rows := screenRows(t, dialog, 0, 0, 48, 6, 48, 6, 6)
	want := []string{
		"┌────────────────────── ? ─────────────────────┐",
		"│                  Delete Node                 │",
		"│                    <node>                    │",
		"│                                              │",
		"│< Yes               >     < No               >│",
		"└──────────────────────────────────────────────┘",
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d:\n  got  %q\n  want %q", i, rows[i], want[i])
		}
	}
}

// TestIngestResultSlotPlacedInListColumn verifies ShowIngestResult is overlaid
// on the conversations LIST column (Python columns_widget.contents[0], 52 wide),
// not screen-centered: the "Ingest message URI" title appears within the left
// 52-column pane, the border is default-style, and the OK button is right-
// aligned (0.6 spacer / 0.4 button).
func TestIngestResultSlotPlacedInListColumn(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowIngestResult(IngestSuccess)
	if cd.listSlotOverlay == nil {
		t.Fatal("ShowIngestResult did not install a list-slot overlay")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	cd.content.SetRect(0, 0, 80, 24)
	screen.Clear()
	cd.content.Draw(screen)
	screen.Sync()

	// Find the title row "Ingest message URI" and confirm it lies within the
	// left 52-column pane (slot-placed), not centered on the 80-wide screen.
	titleX := -1
	titleY := -1
	for y := range 24 {
		for x := range 80 {
			if strings.Contains(cellString(screen, x, y), "I") {
				// scan a window for the title text
				var seg string
				for dx := 0; x+dx < 80 && dx < 24; dx++ {
					seg += cellString(screen, x+dx, y)
				}
				if strings.Contains(seg, "Ingest message URI") {
					titleX, titleY = x, y
					break
				}
			}
		}
		if titleX >= 0 {
			break
		}
	}
	if titleX < 0 {
		t.Fatal("Ingest message URI title not found")
	}
	if titleX >= 52 {
		t.Errorf("title starts at x=%d, must be within the 52-wide list column (slot-placed), not screen-centered", titleX)
	}
	// The dialog border must be default-style (no forced color).
	_, st, _ := screen.Get(titleX, titleY)
	fg, _, _ := st.Decompose()
	if fg != tcell.ColorDefault {
		t.Errorf("title cell fg = %v, want ColorDefault", fg)
	}
}
