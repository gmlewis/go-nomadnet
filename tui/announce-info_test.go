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

	"github.com/rivo/tview"
)

// aiRowInfo holds the expected label prefix and full content for one
// AnnounceInfo text row, used by the golden layout assertions.
type aiRowInfo struct {
	prefix string // the exact label prefix (e.g. "Time  : ")
	full   string // the full row text excluding trailing pad spaces
}

// trimTrailing strips trailing spaces from a rendered row so the golden
// expectations can ignore right-padding (which is fill, not content).
func trimTrailing(s string) string { return strings.TrimRight(s, " ") }

// renderAnnounceInfo builds and renders an AnnounceInfo pile at the given size
// and returns the rendered rows (trailing pad spaces stripped per row).
func renderAnnounceInfo(t *testing.T, nd *NetworkDisplay, ann AnnounceEntry, data AnnounceInfoData, w, h int) []string {
	t.Helper()
	ai := newAnnounceInfoDisplay(nd, ann, data)
	rows := renderPrimitive(t, ai.Widget(), w, h)
	for i := range rows {
		rows[i] = trimTrailing(rows[i])
	}
	return rows
}

// TestAnnounceInfoPeerLayout pins the PEER-branch AnnounceInfo against Python's
// AnnounceInfo (Network.py:235-246, peer case): rows Time/Addr/Type/Name/Trust,
// a divider, the 2-row "Announce Data" block, another divider, and the weighted
// Back/Converse button row. Labels use the exact spacing Python uses
// ("Time  : " 2 spaces before colon; "Trust : " 1 space before colon). The
// trust string is colored via the trust palette style.
func TestAnnounceInfoPeerLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	hash := strings.Repeat("a", 32) // 16-byte destination hash = 32 hex
	ann := AnnounceEntry{
		Timestamp:   ts,
		SourceHash:  hash,
		AppData:     "Hello World",
		Type:        "peer",
		DisplayName: "",
	}
	data := AnnounceInfoData{
		DisplayStr: "<" + hash + ">",
		TrustStr:   "Unknown",
		TrustStyle: "list_unknown",
		OpStr:      "Unknown",
	}

	rows := renderAnnounceInfo(t, nd, ann, data, 50, 12)

	// Row order: Time, Addr, Type, Name, Trust, Divider, AnnounceData(2),
	// Divider, buttons.
	wantPrefix := []string{
		"Time  : ",
		"Addr  : ",
		"Type  : ",
		"Name  : ",
		"Trust : ",
		"",
		"Announce Data:",
		"",
		"",
		"",
	}
	for i, p := range wantPrefix {
		if p != "" && !strings.HasPrefix(rows[i], p) {
			t.Errorf("row %d = %q, want prefix %q", i, rows[i], p)
		}
	}

	if want := "Time  : 2026-01-02 15:04:05"; rows[0] != want {
		t.Errorf("row 0 = %q, want %q", rows[0], want)
	}
	if want := "Addr  : <" + hash + ">"; rows[1] != want {
		t.Errorf("row 1 = %q, want %q", rows[1], want)
	}
	// "Ⓟ " glyph trailing space is stripped by trimTrailing.
	if want := "Type  : Peer Ⓟ"; rows[2] != want {
		t.Errorf("row 2 = %q, want %q", rows[2], want)
	}
	if want := "Name  : <" + hash + ">"; rows[3] != want {
		t.Errorf("row 3 = %q, want %q", rows[3], want)
	}
	if want := "Trust : Unknown"; rows[4] != want {
		t.Errorf("row 4 = %q, want %q", rows[4], want)
	}

	// Dividers are full-width divider1 (┄) lines.
	divider := strings.Repeat("┄", 50)
	if rows[5] != divider {
		t.Errorf("row 5 (divider) = %q, want %q", rows[5], divider)
	}
	if rows[8] != divider {
		t.Errorf("row 8 (divider) = %q, want %q", rows[8], divider)
	}

	// Announce data block: label on row 6 (trailing space trimmed), data on row 7.
	if rows[6] != "Announce Data:" {
		t.Errorf("row 6 = %q, want %q", rows[6], "Announce Data:")
	}
	if rows[7] != "Hello World" {
		t.Errorf("row 7 = %q, want %q", rows[7], "Hello World")
	}

	// Button row: Back (col 23) + spacer (5) + Converse (col 22) = [23,5,22].
	// urwid Button puts ">" at the right edge of its column allocation, so each
	// button is "< label   ...   >" (label left-justified, ">" at the edge).
	btnRow := rows[9]
	if len([]rune(btnRow)) != 50 {
		t.Errorf("button row len = %d, want 50", len([]rune(btnRow)))
	}
	backCol := btnRow[:23]
	if !strings.HasPrefix(backCol, "< Back") || !strings.HasSuffix(backCol, ">") {
		t.Errorf("Back column = %q, want < Back ... >", backCol)
	}
	if got := btnRow[23:28]; got != "     " {
		t.Errorf("spacer = %q, want 5 spaces", got)
	}
	converseCol := btnRow[28:]
	if !strings.HasPrefix(converseCol, "< Converse") || !strings.HasSuffix(converseCol, ">") {
		t.Errorf("Converse column = %q, want < Converse ... >", converseCol)
	}
}

