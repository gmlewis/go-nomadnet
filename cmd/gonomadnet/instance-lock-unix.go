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

//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
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

// listProcessArgs enumerates running processes (PID + full command line) via
// ps, used by the best-effort nomadnet-detection check.
func listProcessArgs() ([]processArg, error) {
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
