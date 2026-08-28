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

// instance-lock.go implements single-instance enforcement for gonomadnet so a
// developer cannot accidentally run two gonomadnet instances (or gonomadnet
// alongside nomadnet) on the same Nomad Network config directory — the exact
// situation that left a stale gonomadnet as the RNS shared instance while a
// second one reported every interface Disconnected.
//
// This is a development-ergonomics enhancement, not a parity requirement.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// processArg is one running process's PID and full command line.
type processArg struct {
	PID  int
	Args string
}

// enforceSingleInstance refuses to start if a nomadnet (Python) process appears
// to be running, then acquires a per-config-dir exclusive lock so only one
// gonomadnet runs per config directory. It returns a release function for the
// acquired lock (callers may defer it; the OS also releases the lock on process
// exit or crash).
func enforceSingleInstance(configDir string) (func(), error) {
	if pids := runningNomadnetPIDs(); len(pids) > 0 {
		if os.Getenv("GONOMADNET_IGNORE_RUNNING_NOMADNET") == "" {
			return nil, fmt.Errorf("nomadnet appears to be running (PID %v)\n"+
				"stop it before starting gonomadnet, or set GONOMADNET_IGNORE_RUNNING_NOMADNET=1\n"+
				"to bypass this check (e.g. if this is a false positive)", pids[0])
		}
		log.Printf("gonomadnet: warning — nomadnet appears to be running (PID %v); "+
			"continuing because GONOMADNET_IGNORE_RUNNING_NOMADNET is set", pids[0])
	}

	lockPath := filepath.Join(configDir, "gonomadnet.lock")
	release, holderPID, err := acquireInstanceLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquire instance lock %v: %w", lockPath, err)
	}
	if release == nil {
		return nil, fmt.Errorf("gonomadnet is already running (PID %v) for config dir %v\n"+
			"stop the existing instance (or use a different --config dir) before starting a new one",
			holderPID, configDir)
	}
	return release, nil
}

// acquireInstanceLock exclusively locks lockPath. It returns a non-nil release
// function when the lock was acquired (the OS releases it on process exit/crash;
// calling release closes the file too). It returns release==nil plus the
// holder's PID when another gonomadnet already holds the lock.
func acquireInstanceLock(lockPath string) (func(), int, error) {
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, 0, err
	}
	locked, lerr := lockFileExclusive(f)
	if lerr != nil {
		_ = f.Close()
		return nil, 0, lerr
	}
	if !locked {
		holder := readPIDFromFile(f)
		_ = f.Close()
		return nil, holder, nil
	}
	if err := writePIDToFile(f, os.Getpid()); err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return func() { _ = f.Close() }, 0, nil
}

// runningNomadnetPIDs returns PIDs of processes that look like a running nomadnet
// (Python) instance, excluding gonomadnet and its launcher (whose command lines
// contain the substring "gonomadnet", which itself contains "nomadnet").
// Best-effort: on platforms without process enumeration it returns nothing.
func runningNomadnetPIDs() []int {
	procs, err := listProcessArgs()
	if err != nil {
		return nil
	}
	return nomadnetPIDsFromProcs(procs, os.Getpid())
}

// nomadnetPIDsFromProcs is the testable core of runningNomadnetPIDs: it selects
// PIDs whose command line looks like nomadnet (Python), excluding gonomadnet
// and the calling process itself.
func nomadnetPIDsFromProcs(procs []processArg, self int) []int {
	var pids []int
	for _, p := range procs {
		if p.PID == self {
			continue
		}
		argsLower := strings.ToLower(p.Args)
		if strings.Contains(argsLower, "gonomadnet") {
			continue
		}
		fields := strings.Fields(p.Args)
		if len(fields) == 0 {
			continue
		}
		exeLower := strings.ToLower(filepath.Base(fields[0]))
		// nomadnet installed as an executable script.
		if exeLower == "nomadnet" {
			pids = append(pids, p.PID)
			continue
		}
		// nomadnet run via Python: "python3 -m nomadnet" / "python3 nomadnet.py".
		if strings.HasPrefix(exeLower, "python") && strings.Contains(argsLower, "nomadnet") {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// readPIDFromFile reads a PID from the start of f.
func readPIDFromFile(f *os.File) int {
	_, _ = f.Seek(0, 0)
	buf := make([]byte, 32)
	n, _ := f.Read(buf)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	return pid
}

// writePIDToFile truncates f and writes pid into it.
func writePIDToFile(f *os.File, pid int) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	_, err := f.WriteString(strconv.Itoa(pid))
	return err
}
