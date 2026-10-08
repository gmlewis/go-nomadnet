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

//go:build !windows && !embedded && !pocket_communicator && !pocket_hub

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProcEntry creates a synthetic procfs entry <root>/<pid>/cmdline holding
// args NUL-separated and NUL-terminated, exactly as the kernel reports them.
func writeProcEntry(t *testing.T, root string, pid string, args ...string) {
	t.Helper()
	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %v: %v", dir, err)
	}
	var raw []byte
	for _, a := range args {
		raw = append(raw, a...)
		raw = append(raw, 0)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), raw, 0o644); err != nil {
		t.Fatalf("write cmdline: %v", err)
	}
}

// TestListProcessArgsFromProcParsesCmdline verifies synthetic procfs entries are
// parsed into PID + space-joined command line, that NUL separators and the
// trailing NUL are handled, and that entries which cannot represent a process
// (non-numeric names, missing or empty cmdline) are skipped rather than
// reported as empty processes.
func TestListProcessArgsFromProcParsesCmdline(t *testing.T) {
	t.Parallel()

	root := tempDir(t)
	writeProcEntry(t, root, "111", "python3", "-m", "nomadnet")
	writeProcEntry(t, root, "222", "gonomadnet", "-t")
	writeProcEntry(t, root, "333") // kernel thread: empty cmdline

	if err := os.MkdirAll(filepath.Join(root, "notapid"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "444"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "444", "cmdline"), nil, 0o644); err != nil {
		t.Fatalf("write empty cmdline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "regularfile"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}

	procs, err := listProcessArgsFromProc(root)
	if err != nil {
		t.Fatalf("listProcessArgsFromProc: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("got %v processes (%+v), want 2", len(procs), procs)
	}
	byPID := make(map[int]string, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p.Args
	}
	if got := byPID[111]; got != "python3 -m nomadnet" {
		t.Errorf("PID 111 args = %q, want %q", got, "python3 -m nomadnet")
	}
	if got := byPID[222]; got != "gonomadnet -t" {
		t.Errorf("PID 222 args = %q, want %q", got, "gonomadnet -t")
	}
}

// TestListProcessArgsFromProcMissingRoot verifies an absent procfs mount is
// reported as an error so callers can fall back to another enumeration method
// rather than silently believing no processes exist.
func TestListProcessArgsFromProcMissingRoot(t *testing.T) {
	t.Parallel()

	if _, err := listProcessArgsFromProc(filepath.Join(tempDir(t), "absent")); err == nil {
		t.Fatal("expected an error for a missing procfs root, got nil")
	}
}

// TestListProcessArgsPrefersProcfs verifies listProcessArgs reads procfs when it
// is available and therefore never executes ps(1). Executing ps is fatal on
// Android, where an application's seccomp policy kills the process with SIGSYS
// on the faccessat2(2) call that os/exec issues while resolving a bare program
// name, and where Termux's $PATH holds no ps at all.
func TestListProcessArgsPrefersProcfs(t *testing.T) {
	root := tempDir(t)
	writeProcEntry(t, root, "111", "python3", "-m", "nomadnet")

	old := procRoot
	procRoot = root
	t.Cleanup(func() { procRoot = old })

	procs, err := listProcessArgs()
	if err != nil {
		t.Fatalf("listProcessArgs: %v", err)
	}
	// The synthetic tree is the only possible source: a ps(1) fallback would
	// have returned this machine's real process table instead.
	if len(procs) != 1 || procs[0].PID != 111 || procs[0].Args != "python3 -m nomadnet" {
		t.Fatalf("got %+v, want exactly the synthetic procfs entry", procs)
	}
}

// TestRunningNomadnetPIDsFromProcfs verifies the procfs-derived process list
// feeds the existing nomadnet-detection rules: a Python nomadnet is detected,
// while gonomadnet and the calling process are not.
func TestRunningNomadnetPIDsFromProcfs(t *testing.T) {
	t.Parallel()

	root := tempDir(t)
	writeProcEntry(t, root, "111", "python3", "-m", "nomadnet")
	writeProcEntry(t, root, "222", "/data/data/com.termux/files/home/gonomadnet", "-t")
	writeProcEntry(t, root, "333", "/usr/bin/nomadnet")

	procs, err := listProcessArgsFromProc(root)
	if err != nil {
		t.Fatalf("listProcessArgsFromProc: %v", err)
	}
	pids := nomadnetPIDsFromProcs(procs, 222)
	if len(pids) != 2 {
		t.Fatalf("pids = %v, want [111 333]", pids)
	}
	seen := map[int]bool{pids[0]: true, pids[1]: true}
	if !seen[111] || !seen[333] {
		t.Fatalf("pids = %v, want [111 333]", pids)
	}
}
