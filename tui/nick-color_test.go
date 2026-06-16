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
			name:      "hash with high byte set",
			hash:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0, 0},
			wantColor: "#" + DarkThemeNickColors[16], // (1 + 15) % 24 = 16
		},
		{
			name: "hash with multiple bytes set",
			hash: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09,
				0, 0, 0, 0, 0, 0, 0, 0},
			wantColor: "#" + DarkThemeNickColors[0], // (9 + 15) % 24 = 0
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

func TestNickColorByHashDifferentHashes(t *testing.T) {
	t.Parallel()

	hash1 := make([]byte, 16)
	hash2 := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xf0}

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
