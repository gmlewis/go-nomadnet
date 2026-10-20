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

	"github.com/gmlewis/go-nomadnet/testutils"
	"github.com/gmlewis/tcell/v2"
)

// The composer footer must be REPLACED by the centered identity-unknown
// warning when the peer's identity keys are not known (Python
// check_editor_allowed, Conversations.py:2195-2215): without the keys the
// editor is unusable, so Python swaps the whole footer for the banner and
// restores it when the identity arrives. The banner is AttrMap+Padding+
// Text(align=CENTER) — NOT a modal dialog.
func TestConversationEditorAllowedBanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "abcdef0123456789abcdef0123456789")

	// Unknown identity: the banner replaces the editor.
	cw.OnEditorAllowed = func(string) bool { return false }
	cw.buildFooter()

	if cw.footerArea == nil {
		t.Fatal("footerArea is nil")
	}
	if cw.footerArea.GetItemCount() == 0 {
		t.Fatal("footer area is empty with an unknown peer identity")
	}
	banner := cw.footerArea.GetItem(0)
	cb, ok := banner.(*cautionBannerView)
	if !ok {
		t.Fatalf("footer item = %T, want *cautionBannerView (full-width footer swap)", banner)
	}
	if cw.cautionBanner == nil || cw.cautionBanner != cb {
		t.Error("cautionBanner field not wired to the footer item")
	}

	// Body must match the Python SOT string (not a dialog title/body pair).
	// PROVENANCE: ui/textui/Conversations.py:2208-2216 (nomadnet 1.2.8). These
	// fragments are a cheap structural check only — the authoritative
	// byte-for-byte comparison against the live Python reference is
	// TestEditorAllowedBannerTextMatchesSOT below.
	joined := cw.editorAllowedBannerText()
	for _, frag := range []string{
		"You cannot currently message this peer",
		"identity keys are not known",
		"you will be able to send messages as soon as they arrive",
		"Ctrl-E, and use the query button",
	} {
		if !strings.Contains(joined, frag) {
			t.Errorf("banner text missing %q in %q", frag, joined)
		}
	}
	// Must not look like a modal confirm dialog.
	for _, banned := range []string{"Confirm", "Yes", "No", "OK"} {
		if strings.Contains(joined, " "+banned+" ") {
			t.Errorf("banner should not contain dialog button %q", banned)
		}
	}

	// Known identity: the editor returns and the caution view is dropped.
	cw.OnEditorAllowed = func(string) bool { return true }
	cw.buildFooter()
	if cw.cautionBanner != nil {
		t.Error("cautionBanner still set with a known peer identity")
	}
	if cw.footerArea.GetItem(0) == banner {
		t.Error("footer still shows the banner with a known peer identity")
	}
}

// TestCautionBannerFullWidthCenterDraw verifies that the caution banner paints
// msg_header_caution across the ENTIRE footer width and centers each line in
// that width, matching Python. A TextView path that colors only ~46 columns
// (hardcoded pre-wrap) would leave the rest of the pane unpainted, with
// mis-centered ragged lines. Draw must fill every cell and place the line with
// urwid ceil-left centering at the real width.
func TestCautionBannerFullWidthCenterDraw(t *testing.T) {
	t.Parallel()

	const width = 80
	const height = 12
	// Short standalone line so centering is unambiguous (urwid ceil-left).
	raw := "\n\nHello\n"

	view := newCautionBannerView(raw, tcell.ColorBlack, tcell.ColorYellow)
	view.SetRect(0, 0, width, height)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	view.Draw(screen)
	screen.Show()

	// Every cell must carry a non-default background (AttrMap full-width fill).
	textRow := -1
	for row := range height {
		painted := 0
		for col := range width {
			main, style, _ := screen.Get(col, row)
			_, bg, _ := style.Decompose()
			if bg == tcell.ColorDefault {
				t.Fatalf("cell (%v,%v) left default bg — banner must fill full footer width", col, row)
			}
			if strings.TrimSpace(main) != "" {
				painted++
			}
		}
		if painted > 0 {
			textRow = row
		}
	}
	if textRow < 0 {
		t.Fatal("no text glyphs drawn on any row")
	}

	// The text row must be centered: leading pad roughly (width-len)/2.
	var b strings.Builder
	for col := range width {
		main, _, _ := screen.Get(col, textRow)
		b.WriteString(main)
	}
	got := strings.TrimRight(b.String(), " ")
	trimmed := strings.TrimSpace(got)
	if trimmed == "" {
		t.Fatal("text row is blank after trim")
	}
	leftPad := len(got) - len(strings.TrimLeft(got, " "))
	// urwid ceil-left: pad = (width - textWidth + 1) / 2.
	wantPad := (width - len("Hello") + 1) / 2
	if leftPad != wantPad {
		t.Errorf("left pad = %v, want urwid ceil-left %v (text centered in width %v)", leftPad, wantPad, width)
	}
}

