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
)

// etcNomadnetDir is the system-wide nomadnet config location Python checks
// first (NomadNetworkApp.py:29-30). It is a package-level variable so tests
// can redirect it to a temp dir without root privileges.
var etcNomadnetDir = "/etc/nomadnetwork"

// resolveDefaultConfigDir picks the default nomadnet config directory using
// Python's resolution order (NomadNetworkApp.py:28-34): /etc/nomadnetwork when
// its config file exists, else ~/.config/nomadnetwork when its config file
// exists, else the ~/.nomadnetwork fallback (which Python creates lazily later,
// so its absence here is expected and not an error).
func resolveDefaultConfigDir(home string) string {
	xdg := filepath.Join(home, ".config", "nomadnetwork")
	fallback := filepath.Join(home, ".nomadnetwork")
	return resolveConfigDir(etcNomadnetDir, xdg, fallback, configFileExists)
}

// resolveConfigDir applies Python's configdir resolution order against three
// ordered candidates using hasConfig to test whether a candidate's "config"
// file exists. The first candidate whose config file exists wins; otherwise the
// fallback is returned. It is split out so the resolution order can be verified
// live against the Python reference with abstract candidate paths.
func resolveConfigDir(etc, xdg, fallback string, hasConfig func(dir string) bool) string {
	if hasConfig(etc) {
		return etc
	}
	if hasConfig(xdg) {
		return xdg
	}
	return fallback
}

// configFileExists reports whether dir/config exists and is a regular file,
// mirroring Python's `os.path.isdir(dir) and os.path.isfile(dir+"/config")`
// (a config file cannot exist without its parent dir, so the file check alone
// is equivalent for an existing dir).
func configFileExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "config"))
	return err == nil && !info.IsDir()
}
