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

package console

import (
	"testing"
	"time"
)

func TestSessionWaitsForAReapedChild(t *testing.T) {
	t.Parallel()

	// The ordinary ending: the child is killed and the kernel reaps it at once,
	// so the status is there and the wait returns the moment it arrives.
	exited := make(chan exitStatus, 1)
	exited <- exitStatus{code: 42}

	start := time.Now()
	status, reaped := waitForReap(exited, time.Second)
	elapsed := time.Since(start)

	if !reaped {
		t.Error("a child that was reaped was reported as abandoned")
	}
	if status.code != 42 {
		t.Errorf("the reap reported status %v, want 42", status.code)
	}
	if elapsed > time.Second/2 {
		t.Errorf("the wait took %v for a status that was already there", elapsed)
	}
}

func TestSessionAbandonsAReapThePlatformNeverFinishes(t *testing.T) {
	t.Parallel()

	// A killed child is not always reaped. macOS can leave a pty child in the
	// kernel's exit path — ps reports it as exiting, "E", with the session
	// leader flag, and never turns it into a zombie — and then wait4 does not
	// return at all. The session must still end: the app closed the socket, and
	// a host that hangs forever holds the socket and the process open on a
	// device that has no way to clean them up. Abandoning the reap is safe,
	// because closing the pty's master is what lets the kernel finish the child
	// off, and that close is what Run does next.
	never := make(chan exitStatus)

	start := time.Now()
	status, reaped := waitForReap(never, 50*time.Millisecond)
	elapsed := time.Since(start)

	if reaped {
		t.Error("a child that was never reaped was reported as reaped")
	}
	if status.code != exitCodeUnknown {
		t.Errorf("an abandoned reap reported status %v, want %v", status.code, exitCodeUnknown)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("the wait returned after %v, before its grace had passed", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the wait took %v, and must be bounded by its grace", elapsed)
	}
}
