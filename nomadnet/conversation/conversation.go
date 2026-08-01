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

// Package conversation implements NomadNet's LXMF conversation management.
//
// Conversations track message history, delivery state, attachments,
// and unread/failed counts for each peer.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Conversation represents an LXMF conversation with a single peer.
type Conversation struct {
	SourceHash    string
	MessagesPath  string
	Messages      []*Message
	SourceKnown   bool
	SourceTrusted bool
	SourceBlocked bool
	Unread        bool
	TrustLevel    byte

	changedCallback func()
	sendDeps        SendDeps

	// attachmentPath is the base directory for this conversation's per-message
	// attachment directories. It is stamped by ConversationCache.Store from
	// the cache's configured attachment path, and ScanStorage copies it onto
	// each message so Message.attachmentDir can locate attachments without a
	// package-level provider.
	attachmentPath string
}

// NewConversation creates a Conversation for the given source hash.
func NewConversation(sourceHash, messagesPath string) *Conversation {
	return &Conversation{
		SourceHash:   sourceHash,
		MessagesPath: messagesPath,
		Messages:     make([]*Message, 0),
	}
}

// SetChangedCallback registers a callback invoked when the conversation changes.
func (c *Conversation) SetChangedCallback(fn func()) {
	c.changedCallback = fn
}

// ScanStorage reads the conversation directory and updates the message list.
func (c *Conversation) ScanStorage() error {
	index := ReadIndex(c.MessagesPath)

	entries, err := os.ReadDir(c.MessagesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading conversation dir: %w", err)
	}

	existingMessages := make(map[string]*Message)
	for _, msg := range c.Messages {
		key := filepath.Base(msg.FilePath)
		existingMessages[key] = msg
	}

	var messages []*Message
	knownCount := len(c.Messages)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip index and flag files
		if name == ".index" || name == "unread" || name == "failed" || name == "read" {
			continue
		}

		// Message filenames are hex hashes (64 chars = 32 bytes)
		if len(name) != 64 {
			continue
		}
		if !isHexString(name) {
			continue
		}

		filePath := filepath.Join(c.MessagesPath, name)

		// Reuse existing message if mtime unchanged
		if existing, ok := existingMessages[name]; ok {
			info, err := entry.Info()
			if err == nil {
				newMtime := float64(info.ModTime().UnixNano()) / 1e9
				if newMtime == existing.SortTimestamp {
					messages = append(messages, existing)
					continue
				}
			}
		}

		msg := NewMessage(filePath)
		msg.AttachmentPath = c.attachmentPath

		// Restore from index if available
		if indexEntry, ok := index[name]; ok {
			if ie, ok := indexEntry.(map[string]any); ok {
				msg.RestoreFromIndex(ie)
			}
		}

		messages = append(messages, msg)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].GetTimestamp() > messages[j].GetTimestamp()
	})

	// Write index for any updated messages
	if err := WriteIndex(c.MessagesPath, messages); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write index: %v\n", err)
	}

	c.Messages = messages

	// Set unread if there are new messages
	newCount := len(messages)
	if newCount > knownCount && knownCount > 0 {
		c.Unread = true
	}

	// Fire callback
	if c.changedCallback != nil {
		c.changedCallback()
	}

	return nil
}

// PurgeFailed removes all failed messages from the conversation.
func (c *Conversation) PurgeFailed() {
	var remaining []*Message
	for _, msg := range c.Messages {
		if msg.GetState() == StateFailed {
			_ = msg.Purge()
		} else {
			remaining = append(remaining, msg)
		}
	}
	c.Messages = remaining
}

// ClearHistory purges all messages in the conversation.
func (c *Conversation) ClearHistory() {
	for _, msg := range c.Messages {
		_ = msg.Purge()
	}
	c.Messages = nil
}

// MessageCount returns the number of messages in the conversation.
func (c *Conversation) MessageCount() int {
	return len(c.Messages)
}

// LastMessage returns the most recent message, or nil.
func (c *Conversation) LastMessage() *Message {
	if len(c.Messages) == 0 {
		return nil
	}
	return c.Messages[0]
}

// ConversationInfo holds summary info for the conversation list display.
type ConversationInfo struct {
	SourceHash   string
	DisplayName  string
	TrustLevel   byte
	SortName     string
	Unread       bool
	UnreadCount  int
	LastActivity float64
	Failed       bool
	FailedCount  int
}

// ConversationList returns a sorted list of all conversations.
func ConversationList(conversationsPath string, displayNames map[string]string, trustLevels map[string]byte) []ConversationInfo {
	entries, err := os.ReadDir(conversationsPath)
	if err != nil {
		return nil
	}

	var list []ConversationInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		hash := entry.Name()
		convPath := filepath.Join(conversationsPath, hash)

		// Check unread/failed flags. Python (Conversation.py:127-148) reads the
		// flag file *content* as the count (defaulting to 1 when the file exists
		// but the content is empty/unparseable), not just file existence.
		unreadCount := readCountFile(filepath.Join(convPath, "unread"))
		failedCount := readCountFile(filepath.Join(convPath, "failed"))
		unread := unreadCount > 0
		failed := failedCount > 0

		// Get last activity time
		lastActivity := float64(0)
		msgs, err := os.ReadDir(convPath)
		if err == nil {
			for _, msg := range msgs {
				if msg.IsDir() || msg.Name() == ".index" || msg.Name() == "unread" || msg.Name() == "failed" || msg.Name() == "read" {
					continue
				}
				info, err := msg.Info()
				if err == nil {
					ts := float64(info.ModTime().UnixNano()) / 1e9
					if ts > lastActivity {
						lastActivity = ts
					}
				}
			}
		}

		displayName := displayNames[hash]
		trustLevel := trustLevels[hash]
		sortName := strings.ToLower(displayName)
		if sortName == "" {
			sortName = hash
		}

		list = append(list, ConversationInfo{
			SourceHash:   hash,
			DisplayName:  displayName,
			TrustLevel:   trustLevel,
			SortName:     sortName,
			Unread:       unread,
			UnreadCount:  unreadCount,
			LastActivity: lastActivity,
			Failed:       failed,
			FailedCount:  failedCount,
		})
	}

	// Sort by last activity descending
	sort.Slice(list, func(i, j int) bool {
		return list[i].LastActivity > list[j].LastActivity
	})

	return list
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readCountFile reads an unread/failed flag file and returns its integer
// content, mirroring Python's Conversation.conversation_list (Conversation.py:
// 127-148): a missing file means 0; a present file whose content parses as an
// int yields that count; a present but empty/unparseable file yields 1 (the
// flag exists, so at least one unread/failed message is implied).
func readCountFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 1
	}
	if n < 0 {
		n = 0
	}
	return n
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
