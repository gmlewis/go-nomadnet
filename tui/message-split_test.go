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
	"reflect"
	"testing"
)

// TestChunkByBytes verifies ChunkByBytes matches Python's _chunk_by_bytes
// (Channels.py:85) exactly. Expected values were captured from the Python
// source run in /tmp/chunk_ref.py. The algorithm slices a byte budget (not a
// rune count), strips trailing UTF-8 continuation bytes, decodes with
// errors="ignore", and splits on a whitespace boundary in the second half of
// the chunk — falling back to the whole chunk (not a single character) when
// no such boundary exists.
func TestChunkByBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		bud  int
		want []string
	}{
		{"fits in budget", "hello world", 20, []string{"hello world"}},
		{"split at word boundary", "hello world", 5, []string{"hello", "world"}},
		{"split at word mid", "a b c d e", 5, []string{"a b", "c d e"}},
		{"three byte budget ascii", "hello world", 3, []string{"hel", "lo", "wor", "ld"}},
		{"multibyte word split", "café résumé", 10, []string{"café", "résumé"}},
		{"single byte ascii", "hello world", 1, []string{"h", "e", "l", "l", "o", "w", "o", "r", "l", "d"}},
		{"two byte ascii", "hello world", 2, []string{"he", "ll", "o", "wo", "rl", "d"}},
		{"budget below multibyte char", "ñ", 1, []string{"ñ"}},
		{"multibyte below char budget", "ññ", 1, []string{"ñ", "ñ"}},
		{"multibyte three chars", "ñññ", 3, []string{"ñ", "ñ", "ñ"}},
		{"no space in second half", "abcdef", 4, []string{"abcd", "ef"}},
		{"space not in second half", "ab cd", 4, []string{"ab", "cd"}},
		{"single char spaces", "a b c d", 3, []string{"a", "b", "c d"}},
		{"leading spaces kept", "   leading", 6, []string{"   lea", "ding"}},
		{"trailing spaces chunk", "trailing   ", 8, []string{"trailing"}},
		{"tab boundary", "tab\there", 6, []string{"tab", "here"}},
		{"newline boundary", "new\nline", 6, []string{"new", "line"}},
		{"single multibyte budget one", "é", 1, []string{"é"}},
		{"multibyte space split", "é é", 4, []string{"é", "é"}},
		{"two word groups", "hello world foo bar baz", 12, []string{"hello world", "foo bar baz"}},
		{"quad groups", "aaaa bbbb cccc dddd", 9, []string{"aaaa", "bbbb", "cccc dddd"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ChunkByBytes(tt.text, tt.bud)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChunkByBytes(%q, %v) = %v, want %v", tt.text, tt.bud, got, tt.want)
			}
		})
	}
}

func TestChunkByBytesBudgetValidation(t *testing.T) {
	t.Parallel()

	if got := ChunkByBytes("hello", 0); len(got) != 0 {
		t.Errorf("budget=0: got %v parts, want 0", len(got))
	}
	if got := ChunkByBytes("hello", -1); len(got) != 0 {
		t.Errorf("budget=-1: got %v parts, want 0", len(got))
	}
	if got := ChunkByBytes("", 10); len(got) != 0 {
		t.Errorf("empty text: got %v parts, want 0", len(got))
	}
}

// TestSplitMessage verifies SplitMessage matches Python's _split_message
// (Channels.py:107), including the "(i/N) " prefix and the convergence loop.
// Expected values captured from /tmp/chunk_ref.py.
func TestSplitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{
			name:  "three parts ascii",
			text:  "hello world this is a long message",
			limit: 20,
			want:  []string{"(1/3) hello world", "(2/3) this is a", "(3/3) long message"},
		},
		{
			name:  "three parts multibyte",
			text:  "café résumé nomadnet test message",
			limit: 18,
			want:  []string{"(1/3) café résum", "(2/3) é nomadnet", "(3/3) test message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SplitMessage(tt.text, tt.limit)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitMessage(%q, %v) = %v, want %v", tt.text, tt.limit, got, tt.want)
			}
		})
	}
}

func TestSplitMessageFitsOne(t *testing.T) {
	t.Parallel()

	got := SplitMessage("short", 100)
	want := []string{"(1/1) short"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitMessage short = %v, want %v", got, want)
	}
}

func TestSplitMessageEmpty(t *testing.T) {
	t.Parallel()

	got := SplitMessage("", 100)
	want := []string{""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitMessage empty = %v, want %v", got, want)
	}
}

func TestSplitMessageReturnsNilForTinyLimit(t *testing.T) {
	t.Parallel()

	if got := SplitMessage("hello world", 5); got != nil {
		t.Errorf("SplitMessage tiny limit = %v, want nil", got)
	}
}

func TestNeedsSplit(t *testing.T) {
	t.Parallel()

	if !NeedsSplit("hello world", 5) {
		t.Error("NeedsSplit should be true when text exceeds limit")
	}
	if NeedsSplit("hi", 100) {
		t.Error("NeedsSplit should be false when text fits")
	}
}
