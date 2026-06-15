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
	"testing"
)

func TestChunkByBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		bud  int
		want []string
	}{
		{
			name: "fits in budget",
			text: "hello world",
			bud:  20,
			want: []string{"hello world"},
		},
		{
			name: "split at space near end",
			text: "hello world",
			bud:  5,
			want: []string{"h", "ello", "world"},
		},
		{
			name: "split at word boundary mid",
			text: "a b c d e",
			bud:  5,
			want: []string{"a b", "c d e"},
		},
		{
			name: "single char budget",
			text: "hello world",
			bud:  3,
			want: []string{"h", "e", "l", "lo", "w", "o", "rld"},
		},
		{
			name: "unicode chars",
			text: "café résumé",
			bud:  10,
			want: []string{"c", "a", "f", "é", " résumé"},
		},
		{
			name: "empty string",
			text: "",
			bud:  10,
			want: []string{},
		},
		{
			name: "exact fit",
			text: "short",
			bud:  100,
			want: []string{"short"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ChunkByBytes(tt.text, tt.bud)
			if len(got) != len(tt.want) {
				t.Errorf("got %d parts, want %d", len(got), len(tt.want))
				for i, p := range got {
					t.Logf("  [%d] %q", i, p)
				}
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("part[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChunkByBytesBudgetValidation(t *testing.T) {
	t.Parallel()

	got := ChunkByBytes("hello", 0)
	if len(got) != 0 {
		t.Errorf("budget=0: got %d parts, want 0", len(got))
	}
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		limit int
		want []string
	}{
		{
			name:  "fits in one part",
			text:  "hello",
			limit: 100,
			want:  []string{"(1/1) hello"},
		},
		{
			name:  "empty text",
			text:  "",
			limit: 100,
			want:  []string{""},
		},
		{
			name:  "split into two at word boundary",
			text:  "word1 word2 word3",
			limit: 20,
			want:  []string{"(1/2) word1 word2", "(2/2) word3"},
		},
		{
			name:  "single char fits",
			text:  "x",
			limit: 12,
			want:  []string{"(1/1) x"},
		},
		{
			name:  "340 chars fits in 350",
			text:  repeatStr("a", 340),
			limit: 350,
			want:  []string{"(1/1) " + repeatStr("a", 340)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SplitMessage(tt.text, tt.limit)
			if len(got) != len(tt.want) {
				t.Errorf("got %d parts, want %d", len(got), len(tt.want))
				for i, p := range got {
					t.Logf("  [%d] %q (%d bytes)", i, p, len([]byte(p)))
				}
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("part[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitMessageConverges(t *testing.T) {
	t.Parallel()

	text := repeatStr("hello ", 100)
	got := SplitMessage(text, 50)
	if len(got) < 2 {
		t.Errorf("expected multiple parts, got %d", len(got))
	}
	// Each part should start with the (i/N) prefix
	for i, p := range got {
		prefix := "(" + itoa(i+1) + "/" + itoa(len(got)) + ") "
		if len(p) < len(prefix) || p[:len(prefix)] != prefix {
			t.Errorf("part[%d] missing prefix %q: %q", i, prefix, p)
		}
	}
	// Each part should be within the limit
	for i, p := range got {
		if len([]byte(p)) > 50 {
			t.Errorf("part[%d] = %d bytes, exceeds limit 50", i, len([]byte(p)))
		}
	}
}

func TestSplitMessageTooLongForOne(t *testing.T) {
	t.Parallel()

	// When prefix overhead makes budget negative, should return nil
	got := SplitMessage("hello", 5)
	// "(1/1) " is 6 bytes which exceeds limit of 5
	// Python returns None in this case
	if got != nil {
		t.Errorf("got %v, want nil (prefix exceeds limit)", got)
	}
}

func TestSplitMessageReturnsNilForTinyLimit(t *testing.T) {
	t.Parallel()

	got := SplitMessage("x", 3)
	// "(1/1) " is 6 bytes > 3
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
