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

package tui

import (
	"testing"
	"time"
)

func TestRelativeTimeWeeks(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		offset time.Duration
		want   string
	}{
		{0, "just now"},
		{-30 * time.Second, "just now"},
		{59 * time.Second, "just now"},
		{60 * time.Second, "1m ago"},
		{300 * time.Second, "5m ago"},
		{3599 * time.Second, "59m ago"},
		{3600 * time.Second, "1h ago"},
		{43200 * time.Second, "12h ago"},
		{86399 * time.Second, "23h ago"},
		{86400 * time.Second, "yesterday"},
		{172799 * time.Second, "yesterday"},
		{172800 * time.Second, "2d ago"},
		{604799 * time.Second, "6d ago"},
		{604800 * time.Second, "1w ago"},
		{1209600 * time.Second, "2w ago"},
		{1814400 * time.Second, "3w ago"},
		{2591999 * time.Second, "4w ago"},
		{2592000 * time.Second, ""},
	}

	for _, tt := range tests {
		got := RelativeTime(now.Add(-tt.offset))
		if tt.want == "" {
			// Date format - just verify it's not a relative string
			if got == "just now" || len(got) < 8 {
				t.Errorf("RelativeTime(%v offset) = %q, want date format", tt.offset, got)
			}
		} else {
			if got != tt.want {
				t.Errorf("RelativeTime(%v offset) = %q, want %q", tt.offset, got, tt.want)
			}
		}
	}
}

func TestFormatSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048575, "1024.0 KB"},
		{1048576, "1.0 MB"},
		{5242880, "5.0 MB"},
		{1073741824, "1024.0 MB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.input)
		if got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExpandShorthands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"nnn", "nomadnetwork.node"},
		{"lxmf", "lxmf.delivery"},
		{"rrc", "rrc.hub.session"},
		{"nomadnetwork.node", "nomadnetwork.node"},
		{"lxmf.delivery", "lxmf.delivery"},
		{"custom.type", "custom.type"},
	}

	for _, tt := range tests {
		got := ExpandShorthands(tt.input)
		if got != tt.want {
			t.Errorf("ExpandShorthands(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseURL(t *testing.T) {
	t.Parallel()

	hash20 := "a1b2c3d4e5f6a1b2c3d4" // 20 hex chars

	tests := []struct {
		name     string
		url      string
		wantHash string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "bare hash",
			url:      hash20,
			wantHash: hash20,
			wantPath: "/page/index.mu",
		},
		{
			name:     "hash with path",
			url:      hash20 + ":/page/about.mu",
			wantHash: hash20,
			wantPath: "/page/about.mu",
		},
		{
			name:     "hash with empty path",
			url:      hash20 + ":",
			wantHash: hash20,
			wantPath: "/page/index.mu",
		},
		{
			name:    "relative URL with no current hash",
			url:     ":/page/foo.mu",
			wantErr: true,
		},
		{
			name:    "too many colons",
			url:     hash20 + ":extra:more",
			wantErr: true,
		},
		{
			name:    "invalid hex in hash",
			url:     "zz" + hash20[2:],
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var currentHash string
			if tt.wantHash != "" && !tt.wantErr {
				currentHash = tt.wantHash
			}
			hash, path, err := ParseURL(tt.url, currentHash)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseURL(%q) did not return error", tt.url)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURL(%q) returned error: %v", tt.url, err)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}

func TestParseLinkTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		wantType string
		wantHash string
		wantArg  string
	}{
		{
			input:    "#anchor-name",
			wantType: "anchor",
			wantHash: "anchor-name",
		},
		{
			input:    "#",
			wantType: "anchor",
			wantHash: "",
		},
		{
			input:    "rrc://hash:dest/room",
			wantType: "rrc",
			wantHash: "hash:dest/room",
		},
		{
			input:    "lxmf@a1b2c3d4e5f6a1b2c3d4",
			wantType: "lxmf.delivery",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
		},
		{
			input:    "nnn@a1b2c3d4e5f6a1b2c3d4",
			wantType: "nomadnetwork.node",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
		},
		{
			input:    "rrc@a1b2c3d4e5f6a1b2c3d4",
			wantType: "rrc.hub.session",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
		},
		{
			input:    "a1b2c3d4e5f6a1b2c3d4",
			wantType: "nomadnetwork.node",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
		},
		{
			input:    "p:id1|id2",
			wantType: "partial",
			wantHash: "id1|id2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			ltype, hash := ParseLinkTarget(tt.input)
			if ltype != tt.wantType {
				t.Errorf("type = %q, want %q", ltype, tt.wantType)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
		})
	}
}

