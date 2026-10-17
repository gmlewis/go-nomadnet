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

	"github.com/gmlewis/tview"
)

// qrDialogWidth is the width of the QR modals in cells. The code itself is 27
// cells wide for a 32-hex address, so this leaves room around it and never
// wraps it — a wrapped code is not a code.
const qrDialogWidth = 50

// ShowQRDialog shows a centered modal holding the QR code for data, with the
// text the code carries on a centered line beneath it.
//
// The encoded line is not decoration: a camera can be missing or refuse to
// focus, and the line under the code is then the only way to carry the address
// to another device by hand, so the code and its text are shown together or not
// at all. Python's show_qr_dialog renders the same shape, the code with
// "< data >" under it (Conversations.py:662-678).
func (dm *DialogManager) ShowQRDialog(title, data string) {
	code, err := GenerateQRASCII(data)
	if err != nil {
		code = ""
	}
	body, rows := qrDialogBody(code, data)
	dm.ShowDialog(title, body, qrDialogWidth, rows+2, nil)
}

// qrDialogBody builds the body of a dialog that shows a QR code: the rendered
// code qrText (empty when the code could not be rendered) with the text the code
// carries, data, on a centered line beneath it. rows is the body's natural
// (PACK) height, so a caller can size the dialog to hold every row; an
// undersized dialog drops rows from the bottom, and the bottom row is the
// encoded text.
func qrDialogBody(qrText, data string) (*tview.Flex, int) {
	body := tview.NewFlex().SetDirection(tview.FlexRow)
	rows := 0
	if qrText != "" {
		code := tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetDynamicColors(true).
			SetText("[white]" + qrText + "[-]")
		qrRows := strings.Count(qrText, "\n") + 1
		body.AddItem(code, qrRows, 0, false)
		body.AddItem(tview.NewTextView(), 1, 0, false) // blank under the code
		rows += qrRows + 1
	}

	encoded := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("< " + data + " >")
	body.AddItem(encoded, 1, 0, false)

	return body, rows + 1
}
