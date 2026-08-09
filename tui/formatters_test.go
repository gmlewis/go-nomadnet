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
	"strings"
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

func TestFormatAnnounceSummary(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name string
		ann  AnnounceEntry
		want string
	}{
		{
			name: "node type",
			ann: AnnounceEntry{
				Type:        "node",
				DisplayName: "MyNode",
				Timestamp:   now.Add(-5 * time.Minute),
			},
			want: "Ⓝ MyNode [node] 5m ago",
		},
		{
			name: "pn type",
			ann: AnnounceEntry{
				Type:        "pn",
				DisplayName: "PN-Relay",
				Timestamp:   now.Add(-1 * time.Hour),
			},
			want: "↑ PN-Relay [pn] 1h ago",
		},
		{
			name: "peer type",
			ann: AnnounceEntry{
				Type:        "peer",
				DisplayName: "Alice",
				Timestamp:   now.Add(-2 * time.Hour),
			},
			want: "Ⓟ Alice [peer] 2h ago",
		},
		{
			name: "unknown type",
			ann: AnnounceEntry{
				Type:        "other",
				DisplayName: "Unknown",
				Timestamp:   now.Add(-30 * time.Second),
			},
			want: "○ Unknown [other] just now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatAnnounceSummary(tt.ann)
			if got != tt.want {
				t.Errorf("FormatAnnounceSummary() = %q, want %q", got, tt.want)
			}
		})
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
		// Python-captured banker's-rounding (round-half-to-even) cases.
		// Python's round(x, 1) and Go's %.1f both round an exact .x5 to the
		// nearest even tenth: 2.25 -> 2.2, 2.75 -> 2.8, 3.25 -> 3.2.
		{2304, "2.2 KB"},          // 2304/1024  = 2.25  -> 2.2
		{2816, "2.8 KB"},          // 2816/1024  = 2.75  -> 2.8
		{3328, "3.2 KB"},          // 3328/1024  = 3.25  -> 3.2
		{2612, "2.6 KB"},          // 2612/1024  = 2.550 -> 2.6
		{2359296, "2.2 MB"},       // 2359296/1048576    = 2.25  -> 2.2
		{29360128, "28.0 MB"},     // 29360128/1048576   = 28.0
		{1076101120, "1026.2 MB"}, // = 1026.1875 -> 1026.2
	}

	for _, tt := range tests {
		got := FormatSize(tt.input)
		if got != tt.want {
			t.Errorf("FormatSize(%v) = %q, want %q", tt.input, got, tt.want)
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

	hash32 := "a1b2c3d4e5f6a1b2c3d4a1b2c3d4e5f6" // 32 hex chars (truncated RNS hash)

	tests := []struct {
		name     string
		url      string
		wantHash string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "bare hash",
			url:      hash32,
			wantHash: hash32,
			wantPath: "/page/index.mu",
		},
		{
			name:     "hash with path",
			url:      hash32 + ":/page/about.mu",
			wantHash: hash32,
			wantPath: "/page/about.mu",
		},
		{
			name:     "hash with empty path",
			url:      hash32 + ":",
			wantHash: hash32,
			wantPath: "/page/index.mu",
		},
		{
			name:    "relative URL with no current hash",
			url:     ":/page/foo.mu",
			wantErr: true,
		},
		{
			name:    "too many colons",
			url:     hash32 + ":extra:more",
			wantErr: true,
		},
		{
			name:    "invalid hex in hash",
			url:     "zz" + hash32[2:],
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

	hash32 := "a1b2c3d4e5f6a1b2c3d4a1b2c3d4e5f6" // 32 hex chars (truncated RNS hash)

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
			url:      hash32 + ":/page/form.mu`name=alice",
			wantHash: hash32,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"name": "alice",
			},
		},
		{
			name:     "hash path with multiple fields",
			url:      hash32 + ":/page/form.mu`name=alice|role=admin",
			wantHash: hash32,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"name": "alice",
				"role": "admin",
			},
		},
		{
			name:         "hash path with wildcard",
			url:          hash32 + ":/page/form.mu`*",
			wantHash:     hash32,
			wantPath:     "/page/form.mu",
			wantWildcard: true,
		},
		{
			name:     "hash path with empty fields after backtick",
			url:      hash32 + ":/page/form.mu`",
			wantHash: hash32,
			wantPath: "/page/form.mu",
		},
		{
			name:     "relative URL with fields",
			url:      ":/page/form.mu`x=1",
			wantHash: hash32,
			wantPath: "/page/form.mu",
			wantFields: map[string]string{
				"x": "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hash, path, fields, wildcard, err := ParseURLWithQuery(tt.url, hash32)
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
			name:       "link with key=value fields",
			input:      "a1b2c3d4e5f6a1b2c3d4`name=alice|role=admin",
			wantType:   "nomadnetwork.node",
			wantHash:   "a1b2c3d4e5f6a1b2c3d4",
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
						t.Errorf("fields[%v] = %q, want %q", i, fields[i], tt.wantFields[i])
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
		t.Errorf("TrustedCount = %v, want 2", stats.TrustedCount)
	}
	if stats.UntrustedCount != 3 {
		t.Errorf("UntrustedCount = %v, want 3", stats.UntrustedCount)
	}
	if stats.TrustedUnread != 1 {
		t.Errorf("TrustedUnread = %v, want 1", stats.TrustedUnread)
	}
	if stats.UntrustedUnread != 2 {
		t.Errorf("UntrustedUnread = %v, want 2", stats.UntrustedUnread)
	}
}

