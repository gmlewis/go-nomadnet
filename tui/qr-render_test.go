// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package tui

import (
	"strings"
	"testing"
)

// TestB3RenderQRText verifies B3: RenderQRText produces a non-empty QR code
// rendering using Unicode half-block characters for a valid LXMF address.
func TestB3RenderQRText(t *testing.T) {
	t.Parallel()

	addr := "2a6105f57145860441a62fe3b2a1352c"
	qr := RenderQRText(addr)
	if qr == "" {
		t.Fatal("B3: RenderQRText returned empty string for valid address")
	}

	// The QR code should contain half-block characters.
	if !strings.ContainsAny(qr, "▀▄█") {
		t.Error("B3: QR text should contain Unicode half-block characters (▀▄█)")
	}

	// The QR code should have multiple lines.
	lines := strings.Split(qr, "\n")
	if len(lines) < 5 {
		t.Errorf("B3: QR text has %v lines, want at least 5", len(lines))
	}
}

// TestB3RenderQRTextEmptyForTooLong verifies RenderQRText returns empty for
// text that cannot be encoded (too long).
func TestB3RenderQRTextEmptyForTooLong(t *testing.T) {
	t.Parallel()

	// A very long string that exceeds QR capacity at level L.
	long := strings.Repeat("A", 10000)
	qr := RenderQRText(long)
	if qr != "" {
		t.Error("B3: RenderQRText should return empty for too-long text")
	}
}
