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

//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows error codes returned by LockFileEx when the lock cannot be taken
// immediately (not exported as named constants by x/sys/windows, so compare by
// errno value). ERROR_LOCK_VIOLATION = 33, ERROR_SHARING_VIOLATION = 32.
const (
	winErrLockViolation     = syscall.Errno(33)
	winErrSharingViolation  = syscall.Errno(32)
	lockfileExclusiveLock   = 0x02
	lockfileFailImmediately = 0x01
)

// lockFileExclusive tries to acquire an exclusive, immediate byte-range lock via
// LockFileEx. (true, nil) if acquired; (false, nil) if already held; (false,
// err) on a real error. The lock is released when f is closed (the OS releases
// it on process exit/crash).
func lockFileExclusive(f *os.File) (bool, error) {
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, &ol)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, winErrLockViolation) || errors.Is(err, winErrSharingViolation) {
		return false, nil
	}
	return false, err
}

// listProcessArgs is a no-op on Windows: nomadnet detection is a best-effort
// convenience for Unix development hosts (where the developer runs Python
// nomadnet alongside gonomadnet). The gonomadnet singleton lock above still
// guards against two gonomadnet instances on Windows.
func listProcessArgs() ([]processArg, error) {
	return nil, nil
}
