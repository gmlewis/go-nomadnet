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

import "testing"

func TestFormatTabLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		label       string
		total       int
		unread      int
		unreadGlyph string
		want        string
	}{
		{"no unread", "Trusted", 5, 0, "✉", "Trusted (5)"},
		{"with unread", "Trusted", 5, 2, "✉", "Trusted (5) ✉ 2"},
		{"zero total no unread", "Untrusted", 0, 0, "✉", "Untrusted (0)"},
		{"zero total with unread", "Untrusted", 0, 3, "✉", "Untrusted (0) ✉ 3"},
		{"single item unread", "Trusted", 1, 1, "✉", "Trusted (1) ✉ 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatTabLabel(tt.label, tt.total, tt.unread, tt.unreadGlyph)
			if got != tt.want {
				t.Errorf("FormatTabLabel(%q, %d, %d, %q) = %q, want %q",
					tt.label, tt.total, tt.unread, tt.unreadGlyph, got, tt.want)
			}
		})
	}
}

func TestConversationFilterPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		trustLevel string
		filter     string
		want       bool
	}{
		{"trusted passes trusted", TrustTrusted, "trusted", true},
		{"untrusted fails trusted", TrustUntrusted, "trusted", false},
		{"unknown fails trusted", TrustUnknown, "trusted", false},
		{"warning fails trusted", TrustWarning, "trusted", false},
		{"trusted fails untrusted", TrustTrusted, "untrusted", false},
		{"untrusted passes untrusted", TrustUntrusted, "untrusted", true},
		{"unknown passes untrusted", TrustUnknown, "untrusted", true},
		{"warning passes untrusted", TrustWarning, "untrusted", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ConversationFilterPredicate(tt.trustLevel, tt.filter)
			if got != tt.want {
				t.Errorf("ConversationFilterPredicate(%q, %q) = %v, want %v",
					tt.trustLevel, tt.filter, got, tt.want)
			}
		})
	}
}

func TestConversationHasAlerts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		unread bool
		failed bool
		want   bool
	}{
		{"neither", false, false, false},
		{"unread only", true, false, true},
		{"failed only", false, true, true},
		{"both", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ConversationHasAlerts(tt.unread, tt.failed)
			if got != tt.want {
				t.Errorf("ConversationHasAlerts(%v, %v) = %v, want %v",
					tt.unread, tt.failed, got, tt.want)
			}
		})
	}
}
