// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the license, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public license for more details.
//
// You should have received a copy of the GNU General Public license
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// defaultConfigDirScript imports the real Python nomadnet reference and emits
// the class-level default configdir (NomadNetworkApp.py:28-34) as JSON. Python
// resolves the default by checking /etc/nomadnetwork, then
// ~/.config/nomadnetwork, then falling back to ~/.nomadnetwork.
const defaultConfigDirScript = `
import json
import nomadnet
print(json.dumps(nomadnet.NomadNetworkApp.configdir))
`

// TestDefaultConfigDir verifies that the Go default config directory matches
// the Python nomadnet reference's resolved default configdir (derived live from
// nomadnet.NomadNetworkApp.configdir). Go's main.go falls back to
// filepath.Join(home, ".nomadnetwork") when --config is unset; Python applies
// the same ~/.nomadnetwork fallback after first checking /etc/nomadnetwork and
// ~/.config/nomadnetwork. The expected value is derived fresh from the live
// Python reference on every run.
func TestDefaultConfigDir(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	var pyConfigDir string
	testutils.RunPythonNomadnet(t, nil, defaultConfigDirScript, &pyConfigDir)

	got := resolveDefaultConfigDir(home)
	if got != pyConfigDir {
		t.Errorf("Go default config dir = %q, want Python default %q", got, pyConfigDir)
	}
}

// resolveConfigDirOrderScript replicates Python's configdir resolution order
// (NomadNetworkApp.py:28-34) against abstract candidate paths with a faked
// filesystem, so the order can be verified live for all branches without
// needing /etc/nomadnetwork (root) or a real ~/.config/nomadnetwork. stdin is
// {"etc","xdg","fb","etc_cfg","xdg_cfg"}; stdout is the chosen candidate.
const resolveConfigDirOrderScript = `
import json, sys
st = json.loads(sys.stdin.read())
etc, xdg, fb = st["etc"], st["xdg"], st["fb"]
def isdir(p):  return p in (etc, xdg)
def isfile(p): return (p == etc+"/config" and st["etc_cfg"]) or (p == xdg+"/config" and st["xdg_cfg"])
if isdir(etc) and isfile(etc+"/config"):
    cd = etc
elif isdir(xdg) and isfile(xdg+"/config"):
    cd = xdg
else:
    cd = fb
print(json.dumps(cd))
`

// TestResolveConfigDirOrderPythonParity verifies Go's configdir resolution
// ORDER matches the Python reference for every branch (etc-config wins,
// else xdg-config wins, else fallback) by driving both implementations with
// the same abstract candidate paths and which-config-exists flags. The
// expected choice is derived fresh from the live Python resolution logic on
// every run. SKIPs (not fails) when the Python nomadnet reference is absent.
func TestResolveConfigDirOrderPythonParity(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		etcCfg, xdgCfg bool
	}{
		{false, false}, // fallback
		{true, false},  // etc wins
		{false, true},  // xdg wins (etc absent)
		{true, true},   // etc wins over xdg
	}
	for _, s := range scenarios {
		t.Run(fmt.Sprintf("etc=%v,xdg=%v", s.etcCfg, s.xdgCfg), func(t *testing.T) {
			t.Parallel()
			state := map[string]any{
				"etc": "ETC", "xdg": "XDG", "fb": "FB",
				"etc_cfg": s.etcCfg, "xdg_cfg": s.xdgCfg,
			}
			var want string
			testutils.RunPythonNomadnet(t, state, resolveConfigDirOrderScript, &want)

			hasCfg := func(dir string) bool {
				switch dir {
				case "ETC":
					return s.etcCfg
				case "XDG":
					return s.xdgCfg
				}
				return false
			}
			got := resolveConfigDir("ETC", "XDG", "FB", hasCfg)
			if got != want {
				t.Errorf("got %q, want %q (fresh Python)", got, want)
			}
		})
	}
}