// TestAnnounceInfoNodeLayout pins the NODE-branch AnnounceInfo: the Operator
// row ("Oprtr : ") is inserted between Name and Trust (Network.py:248-250), and
// the button row is Back/Connect/Msg Op/Save (weights 0.45/0.1/0.45/0.1/0.45/
// 0.1/0.45 → widths [11,2,11,2,11,2,11] at inner 50).
func TestAnnounceInfoNodeLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	hash := strings.Repeat("b", 32)
	ann := AnnounceEntry{
		Timestamp:   ts,
		SourceHash:  hash,
		AppData:     "Node app data string",
		Type:        "node",
		DisplayName: "MyNode",
	}
	data := AnnounceInfoData{
		DisplayStr: "MyNode",
		TrustStr:   "Trusted",
		TrustStyle: "list_trusted",
		OpStr:      "Unknown",
	}

	rows := renderAnnounceInfo(t, nd, ann, data, 50, 14)

	if want := "Type  : Nomad Network Node Ⓝ"; rows[2] != want {
		t.Errorf("row 2 = %q, want %q", rows[2], want)
	}
	if want := "Name  : MyNode"; rows[3] != want {
		t.Errorf("row 3 = %q, want %q", rows[3], want)
	}
	// Operator inserted at index 4 (between Name and Trust).
	if want := "Oprtr : Unknown"; rows[4] != want {
		t.Errorf("row 4 = %q, want %q (operator)", rows[4], want)
	}
	if want := "Trust : Trusted"; rows[5] != want {
		t.Errorf("row 5 = %q, want %q (trust)", rows[5], want)
	}
	// Dividers at rows 6 and 9.
	divider := strings.Repeat("┄", 50)
	if rows[6] != divider {
		t.Errorf("row 6 (divider) = %q, want %q", rows[6], divider)
	}
	if rows[9] != divider {
		t.Errorf("row 9 (divider) = %q, want %q", rows[9], divider)
	}
	// Button row at index 10: 7 columns [11,2,11,2,11,2,11].
	btnRow := rows[10]
	wantButtons := []struct {
		start, end int
		label      string
	}{
		{0, 11, "< Back"},
		{13, 24, "< Connect"},
		{26, 37, "< Msg Op"},
		{39, 50, "< Save"},
	}
	for _, b := range wantButtons {
		seg := btnRow[b.start:b.end]
		if !strings.HasPrefix(seg, b.label) {
			t.Errorf("button segment [%d:%d] = %q, want prefix %q", b.start, b.end, seg, b.label)
		}
	}
}

