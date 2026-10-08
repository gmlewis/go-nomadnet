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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// lockFileExclusive tries to acquire an exclusive, non-blocking flock on f. It
// returns (true, nil) if acquired, (false, nil) if already held by another
// process, or (false, err) on a real error. The lock is held until f is closed;
// the OS releases it automatically on process exit or crash, so no stale-lock
// file can survive a crash.
func lockFileExclusive(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

// procRoot is the procfs mount point used to enumerate running processes. It is
// a variable so tests can point the scan at a synthetic process tree.
var procRoot = "/proc"

// listProcessArgs enumerates running processes (PID + full command line), used
// by the best-effort nomadnet-detection check.
//
// Procfs is read directly in preference to executing ps(1). That is not merely
// an optimisation: on Android an application runs under a seccomp policy that
// terminates the process with SIGSYS on the faccessat2(2) call os/exec issues
// while resolving a bare program name, and Termux's $PATH contains no ps at all
// — so the ps route kills gonomadnet outright on that platform. Reading procfs
// is pure Go and needs no external program. Where the kernel hides other users'
// processes (as Android does for apps) enumeration returns only what is visible.
func listProcessArgs() ([]processArg, error) {
	procs, err := listProcessArgsFromProc(procRoot)
	if err == nil {
		return procs, nil
	}
	return listProcessArgsFromPS()
}

// listProcessArgsFromProc reads <root>/<pid>/cmdline for every numeric entry of
// the procfs root. Command lines are NUL-separated in procfs and are joined with
// spaces to match the ps output this replaces. Entries that cannot represent a
// process — non-numeric names, unreadable or empty cmdlines (kernel threads) —
// are skipped, as are processes that exit mid-scan.
func listProcessArgsFromProc(root string) ([]processArg, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var procs []processArg
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		fields := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		procs = append(procs, processArg{PID: pid, Args: strings.Join(fields, " ")})
	}
	return procs, nil
}

// listProcessArgsFromPS enumerates processes by executing ps(1). It is the
// fallback for platforms that mount no procfs (macOS, the BSDs), where spawning
// ps is safe.
func listProcessArgsFromPS() ([]processArg, error) {
	out, err := exec.Command("ps", "-Ao", "pid,args").Output()
	if err != nil {
		return nil, err
	}
	var procs []processArg
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		procs = append(procs, processArg{PID: pid, Args: strings.Join(fields[1:], " ")})
	}
	return procs, nil
}
