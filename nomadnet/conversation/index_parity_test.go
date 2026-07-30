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

package conversation

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

//go:embed testdata/index_py.bin
var pyIndexBin []byte

//go:embed testdata/index_py.json
var pyIndexFS embed.FS

// TestReadIndexPythonCompat verifies that Go's ReadIndex parses a msgpack
// .index file written by Python's ConversationMessage.write_index, with every
// field value matching what Python itself unpacks from the same bytes.
func TestReadIndexPythonCompat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".index")
	if err := os.WriteFile(indexPath, pyIndexBin, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	got := ReadIndex(dir)
	want := loadPyIndexJSON(t)

	if len(got) != len(want) {
		t.Fatalf("index entry count = %d, want %d", len(got), len(want))
	}
	for fn, wantEntry := range want {
		gotEntry, ok := got[fn]
		if !ok {
			t.Errorf("missing entry %q", fn)
			continue
		}
		gotMap, ok := gotEntry.(map[string]any)
		if !ok {
			t.Errorf("entry %q: not a map, got %T", fn, gotEntry)
			continue
		}
		if !reflect.DeepEqual(normEntry(gotMap), normWantEntry(wantEntry)) {
			t.Errorf("entry %q mismatch:\n got  %#v\n want %#v", fn, normEntry(gotMap), normWantEntry(wantEntry))
		}
	}
}

func loadPyIndexJSON(t *testing.T) map[string]map[string]any {
	t.Helper()
	data, err := pyIndexFS.ReadFile("testdata/index_py.json")
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var ex map[string]map[string]any
	if err := json.Unmarshal(data, &ex); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	return ex
}

// normEntry normalizes a Go-msgpack-parsed entry so numbers from msgpack (which
// may decode as int/int64/uint64/float64) collapse to float64 for comparison.
func normEntry(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normValue(v)
	}
	return out
}

// normWantEntry normalizes the JSON-captured entry: numbers are float64 (from
// JSON), and {"__bytes_hex__": "..."} markers decode back to []byte.
func normWantEntry(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normValue(v)
	}
	return out
}

func normValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return x
	case string:
		return x
	case bool:
		return x
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normValue(e)
		}
		return out
	case map[string]any:
		// {"__bytes_hex__": "<hex>"} -> []byte
		if h, ok := x["__bytes_hex__"]; ok {
			if hs, ok := h.(string); ok {
				b, err := hex.DecodeString(hs)
				if err == nil {
					return b
				}
			}
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normValue(e)
		}
		return out
	}
	return v
}
