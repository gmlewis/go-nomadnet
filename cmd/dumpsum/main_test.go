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

package main

import (
	"strings"
	"testing"
)

// miniDump mirrors the shape of a real GOTRACEBACK=all SIGQUIT capture: a
// preamble, goroutine blocks in several states with runtime and user frames,
// creation-site lines, and a trailing register dump.
const miniDump = `SIGQUIT: quit
PC=0x411b2e m=0 sigcode=0

goroutine 0 gp=0x123a680 m=0 mp=0x123b860 [idle]:
internal/runtime/syscall/linux.Syscall6()
	/usr/local/go/src/internal/runtime/syscall/linux/asm_linux_amd64.s:36 +0xe
runtime.netpoll(0x0)
	/usr/local/go/src/runtime/netpoll_epoll.go:119 +0xd3

goroutine 1 gp=0x1 m=nil [select]:
runtime.gopark(0x0?, 0x0?, 0x0?, 0x0?, 0x0?)
	/usr/local/go/src/runtime/proc.go:474 +0xca
github.com/rivo/tview.(*Application).Run(0x12776203ca50)
	/home/glenn/go/pkg/mod/tview/application.go:408 +0x365
main.runTextUI({0x127761fe85e0, 0x19}, {0x0, 0x0})
	/home/glenn/go/src/github.com/gmlewis/go-nomadnet/cmd/gonomadnet/textui.go:165 +0x594

goroutine 25 gp=0x2 m=nil [select]:
runtime.gopark(0x0?, 0x0?, 0x0?, 0x0?, 0x0?)
	/usr/local/go/src/runtime/proc.go:474 +0xca
github.com/gmlewis/go-nomadnet/tui.(*App).drainUpdates(0x1277620ba3f0)
	/home/glenn/go/src/github.com/gmlewis/go-nomadnet/tui/app.go:249 +0x118
github.com/gmlewis/go-nomadnet/tui.NewApp.gowrap1()
	/home/glenn/go/src/github.com/gmlewis/go-nomadnet/tui/app.go:113 +0x17
created by github.com/gmlewis/go-nomadnet/tui.NewApp in goroutine 1
	/home/glenn/go/src/github.com/gmlewis/go-nomadnet/tui/app.go:113 +0x32b

goroutine 31 gp=0x3 m=nil [IO wait]:
internal/poll.runtime_pollWait(0x720204943400, 0x72)
	/usr/local/go/src/runtime/netpoll.go:351 +0x85
github.com/gmlewis/go-reticulum/rns/interfaces.(*TCPClientInterface).readLoop(0x1277624d8410)
	/home/glenn/go/src/github.com/gmlewis/go-reticulum/rns/interfaces/tcp.go:273 +0x1d5
created by github.com/gmlewis/go-reticulum/rns/interfaces.newTCPClientInterface in goroutine 24
	/home/glenn/go/src/github.com/gmlewis/go-reticulum/rns/interfaces/tcp.go:199 +0x2b8

goroutine 615571 gp=0x4 m=nil [sleep]:
time.Sleep(0x12a05f200)
	/usr/local/go/src/runtime/time.go:368 +0x165
github.com/gmlewis/go-reticulum/rns/interfaces.(*TCPClientInterface).reconnectLoop(0x1277624d80d0)
	/home/glenn/go/src/github.com/gmlewis/go-reticulum/rns/interfaces/tcp.go:234 +0x45
created by github.com/gmlewis/go-reticulum/rns/interfaces.(*TCPClientInterface).failConn in goroutine 615570
	/home/glenn/go/src/github.com/gmlewis/go-reticulum/rns/interfaces/tcp.go:458 +0x40b

goroutine 32 gp=0x5 m=20 mp=0x127762881808 [syscall]:
syscall.Syscall(0x0, 0x11, 0x127762131800, 0x200)
	/usr/local/go/src/syscall/syscall_linux.go:74 +0x25
github.com/gmlewis/go-reticulum/rns/interfaces.(*RNodeInterface).readLoopOnce(0x127761ffa708)
	/home/glenn/go/src/github.com/gmlewis/go-reticulum/rns/interfaces/rnode-unix.go:455 +0x104

rax    0xfffffffffffffffc
rbx    0x5
rip    0x411b2e
`

