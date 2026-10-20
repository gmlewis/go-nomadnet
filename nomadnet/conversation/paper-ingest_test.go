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

package conversation

import (
	"bytes"
	"encoding/hex"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
	"github.com/gmlewis/go-reticulum/lxmf"
)

// TestPaperOutputAppendsIngestedMessage pins which message object the paper path
// adds to the conversation. Python's paper_output ingests the outbound message and
// appends the INGESTED path in every mode that ingests:
//
//	message_path = Conversation.ingest(lxm, self.app, originator=True)
//	self.messages.append(ConversationMessage(message_path))
//
// (Conversation.py:358-360, 368-370, 377-379), so the appended entry always
// points at a file inside the conversation directory — never at the downloads
// artefact or the temporary print file.
func TestPaperOutputAppendsIngestedMessage(t *testing.T) {
	t.Parallel()

	modes := []struct {
		name string
		mode PaperOutputMode
	}{
		{"save_uri", PaperSaveURI},
		{"save_qr", PaperSaveQR},
		{"print_qr", PaperPrintQR},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps, dest, downloads, tmpFiles := paperDeps(t)
			c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
			c.SetSendDeps(deps)

			if _, ok := c.PaperOutput("hello paper", "a title", tc.mode); !ok {
				t.Fatalf("PaperOutput %v returned !ok", tc.name)
			}

			if len(c.Messages) != 1 {
				t.Fatalf("conversation has %v messages, want the 1 ingested message", len(c.Messages))
			}
			got := c.Messages[0].FilePath

			// The appended message is the file the ingest wrote into the
			// conversation directory.
			rel, err := filepath.Rel(deps.convPath, got)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Errorf("appended message %q is not inside the conversation directory %q", got, deps.convPath)
			}
			// And it exists on disk.
			if _, err := os.Stat(got); err != nil {
				t.Errorf("appended conversation message %q does not exist: %v", got, err)
			}
			// It is NOT the downloads artefact nor the temp print file.
			for _, forbidden := range []string{downloads, tmpFiles} {
				if rel2, err := filepath.Rel(forbidden, got); err == nil && rel2 != ".." && !strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
					t.Errorf("appended message %q points into %q", got, forbidden)
				}
			}
		})
	}
}

// TestPaperOutputQRGeometryMatchesPython pins the saved QR's geometry to the
// reference. Python renders it with qrcode.make(error_correction=QR_ERROR_CORRECTION,
// border=1, data=self.as_uri(finalise=False)) — box_size is left at the library
// default of 10 (LXMessage.py:718-736). lxmf.Message.AsQR renders a 1-module
// border at 10 pixels per module; this test holds it to that.
//
// The expected module count comes from the Python reference itself (qrcode's
// modules_count for the same URI and error-correction level), so no size is
// transcribed here.
func TestPaperOutputQRGeometryMatchesPython(t *testing.T) {
	t.Parallel()

	const (
		boxSize = 10
		border  = 1
	)

	deps, dest, downloads, _ := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	path, ok := c.PaperOutput("geometry probe body", "a title", PaperSaveQR)
	if !ok {
		t.Fatal("PaperOutput save_qr returned !ok")
	}
	if !strings.HasPrefix(filepath.Base(path), "LXM_") || !strings.HasSuffix(path, ".png") {
		t.Fatalf("unexpected QR path %q", path)
	}

	data, err := os.ReadFile(filepath.Join(downloads, filepath.Base(path)))
	if err != nil {
		t.Fatalf("reading the saved QR: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding the saved QR: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != bounds.Dy() {
		t.Errorf("QR image is %vx%v, want square", bounds.Dx(), bounds.Dy())
	}

	// The URI the QR encodes: rebuilding the message the same way PaperOutput does
	// yields the same URI length, and a QR version is chosen by length.
	probe, err := lxmf.NewMessage(dest, deps.source, "geometry probe body", "a title", nil)
	if err != nil {
		t.Fatal(err)
	}
	probe.DesiredMethod = lxmf.MethodPaper
	uri, err := probe.AsURI(false)
	if err != nil {
		t.Fatalf("AsURI: %v", err)
	}

	want := paperQRExpectedSide(t, uri, boxSize, border)
	if got := bounds.Dx(); got != want {
		t.Errorf("QR image side = %v px, want %v ((modules+%v)*%v for the reference's border and box size)",
			got, want, 2*border, boxSize)
	}
}

// paperQRExpectedSide asks the Python reference for the module count of a QR code
// carrying uri at LXMessage.QR_ERROR_CORRECTION and returns the pixel side that
// border/boxSize imply. PIL is not needed: qrcode computes the matrix without it.
func paperQRExpectedSide(t *testing.T, uri string, boxSize, border int) int {
	t.Helper()
	testutils.SkipIfNoPythonNomadnet(t)

	req := map[string]any{
		"uri":              uri,
		"box_size":         boxSize,
		"border":           border,
		"error_correction": lxmf.QRErrorCorrection,
	}
	var side int
	testutils.RunPythonNomadnet(t, req, paperQRSideScript, &side)
	return side
}

// paperQRSideScript builds the QR matrix with the reference library at the same
// error-correction level Python's LXMessage uses, and reports the image side the
// given border and box size imply.
const paperQRSideScript = `
import json, sys
import qrcode
req = json.loads(sys.stdin.read() or "{}")
ec = req["error_correction"]
qr = qrcode.QRCode(
    border=req["border"],
    box_size=req["box_size"],
    error_correction=getattr(qrcode.constants, ec),
)
qr.add_data(req["uri"])
qr.make(fit=True)
print(json.dumps((qr.modules_count + 2 * req["border"]) * req["box_size"]))
`
