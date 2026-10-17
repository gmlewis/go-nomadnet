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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// TestQRDialogBodyAlwaysShowsEncodedText pins the rule that a QR dialog carries
// the text the code encodes, on a line beneath the code, whether or not the code
// itself could be rendered: a camera can be missing or refuse to focus, and the
// line under the code is then the only way to carry the address to another
// device by hand. Python renders the same "< data >" line (Conversations.py:
// 662-678).
func TestQRDialogBodyAlwaysShowsEncodedText(t *testing.T) {
	t.Parallel()

	const addr = "2a6105f57145860441a62fe3b2a1352c"
	tests := []struct {
		name   string
		qrText string
	}{
		{name: "with QR code", qrText: RenderQRText(addr)},
		{name: "QR code unavailable", qrText: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, rows := qrDialogBody(tc.qrText, addr)
			lines := dialogRowTexts(body)
			joined := strings.Join(lines, "\n")

			want := "< " + addr + " >"
			if !strings.Contains(joined, want) {
				t.Errorf("encoded text %q missing from the QR dialog body:\n%v", want, joined)
			}
			// The text belongs below the code, so it must be the body's last row.
			if got := lines[len(lines)-1]; got != want {
				t.Errorf("last body row = %q, want the encoded text %q", got, want)
			}
			// The declared height has to hold every row the body draws, or the
			// encoded text is the row that gets clipped.
			if len(lines) != rows {
				t.Errorf("body draws %v rows but reports %v", len(lines), rows)
			}
		})
	}
}

// TestQRDialogBodyRendersCodeAboveText pins the layout order: the code first,
// then the text it carries. A body that showed only the text would satisfy a
// "contains" check while dropping the code entirely.
func TestQRDialogBodyRendersCodeAboveText(t *testing.T) {
	t.Parallel()

	const addr = "2a6105f57145860441a62fe3b2a1352c"
	qrText := RenderQRText(addr)
	if qrText == "" {
		t.Fatal("RenderQRText returned no code for a valid address")
	}

	body, _ := qrDialogBody(qrText, addr)
	lines := dialogRowTexts(body)

	codeRow := -1
	textRow := -1
	for i, line := range lines {
		if codeRow < 0 && strings.ContainsAny(line, "█▀▄") {
			codeRow = i
		}
		if textRow < 0 && strings.Contains(line, "< "+addr+" >") {
			textRow = i
		}
	}
	if codeRow < 0 {
		t.Fatalf("no QR code row in the dialog body:\n%v", strings.Join(lines, "\n"))
	}
	if textRow < 0 {
		t.Fatalf("no encoded-text row in the dialog body:\n%v", strings.Join(lines, "\n"))
	}
	if textRow <= codeRow {
		t.Errorf("encoded text at row %v, code at row %v: the text must be below the code", textRow, codeRow)
	}
}

// TestShowQRDialogDrawsEncodedTextInsideTheModal drives the dialog the peer LXMF
// button opens (ShowQRDialog, the OnLXMFQR path) and asserts the address the
// code carries is drawn on a row inside the modal's border on a real screen. A
// dialog sized two rows short of its body drops the bottom row silently, and the
// bottom row is the encoded text — the modal would then show a code that cannot
// be scanned and no way to read the address out of it.
func TestShowQRDialogDrawsEncodedTextInsideTheModal(t *testing.T) {
	t.Parallel()

	const addr = "2a6105f57145860441a62fe3b2a1352c"
	app := newTestApp()
	main := tview.NewTextView().SetText("main")
	pages := app.Dialogs.Init(app.Application, main)
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(main)

	app.Dialogs.ShowQRDialog("LXMF", addr)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	const w, h = 100, 40
	screen.SetSize(w, h)
	pages.SetRect(0, 0, w, h)
	pages.Draw(screen)
	screen.Sync()

	addrRow, borderRow, codeRow := -1, -1, -1
	var drawn strings.Builder
	for y := range h {
		var line strings.Builder
		for x := range w {
			c, _, _, _ := cellContent(screen, x, y)
			line.WriteRune(c)
		}
		drawn.WriteString(line.String())
		drawn.WriteByte('\n')
		switch {
		case addrRow < 0 && strings.Contains(line.String(), "< "+addr+" >"):
			addrRow = y
		case codeRow < 0 && strings.ContainsAny(line.String(), "█▀▄"):
			codeRow = y
		}
		if borderRow < 0 && strings.Contains(line.String(), "└") {
			borderRow = y
		}
	}

	if addrRow < 0 {
		t.Fatalf("the encoded address is on no drawn row — the modal shows a code with nothing to read from:\n%v", drawn.String())
	}
	if codeRow < 0 {
		t.Fatalf("no QR code is drawn in the modal:\n%v", drawn.String())
	}
	if borderRow < 0 {
		t.Fatalf("the modal's bottom border is not drawn (the dialog was clipped):\n%v", drawn.String())
	}
	if addrRow >= borderRow {
		t.Errorf("encoded address row %v sits on/after the modal's bottom border row %v — the dialog is too short to hold it", addrRow, borderRow)
	}
	if addrRow <= codeRow {
		t.Errorf("encoded address row %v is not below the code row %v", addrRow, codeRow)
	}
}