// TestAnnounceInfoPNLayout pins the PN-branch AnnounceInfo (Network.py:226-233):
// only Time/Addr/Type, a divider, then the Back/Use-as-default button row.
func TestAnnounceInfoPNLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	hash := strings.Repeat("c", 32)
	ann := AnnounceEntry{
		Timestamp:   ts,
		SourceHash:  hash,
		AppData:     "",
		Type:        "pn",
		DisplayName: "",
	}
	data := AnnounceInfoData{
		DisplayStr: "<" + hash + ">",
		TrustStr:   "Unknown",
		TrustStyle: "list_unknown",
		OpStr:      "Unknown",
	}

	rows := renderAnnounceInfo(t, nd, ann, data, 50, 8)

	if want := "Type  : LXMF Propagation Node ↑"; rows[2] != want {
		t.Errorf("row 2 = %q, want %q", rows[2], want)
	}
	// Divider at row 3, button row at row 4.
	divider := strings.Repeat("┄", 50)
	if rows[3] != divider {
		t.Errorf("row 3 (divider) = %q, want %q", rows[3], divider)
	}
	btnRow := rows[4]
	backCol := btnRow[:23]
	if !strings.HasPrefix(backCol, "< Back") || !strings.HasSuffix(backCol, ">") {
		t.Errorf("Back column = %q, want < Back ... >", backCol)
	}
	useCol := btnRow[28:]
	if !strings.HasPrefix(useCol, "< Use as default") || !strings.HasSuffix(useCol, ">") {
		t.Errorf("Use-as-default column = %q, want < Use as default ... >", useCol)
	}
}

// TestAnnounceInfoDataTruncation pins the non-trusted announce-data truncation
// (Network.py:96-97): when trust != TRUSTED and len > 32, the data is cut to 32
// chars + " [...]". Trusted announces show the full data.
func TestAnnounceInfoDataTruncation(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	long := strings.Repeat("x", 60)
	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	ann := AnnounceEntry{Timestamp: ts, SourceHash: strings.Repeat("d", 32), AppData: long, Type: "peer"}

	// Unknown trust → truncated to 32 + " [...]" (38 chars, fits the 50-wide
	// pane without wrapping).
	rows := renderAnnounceInfo(t, nd, ann, AnnounceInfoData{
		DisplayStr: "<x>", TrustStr: "Unknown", TrustStyle: "list_unknown",
	}, 50, 12)
	want := strings.Repeat("x", 32) + " [...]"
	if rows[7] != want {
		t.Errorf("unknown-trust data = %q, want %q", rows[7], want)
	}

	// Trusted trust → full data (not truncated). Use 40 chars so it fits the
	// 50-wide pane on one row (the announce-data block is 2 rows: label + data).
	ann.AppData = strings.Repeat("y", 40)
	rows = renderAnnounceInfo(t, nd, ann, AnnounceInfoData{
		DisplayStr: "<x>", TrustStr: "Trusted", TrustStyle: "list_trusted",
	}, 50, 12)
	if want := strings.Repeat("y", 40); rows[7] != want {
		t.Errorf("trusted data = %q, want %q", rows[7], want)
	}
}

// TestAnnounceInfoEscReturnsToStream verifies showAnnounceDetailFor enters the
// info view and HandleEsc returns to the stream (Python keypress esc →
// show_announce_stream, Network.py:60-65).
func TestAnnounceInfoEscReturnsToStream(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	announces := []AnnounceEntry{
		{DisplayName: "Peer1", Type: "peer", SourceHash: strings.Repeat("e", 32), Timestamp: time.Now()},
	}
	nd := NewNetworkDisplay(app, announces, nil)

	nd.showAnnounceDetail(0)
	if !nd.inInfoView {
		t.Fatal("showAnnounceDetail did not enter info view")
	}
	if !nd.HandleEsc() {
		t.Fatal("HandleEsc returned false in info view")
	}
	if nd.inInfoView {
		t.Fatal("HandleEsc did not leave info view")
	}
}

