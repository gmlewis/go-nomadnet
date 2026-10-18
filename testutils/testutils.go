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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// SkipShortIntegration skips a test when -short is in effect. The -short
// integration run (scripts/test-integration.sh -short) is a fast feedback loop
// where every test should run in well under 5 seconds by definition; any test
// that cannot meet that budget (a cross-process Python interop, a daemon
// build + boot, a full-package type check, ...) calls this at its top so the
// short run stays quick while the full unit and full integration runs still
// execute the test.
func SkipShortIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
}

// Source-of-truth (SOT) version pins. Parity tests compare Go output against the
// INSTALLED Python reference, so that reference must be the release the port was
// written against: comparing against a different one asserts the wrong behaviour
// in a way that reads as a port defect. Several nomadnet copies normally coexist
// on a development machine — a git checkout, a Homebrew site-packages copy, and
// the user-site copy that `nomadnet` actually imports — and the same is true of
// RNS and LXMF, so an ambiguous reference is the ordinary case rather than an
// exotic one. Bump these when the port is retargeted, in the same change that
// updates the affected parity tests.
const (
	SOTNomadnetVersion = "1.2.8"
	SOTRNSVersion      = "1.5.0"
	SOTLXMFVersion     = "1.1.1"
)

// allowSOTDriftEnv downgrades a source-of-truth version mismatch from a test
// failure to a logged warning when set to a non-empty value, for the deliberate
// case of comparing against a different release.
const allowSOTDriftEnv = "GONOMADNET_ALLOW_SOT_DRIFT"

// SOTVersionSet describes the versions reported by the probed Python reference.
type SOTVersionSet struct {
	Nomadnet string `json:"nomadnet"`
	RNS      string `json:"rns"`
	LXMF     string `json:"lxmf"`
}

// pinned reports whether these are the versions the port targets.
func (v SOTVersionSet) pinned() bool {
	return v.Nomadnet == SOTNomadnetVersion &&
		v.RNS == SOTRNSVersion &&
		v.LXMF == SOTLXMFVersion
}

// sotProbeScript prints one SOT:-prefixed JSON line describing the installed
// reference. nomadnet.ui.textui.Channels transitively pulls in urwid, so a
// successful import proves the full TUI reference stack is available.
//
// The SOT: prefix, and taking the LAST such line, exist because importing the
// reference can write to stdout — RNS logs there by default, which would corrupt
// a bare JSON document. RNS.loglevel is set to LOG_NONE first for the same
// reason. nomadnet has no top-level __version__, so the probe falls back to the
// _version submodule.
const sotProbeScript = `
import importlib, json
import RNS
RNS.loglevel = RNS.LOG_NONE
import nomadnet.ui.textui.Channels

def ver(name):
    mod = importlib.import_module(name)
    v = getattr(mod, "__version__", None)
    if v is None:
        v = getattr(importlib.import_module(name + "._version"), "__version__", None)
    return v

print("SOT:" + json.dumps({"nomadnet": ver("nomadnet"), "rns": ver("RNS"), "lxmf": ver("LXMF")}))
`

// pythonNomadnetCandidates are the interpreter candidates probed for an
// importable nomadnet reference. The system `python3` is tried first (so a
// venv/PATH python with nomadnet wins), then the Homebrew py3.14 interpreter
// that ships nomadnet+urwid on macOS. Adding more candidates here is safe; the
// first that can import the reference and report its versions is used.
var pythonNomadnetCandidates = []string{
	"python3",
	"/opt/homebrew/bin/python3",
	"python3.14",
	"python3.13",
}

// pythonNomadnetOnce lazily resolves the reference the first time a test
// actually needs it. Probing eagerly at package init would exec up to 4 python
// candidates in every test binary that imports testutils — including binaries
// whose parity tests all skip — so the probe is deferred until a parity helper
// is called.
var (
	pythonNomadnetOnce sync.Once
	pythonNomadnetExe  string
	pythonNomadnetVer  SOTVersionSet
)

