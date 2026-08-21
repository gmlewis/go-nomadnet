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

package tui

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// runPythonNomadnet executes a live cross-implementation parity check against
// the installed Python nomadnet reference. It marshals stdin as JSON, runs the
// given Python script (which reads JSON from stdin and writes JSON to stdout),
// and decodes stdout into out.
//
// The script is expected to import the real nomadnet module(s) — e.g.
// `import nomadnet.ui.textui.Channels as C` — and call the reference function
// under test on each input, so the expected output is derived FRESH from the
// current Python source on every run rather than from a stale hardcoded
// literal or captured golden file. The Go test owns the input battery; Python
// owns the reference behavior.
//
// The test is skipped (not failed) when the Python nomadnet reference is not
// importable, via testutils.SkipIfNoPythonNomadnet.
func runPythonNomadnet(t *testing.T, stdin any, script string, out any) {
	t.Helper()
	testutils.SkipIfNoPythonNomadnet(t)

	in, err := json.Marshal(stdin)
	if err != nil {
		t.Fatalf("marshal python stdin: %v", err)
	}

	cmd := exec.Command(testutils.PythonNomadnetExe(), "-c", script)
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("python3 nomadnet reference failed: %v\nstderr:\n%s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		t.Fatalf("decode python output: %v\nstderr:\n%s\nraw stdout:\n%s", err, stderr.String(), stdout)
	}
}

// runPythonNomadnetScript runs a standalone Python script FILE (relative to the
// test's working directory, i.e. the package dir) against the installed Python
// nomadnet reference and decodes its JSON stdout into out. Unlike
// runPythonNomadnet it passes no stdin — the script generates its own cases
// (the stubbed-urwid golden capture scripts in tooling/tui-parity/ stub gi and
// urwid before importing nomadnet, so they must be run as files rather than
// inlined). The test is skipped (not failed) when the Python reference is not
// importable or the script file cannot be read.
func runPythonNomadnetScript(t *testing.T, scriptPath string, out any) {
	t.Helper()
	testutils.SkipIfNoPythonNomadnet(t)

	if _, err := os.Stat(scriptPath); err != nil {
		t.Skipf("script %s not accessible from package cwd: %v", scriptPath, err)
	}

	cmd := exec.Command(testutils.PythonNomadnetExe(), scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("python3 nomadnet script %s failed: %v\nstderr:\n%s", scriptPath, err, stderr.String())
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		t.Fatalf("decode python output: %v\nstderr:\n%s\nraw stdout:\n%s", err, stderr.String(), stdout)
	}
}