// TestAnnounceInfoFallsBackWithoutResolver verifies the view still builds when
// no OnResolveAnnounceInfo is wired: it falls back to the AnnounceEntry's own
// fields (display name, trust derived from the entry's TrustLevel).
func TestAnnounceInfoFallsBackWithoutResolver(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  strings.Repeat("f", 32),
		AppData:     "data",
		Type:        "peer",
		DisplayName: "FallingBack",
		TrustLevel:  "trusted",
	}
	// No OnResolveAnnounceInfo wired: showAnnounceDetailFor must still build.
	nd.showAnnounceDetailFor(ann)
	if !nd.inInfoView {
		t.Fatal("showAnnounceDetailFor did not enter info view without resolver")
	}
	// The pile is swapped into listBox; rendering it must not panic.
	_ = tview.NewApplication()
	if nd.listBox == nil {
		t.Fatal("listBox is nil")
	}
}

// TestTrustStringAndStyleMapping pins the trust-level → display string + palette
// style mapping (Network.py:103-122).
func TestTrustStringAndStyleMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		level, str, style string
	}{
		{"trusted", "Trusted", "list_trusted"},
		{"untrusted", "Untrusted", "list_untrusted"},
		{"warning", "Warning", "list_untrusted"},
		{"unknown", "Unknown", "list_unknown"},
		{"", "Unknown", "list_unknown"},
	}
	for _, c := range cases {
		if got := trustStringFromLevel(c.level); got != c.str {
			t.Errorf("trustStringFromLevel(%q) = %q, want %q", c.level, got, c.str)
		}
		if got := trustStyleFromLevel(c.level); got != c.style {
			t.Errorf("trustStyleFromLevel(%q) = %q, want %q", c.level, got, c.style)
		}
	}
}

// TestAnnounceInfoTopTrimClipsHeader pins the urwid Filler cursor-trim: when the
// node AnnounceInfo Pile (11 rows) is rendered into a 9-row-inner slot (the
// 80x24 left-pane slot), the TOP two rows (Time + Addr) are clipped to keep the
// focused button row visible at the bottom (urwid/widget/filler.py:228-238).
// The visible rows are Type, Name, Oprtr, Trust, divider, Announce Data:, data,
// divider, buttons — and the button row lands on the last row.
func TestAnnounceInfoTopTrimClipsHeader(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	hash := strings.Repeat("b", 32)
	ann := AnnounceEntry{
		Timestamp:   ts,
		SourceHash:  hash,
		AppData:     "Node app data string",
		Type:        "node",
		DisplayName: "MyNode",
	}
	data := AnnounceInfoData{
		DisplayStr: "MyNode",
		TrustStr:   "Trusted",
		TrustStyle: "list_trusted",
		OpStr:      "Unknown",
	}

	// 9-row-inner slot (height 9) forces a 2-row top-trim.
	rows := renderAnnounceInfo(t, nd, ann, data, 50, 9)

	// Time + Addr are clipped from the top.
	if strings.HasPrefix(rows[0], "Time") {
		t.Errorf("row 0 should be clipped (Time trimmed), got %q", rows[0])
	}
	// First visible row is Type.
	if want := "Type  : Nomad Network Node Ⓝ"; rows[0] != want {
		t.Errorf("row 0 = %q, want %q (Type after trim)", rows[0], want)
	}
	if want := "Name  : MyNode"; rows[1] != want {
		t.Errorf("row 1 = %q, want %q", rows[1], want)
	}
	if want := "Oprtr : Unknown"; rows[2] != want {
		t.Errorf("row 2 = %q, want %q", rows[2], want)
	}
	if want := "Trust : Trusted"; rows[3] != want {
		t.Errorf("row 3 = %q, want %q", rows[3], want)
	}
	divider := strings.Repeat("┄", 50)
	if rows[4] != divider {
		t.Errorf("row 4 (divider) = %q, want %q", rows[4], divider)
	}
	if rows[5] != "Announce Data:" {
		t.Errorf("row 5 = %q, want %q", rows[5], "Announce Data:")
	}
	if rows[6] != "Node app data string" {
		t.Errorf("row 6 = %q, want %q", rows[6], "Node app data string")
	}
	if rows[7] != divider {
		t.Errorf("row 7 (divider) = %q, want %q", rows[7], divider)
	}
	// Button row on the last visible row (row 8), focused/visible.
	btnRow := rows[8]
	if !strings.HasPrefix(btnRow, "< Back") {
		t.Errorf("row 8 (buttons) = %q, want < Back ...", btnRow)
	}
	if !strings.HasSuffix(btnRow, ">") {
		t.Errorf("row 8 (buttons) = %q, want suffix >", btnRow)
	}
}

