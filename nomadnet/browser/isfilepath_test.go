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

package browser

import "testing"

// TestIsFilePath pins the routing predicate Python uses to send a request down
// the download path: `if path.startswith("/file/")` (Browser.py:932). Everything
// else is a page fetch. The port had no such predicate in the UI path, so a
// remote /file/ URL went to the page renderer and the file bytes were displayed
// as markup instead of being saved.
func TestIsFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/file/report.txt", true},
		{"/file/docs/readme.md", true},
		{"/file/", true},
		{"/page/index.mu", false},
		{"/page/", false},
		{"/file", false}, // no trailing slash: Python's prefix check rejects it
		{"", false},
		{"/", false},
	}

	for _, tc := range tests {
		if got := IsFilePath(tc.path); got != tc.want {
			t.Errorf("IsFilePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
