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

package micron

import (
	"embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/slugify_parity.json
var slugifyFS embed.FS

func TestSlugifyPythonParity(t *testing.T) {
	t.Parallel()
	data, err := slugifyFS.ReadFile("testdata/slugify_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var cases [][2]any
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for i, c := range cases {
		var in string
		if c[0] != nil {
			s, ok := c[0].(string)
			if !ok {
				t.Fatalf("case %v: input not string", i)
			}
			in = s
		}
		want, _ := c[1].(string)
		got := Slugify(in)
		if got != want {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, want)
		}
	}
}
