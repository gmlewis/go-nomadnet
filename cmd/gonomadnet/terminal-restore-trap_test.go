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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/tui"
)

// TestLauncherTrapEmitsTheClientsOwnRestoreSequence keeps gonomadnet.sh's EXIT
// trap honest.
//
// The trap is the client's backstop for the two deaths that run no code of the
// client's own: a panic in a goroutine the client does not wrap (go-reticulum's
// transports and timers) and a SIGKILL. It therefore carries a copy of the
// sequence tui.RestoreTerminal writes, and the two lists must be identical,
// code for code; a copy that has fallen behind leaves the terminal in whatever
// mode was added since.
//
// The launcher is the only place in the tree where this sequence is written by
// anything other than the client, which is why the check is here and not in the
// tui package.
func TestLauncherTrapEmitsTheClientsOwnRestoreSequence(t *testing.T) {
	t.Parallel()

	script := readLauncher(t)
	// The launcher writes the codes in bash's ANSI-C form ($'\033…'), which is
	// the same bytes with the escape spelled out as an octal escape.
	want := strings.ReplaceAll(tui.TerminalRestoreSeq(), "\x1b", `\033`)
	if !strings.Contains(script, want) {
		t.Errorf("gonomadnet.sh does not write the client's own restore sequence.\n"+
			"the trap must contain this, verbatim and contiguous:\n\t%v\n"+
			"the launcher's copy is the backstop for a SIGKILL or for a panic in a goroutine the "+
			"client cannot wrap, and a terminal left by one of those would keep reporting the "+
			"mouse and stay in raw mode", want)
	}
}

// TestLauncherTrapRestoresTheTerminalItRecorded checks that the trap puts back
// the settings the script recorded before the client started, rather than
// imposing a generic `stty sane` on a terminal that a clean exit has already
// restored — which would silently discard settings the user had set
// themselves.
func TestLauncherTrapRestoresTheTerminalItRecorded(t *testing.T) {
	t.Parallel()

	script := launcherCode(readLauncher(t))
	for _, want := range []string{
		`stty -g </dev/tty`,  // the settings, recorded before the client runs
		`stty "$saved_stty"`, // and put back by the trap
		`>/dev/tty`,          // on the terminal the client draws into, not on stdout
		`trap 'rc=$?; restore_terminal; exit $rc' EXIT`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("gonomadnet.sh no longer contains %q; the trap's terminal restore has changed shape", want)
		}
	}
	if strings.Contains(script, "stty sane") {
		t.Error("gonomadnet.sh falls back to `stty sane`, which replaces the user's own terminal " +
			"settings on every exit rather than restoring the ones the client was given")
	}
}

// launcherCode returns the launcher's executable lines, with comments removed:
// the checks are about what the script runs, and the prose around it is free to
// name the very things the script must not do.
func launcherCode(script string) string {
	var code strings.Builder
	for line := range strings.SplitSeq(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	return code.String()
}

// readLauncher returns the launcher script at the root of the repository.
func readLauncher(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %v", dir)
		}
		dir = parent
	}
	path := filepath.Join(dir, "gonomadnet.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %v: %v", path, err)
	}
	return string(data)
}
