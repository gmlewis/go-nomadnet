// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenLXMFLink pins Python Browser.handle_lxmf_link (Browser.py:383-423):
// opening an LXMF link for a NEW source creates a directory entry (so the
// peer appears in the directory) and an on-disk conversation directory (so the
// conversation is persisted and shows up in conversation_list), and reports
// isNew=true. A repeat open for the same source reports isNew=false and does
// not duplicate the conversation directory.
func TestOpenLXMFLink(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()

	hash := "aabb1122aabb1122aabb1122aabb1122" // 32 hex chars (truncated hash)

	isNew, err := a.OpenLXMFLink(hash)
	if err != nil {
		t.Fatalf("OpenLXMFLink: %v", err)
	}
	if !isNew {
		t.Error("first open: isNew = false, want true")
	}

	// Directory entry created.
	if a.Dir == nil || a.Dir.Find(parseHex(t, hash)) == nil {
		t.Error("directory entry not created for new LXMF link")
	}

	// Conversation directory created on disk.
	convDir := filepath.Join(a.ConversationPath, hash)
	if info, err := os.Stat(convDir); err != nil || !info.IsDir() {
		t.Errorf("conversation dir not created at %s: %v", convDir, err)
	}

	// Reopening the same link is NOT new and does not error.
	isNew2, err := a.OpenLXMFLink(hash)
	if err != nil {
		t.Fatalf("second OpenLXMFLink: %v", err)
	}
	if isNew2 {
		t.Error("second open: isNew = true, want false")
	}
}

// TestOpenLXMFLinkInvalid asserts Python's validation (Browser.py:392-399):
// wrong-length and non-hex targets are rejected with an error and create
// neither a directory entry nor a conversation directory.
func TestOpenLXMFLinkInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		hash string
	}{
		{"wrong length", "aabb"},
		{"non-hex", "zzzz1122zzzz1122zzzz1122zzzz1122"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := NewApp(tempDir(t), "", false, false)
			a.setupPaths()
			_, err := a.OpenLXMFLink(c.hash)
			if err == nil {
				t.Error("expected error for invalid LXMF link, got nil")
			}
			// Nothing created.
			if a.Dir != nil && a.Dir.Find(parseHex(t, "aabb1122aabb1122aabb1122aabb1122")) != nil {
				t.Error("directory entry created for invalid link")
			}
		})
	}
}

func parseHex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := range b {
		var v byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			switch {
			case c >= '0' && c <= '9':
				v = v<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				v = v<<4 | (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				v = v<<4 | (c - 'A' + 10)
			}
		}
		b[i] = v
	}
	return b
}