func TestConversationTabStatsEmpty(t *testing.T) {
	t.Parallel()

	stats := ComputeConversationTabStats(nil)
	if stats.TrustedCount != 0 || stats.UntrustedCount != 0 {
		t.Errorf("empty stats: trusted=%v untrusted=%v, want both 0",
			stats.TrustedCount, stats.UntrustedCount)
	}
}

func TestFormatHubStatus(t *testing.T) {
	t.Parallel()

	hub := HubEntry{
		Name:   "My Hub",
		Status: HubConnected,
		Rooms: map[string]*HubRoom{
			"general": {Name: "general", Joined: true, Unread: true},
			"random":  {Name: "random", Joined: true},
		},
	}

	got := FormatHubStatus(&hub)
	if !strings.Contains(got, "My Hub") {
		t.Errorf("FormatHubStatus() missing hub name: %q", got)
	}
	if !strings.Contains(got, "Connected") {
		t.Errorf("FormatHubStatus() missing connected status: %q", got)
	}
	if !strings.Contains(got, "1 unread") {
		t.Errorf("FormatHubStatus() missing unread count: %q", got)
	}
}

func TestFormatSyncProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		progress int
		want     string
	}{
		{0, "[                    ] 0%"},
		{50, "[==========          ] 50%"},
		{100, "[====================] 100%"},
		{-5, "[                    ] 0%"},
		{150, "[====================] 100%"},
	}

	for _, tt := range tests {
		got := FormatSyncProgress(tt.progress)
		if got != tt.want {
			t.Errorf("FormatSyncProgress(%v) = %q, want %q", tt.progress, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input float64
		want  string
	}{
		{0, "0 bytes"},
		{512, "512 bytes"},
		{1023, "1023 bytes"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		// Python-captured edge cases.
		{1, "1 bytes"},
		{1025, "1.0 KB"},
		{1099511627776, "1.0 TB"}, // 1 TiB reaches the TB unit
		{2304, "2.2 KB"},          // 2.25 -> banker's rounding -> 2.2
		{2816, "2.8 KB"},          // 2.75 -> 2.8
		{5368709120, "5.0 GB"},
		{0.5, "0 bytes"},       // int(0.5) truncates to 0
		{1023.9, "1023 bytes"}, // int() truncates the fraction
	}

	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSyncStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lastSync  time.Time
		hasSynced bool
		nodeLabel string
		want      string
	}{
		{
			name:      "never synced",
			lastSync:  time.Time{},
			hasSynced: false,
			nodeLabel: "",
			want:      "Last sync: never",
		},
		{
			name:      "synced recently with node",
			lastSync:  time.Now().Add(-5 * time.Minute),
			hasSynced: true,
			nodeLabel: "TestNode",
			want:      "Last sync: 5m ago  (TestNode)",
		},
		{
			name:      "synced no node",
			lastSync:  time.Now().Add(-1 * time.Hour),
			hasSynced: true,
			nodeLabel: "",
			want:      "Last sync: 1h ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSyncStatus(tt.lastSync, tt.hasSynced, tt.nodeLabel)
			if got != tt.want {
				t.Errorf("FormatSyncStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseQueryFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantKeys map[string]string
		wantWild bool
	}{
		{
			name:     "wildcard",
			query:    "*",
			wantKeys: nil,
			wantWild: true,
		},
		{
			name:     "empty string",
			query:    "",
			wantKeys: map[string]string{},
			wantWild: false,
		},
		{
			name:     "single key",
			query:    "name",
			wantKeys: map[string]string{"name": ""},
			wantWild: false,
		},
		{
			name:     "key=value",
			query:    "name=alice",
			wantKeys: map[string]string{"name": "alice"},
			wantWild: false,
		},
		{
			name:     "pipe separated",
			query:    "name=alice|role=admin",
			wantKeys: map[string]string{"name": "alice", "role": "admin"},
			wantWild: false,
		},
		{
			name:     "mixed keys and key-value",
			query:    "verbose|name=bob|debug",
			wantKeys: map[string]string{"verbose": "", "name": "bob", "debug": ""},
			wantWild: false,
		},
		{
			name:     "value with special chars",
			query:    "name=alice bob",
			wantKeys: map[string]string{"name": "alice bob"},
			wantWild: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotFields, gotWild := ParseQueryFields(tt.query)
			if gotWild != tt.wantWild {
				t.Errorf("ParseQueryFields(%q) wildcard = %v, want %v", tt.query, gotWild, tt.wantWild)
			}
			if tt.wantWild {
				return
			}
			if len(gotFields) != len(tt.wantKeys) {
				t.Errorf("ParseQueryFields(%q) got %v fields, want %v", tt.query, len(gotFields), len(tt.wantKeys))
				return
			}
			for k, v := range tt.wantKeys {
				if gotFields[k] != v {
					t.Errorf("ParseQueryFields(%q)[%q] = %q, want %q", tt.query, k, gotFields[k], v)
				}
			}
		})
	}
}

// TestPrettyDate verifies PrettyDate matches Python's pretty_date at
// Network.py:1933 for a range of ages. Expected values were captured from the
// Python source run with a fixed now (2026-07-31 12:00:00 UTC), so the test
// drives the time-injected prettyDateAt with that same fixed now.
func TestPrettyDate(t *testing.T) {
	t.Parallel()

	// Fixed now matching /tmp/prettydate_ref.py: 2026-07-31 12:00:00 UTC.
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		offset time.Duration
		want   string
	}{
		{"now", 0, "just now"},
		{"5s", 5 * time.Second, "just now"},
		{"45s", 45 * time.Second, "45 seconds ago"},
		{"90s", 90 * time.Second, "a minute ago"},
		{"30m", 30 * time.Minute, "30 minutes ago"},
		{"90m", 90 * time.Minute, "an hour ago"},
		{"5h", 5 * time.Hour, "5 hours ago"},
		{"23h", 23 * time.Hour, "23 hours ago"},
		{"25h", 25 * time.Hour, "Yesterday"},
		{"2d", 2 * 24 * time.Hour, "2 days ago"},
		{"6d", 6 * 24 * time.Hour, "6 days ago"},
		{"10d", 10 * 24 * time.Hour, "1 weeks ago"},
		{"40d", 40 * 24 * time.Hour, "1 months ago"},
		{"400d", 400 * 24 * time.Hour, "1 years ago"},
		{"future", -100 * time.Second, ""},
	}

	for _, tt := range tests {
		got := prettyDateAt(now.Add(-tt.offset), now)
		if got != tt.want {
			t.Errorf("PrettyDate(%v) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestPrettyDateBoundaryDays verifies the day-component boundary matches
// Python's timedelta semantics: a 24h age is exactly "Yesterday", and 23h59m
// is still in the same-day "hours ago" range.
func TestPrettyDateBoundaryDays(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	if got := prettyDateAt(now.Add(-24*time.Hour), now); got != "Yesterday" {
		t.Errorf("24h = %q, want %q", got, "Yesterday")
	}
	if got := prettyDateAt(now.Add(-(24*time.Hour - time.Second)), now); got != "23 hours ago" {
		t.Errorf("23h59m59s = %q, want %q", got, "23 hours ago")
	}
}