func TestParseURLWithQuery(t *testing.T) {
	t.Parallel()

	hash20 := "a1b2c3d4e5f6a1b2c3d4"

	tests := []struct {
		name         string
		url          string
		wantHash     string
		wantPath     string
		wantFields   map[string]string
		wantWildcard bool
		wantErr      bool
	}{
		{
			name:     "hash path with single field",
			url:      hash20 + ":/page/form.mu`name=alice",
			wantHash: hash20,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"name": "alice",
			},
		},
		{
			name:     "hash path with multiple fields",
			url:      hash20 + ":/page/form.mu`name=alice|role=admin",
			wantHash: hash20,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"name": "alice",
				"role": "admin",
			},
		},
		{
			name:         "hash path with wildcard",
			url:          hash20 + ":/page/form.mu`*",
			wantHash:     hash20,
			wantPath:     "/page/form.mu",
			wantWildcard: true,
		},
		{
			name:     "hash path with empty fields after backtick",
			url:      hash20 + ":/page/form.mu`",
			wantHash: hash20,
			wantPath: "/page/form.mu",
		},
		{
			name:     "relative URL with fields",
			url:      ":/page/form.mu`x=1",
			wantHash: hash20,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"x": "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hash, path, fields, wildcard, err := ParseURLWithQuery(tt.url, hash20)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseURLWithQuery(%q) did not return error", tt.url)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURLWithQuery(%q) returned error: %v", tt.url, err)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if wildcard != tt.wantWildcard {
				t.Errorf("wildcard = %v, want %v", wildcard, tt.wantWildcard)
			}
			if len(fields) != len(tt.wantFields) {
				t.Errorf("fields = %v, want %v", fields, tt.wantFields)
			} else {
				for k, v := range tt.wantFields {
					if fields[k] != v {
						t.Errorf("fields[%q] = %q, want %q", k, fields[k], v)
					}
				}
			}
		})
	}
}

func TestParseLinkTargetWithFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantType   string
		wantHash   string
		wantFields []string
		wantAll    bool
	}{
		{
			name:       "link with specific fields",
			input:      "a1b2c3d4e5f6a1b2c3d4`name|role",
			wantType:   "nomadnetwork.node",
			wantHash:   "a1b2c3d4e5f6a1b2c3d4",
			wantFields: []string{"name", "role"},
		},
		{
			name:     "link with wildcard",
			input:    "a1b2c3d4e5f6a1b2c3d4`*",
			wantType: "nomadnetwork.node",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
			wantAll:  true,
		},
		{
			name:     "link with key=value fields",
			input:    "a1b2c3d4e5f6a1b2c3d4`name=alice|role=admin",
			wantType: "nomadnetwork.node",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
			wantFields: []string{"name=alice", "role=admin"},
		},
		{
			name:     "plain link without fields",
			input:    "a1b2c3d4e5f6a1b2c3d4",
			wantType: "nomadnetwork.node",
			wantHash: "a1b2c3d4e5f6a1b2c3d4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ltype, hash, fields, all := ParseLinkTargetWithFields(tt.input)
			if ltype != tt.wantType {
				t.Errorf("type = %q, want %q", ltype, tt.wantType)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
			if all != tt.wantAll {
				t.Errorf("all = %v, want %v", all, tt.wantAll)
			}
			if len(fields) != len(tt.wantFields) {
				t.Errorf("fields = %v, want %v", fields, tt.wantFields)
			} else {
				for i := range fields {
					if fields[i] != tt.wantFields[i] {
						t.Errorf("fields[%d] = %q, want %q", i, fields[i], tt.wantFields[i])
					}
				}
			}
		})
	}
}

func TestFormatAnnounceDetail(t *testing.T) {
	t.Parallel()

	ann := AnnounceEntry{
		Timestamp:   time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		SourceHash:  "a1b2c3d4e5f6a1b2c3d4",
		AppData:     "Alice's Node",
		Type:        "node",
		TrustLevel:  "trusted",
		DisplayName: "Alice",
	}

	detail := FormatAnnounceDetail(ann)
	if detail == "" {
		t.Error("FormatAnnounceDetail returned empty string")
	}

	// Should contain key fields
	for _, field := range []string{"Alice", "trusted", "node", "a1b2c3d4e5f6a1b2c3d4"} {
		if !containsStr(detail, field) {
			t.Errorf("FormatAnnounceDetail missing %q", field)
		}
	}
}

func TestConversationTabStats(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{DisplayName: "Alice", TrustLevel: "trusted", Unread: true},
		{DisplayName: "Bob", TrustLevel: "trusted", Unread: false},
		{DisplayName: "Eve", TrustLevel: "untrusted", Unread: true},
		{DisplayName: "Mallory", TrustLevel: "untrusted", Unread: false},
		{DisplayName: "Trudy", TrustLevel: "unknown", Unread: true},
	}

	stats := ComputeConversationTabStats(convs)

	if stats.TrustedCount != 2 {
		t.Errorf("TrustedCount = %d, want 2", stats.TrustedCount)
	}
	if stats.UntrustedCount != 3 {
		t.Errorf("UntrustedCount = %d, want 3", stats.UntrustedCount)
	}
	if stats.TrustedUnread != 1 {
		t.Errorf("TrustedUnread = %d, want 1", stats.TrustedUnread)
	}
	if stats.UntrustedUnread != 2 {
		t.Errorf("UntrustedUnread = %d, want 2", stats.UntrustedUnread)
	}
}

func TestConversationTabStatsEmpty(t *testing.T) {
	t.Parallel()

	stats := ComputeConversationTabStats(nil)
	if stats.TrustedCount != 0 || stats.UntrustedCount != 0 {
		t.Errorf("empty stats: trusted=%d untrusted=%d, want both 0",
			stats.TrustedCount, stats.UntrustedCount)
	}
}


