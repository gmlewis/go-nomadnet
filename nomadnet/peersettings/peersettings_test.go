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

package peersettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	t.Parallel()

	s := DefaultSettings(21600)

	if s.DisplayName != "Anonymous Peer" {
		t.Errorf("DisplayName = %q, want %q", s.DisplayName, "Anonymous Peer")
	}
	if s.AnnounceInterval != 21600 {
		t.Errorf("AnnounceInterval = %v, want 21600", s.AnnounceInterval)
	}
	if s.LastAnnounce != nil {
		t.Errorf("LastAnnounce = %v, want nil", s.LastAnnounce)
	}
	if s.PropagationNode != nil {
		t.Errorf("PropagationNode = %v, want nil", s.PropagationNode)
	}
	if s.LastLXMFSync != 0 {
		t.Errorf("LastLXMFSync = %v, want 0", s.LastLXMFSync)
	}
	if s.NodeConnects != 0 {
		t.Errorf("NodeConnects = %v, want 0", s.NodeConnects)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "peersettings")

	s, err := Load(path, 21600)
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if s.DisplayName != "Anonymous Peer" {
		t.Errorf("DisplayName = %q, want %q", s.DisplayName, "Anonymous Peer")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "peersettings")

	s := DefaultSettings(21600)
	s.DisplayName = "My Peer"
	s.NodeConnects = 42
	s.ServedPageRequests = 10
	s.ServedFileRequests = 5

	if err := Save(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path, 21600)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.DisplayName != "My Peer" {
		t.Errorf("DisplayName = %q, want %q", loaded.DisplayName, "My Peer")
	}
	if loaded.NodeConnects != 42 {
		t.Errorf("NodeConnects = %v, want 42", loaded.NodeConnects)
	}
	if loaded.ServedPageRequests != 10 {
		t.Errorf("ServedPageRequests = %v, want 10", loaded.ServedPageRequests)
	}
	if loaded.ServedFileRequests != 5 {
		t.Errorf("ServedFileRequests = %v, want 5", loaded.ServedFileRequests)
	}
}

func TestSaveAtomicWrite(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "peersettings")

	s := DefaultSettings(21600)
	if err := Save(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify no .tmp file remains
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tmp file still exists after save")
	}

	// Verify the main file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("main file not created: %v", err)
	}
}

func TestSaveCorruptFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "peersettings")

	// Write corrupt data
	if err := os.WriteFile(path, []byte("not valid msgpack"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path, 21600)
	if err == nil {
		t.Error("Load corrupt file should return error")
	}
}

func TestMsgpackCompatibility(t *testing.T) {
	t.Parallel()

	// Verify that Save produces msgpack that Load can decode (a round-trip
	// through the rns/msgpack codec the production code uses, rather than a
	// third-party reflection codec).
	s := DefaultSettings(21600)
	s.DisplayName = "Test Peer"

	dir := tempDir(t)
	path := filepath.Join(dir, "peersettings")
	if err := Save(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	decoded, err := Load(path, 21600)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if decoded.DisplayName != "Test Peer" {
		t.Errorf("DisplayName = %q, want %q", decoded.DisplayName, "Test Peer")
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-peersettings-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