// The delete-conversation confirmation must be a LIST-SLOT overlay with
// Python's exact chrome (installed 1.2.8, Conversations.py:571-598): the "?"
// DialogLineBox title, the centered two-line "Delete conversation with\n<name>"
// body, and flat Yes/No buttons — not the global DialogManager form with its
// "Confirm" title and whole-screen centering.
func TestDeleteConversationConfirmListSlotDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var yes, no int
	cd.DeleteConversationConfirm("Trusted Seed Peer",
		func() { yes++ },
		func() { no++ })

	if cd.listSlotOverlay == nil {
		t.Fatal("delete confirm must be a list-slot overlay (Python overlays the list column)")
	}
	dialog := cd.listSlotOverlay.Dialog()
	if title := dialog.GetTitle(); title != "?" {
		t.Errorf("delete dialog title = %q, want %q", title, "?")
	}
	rows := dialogRowTexts(dialog)
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"Delete conversation with", "Trusted Seed Peer", "Yes", "No"} {
		if !strings.Contains(joined, want) {
			t.Errorf("delete dialog missing %q in %q", want, rows)
		}
	}
	for _, banned := range []string{"Confirm", "Delete conversation with Trusted"} {
		if strings.Contains(joined, banned) {
			t.Errorf("old global-dialog string %q still present in %q", banned, rows)
		}
	}

	// Yes fires the onYes path and dismisses; No fires onNo.
	cd.listSlotOverlay.Dialog().InputHandler()
	if cd.listSlotOverlay == nil {
		t.Fatal("dialog dismissed prematurely")
	}
	if yes != 0 || no != 0 {
		t.Fatalf("callbacks fired without activation (yes=%v no=%v)", yes, no)
	}
}

// bannerTextProbeScript extracts the identity-unknown banner body from the
// INSTALLED nomadnet reference source. It locates the package directory via
// nomadnet's own __file__ and parses Conversations.py with ast, so the expected
// string is derived from the reference's real source instead of being
// transcribed by hand (see editorAllowedBannerText for why that matters). RNS
// output is silenced because it prints to stdout and would corrupt the JSON
// channel RunPythonNomadnet decodes.
const bannerTextProbeScript = `
import ast, json, os
import RNS
RNS.loglevel = RNS.LOG_NONE
import nomadnet

path = os.path.join(os.path.dirname(nomadnet.__file__), "ui", "textui", "Conversations.py")
with open(path, encoding="utf-8") as f:
    src = f.read()
marker = "You cannot currently message this peer"
matches = [
    node.value
    for node in ast.walk(ast.parse(src))
    if isinstance(node, ast.Constant)
    and isinstance(node.value, str)
    and marker in node.value
]
print(json.dumps({"file": path, "matches": matches}))
`

// TestEditorAllowedBannerTextMatchesSOT asserts the identity-unknown banner
// wording byte-for-byte against the installed Python nomadnet reference,
// extracted from the reference's own source with ast. Deriving the expectation
// from the reference rather than hand-copying it keeps a nomadnet upgrade from
// being silently absorbed: nomadnet 1.2.8 reworded this banner, and a
// transcribed literal would have pinned the superseded text.
//
// Skips (does not fail) when no Python nomadnet reference is importable, per the
// testutils convention.
func TestEditorAllowedBannerTextMatchesSOT(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "abcdef0123456789abcdef0123456789")

	var ref struct {
		File    string   `json:"file"`
		Matches []string `json:"matches"`
	}
	testutils.RunPythonNomadnet(t, nil, bannerTextProbeScript, &ref)

	if len(ref.Matches) != 1 {
		t.Fatalf("expected exactly 1 string containing the banner marker in %v, got %d: %q",
			ref.File, len(ref.Matches), ref.Matches)
	}

	// Python builds "\n" + g["info"] + "\n\n" + <body>; the Go side
	// interpolates the same glyph, so strip that prefix and compare the body.
	text := cw.editorAllowedBannerText()
	body, ok := strings.CutPrefix(text, "\n"+cw.glyphs()["info"])
	if !ok {
		t.Fatalf("banner text %q does not start with the info-glyph prefix", text)
	}
	if body != ref.Matches[0] {
		t.Errorf("banner body differs from the nomadnet %v reference (%v)\ngot:  %q\nwant: %q",
			testutils.SOTNomadnetVersion, ref.File, body, ref.Matches[0])
	}
}
