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
	"encoding/hex"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
)

const testHash = "c388d720f56483a8dc8668ee5bea3577"

func TestParseRemoteAddress(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantPath string
		wantData map[string]string
		wantErr  bool
	}{
		{"bare hash", testHash, browser.DefaultPath, nil, false},
		{"upper bare hash", "C388D720F56483A8DC8668EE5BEA3577", browser.DefaultPath, nil, false},
		{"lxm prefix", "lxm:" + testHash, browser.DefaultPath, nil, false},
		{"lxmf prefix", "lxmf:" + testHash, browser.DefaultPath, nil, false},
		{"lxmf@ prefix", "lxmf@" + testHash, browser.DefaultPath, nil, false},
		{"lxmf scheme", "lxmf://" + testHash, browser.DefaultPath, nil, false},
		{"node scheme", "node://" + testHash, browser.DefaultPath, nil, false},
		{"nomadnetwork scheme", "nomadnetwork://" + testHash, browser.DefaultPath, nil, false},
		{"colon path", testHash + ":/page/conversations.mu", "/page/conversations.mu", nil, false},
		{"slash path", "nomadnetwork://" + testHash + "/page/conversations.mu", "/page/conversations.mu", nil, false},
		{"colon empty path defaults", testHash + ":", browser.DefaultPath, nil, false},
		{"pipe fields", testHash + ":/page/search.mu|q=robots", "/page/search.mu", map[string]string{"var_q": "robots"}, false},
		{"backtick fields", testHash + ":/page/search.mu`q=robots", "/page/search.mu", map[string]string{"var_q": "robots"}, false},
		{"relative no current", ":/page/index.mu", "", nil, true},
		{"too short", "c388d720", "", nil, true},
		{"non-hex", "zz88d720f56483a8dc8668ee5bea3577", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, path, data, friendly, err := parseRemoteAddress(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got hash=%x path=%q friendly=%q", h, path, friendly)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantBytes, _ := hex.DecodeString(testHash)
			if !equalBytes(h, wantBytes) {
				t.Errorf("hash = %x, want %x", h, wantBytes)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			if !equalData(data, tc.wantData) {
				t.Errorf("requestData = %v, want %v", data, tc.wantData)
			}
		})
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalData(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
