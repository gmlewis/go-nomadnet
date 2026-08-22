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
	"encoding/hex"
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

// TestNickColorByHashPythonParity is a LIVE cross-implementation check: it
// execs Python's real Channels.get_nick_color (nomadnet.ui.textui.Channels)
// with a mock app whose rrc_nick_colors_theme is None (so the theme palette is
// used) and derives the expected nick color freshly on every run. Go owns the
// input battery (byte hashes of varied lengths, including the empty-hash edge
// case); Python owns the reference behavior:
//
//	nick_colors[(int.from_bytes(sender_hash, "big") + shift) % len(nick_colors)]
//
// with the default shift of 15. The same battery is run against both the dark
// and light theme palettes. The test SKIPs, not fails, when the Python
// reference is not importable.
//
// Python returns the bare hex (no '#' prefix); Go prefixes '#', so the '#'
// is stripped before comparison. Python's get_nick_color returns
// theme["nick_peer"] only for a non-bytes sender_hash; the battery always
// passes bytes (including empty bytes b""), for which Python computes
// (int.from_bytes(b"","big")+15)%24 == 15, i.e. palette[15] — NOT palette[0].
func TestNickColorByHashPythonParity(t *testing.T) {
	t.Parallel()

	// Battery of byte hashes as hex strings, exercising lengths beyond the
	// first 8 bytes (the old uint64-truncating implementation diverged here),
	// the empty-hash edge case, single bytes, and string/multibyte content.
	hashHex := []string{
		"",                                  // empty bytes
		strings.Repeat("00", 16),            // 16 zero bytes
		"00000000000000000000000000000001",  // last byte set, 16B
		"01" + strings.Repeat("00", 15),     // first byte set, 16B
		strings.Repeat("ff", 16),            // all 0xff, 16B
		"000102030405060708090a0b0c0d0e0f",  // range 0..15, 16B
		"0102030405060708090a0b0c0d0e0f10",  // range 1..16, 16B
		strings.Repeat("07", 32),            // all 0x07, 32B
		strings.Repeat("00", 31) + "17",     // 31 zeros then 23, 32B
		"42",                                // single byte
		hexEncodeString("some-nick-name!!"), // nick string bytes
		"c3a9",                              // multibyte 'é'
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // full 32-byte hash
	}

	type nickInput struct {
		Hex   string `json:"hex"`
		Theme string `json:"theme"` // "dark" | "light"
	}
	var inputs []nickInput
	for _, h := range hashHex {
		inputs = append(inputs, nickInput{Hex: h, Theme: "dark"}, nickInput{Hex: h, Theme: "light"})
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
class MockApp:
    rrc_nick_colors_theme = None
app = MockApp()
cases = json.load(sys.stdin)
out = []
for c in cases:
    hb = bytes.fromhex(c["hex"]) if c["hex"] else b""
    theme = C.theme_dark if c["theme"] == "dark" else C.theme_light
    out.append(C.get_nick_color(hb, theme, app, 15))
json.dump(out, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, inputs, script, &want)

	for i, inp := range inputs {
		name := inp.Theme + "/" + inp.Hex
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			hb, err := hexDecode(inp.Hex)
			if err != nil {
				t.Fatalf("bad hex %q: %v", inp.Hex, err)
			}
			palette := DarkThemeNickColors
			if inp.Theme == "light" {
				palette = LightThemeNickColors
			}
			got := strings.TrimPrefix(NickColorByHash(hb, palette), "#")
			if got != want[i] {
				t.Errorf("NickColorByHash(%x, %s) = #%s, want #%s (Python)", hb, inp.Theme, got, want[i])
			}
		})
	}
}

func bytes16(b ...byte) []byte { return b }

// hexEncodeString returns the lowercase hex of the UTF-8 bytes of s.
func hexEncodeString(s string) string {
	return hex.EncodeToString([]byte(s))
}

// hexDecode decodes a hex string; the empty string decodes to empty bytes.
func hexDecode(s string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}
	return hex.DecodeString(s)
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
	for i := range 1000 {
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