// pythonNomadnet returns the probed reference interpreter and its versions, or
// ("", SOTVersionSet{}) when no candidate can import the reference.
func pythonNomadnet() (string, SOTVersionSet) {
	pythonNomadnetOnce.Do(func() {
		pythonNomadnetExe, pythonNomadnetVer = probePythonNomadnetExe()
	})
	return pythonNomadnetExe, pythonNomadnetVer
}

// probePythonNomadnetExe returns the first candidate interpreter that can import
// the nomadnet reference, together with the versions it reports.
func probePythonNomadnetExe() (string, SOTVersionSet) {
	for _, exe := range pythonNomadnetCandidates {
		out, err := exec.Command(exe, "-c", sotProbeScript).Output()
		if err != nil {
			continue
		}
		if v, ok := parseSOTLine(out); ok {
			return exe, v
		}
	}
	return "", SOTVersionSet{}
}

// parseSOTLine decodes the last SOT:-prefixed line of out, ignoring any other
// stdout the reference emitted while importing.
func parseSOTLine(out []byte) (SOTVersionSet, bool) {
	var v SOTVersionSet
	found := false
	for line := range strings.SplitSeq(string(out), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "SOT:")
		if !ok {
			continue
		}
		if err := json.Unmarshal([]byte(rest), &v); err != nil {
			continue
		}
		found = true
	}
	return v, found
}

// PythonNomadnetExe returns the path of the Python interpreter that can import
// the nomadnet reference, or "" if none is available. The probe runs at most
// once per test binary. Live cross-implementation parity tests in the tui
// package use this to exec the reference so they always reach the interpreter
// that actually has nomadnet installed (which may differ from the bare
// `python3` on PATH).
func PythonNomadnetExe() string {
	exe, _ := pythonNomadnet()
	return exe
}

// SOTVersions returns the versions of the probed Python reference, or the zero
// value when no reference is importable.
func SOTVersions() SOTVersionSet {
	_, v := pythonNomadnet()
	return v
}

// SkipIfNoPythonNomadnet skips the calling test when no probed Python
// interpreter can import the nomadnet reference. Call it at the top of any test
// that execs python3 to diff Go output against fresh nomadnet output.
//
// It also ENFORCES the source-of-truth version pin (SOTNomadnetVersion and
// friends): when a reference is importable but is not the pinned release, the
// test fails rather than skipping or quietly proceeding. A parity comparison
// against a different release asserts the wrong behaviour and is
// indistinguishable from a port defect, so this is deliberately a failure. The
// message names both versions; set GONOMADNET_ALLOW_SOT_DRIFT=1 to downgrade it
// to a warning when comparing against another release on purpose.
func SkipIfNoPythonNomadnet(t *testing.T) {
	t.Helper()
	exe, v := pythonNomadnet()
	if exe == "" {
		t.Skip("skipping live parity test: no python3 interpreter with nomadnet found")
	}
	assertSOTVersions(t, exe, v)
}

// assertSOTVersions fails t unless the probed reference is the pinned release.
func assertSOTVersions(t *testing.T, exe string, got SOTVersionSet) {
	t.Helper()
	if got.pinned() {
		return
	}
	msg := fmt.Sprintf(
		"python nomadnet reference at %v is nomadnet %v / RNS %v / lxmf %v, but this port targets nomadnet %v / RNS %v / lxmf %v",
		exe, got.Nomadnet, got.RNS, got.LXMF,
		SOTNomadnetVersion, SOTRNSVersion, SOTLXMFVersion)
	if os.Getenv(allowSOTDriftEnv) != "" {
		t.Logf("WARNING: %v (comparison allowed by %v)", msg, allowSOTDriftEnv)
		return
	}
	t.Fatalf("%v\nParity comparisons against a different release assert the wrong behaviour.\n"+
		"Install the pinned reference, or set %v=1 to compare against this one deliberately.",
		msg, allowSOTDriftEnv)
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
		t.Fatalf("python3 nomadnet reference failed: %v\nstderr:\n%v", err, stderr.String())
	}
	if err := json.Unmarshal(stdout, out); err != nil {
		t.Fatalf("decode python output: %v\nstderr:\n%v\nraw stdout:\n%s", err, stderr.String(), stdout)
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
		t.Fatalf("python3 nomadnet reference failed: %v\nstderr:\n%v", err, stderr.String())
	}
	return stdout
}