func parseString(t *testing.T, dump string) []goroutine {
	t.Helper()
	gs, err := parseDump(strings.NewReader(dump))
	if err != nil {
		t.Fatalf("parseDump: %v", err)
	}
	return gs
}

func TestParseDumpCountAndIDs(t *testing.T) {
	t.Parallel()
	gs := parseString(t, miniDump)
	if len(gs) != 6 {
		t.Fatalf("parsed %d goroutines, want 6", len(gs))
	}
	wantIDs := map[int]bool{0: true, 1: true, 25: true, 31: true, 615571: true, 32: true}
	for _, g := range gs {
		if !wantIDs[g.id] {
			t.Errorf("unexpected goroutine id %d (header %q)", g.id, g.header)
		}
		delete(wantIDs, g.id)
	}
	if len(wantIDs) != 0 {
		t.Errorf("missing goroutine ids: %v", wantIDs)
	}
}

func TestSummarizeStatesAndMaxID(t *testing.T) {
	t.Parallel()
	s := summarize(parseString(t, miniDump))
	if s.Total != 6 {
		t.Errorf("Total = %d, want 6", s.Total)
	}
	if s.MaxID != 615571 {
		t.Errorf("MaxID = %d, want 615571", s.MaxID)
	}
	want := map[string]int{"idle": 1, "select": 2, "IO wait": 1, "sleep": 1, "syscall": 1}
	for state, n := range want {
		if got := s.States[state]; got != n {
			t.Errorf("States[%q] = %d, want %d", state, got, n)
		}
	}
}

func TestSummarizeCalloutGroups(t *testing.T) {
	t.Parallel()
	s := summarize(parseString(t, miniDump))
	// The fixture has no running or runnable goroutines: the quiet-freeze case.
	if len(s.Running) != 0 || len(s.Runnable) != 0 {
		t.Errorf("expected no running/runnable goroutines, got %d/%d", len(s.Running), len(s.Runnable))
	}
	if len(s.Syscalls) != 1 || s.Syscalls[0].id != 32 {
		t.Errorf("Syscalls = %+v, want exactly goroutine 32", s.Syscalls)
	}
	if role := s.Syscalls[0].role(); !strings.Contains(role, "RNodeInterface") {
		t.Errorf("syscall role = %q, want RNodeInterface read frame", role)
	}
}

func TestCreatedByHistogram(t *testing.T) {
	t.Parallel()
	s := summarize(parseString(t, miniDump))
	if got := s.CreatedBy["created by github.com/gmlewis/go-reticulum/rns/interfaces.(*TCPClientInterface).failConn in goroutine 615570"]; got != 1 {
		t.Errorf("failConn creation site count = %d, want 1 (got %v)", got, s.CreatedBy)
	}
}

func TestRoleFallsBackToCreationSite(t *testing.T) {
	t.Parallel()
	gs := parseString(t, miniDump)
	var sleeper goroutine
	for _, g := range gs {
		if g.state == "sleep" {
			sleeper = g
		}
	}
	if !strings.Contains(sleeper.role(), "reconnectLoop") {
		t.Errorf("role = %q, want reconnectLoop user frame", sleeper.role())
	}
}

func TestFrameFuncSkipsFileLines(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"\t/usr/local/go/src/runtime/proc.go:474 +0xca", "", "/usr/local/go/x"} {
		if fn := frameFunc(line); fn != "" {
			t.Errorf("frameFunc(%q) = %q, want empty", line, fn)
		}
	}
	if fn := frameFunc("github.com/x/y.(*T).Foo(0x1, ...)"); fn != "github.com/x/y.(*T).Foo" {
		t.Errorf("frameFunc symbol = %q", fn)
	}
}
