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
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gmlewis/go-reticulum/lxmf"
)

// ConversationCache is the per-app in-memory cache of conversations and their
// unread/failed flags, together with the configured attachment base directory.
// It is owned by app.App (App.Conversations) so each App — and each parallel
// test — gets isolated state instead of sharing package-level maps. Moving the
// former package globals (cachedConversations, unreadConversations,
// failedConversations, cachedMu, and the attachment-path provider) onto this
// struct removes the last mutable package-level state from the conversation
// package.
type ConversationCache struct {
	mu             sync.Mutex
	cached         map[string]*Conversation
	unread         map[string]bool
	failed         map[string]bool
	attachmentPath string
}

// NewConversationCache returns an empty, ready-to-use cache.
func NewConversationCache() *ConversationCache {
	return &ConversationCache{
		cached: make(map[string]*Conversation),
		unread: make(map[string]bool),
		failed: make(map[string]bool),
	}
}

// SetAttachmentPath configures the base directory under which per-message
// attachment directories live. Conversations stored after this call (and the
// messages they scan via ScanStorage) carry this path on their AttachmentPath
// field so they can locate attachments without consulting a package global.
func (cc *ConversationCache) SetAttachmentPath(path string) {
	cc.mu.Lock()
	cc.attachmentPath = path
	cc.mu.Unlock()
}

// Store records c in the cache, stamping it with the cache's attachment path so
// messages scanned by c.ScanStorage can locate their attachment directories.
func (cc *ConversationCache) Store(c *Conversation) {
	cc.mu.Lock()
	c.attachmentPath = cc.attachmentPath
	cc.cached[c.SourceHash] = c
	cc.mu.Unlock()
}

// Get returns the cached conversation for sourceHash, or nil if none is cached.
func (cc *ConversationCache) Get(sourceHash string) *Conversation {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.cached[sourceHash]
}

// Delete removes the conversation directory for sourceHash from disk and drops
// its cached conversation and unread/failed entries.
func (cc *ConversationCache) Delete(sourceHash, conversationsPath string) error {
	convPath := filepath.Join(conversationsPath, sourceHash)
	if err := os.RemoveAll(convPath); err != nil {
		return fmt.Errorf("removing conversation: %w", err)
	}

	cc.mu.Lock()
	delete(cc.cached, sourceHash)
	delete(cc.unread, sourceHash)
	delete(cc.failed, sourceHash)
	cc.mu.Unlock()

	return nil
}

// MarkRead clears the on-disk unread and failed flags for the conversation
// identified by sourceHash and removes its entries from the in-memory
// unread/failed caches so subsequent lookups reflect the cleared state.
func (cc *ConversationCache) MarkRead(sourceHash, conversationsPath string) {
	conv := filepath.Join(conversationsPath, sourceHash)
	removeFlagFile(filepath.Join(conv, "unread"))
	removeFlagFile(filepath.Join(conv, "failed"))

	cc.mu.Lock()
	delete(cc.unread, sourceHash)
	delete(cc.failed, sourceHash)
	cc.mu.Unlock()
}

// IsUnread reports whether sourceHash is currently held in the in-memory
// unread cache.
func (cc *ConversationCache) IsUnread(sourceHash string) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.unread[sourceHash]
}

// IsFailed reports whether sourceHash is currently held in the in-memory
// failed cache.
func (cc *ConversationCache) IsFailed(sourceHash string) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.failed[sourceHash]
}

// Ingest writes an incoming or outgoing LXMF message to the appropriate
// conversation directory, creating it if necessary. The originator flag
// indicates whether this message was sent by the local user (true) or received
// from a peer (false). When the conversation is already cached, its storage is
// rescanned so the new message appears immediately. Received messages are
// flagged unread. Returns the path of the ingested message file.
//
// This matches Python's Conversation.ingest() in Conversation.py:56.
func (cc *ConversationCache) Ingest(msg *lxmf.Message, conversationsPath string, originator bool) (string, error) {
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

	// Extract file/image/audio attachments to the per-message attachment
	// directory, mirroring Python Conversation.ingest →
	// ConversationMessage.extract_attachments_from_lxm (Conversation.py:73-76).
	// Extraction is best-effort: a failure logs and continues (Python wraps the
	// call in try/except), so the message is still ingested. Skipped when no
	// attachment path is configured (extraction would have nowhere to write).
	if cc.attachmentPath != "" {
		_ = ExtractAttachmentsFromLXM(msg, cc.attachmentPath)
	}

	cc.mu.Lock()
	if cached, ok := cc.cached[sourceHex]; ok {
		cc.mu.Unlock()
		_ = cached.ScanStorage()
	} else {
		cc.mu.Unlock()
	}

	if !originator {
		unreadPath := filepath.Join(convDir, "unread")
		_ = os.WriteFile(unreadPath, []byte("1"), 0o644)
		cc.mu.Lock()
		cc.unread[sourceHex] = true
		cc.mu.Unlock()
	}

	return ingestedPath, nil
}
