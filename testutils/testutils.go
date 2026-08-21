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

// Package testutils provides shared helper functions for tests across this
// repository. It mirrors the testutils package in go-reticulum so the two
// repositories share a common convention for guarding slow tests.
package testutils

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

// SkipShortIntegration skips a test when -short is in effect. The -short
// integration run (scripts/test-integration.sh -short) is a fast feedback loop
// where every test should run in well under 5 seconds by definition; any test
// that cannot meet that budget (a full-package type check, a live network
// round-trip, a cross-process tmux harness, ...) calls this at its top so the
// short run stays quick while the full unit and full integration runs still
// execute the test.
func SkipShortIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
}

// pythonNomadnetCandidates are the interpreter candidates probed for an
// importable nomadnet reference. The system `python3` is tried first (so a
// venv/PATH python with nomadnet wins), then the Homebrew py3.14 interpreter
// that ships nomadnet+urwid on macOS (see memory: python-nomadnet-launch-via-
// homebrew-py314). Adding more candidates here is safe; the first that can
// `import nomadnet.ui.textui.Channels` is used.
var pythonNomadnetCandidates = []string{
	"python3",
	"/opt/homebrew/bin/python3",
	"python3.14",
	"python3.13",
}

// nomadnetImportScript is the import probe; nomadnet.ui.textui.Channels
// transitively pulls in urwid, so a successful import proves the full TUI
// reference stack is available.
const nomadnetImportScript = "import nomadnet.ui.textui.Channels"

// pythonNomadnetExe is the path of the first probed interpreter that can
// import the nomadnet TUI reference, or "" if none could. It is resolved once
// per test binary so the live parity tests gate on a real, cached answer.
var pythonNomadnetExe = func() string {
	for _, exe := range pythonNomadnetCandidates {
		cmd := exec.Command(exe, "-c", nomadnetImportScript)
		if err := cmd.Run(); err == nil {
			return exe
		}
	}
	return ""
}()

// PythonNomadnetExe returns the path of the Python interpreter that can import
// the nomadnet reference, or "" if none is available. Live cross-implementation
// parity tests in the tui package use this to exec the reference so they always
// reach the interpreter that actually has nomadnet installed (which may differ
// from the bare `python3` on PATH).
func PythonNomadnetExe() string { return pythonNomadnetExe }

// SkipIfNoPythonNomadnet skips the calling test when no probed Python
// interpreter can import the nomadnet reference. Call it at the top of any test
// that execs python3 to diff Go output against fresh nomadnet output.
func SkipIfNoPythonNomadnet(t *testing.T) {
	t.Helper()
	if pythonNomadnetExe == "" {
		t.Skip("skipping live parity test: no python3 interpreter with nomadnet found")
	}
}

// RunPythonNomadnet executes a live cross-implementation parity check against
// the installed Python nomadnet reference. It marshals stdin as JSON, runs the
// given Python script (which reads JSON from stdin and writes JSON to stdout)
// via the probed nomadnet interpreter, and decodes stdout into out.
//
// The script is expected to import the real nomadnet module(s) and call the
// reference function under test on each input, so the expected output is
// derived FRESH from the current Python source on every run rather than from a
// stale hardcoded literal or captured golden file. The Go test owns the input
// battery; Python owns the reference behavior.
//
// The test is skipped (not failed) when the Python nomadnet reference is not
// importable, via SkipIfNoPythonNomadnet.
func RunPythonNomadnet(t *testing.T, stdin any, script string, out any) {
	t.Helper()
	SkipIfNoPythonNomadnet(t)

	in, err := json.Marshal(stdin)
	if err != nil {
		t.Fatalf("marshal python stdin: %v", err)
	}

	cmd := exec.Command(PythonNomadnetExe(), "-c", script)
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

// RunPythonNomadnetRaw is like RunPythonNomadnet but returns the raw stdout
// bytes of the Python script (which may emit arbitrary bytes, e.g. msgpack
// frames). The script still receives JSON on stdin. Use this for byte-parity
// checks where the Python reference writes a binary on-disk format that must be
// compared byte-for-byte. The test is skipped (not failed) when the Python
// nomadnet reference is not importable.
func RunPythonNomadnetRaw(t *testing.T, stdin any, script string) []byte {
	t.Helper()
	SkipIfNoPythonNomadnet(t)

	in, err := json.Marshal(stdin)
	if err != nil {
		t.Fatalf("marshal python stdin: %v", err)
	}

	cmd := exec.Command(PythonNomadnetExe(), "-c", script)
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("python3 nomadnet reference failed: %v\nstderr:\n%s", err, stderr.String())
	}
	return stdout
}
