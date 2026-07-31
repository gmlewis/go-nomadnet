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

// DarkThemeNickColors is the 24-color nick palette for the dark theme.
// Matches Python nomadnet theme_dark["nick_colors"] exactly.
var DarkThemeNickColors = []string{
	"f68787", "00c394", "d59e00", "62be00", "a1ac76", "95b600",
	"76a9ee", "81b385", "7eb1a1", "e89264", "7cb0b0", "00c0c0",
	"8cacbb", "32b4db", "98a8c3", "bbab00", "95a0fd", "a9a2ca",
	"ad98fe", "c58ffa", "df83f4", "c49abf", "f380c7", "f484a7",
}

// LightThemeNickColors is the 24-color nick palette for the light theme.
// Matches Python nomadnet theme_light["nick_colors"] exactly.
var LightThemeNickColors = []string{
	"ca0000", "008000", "9d1c00", "007800", "2c5200", "006800",
	"004ac0", "006100", "005d2c", "b70000", "005b5b", "007b7a",
	"005071", "0064a5", "004580", "714f00", "0026d3", "48318c",
	"5200d5", "8400cf", "aa00c8", "820079", "c60086", "c80043",
}

// NickColorByHash returns a hex color string from the given palette
// based on the sender's identity hash. Matches Python's get_nick_color():
// (int.from_bytes(hash, "big") + shift) % len(palette).
// The shift defaults to 15 to match Python's default.
func NickColorByHash(hash []byte, palette []string) string {
	if len(palette) == 0 {
		return "#bbbbbb"
	}
	if len(hash) == 0 {
		return "#" + palette[0]
	}

	// Reduce the full hash modulo len(palette) using Horner's method so the
	// result matches Python's int.from_bytes(hash, "big") % len(palette) for
	// any hash length, without needing a big.Int. val stays below len(palette)
	// at every step, so there is no overflow even for very long hashes.
	m := uint64(len(palette))
	var val uint64
	for _, b := range hash {
		val = (val*256 + uint64(b)) % m
	}

	const shift uint64 = 15
	idx := (val + shift) % m
	return "#" + palette[idx]
}

// NickColor returns a tview color tag string for the given nick.
// Uses NickColorByHash to pick from the palette.
func NickColor(nick string, theme int) string {
	var palette []string
	if theme == ThemeLight {
		palette = LightThemeNickColors
	} else {
		palette = DarkThemeNickColors
	}
	return NickColorByHash([]byte(nick), palette)
}
