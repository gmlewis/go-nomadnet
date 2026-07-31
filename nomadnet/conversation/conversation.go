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
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gmlewis/go-reticulum/lxmf"
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
}

var (
	cachedConversations = make(map[string]*Conversation)
	unreadConversations = make(map[string]bool)
	failedConversations = make(map[string]bool)
	cachedMu            sync.Mutex
)

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
	LastActivity float64
	Failed       bool
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

		// Check unread/failed flags
		unread := fileExists(filepath.Join(convPath, "unread"))
		failed := fileExists(filepath.Join(convPath, "failed"))

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
			LastActivity: lastActivity,
			Failed:       failed,
		})
	}

	// Sort by last activity descending
	sort.Slice(list, func(i, j int) bool {
		return list[i].LastActivity > list[j].LastActivity
	})

	return list
}

// CacheConversation stores a conversation in the global cache.
func CacheConversation(c *Conversation) {
	cachedMu.Lock()
	defer cachedMu.Unlock()
	cachedConversations[c.SourceHash] = c
}

// CachedConversation retrieves a conversation from the global cache.
func CachedConversation(sourceHash string) *Conversation {
	cachedMu.Lock()
	defer cachedMu.Unlock()
	return cachedConversations[sourceHash]
}

// DeleteConversation removes a conversation from disk and the cache.
func DeleteConversation(sourceHash, conversationsPath string) error {
	convPath := filepath.Join(conversationsPath, sourceHash)
	if err := os.RemoveAll(convPath); err != nil {
		return fmt.Errorf("removing conversation: %w", err)
	}

	cachedMu.Lock()
	delete(cachedConversations, sourceHash)
	delete(unreadConversations, sourceHash)
	delete(failedConversations, sourceHash)
	cachedMu.Unlock()

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Ingest writes an incoming LXMF message to the appropriate conversation
// directory, creating it if necessary. The originator flag indicates whether
// this message was sent by the local user (true) or received from a peer
// (false). Returns the path of the ingested message file.
//
// This matches Python's Conversation.ingest() in Conversation.py:56.
func Ingest(msg *lxmf.Message, conversationsPath string, originator bool) (string, error) {
	var sourceHash []byte
	if originator {
		sourceHash = msg.DestinationHash
	} else {
		sourceHash = msg.SourceHash
	}

	sourceHex := hex.EncodeToString(sourceHash)
	convDir := filepath.Join(conversationsPath, sourceHex)

	if err := os.MkdirAll(convDir, 0o755); err != nil {
		return "", fmt.Errorf("creating conversation dir: %w", err)
	}

	ingestedPath, err := msg.WriteToDirectory(convDir)
	if err != nil {
		return "", fmt.Errorf("writing message to directory: %w", err)
	}

	cachedMu.Lock()
	if cached, ok := cachedConversations[sourceHex]; ok {
		cachedMu.Unlock()
		_ = cached.ScanStorage()
	} else {
		cachedMu.Unlock()
	}

	if !originator {
		unreadPath := filepath.Join(convDir, "unread")
		_ = os.WriteFile(unreadPath, []byte("1"), 0o644)
		cachedMu.Lock()
		unreadConversations[sourceHex] = true
		cachedMu.Unlock()
	}

	return ingestedPath, nil
}
