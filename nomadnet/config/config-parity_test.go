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

package config

import (
	"embed"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

//go:embed testdata/default.conf
var defaultConfFS embed.FS

// configObjParityScript imports the real Python nomadnet ConfigObj parser (the
// same RNS.vendor.configobj.ConfigObj that NomadNetworkApp uses), parses the
// config text supplied as a JSON string on stdin, and emits the
// section→key→value structure as JSON on stdout. Commented-out options are
// excluded by ConfigObj; only active `key = value` lines appear.
const configObjParityScript = `
import sys, json
from RNS.vendor.configobj import ConfigObj
text = json.loads(sys.stdin.read() or "\"\"")
config = ConfigObj(text.splitlines())
out = {}
for sec in config.sections:
    out[sec] = {}
    for key in config[sec].keys():
        out[sec][key] = config[sec][key]
print(json.dumps(out, ensure_ascii=False))
`

// configObjPythonOnce caches the single live Python ConfigObj run so the test
// below does not re-exec python3 if it is called multiple times.
var (
	configObjPythonOnce sync.Once
	configObjPythonOut  map[string]map[string]string
)

// configObjPython execs the real Python ConfigObj parser on the embedded
// default.conf and returns the fresh section→key→value structure. It is
// skipped (not failed) when the Python nomadnet reference is not importable.
func configObjPython(t *testing.T) map[string]map[string]string {
	t.Helper()
	configObjPythonOnce.Do(func() {
		confBytes, err := defaultConfFS.ReadFile("testdata/default.conf")
		if err != nil {
			t.Fatalf("read embed: %v", err)
		}
		testutils.RunPythonNomadnet(t, string(confBytes), configObjParityScript, &configObjPythonOut)
	})
	return configObjPythonOut
}

// TestLoadPythonCompat verifies that Go's INI parser produces the same
// section→key→value structure as Python's ConfigObj for the NomadNet default
// config. Commented-out options are excluded by both parsers; only active
// `key = value` lines appear. The expected structure is derived FRESH on every
// run by execing the real Python ConfigObj reference.
func TestLoadPythonCompat(t *testing.T) {
	t.Parallel()
	confBytes, err := defaultConfFS.ReadFile("testdata/default.conf")
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nomadnet.conf")
	if err := os.WriteFile(path, confBytes, 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := configObjPython(t)

	if len(c.Raw) != len(want) {
		t.Errorf("section count = %v, want %v", len(c.Raw), len(want))
	}
	for sec, wantKeys := range want {
		gotKeys, ok := c.Raw[sec]
		if !ok {
			t.Errorf("missing section %q", sec)
			continue
		}
		if len(gotKeys) != len(wantKeys) {
			t.Errorf("section %q: key count = %v, want %v", sec, len(gotKeys), len(wantKeys))
		}
		// Compare as sorted key lists to ignore map iteration order.
		gotList := sortedKeys(gotKeys)
		wantList := sortedKeys(wantKeys)
		if !reflect.DeepEqual(gotList, wantList) {
			t.Errorf("section %q: keys mismatch:\n got  %v\n want %v", sec, gotList, wantList)
		}
		for k, wv := range wantKeys {
			gv, ok := gotKeys[k]
			if !ok {
				t.Errorf("section %q: missing key %q", sec, k)
				continue
			}
			if gv != wv {
				t.Errorf("section %q key %q = %q, want %q", sec, k, gv, wv)
			}
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
