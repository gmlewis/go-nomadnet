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

package micron

import "strings"

// Color-depth selection, matching Python MicronParser.make_style's nested
// mono_color/low_color/high_color helpers (MicronParser.py:442-591).
//
// urwid palette entries carry one color value per supported terminal color
// depth: mono, low (16-color), and high (88/256/24-bit). make_style registers
// a palette entry by computing these variants from the current Micron fg/bg
// color. These functions implement that mapping so the Go port can produce
// equivalent specs without urwid.

// monoColor is the monochrome terminal color. Python always returns "default".
func monoColor(fg, bg string) string {
	return "default"
}

// lowColor maps a Micron color to a low-color (16-color) terminal name.
// Matches Python's low_color (MicronParser.py:446-516).
func lowColor(color string) string {
	// Mirrors Python's try/except: the operations below are guarded so no
	// panic can occur, but any unexpected input falls through to "default".
	result := "default"
	if color == "default" {
		return "default"
	} else if len(color) == 6 {
		// Reduce a 6-digit color to its 3-digit representative by taking
		// the high nibble of each channel (chars 0, 2, 4).
		r := string(color[0])
		g := string(color[2])
		b := string(color[4])
		color = r + g + b
	}

	if len(color) == 3 {
		const t = 7
		if color[0] == 'g' {
			// Grayscale: Python uses int(color[1:2]), i.e. just the first
			// digit after 'g', for the threshold comparison.
			val, ok := parseDecDigit(color[1])
			if !ok {
				return "default"
			}
			switch {
			case val < 25:
				result = "black"
			case val < 50:
				result = "dark gray"
			case val < 75:
				result = "light gray"
			default:
				result = "white"
			}
			return result
		}

		r, okR := parseHexDigit(color[0])
		g, okG := parseHexDigit(color[1])
		b, okB := parseHexDigit(color[2])
		if !okR || !okG || !okB {
			return "default"
		}

		if r == g && g == b {
			val := r * 6
			switch {
			case val < 12:
				result = "black"
			case val < 50:
				result = "dark gray"
			case val < 80:
				result = "light gray"
			default:
				result = "white"
			}
			return result
		}

		// The remaining conditions are independent `if` statements in Python
		// (not elif), so later matches overwrite earlier results. Replicate
		// that override semantics exactly.
		if r == b {
			if r > g {
				if r > t {
					result = "light magenta"
				} else {
					result = "dark magenta"
				}
			} else {
				if g > t {
					result = "light green"
				} else {
					result = "dark green"
				}
			}
		}
		if b == g {
			if b > r {
				if b > t {
					result = "light cyan"
				} else {
					result = "dark cyan"
				}
			} else {
				if r > t {
					result = "light red"
				} else {
					result = "dark red"
				}
			}
		}
		if g == r {
			if g > b {
				if g > t {
					result = "yellow"
				} else {
					result = "brown"
				}
			} else {
				if b > t {
					result = "light blue"
				} else {
					result = "dark blue"
				}
			}
		}
		if r > g && r > b {
			if r > t {
				result = "light red"
			} else {
				result = "dark red"
			}
		}
		if g > r && g > b {
			if g > t {
				result = "light green"
			} else {
				result = "dark green"
			}
		}
		if b > g && b > r {
			if b > t {
				result = "light blue"
			} else {
				result = "dark blue"
			}
		}
		return result
	}

	return result
}

// highColor maps a Micron color to a high-color (88/256/24-bit) terminal
// color spec. Matches Python's high_color (MicronParser.py:518-567).
func highColor(color string) string {
	if color == "default" {
		return "default"
	}
	if len(color) == 6 {
		// parseval_hex clamps each hex char to [0,16] and lowercases it.
		// For valid hex digits this is just the lowercase digit; an invalid
		// digit yields "default" (Python's except branch).
		var sb strings.Builder
		sb.WriteByte('#')
		for i := 0; i < 6; i++ {
			v, ok := parseHexDigit(color[i])
			if !ok {
				return "default"
			}
			sb.WriteByte(toHexLower(v))
		}
		return sb.String()
	}
	if len(color) == 3 {
		if color[0] == 'g' {
			// parseval_dec clamps a decimal digit to [0,9].
			v1, ok1 := parseDecDigit(color[1])
			v2, ok2 := parseDecDigit(color[2])
			if !ok1 || !ok2 {
				return "default"
			}
			return "g" + string(decDigit(v1)) + string(decDigit(v2))
		}
		v1, ok1 := parseHexDigit(color[0])
		v2, ok2 := parseHexDigit(color[1])
		v3, ok3 := parseHexDigit(color[2])
		if !ok1 || !ok2 || !ok3 {
			return "default"
		}
		r := toHexLower(v1)
		g := toHexLower(v2)
		b := toHexLower(v3)
		return "#" + strings.Repeat(string(r), 2) + strings.Repeat(string(g), 2) + strings.Repeat(string(b), 2)
	}
	return "default"
}

// parseHexDigit parses a single hex digit (0-9, a-f, A-F) to its value 0-15.
func parseHexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// parseDecDigit parses a single decimal digit (0-9) to its value.
func parseDecDigit(c byte) (int, bool) {
	if c >= '0' && c <= '9' {
		return int(c - '0'), true
	}
	return 0, false
}

// toHexLower converts a value 0-15 to a lowercase hex digit byte.
func toHexLower(v int) byte {
	if v < 10 {
		return byte('0' + v)
	}
	return byte('a' + v - 10)
}

// decDigit converts a value 0-9 to its decimal digit byte. Matches Python's
// parseval_dec, which clamps to [0,9]; callers already guarantee 0-9.
func decDigit(v int) byte {
	if v > 9 {
		v = 9
	}
	if v < 0 {
		v = 0
	}
	return byte('0' + v)
}
