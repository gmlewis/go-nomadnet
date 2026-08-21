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
	"path/filepath"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// pathsPythonInput is the fixed config directory and sample hashes the Go test
// uses to derive storage.Paths. The same values are sent to the live Python
// reference so both sides compute paths from the identical roots.
var pathsPythonInput = map[string]string{
	"configdir":   "/home/user/.nomadnetwork",
	"source_hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	"message_id":  "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
}

// pathsParityScript imports the real nomadnet.NomadNetworkApp and
// nomadnet.Conversation references and derives every storage path FRESH from
// the current Python source on each run. It introspects the actual
// NomadNetworkApp.__init__ source for the verbatim `self.X = self.configdir +
// "/..."` assignments and the Conversation.py source for the per-conversation
// / per-message path conventions, then emits the resolved paths as a JSON
// object keyed by the short names the Go test compares against. If a future
// Python release changes a path suffix or a conversation convention, this
// script raises (and the test fails) rather than silently passing a stale
// golden.
const pathsParityScript = `
import sys, json, re, importlib, inspect
req = json.loads(sys.stdin.read() or "{}")
configdir = req["configdir"]
source_hash = req["source_hash"]
message_id = req["message_id"]

app_mod = importlib.import_module("nomadnet.NomadNetworkApp")
init_src = inspect.getsource(app_mod.NomadNetworkApp.__init__)
# Map each Python path attribute to the short key the Go test compares.
key_map = {
    "configpath": "config", "ignoredpath": "ignored", "logfilepath": "logfile",
    "errorfilepath": "errors", "pnannouncedpath": "pnannounced",
    "storagepath": "storage", "identitypath": "identity", "cachepath": "cache",
    "resourcepath": "resources", "conversationpath": "conversations",
    "directorypath": "directory", "peersettingspath": "peersettings",
    "tmpfilespath": "tmp", "attachmentpath": "attachments",
    "pagespath": "pages", "filespath": "files", "examplespath": "examples",
}
pat = re.compile(r'self\.(\w+)\s*=\s*self\.configdir\s*\+\s*["\']([^"\']*)["\']')
paths = {}
for mt in pat.finditer(init_src):
    attr = mt.group(1)
    key = key_map.get(attr)
    if key:
        paths[key] = configdir + mt.group(2)

# Verify the Conversation.py per-conversation path conventions still hold in
# the current source, then build the derived paths from the live
# conversationpath. messages_path == conversation dir (messages live directly
# under it); unread/failed flags and per-message paths hang off that dir.
conv_mod = importlib.import_module("nomadnet.Conversation")
conv_src = inspect.getsource(conv_mod.Conversation)
if not re.search(r'messages_path\s*=\s*app\.conversationpath\s*\+\s*"/"\s*\+\s*source_hash', conv_src):
    raise RuntimeError("Conversation.py messages_path convention changed")
if '"/unread"' not in conv_src or '"/failed"' not in conv_src:
    raise RuntimeError("Conversation.py unread/failed flag convention changed")

conv_dir = paths["conversations"] + "/" + source_hash
paths["_conversation_dir"] = conv_dir
paths["_messages_path"] = conv_dir
paths["_unread_flag"] = conv_dir + "/unread"
paths["_failed_flag"] = conv_dir + "/failed"
paths["_message_path"] = conv_dir + "/" + message_id

print(json.dumps(paths, ensure_ascii=False, sort_keys=True))
`

// pathsPythonOnce caches the single live Python run that derives fresh expected
// paths, so the per-field subtests below share one python3 exec.
var (
	pathsPythonOnce sync.Once
	pathsPythonOut  map[string]string
)

func pathsPython(t *testing.T) map[string]string {
	t.Helper()
	pathsPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, pathsPythonInput, pathsParityScript, &pathsPythonOut)
	})
	return pathsPythonOut
}

// TestPathsPythonParity verifies that every storage.Paths field and path
// helper matches the path conventions Python's NomadNetworkApp assigns in its
// constructor (and that Conversation.py uses for per-conversation/per-message
// paths). The expected paths are derived FRESH on every run by introspecting
// the real Python nomadnet source via python3, not from a committed golden.
//
// On POSIX platforms filepath.Join uses "/" separators, matching Python's
// string concatenation. We compare via filepath.Clean so trailing-slash /
// separator differences don't cause false failures across platforms (the CI
// runners are Linux; darwin matches here too).
func TestPathsPythonParity(t *testing.T) {
	t.Parallel()

	const root = "/home/user/.nomadnetwork"
	const sampleHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const sampleMsg = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	want := pathsPython(t)
	p := New(root)

	cases := []struct {
		name string
		got  string
		key  string
	}{
		{"Root", p.Root, "config"}, // Root itself is the bare configdir.
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
				t.Fatalf("python reference missing key %q", c.key)
			}
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
