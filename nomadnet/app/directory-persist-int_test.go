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

//go:build integration

package app

import (
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestDirectoryPersistsAcrossShutdown verifies the graceful-shutdown directory
// persistence that mirrors Python's NomadNetworkApp exit_handler
// (self.directory.save_to_disk, NomadNetworkApp.py:42) and the load-at-boot in
// Directory.__init__ (Directory.py:82 self.load_from_disk). The Go port must:
//
//   - on Shutdown, write the in-memory directory to <storage>/directory
//     (directory.SaveToDisk, msgpack — already Python-format compatible);
//   - on Init/InitWithTransport, load that file back so remembered peers,
//     trust levels, and the announce stream survive a restart.
//
// Without save-on-shutdown the directory is ephemeral (every restart loses all
// known peers); without load-at-boot a saved file is ignored. This test pins
// the round-trip end-to-end through the App lifecycle.
func TestDirectoryPersistsAcrossShutdown(t *testing.T) {
	dir := testutils.TempDir(t, "nomadnet-dir-persist")

	// --- Boot app #1, remember a trusted peer, then shut down. ---
	ts, tsCleanup := newStartedTSApp(t, dir)
	defer tsCleanup()

	app1 := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(ts.Identity()))
	if err := app1.InitWithTransport(ts, ts.Identity()); err != nil {
		t.Fatalf("InitWithTransport #1: %v", err)
	}

	peerHash := make([]byte, 32)
	for i := range peerHash {
		peerHash[i] = byte(i + 1)
	}
	app1.Dir.Remember(&directory.Entry{
		SourceHash:  peerHash,
		DisplayName: "TestPeer",
		TrustLevel:  directory.TrustTrusted,
		HostsNode:   true,
	})

	// Sanity: the entry is in memory before shutdown.
	if e := app1.Dir.Find(peerHash); e == nil || e.DisplayName != "TestPeer" {
		t.Fatalf("pre-shutdown Find = %v, want TestPeer", e)
	}

	// Graceful shutdown must persist the directory to a.DirectoryPath.
	dirPath := app1.DirectoryPath
	app1.Shutdown()

	// --- Boot app #2 from the same storage dir; it must load the entry. ---
	ts2, ts2Cleanup := newStartedTSApp(t, dir)
	defer ts2Cleanup()

	app2 := NewAppWithTransport(dir, WithTransport(ts2), WithIdentity(ts2.Identity()))
	if err := app2.InitWithTransport(ts2, ts2.Identity()); err != nil {
		t.Fatalf("InitWithTransport #2: %v", err)
	}
	defer app2.Shutdown()

	got := app2.Dir.Find(peerHash)
	if got == nil {
		t.Fatalf("app2 did not load the directory entry from %s", dirPath)
	}
	if got.DisplayName != "TestPeer" {
		t.Errorf("loaded DisplayName = %q, want %q", got.DisplayName, "TestPeer")
	}
	if got.TrustLevel != directory.TrustTrusted {
		t.Errorf("loaded TrustLevel = %d, want %d (Trusted)", got.TrustLevel, directory.TrustTrusted)
	}
	if !got.HostsNode {
		t.Errorf("loaded HostsNode = false, want true")
	}
}
