// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package conversation

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSafeAttachmentNameUTF8Truncation verifies that over-long attachment
// filenames containing multibyte UTF-8 characters are truncated by character
// (rune) count, matching Python's Conversation.safe_attachment_name — which
// slices `base[:200-len(ext)]` on a Python str (a sequence of code points),
// never splitting a multibyte rune. The previous Go port sliced by byte
// count, which both diverged from the Python output and produced incomplete
// UTF-8 sequences that render as U+FFFD.
//
// Golden values were captured from the Python original
// (nomadnet/Conversation.py:815 safe_attachment_name) by running its exact
// truncation logic: every case yields exactly 200 characters with no
// replacement character.
func TestSafeAttachmentNameUTF8Truncation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  string
		expect string // exact Python output
	}{
		{
			name:   "euro_250_txt",
			input:  strings.Repeat("€", 250) + ".txt",
			expect: strings.Repeat("€", 196) + ".txt",
		},
		{
			name:   "hiragana_200_txt",
			input:  strings.Repeat("日", 200) + ".txt",
			expect: strings.Repeat("日", 196) + ".txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SafeAttachmentName(tc.input, "attachment")
			if got != tc.expect {
				t.Errorf("SafeAttachmentName byte-exact mismatch:\n  got  (%v bytes, %v runes): %q\n  want (%v bytes, %v runes): %q",
					len(got), utf8.RuneCountInString(got), got,
					len(tc.expect), utf8.RuneCountInString(tc.expect), tc.expect)
			}
			if !utf8.ValidString(got) {
				t.Errorf("output is not valid UTF-8: %q", got)
			}
			if strings.ContainsRune(got, '\uFFFD') {
				t.Errorf("output contains U+FFFD: %q", got)
			}
		})
	}
}

// TestSafeAttachmentNameUTF8TruncationMixed covers a mixed-script name where
// the 200-character cut lands mid-word. The exact Python output is not
// asserted byte-for-byte (it depends on splitext/repeat alignment); instead
// the invariant is asserted: 200 characters, valid UTF-8, no U+FFFD, and the
// extension preserved.
func TestSafeAttachmentNameUTF8TruncationMixed(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("Müller 日本 €", 30) + ".dat"
	got := SafeAttachmentName(input, "attachment")
	if rc := utf8.RuneCountInString(got); rc != 200 {
		t.Errorf("rune count = %v, want 200 (got %q)", rc, got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("output is not valid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("output contains U+FFFD: %q", got)
	}
	if !strings.HasSuffix(got, ".dat") {
		t.Errorf("extension lost: %q", got)
	}
}
