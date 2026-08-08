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

package storage

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"testing"
)

//go:embed testdata/paths_parity.json
var pathsFS embed.FS

// TestPathsPythonParity verifies that every storage.Paths field and path
// helper matches the path conventions Python's NomadNetworkApp assigns in its
// constructor (and that Conversation.py uses for per-conversation/per-message
// paths). The golden values in testdata/paths_parity.json were captured from a
// script that reproduces the verbatim path assignments from
// NomadNetworkApp.__init__ lines 106-124 plus the Conversation.py conventions,
// pinned to a fixed configdir.
//
// On POSIX platforms filepath.Join uses "/" separators, matching Python's
// string concatenation. The golden file stores "/"-separated paths; we compare
// via filepath.Join semantics so the test is correct on the CI runners (Linux)
// and darwin.
func TestPathsPythonParity(t *testing.T) {
	t.Parallel()

	const root = "/home/user/.nomadnetwork"
	const sampleHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const sampleMsg = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	data, err := pathsFS.ReadFile("testdata/paths_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var want map[string]string
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	p := New(root)

	cases := []struct {
		name string
		got  string
		key  string
	}{
		{"Root", p.Root, "config"}, // Root itself is the configdir; "config" key is configdir+"/config"
		{"Storage", p.Storage, "storage"},
		{"Identity", p.Identity, "identity"},
		{"Cache", p.Cache, "cache"},
		{"Resources", p.Resources, "resources"},
		{"Conversations", p.Conversations, "conversations"},
		{"Directory", p.Directory, "directory"},
		{"PeerSettings", p.PeerSettings, "peersettings"},
		{"TmpFiles", p.TmpFiles, "tmp"},
		{"Attachments", p.Attachments, "attachments"},
		{"Pages", p.Pages, "pages"},
		{"Files", p.Files, "files"},
		{"LogFile", p.LogFile, "logfile"},
		{"ErrorFile", p.ErrorFile, "errors"},
		{"Examples", p.Examples, "examples"},
		{"ConversationDir", p.ConversationDir(sampleHash), "_conversation_dir"},
		{"UnreadFlag", p.UnreadFlag(sampleHash), "_unread_flag"},
		{"FailedFlag", p.FailedFlag(sampleHash), "_failed_flag"},
		{"MessageDir", p.MessageDir(sampleHash, sampleMsg), "_message_path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.name == "Root" {
				// Root is the bare configdir (no trailing component).
				if c.got != root {
					t.Errorf("Root = %q, want %q", c.got, root)
				}
				return
			}
			w, ok := want[c.key]
			if !ok {
				t.Fatalf("golden file missing key %q", c.key)
			}
			// Compare via filepath.Clean so trailing-slash / separator
			// differences don't cause false failures across platforms.
			if filepath.Clean(c.got) != filepath.Clean(w) {
				t.Errorf("%v = %q, want %q", c.name, c.got, w)
			}
		})
	}

	// Conversations dir and per-message messages_path are the same directory
	// in Python (messages live directly under the conversation dir).
	if p.ConversationDir(sampleHash) != want["_messages_path"] {
		t.Errorf("ConversationDir (=messages_path) = %q, want %q",
			p.ConversationDir(sampleHash), want["_messages_path"])
	}
}
