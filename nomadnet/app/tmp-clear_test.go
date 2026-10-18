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

	"github.com/gmlewis/go-reticulum/rns"
)

// TestStartupClearsTmpDirOnANonFirstRun pins the startup tmp-dir clear. Python's
// __init__ does `if not os.path.isdir(self.attachmentpath): os.makedirs(...) else:
// self.clear_tmp_dir()` (NomadNetworkApp.py:193-198) — the clear is gated on the
// attachments directory ALREADY existing, which is its "not a first run" test. The
// port implemented ClearTmpDir but never called it, so leftovers from an
// interrupted print or download accumulated in storage/tmp forever.
func TestStartupClearsTmpDirOnANonFirstRun(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	// An existing attachments directory marks this as a later run, and a stale
	// scratch file marks what a previous interrupted operation left behind.
	attachments := filepath.Join(dir, "storage", "attachments")
	tmpFiles := filepath.Join(dir, "storage", "tmp")
	for _, d := range []string{attachments, tmpFiles} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stale := filepath.Join(tmpFiles, "LXM_leftover")
	if err := os.WriteFile(stale, []byte("interrupted"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale tmp file %v survived startup, stat err = %v", stale, err)
	}
	if _, err := os.Stat(tmpFiles); err != nil {
		t.Errorf("the tmp directory itself must still exist: %v", err)
	}
}

// TestStartupKeepsTmpDirOnAFirstRun pins the other side of the gate: with no
// attachments directory the run is a first one, and Python's else-branch is not
// taken. The observable is the same (a fresh tmp dir is empty), so this asserts the
// gate exists rather than a different byte — a non-empty tmp dir with no attachments
// directory must be left alone.
func TestStartupKeepsTmpDirOnAFirstRun(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	// tmp exists and holds a file, but attachments does not: a first run.
	tmpFiles := filepath.Join(dir, "storage", "tmp")
	if err := os.MkdirAll(tmpFiles, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(tmpFiles, "pre_existing")
	if err := os.WriteFile(kept, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)

	if _, err := os.Stat(kept); err != nil {
		t.Errorf("a first run must not clear tmp, but %v is gone: %v", kept, err)
	}
}