// TestAnnounceInfoPeerTopTrim pins the peer branch's 1-row top-trim: the peer
// Pile is 10 rows; in a 9-row slot the top row (Time) is clipped, leaving Addr
// as the first visible row and the button row at the bottom.
func TestAnnounceInfoPeerTopTrim(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	hash := strings.Repeat("a", 32)
	ann := AnnounceEntry{
		Timestamp:   ts,
		SourceHash:  hash,
		AppData:     "Hello World",
		Type:        "peer",
		DisplayName: "",
	}
	data := AnnounceInfoData{
		DisplayStr: "<" + hash + ">",
		TrustStr:   "Unknown",
		TrustStyle: "list_unknown",
		OpStr:      "Unknown",
	}

	rows := renderAnnounceInfo(t, nd, ann, data, 50, 9)

	// Time clipped; Addr is the first visible row.
	if strings.HasPrefix(rows[0], "Time") {
		t.Errorf("row 0 should be clipped (Time trimmed), got %q", rows[0])
	}
	if want := "Addr  : <" + hash + ">"; rows[0] != want {
		t.Errorf("row 0 = %q, want %q (Addr after trim)", rows[0], want)
	}
	// Button row on the last row.
	if !strings.HasPrefix(rows[8], "< Back") || !strings.HasSuffix(rows[8], ">") {
		t.Errorf("row 8 (buttons) = %q, want < Back ... >", rows[8])
	}
}

// TestPileFillerPadsBottomWhenContentFits verifies that when the content fits
// the box (no overflow), the pile is top-aligned and the bottom rows are blank
// (urwid Filler valign=TOP pads the bottom).
func TestPileFillerPadsBottomWhenContentFits(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	ann := AnnounceEntry{
		Timestamp: ts, SourceHash: strings.Repeat("c", 32), AppData: "",
		Type: "pn", DisplayName: "",
	}
	data := AnnounceInfoData{
		DisplayStr: "<c>", TrustStr: "Unknown", TrustStyle: "list_unknown",
	}

	// PN pile is 5 rows; render into 8 rows → top-aligned, 3 blank rows below.
	rows := renderAnnounceInfo(t, nd, ann, data, 50, 8)
	if want := "Time  : 2026-01-02 15:04:05"; rows[0] != want {
		t.Errorf("row 0 = %q, want %q", rows[0], want)
	}
	divider := strings.Repeat("┄", 50)
	if rows[3] != divider {
		t.Errorf("row 3 (divider) = %q, want %q", rows[3], divider)
	}
	if !strings.HasPrefix(rows[4], "< Back") {
		t.Errorf("row 4 (buttons) = %q, want < Back ...", rows[4])
	}
	// Rows 5-7 are blank (bottom padding).
	for i := 5; i < 8; i++ {
		if rows[i] != "" {
			t.Errorf("row %d = %q, want blank (bottom pad)", i, rows[i])
		}
	}
}
