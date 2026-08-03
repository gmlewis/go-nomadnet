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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

//go:embed testdata/default.conf
var defaultConfFS embed.FS

//go:embed testdata/default_parsed.json
var defaultParsedFS embed.FS

// TestLoadPythonCompat verifies that Go's INI parser produces the same
// section→key→value structure as Python's ConfigObj for the NomadNet default
// config. Commented-out options are excluded by both parsers; only active
// `key = value` lines appear.
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

	wantBytes, err := defaultParsedFS.ReadFile("testdata/default_parsed.json")
	if err != nil {
		t.Fatalf("read parsed json: %v", err)
	}
	var want map[string]map[string]string
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatalf("unmarshal parsed json: %v", err)
	}

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
