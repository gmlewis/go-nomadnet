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
	"os"
	"path/filepath"
)

// HasUnreadConversations reports whether any conversation directory under
// conversationsPath is currently flagged unread or failed. It mirrors the
// Python NomadNet has_unread_conversations by inspecting the on-disk flags
// that populate the in-memory unread/failed maps.
func HasUnreadConversations(conversationsPath string) bool {
	entries, err := os.ReadDir(conversationsPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		conv := filepath.Join(conversationsPath, entry.Name())
		if fileExists(filepath.Join(conv, "unread")) || fileExists(filepath.Join(conv, "failed")) {
			return true
		}
	}
	return false
}

// ConversationIsUnread reports whether the conversation identified by
// sourceHash (hex) is flagged unread or failed on disk. This mirrors the
// Python NomadNet conversation_is_unread, which checks both the unread and
// failed conversation sets.
func ConversationIsUnread(sourceHash string, conversationsPath string) bool {
	conv := filepath.Join(conversationsPath, sourceHash)
	return fileExists(filepath.Join(conv, "unread")) || fileExists(filepath.Join(conv, "failed"))
}

func removeFlagFile(path string) {
	if fileExists(path) {
		_ = os.Remove(path)
	}
}
