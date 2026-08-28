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
	"github.com/rivo/tview"
)

// TestB1MessageEntryRowOrder pins B1: each message entry renders the HEADER
// row(s) FIRST, then the indented content rows, then one trailing empty row —
// the Python LXMessageWidget Pile order [title, content, ""] (source:
// Conversations.py:2670-2762 — pile_widgets starts with the title AttrMap,
// appends the indented content, and ends with urwid.Text("")). The earlier Go
// build rendered the content above the header and an extra blank row between
// header and content; this test fails if either ever returns.
func TestB1MessageEntryRowOrder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cw := NewConversationWidget(app, "b1b1b1b1")
	cw.OwnHash = []byte{7}
	cw.TimeFormat = "%Y-%m-%d %H:%M:%S"

	msg := ConversationMessage{
		Content:            "first body line\nsecond body line",
		Timestamp:          time.Unix(1700000000, 0).UTC(),
		State:              lxmfStateDelivered,
		SourceHash:         []byte{7}, // == OwnHash → outbound
		TransportEncrypted: true,
	}
	entry := cw.renderMessageEntry(msg)

	// The entry text (with tags) split into rows must be exactly:
	//   [header row, "  first body line", "  second body line", ""].
	rows := strings.Split(entry.GetText(false), "\n")
	if len(rows) < 5 {
		t.Fatalf("entry rows = %v (%q), want header + 2 content rows + trailing blank", len(rows), entry.GetText(false))
	}
	headerRow := rows[0]
	if !strings.Contains(headerRow, "✓ → ") {
		t.Errorf("row 0 = %q, want the header (✓ → relative time | timestamp ⚿)", headerRow)
	}
	if strings.Contains(headerRow, "first body line") {
		t.Errorf("row 0 contains content: %q (content must come AFTER the header)", headerRow)
	}
	if rows[1] != "  first body line" {
		t.Errorf("row 1 = %q, want \"  first body line\" (Python indents content two columns)", rows[1])
	}
	if rows[2] != "  second body line" {
		t.Errorf("row 2 = %q, want \"  second body line\"", rows[2])
	}
	if rows[3] != "" {
		t.Errorf("row 3 = %q, want \"\" (the LXMessageWidget trailing empty row)", rows[3])
	}

	// No blank row between the header and the content (Python's Pile has
	// title and content adjacent).
	if headerRow == "" || rows[1] == "" {
		t.Error("header and content rows must be adjacent (no blank between)")
	}
}

// TestB1MultiLineHeaderKeepsContentAfter pins the caution-header case: an
// inbound unvalidated signature renders a TWO-line header ("⚠ ← <desc>\n  <rel>
// | <ts>") and the content still follows the whole header block.
func TestB1MultiLineHeaderKeepsContentAfter(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cw := NewConversationWidget(app, "b1b1b1b1")

	msg := ConversationMessage{
		Content:              "body text",
		Timestamp:            time.Unix(1700000000, 0).UTC(),
		SourceHash:           []byte{9}, // != OwnHash (nil) → inbound
		SignatureValidated:   false,
		SignatureDescription: "Invalid Signature",
	}
	entry := cw.renderMessageEntry(msg)
	rows := strings.Split(entry.GetText(false), "\n")

	if !strings.Contains(rows[0], "⚠") || !strings.Contains(rows[0], "Invalid Signature") {
		t.Errorf("row 0 = %q, want the caution header first line (⚠ ← Invalid Signature)", rows[0])
	}
	if !strings.Contains(strings.Join(rows[0:3], "\n"), "body text") {
		t.Errorf("content must appear after the two-line header block; rows: %q", rows[:min(4, len(rows))])
	}
}

// TestB2MinimalEditorInvisible pins B2: the minimal composer is an INVISIBLE
// empty one-line footer — no caption and no placeholder text (Python:
// MessageEdit(caption="", edit_text="") wrapped in AttrMap(..., "msg_editor"),
// Conversations.py:1916). The Go port used to paint a visible
// "Type a message... (Ctrl-D to send)" placeholder row.
func TestB2MinimalEditorInvisible(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cw := NewConversationWidget(app, "b2b2b2b2")

	// The placeholder is not directly readable (tview keeps it in the inner
	// TextArea), so the blank-render assertion below is the observable check:
	// any placeholder text would paint glyphs in the one-row footer.
	if got := cw.editor.GetLabel(); got != "" {
		t.Errorf("editor label = %q, want \"\" (Python MessageEdit caption=\"\")", got)
	}
	if got := cw.editor.GetText(); got != "" {
		t.Errorf("editor text = %q, want \"\"", got)
	}

	// The footer row must render BLANK: draw the editor on a simulation
	// screen and confirm no glyph is painted anywhere in its one-row rect.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 1)
	cw.editor.SetRect(0, 0, 60, 1)
	cw.editor.Draw(screen)
	for x := range 60 {
		if c, _, _, _ := cellContent(screen, x, 0); c != ' ' && c != 0 {
			t.Errorf("editor cell (%v,0) = %q, want blank (invisible footer)", x, string(c))
		}
	}
	_ = tview.Escape // keep the tview import if assertions above change
}
