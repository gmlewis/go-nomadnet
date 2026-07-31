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
)

func TestNickColorByHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hash      []byte
		wantColor string
	}{
		{
			name:      "zero hash returns palette[15] due to shift",
			hash:      make([]byte, 16),
			wantColor: "#" + DarkThemeNickColors[15], // (0 + 15) % 24 = 15
		},
		{
			// byte index 7 set: int.from_bytes = 1<<64. The full-hash mod
			// gives (val + 15) % 24 = 7, NOT 16 — the old 8-byte-truncating
			// implementation returned 16 here, which diverged from Python.
			name:      "hash with high byte set",
			hash:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0, 0},
			wantColor: "#" + DarkThemeNickColors[7], // (val + 15) % 24 = 7
		},
		{
			name: "hash with multiple bytes set",
			hash: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09,
				0, 0, 0, 0, 0, 0, 0, 0},
			wantColor: "#" + DarkThemeNickColors[15], // (val + 15) % 24 = 15
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NickColorByHash(tt.hash, DarkThemeNickColors)
			if got != tt.wantColor {
				t.Errorf("NickColorByHash(%x) = %q, want %q", tt.hash, got, tt.wantColor)
			}
		})
	}
}

func TestNickColorByHashConsistency(t *testing.T) {
	t.Parallel()

	hash := []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}

	c1 := NickColorByHash(hash, DarkThemeNickColors)
	c2 := NickColorByHash(hash, DarkThemeNickColors)
	if c1 != c2 {
		t.Errorf("NickColorByHash not deterministic: %q != %q", c1, c2)
	}
}

// TestNickColorByHashPythonParity verifies the full-hash modular reduction
// against Python's get_nick_color (Channels.py:1254):
//
//	nick_colors[(int.from_bytes(sender_hash, "big") + shift) % len(nick_colors)]
//
// with the default shift of 15. Expected values were captured from the Python
// source. These cases exercise bytes beyond the first 8, which the previous
// implementation (truncating to the first 8 bytes as a uint64) got wrong.
func TestNickColorByHashPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hash      []byte
		wantColor string
	}{
		{"last byte set 16B", bytes16(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1), "#95a0fd"},
		{"first byte set 16B", bytes16(1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0), "#81b385"},
		{"all 0xff 16B", bytes16(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff), "#76a9ee"},
		{"range 0..15 16B", bytes16(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15), "#76a9ee"},
		{"range 1..16 16B", bytes16(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16), "#81b385"},
		{"all 7 32B", bytes32(7), "#98a8c3"},
		{"31 zeros then 23 32B", append(make([]byte, 31), 23), "#98a8c3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NickColorByHash(tt.hash, DarkThemeNickColors)
			if got != tt.wantColor {
				t.Errorf("NickColorByHash(%x) = %q, want %q", tt.hash, got, tt.wantColor)
			}
		})
	}
}

// TestNickColorByHashPythonParityLight runs the same parity check against the
// light-theme palette (captured from Python).
func TestNickColorByHashPythonParityLight(t *testing.T) {
	t.Parallel()

	got := NickColorByHash(bytes16(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15), LightThemeNickColors)
	if want := "#004ac0"; got != want {
		t.Errorf("light parity: got %q, want %q", got, want)
	}
}

func bytes16(b ...byte) []byte { return b }

func bytes32(fill byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = fill
	}
	return b
}

func TestNickColorByHashDifferentHashes(t *testing.T) {
	t.Parallel()

	hash1 := make([]byte, 16)
	hash2 := bytes16(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)

	c1 := NickColorByHash(hash1, DarkThemeNickColors)
	c2 := NickColorByHash(hash2, DarkThemeNickColors)
	if c1 == c2 {
		t.Errorf("Different hashes should produce different colors: both %q", c1)
	}
}

func TestNickColorByHashPaletteSize(t *testing.T) {
	t.Parallel()

	// All colors returned must be valid palette entries
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		hash := make([]byte, 16)
		hash[15] = byte(i % 256)
		hash[14] = byte(i / 256)
		color := NickColorByHash(hash, DarkThemeNickColors)
		seen[color] = true
		if !isValidHexColor(color) {
			t.Errorf("invalid hex color: %q", color)
		}
	}
}

func TestNickColorByHashStringInput(t *testing.T) {
	t.Parallel()

	// Test with a string hash (like nick string)
	hash := []byte("some-nick-name!!")
	got := NickColorByHash(hash, DarkThemeNickColors)
	if got == "" {
		t.Error("NickColorByHash returned empty string for string input")
	}
}

func TestNickColorByHashEmpty(t *testing.T) {
	t.Parallel()

	// Empty hash should still return a valid color (falls back to default)
	got := NickColorByHash(nil, DarkThemeNickColors)
	if got == "" {
		t.Error("NickColorByHash returned empty string for nil input")
	}
}

func isValidHexColor(s string) bool {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
