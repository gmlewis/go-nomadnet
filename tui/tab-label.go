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

import "fmt"

// FormatTabLabel produces a tab label like "Trusted (5) ✉ 2" with unread
// count. Matches Python's _label() helper in update_listbox.
func FormatTabLabel(name string, total, unread int, unreadGlyph string) string {
	if unread > 0 {
		return fmt.Sprintf("%v (%v) %v %v", name, total, unreadGlyph, unread)
	}
	return fmt.Sprintf("%v (%v)", name, total)
}

// ConversationFilterPredicate tests whether a conversation passes
// the given list filter. Matches Python's _conversation_filter_predicate.
func ConversationFilterPredicate(trustLevel string, listFilter string) bool {
	if listFilter == "untrusted" {
		return trustLevel == TrustUntrusted ||
			trustLevel == TrustWarning ||
			trustLevel == TrustUnknown
	}
	return trustLevel == TrustTrusted
}

// ConversationHasAlerts returns true if a conversation has unread or failed
// messages. Matches Python's _alerts(c) helper.
func ConversationHasAlerts(unread, failed bool) bool {
	return unread || failed
}
