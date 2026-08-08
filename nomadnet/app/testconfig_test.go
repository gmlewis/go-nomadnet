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

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestNomadNetConfig writes a minimal private NomadNet config to dir/config
// before InitWithTransport, so a test controls its own settings and never
// depends on the default/user config.
//
// InitWithTransport mirrors the production initRNS path: it calls startNode,
// which auto-hosts a node when EnableNode is true. DefaultConfig sets
// EnableNode = yes and announce_at_start = yes (matching Python), so without a
// private config every test app would auto-start a node and fire a spurious
// announce-at-start — polluting announce-count/ordering assertions and creating
// background goroutines the test never asked for. This helper sets
// enable_node = no so tests start with no hosted node; tests that need one call
// startNode explicitly with their own settings (EnableNode/NodeName/...).
//
// Only enable_node is pinned. config.Load applies the INI on top of
// DefaultConfig, so every other key keeps its default — identical to the
// no-config-file case for all settings except enable_node. The test suite thus
// depends only on settings it explicitly sets, never on defaults that may
// change in the user's config (e.g. the EnableNode default flipped from no to
// yes, which previously broke the node-hosting tests).
func writeTestNomadNetConfig(t *testing.T, dir string) {
	t.Helper()
	const contents = "[node]\nenable_node = no\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644); err != nil {
		t.Fatalf("writeTestNomadNetConfig(%q): %v", dir, err)
	}
}

// writeTestRNSConfig creates an isolated RNS config dir for tests that drive
// the real Init() -> initRNS path, so they never touch the user's real
// ~/.reticulum. It returns the new dir path (a fresh temp dir, cleaned up via
// tempDir's t.Cleanup). share_instance = No makes the instance standalone
// (no shared-instance socket, no clobbering a real instance) and the empty
// [interfaces] section means no network I/O — these tests don't need a
// reachable peer, just an initialized RNS stack in isolation.
func writeTestRNSConfig(t *testing.T) string {
	t.Helper()
	dir := tempDir(t)
	const contents = "[reticulum]\n  share_instance = No\n\n[logging]\n  loglevel = 4\n\n[interfaces]\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o600); err != nil {
		t.Fatalf("writeTestRNSConfig: %v", err)
	}
	return dir
}
