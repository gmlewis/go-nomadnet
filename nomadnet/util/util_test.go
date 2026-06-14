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

package util

import (
	"testing"
)

func ptr(s string) *string { return &s }

func TestStripModifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *string
		want *string
	}{
		{"nil input", nil, nil},
		{"empty string", ptr(""), ptr("")},
		{"plain text", ptr("hello world"), ptr("hello world")},
		{"null byte removed", ptr("hello\x00world"), ptr("helloworld")},
		{"crlf to lf", ptr("hello\r\nworld"), ptr("hello\nworld")},
		{"cr to lf", ptr("hello\rworld"), ptr("hello\nworld")},
		{"spy emoji stripped", ptr("\U0001f575\ufe0f"), ptr("")},
		{"spy emoji no variant", ptr("\U0001f575"), ptr("🕵")},
		{"cafe accented", ptr("café"), ptr("café")},
		{"zero-width stripped", ptr("\u200b\u200chello\u200d\u200c"), ptr("hello")},
		{"variation selector", ptr("test\uFE0F"), ptr("test")},
		{"variation selector supplement", ptr("test\U000E0100"), ptr("test")},
		{"skin tone modifier", ptr("test\U0001F3FB"), ptr("test")},
		{"multiple crlf", ptr("line1\r\nline2\r\nline3"), ptr("line1\nline2\nline3")},
		{"spaces preserved", ptr("  spaces  "), ptr("spaces")},
		{"tabs stripped (ws-only)", ptr("\t"), ptr("")},
		{"newlines stripped (ws-only)", ptr("\n"), ptr("")},
		{"zwj stripped", ptr("abc\u200ddef"), ptr("abcdef")},
		{"ZWSP stripped", ptr("\u200btest\u200b"), ptr("test")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripModifiers(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("StripModifiers() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("StripModifiers() = nil, want %v", *tt.want)
				return
			}
			if *got != *tt.want {
				t.Errorf("StripModifiers(%q) = %q, want %q", func() string {
					if tt.in != nil {
						return *tt.in
					}
					return "<nil>"
				}(), *got, *tt.want)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *string
		want *string
	}{
		{"nil input", nil, nil},
		{"empty string", ptr(""), ptr("")},
		{"plain text", ptr("hello world"), ptr("hello world")},
		{"cafe accented", ptr("café"), ptr("café")},
		{"snowman emoji stripped", ptr("☃snowman"), ptr("snowman")},
		{"zero-width stripped", ptr("test\u200b\u200c"), ptr("test")},
		{"multiple spaces collapsed", ptr("hello  world  test"), ptr("hello world test")},
		{"combining mark stripped", ptr("\u0301combining"), ptr("combining")},
		{"emoji stripped", ptr("\U0001F600emoji\U0001F601"), ptr("emoji")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeName(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("SanitizeName() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("SanitizeName() = nil, want %v", *tt.want)
				return
			}
			if *got != *tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", func() string {
					if tt.in != nil {
						return *tt.in
					}
					return "<nil>"
				}(), *got, *tt.want)
			}
		})
	}
}

func TestStripMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"plain text", "hello", "hello"},
		{"color code 4-digit", "`F000`B123", ""},
		{"color code 8-digit", "`FT0000FF`BT123456", ""},
		{"format tags", "`*`!`_`=", ""},
		{"bold+italic combined", "`f`b", ""},
		{"heading tags", "`<heading`>", "heading"},
		{"image tag", "`{image_data}", "image_data}"},
		{"inline color", "before`F000after", "beforeafter"},
		{"no markup", "normal text", "normal text"},
		{"partial match kept", "`FAB`BCD", "`FAB`BCD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripMicron(tt.in)
			if got != tt.want {
				t.Errorf("StripMicron(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripEscapedMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"plain text", "hello", "hello"},
		{"escaped color 4-digit", "\u00A6F000", ""},
		{"escaped color 8-digit", "\u00A6FT0000FF", ""},
		{"escaped format tags", "\u00A6*\u00A6!\u00A6_", ""},
		{"escaped bold+italic", "\u00A6f\u00A6b", ""},
		{"escaped heading", "\u00A6<heading\u00A6>", "heading"},
		{"escaped image", "\u00A6{image_data}", "image_data}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripEscapedMicron(tt.in)
			if got != tt.want {
				t.Errorf("StripEscapedMicron(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnescapeMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"plain text", "hello", "hello"},
		{"escaped color 4-digit", "\u00A6F000", "`F000"},
		{"escaped color 8-digit", "\u00A6FT0000FF", "`FT0000FF"},
		{"escaped format tags", "\u00A6*\u00A6!\u00A6_", "`*`!`_"},
		{"escaped bold+italic", "\u00A6f\u00A6b", "`f`b"},
		{"escaped heading", "\u00A6<\u00A6>", "`<`>"},
		{"no escapes", "no escapes here", "no escapes here"},
		{"escaped brace", "\u00A6{\u00A6}", "`{\u00A6}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := UnescapeMicron(tt.in)
			if got != tt.want {
				t.Errorf("UnescapeMicron(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripNonFormattingTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"plain text", "hello", "hello"},
		{"all non-formatting tags", "`<`>`{`r`c`l", ""},
		{"heading tags only", "`<heading`>", "heading"},
		{"all tags stripped", "all`<`>`{`r`c`lstripped", "allstripped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripNonFormattingTags(tt.in)
			if got != tt.want {
				t.Errorf("StripNonFormattingTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
