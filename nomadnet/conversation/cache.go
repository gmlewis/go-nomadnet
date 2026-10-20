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
	"strconv"
	"sync"

	"github.com/gmlewis/go-reticulum/lxmf"
)

// ConversationCache is the per-app in-memory cache of conversations and their
// unread/failed flags, together with the configured attachment base directory.
// It is owned by app.App (App.Conversations) so each App — and each parallel
// test — gets isolated state instead of sharing package-level maps.
type ConversationCache struct {
	mu             sync.Mutex
	cached         map[string]*Conversation
	unread         map[string]int
	failed         map[string]int
	attachmentPath string

	// OnChanged, when set, is fired by Ingest after a message has been written
	// to disk + the cached conversation (if any) re-scanned, carrying the hex
	// source hash of the changed conversation. The TUI wiring layer uses it to
	// refresh the currently-open conversation widget's message list + trust
	// banner when an inbound (or outbound-delivery-confirmed) message lands —
	// mirroring Python's per-conversation changed-callback chain
	// (Conversation.ingest → scan_storage → Conversation.__changed_callback →
	// ConversationWidget.conversation_changed → update_message_widgets,
	// Conversation.py:71-80 + 271-272, Conversations.py:1896 + 2246-2252).
	// Fired from the LXMF delivery goroutine; callers must marshal to the UI
	// thread. Nil is a no-op (tests/headless use).
	OnChanged func(sourceHex string)
}

// NewConversationCache returns an empty, ready-to-use cache.
func NewConversationCache() *ConversationCache {
	return &ConversationCache{
		cached: make(map[string]*Conversation),
		unread: make(map[string]int),
		failed: make(map[string]int),
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

// IsUnread reports whether sourceHash is currently flagged unread in the
// in-memory cache, i.e. its unread count is non-zero.
func (cc *ConversationCache) IsUnread(sourceHash string) bool {
	return cc.UnreadCount(sourceHash) > 0
}

// IsFailed reports whether sourceHash is currently flagged failed in the
// in-memory cache, i.e. its failure count is non-zero.
func (cc *ConversationCache) IsFailed(sourceHash string) bool {
	return cc.FailedCount(sourceHash) > 0
}

// UnreadCount returns the cached number of unread messages for sourceHash, or 0
// when the conversation is not flagged. Python keeps the same per-conversation
// counter (Conversation.unread_conversations) and the conversation list renders
// it directly, so the badge reads "(N)" for N waiting messages.
func (cc *ConversationCache) UnreadCount(sourceHash string) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.unread[sourceHash]
}

// FailedCount returns the cached number of failed outbound messages for
// sourceHash, or 0 when the conversation has no failures.
func (cc *ConversationCache) FailedCount(sourceHash string) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.failed[sourceHash]
}

// increment bumps the count for sourceHash in the given counter map and returns
// the new value. A hash already present is incremented; a new one starts from
// whatever the flag file on disk already holds, so a restart does not reset the
// tally. Python gets the same effect from a class-level dict that
// conversation_list seeds out of the flag file (Conversation.py:88-91, 101-104,
// 139, 151). Ingest can run before any conversation listing has happened, so
// the seed is read here rather than only at listing time.
func (cc *ConversationCache) increment(counters map[string]int, sourceHash, flagPath string) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if _, ok := counters[sourceHash]; !ok {
		counters[sourceHash] = readCountFile(flagPath)
	}
	counters[sourceHash]++
	return counters[sourceHash]
}

// writeCountFile writes a flag file holding the decimal count, mirroring
// Python's `uf.write(str(count))` (Conversation.py:94-97). Python wraps the
// write in a bare except and lets the failure pass, so an unwritable flag file
// never breaks an ingest.
func writeCountFile(path string, count int) {
	_ = os.WriteFile(path, []byte(strconv.Itoa(count)), 0o644)
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

	// A cached conversation's storage is NOT rescanned here. Ingest runs on the
	// LXMF delivery goroutine (and the outbound delivery callback goroutine),
	// while ConversationMessages/DisplayMessages read c.Messages on the tview
	// event loop. Calling cached.ScanStorage() here would reassign c.Messages
	// concurrently with the UI loop, a data race on the unsynchronized
	// []*Message slice header. The on-disk file written above is picked up by
	// the UI loop's ScanStorage, triggered by OnChanged below (marshalled to the
	// event loop in the wiring layer), so the new message still appears without
	// a manual reload.

	// Flag the conversation unread or failed, mirroring Python's ingest
	// (Conversation.py:82-111). The branches are exclusive: an inbound message
	// only ever increments the unread counter, while an outbound message whose
	// LXMF state is FAILED or REJECTED increments the failure counter. The
	// counter *value* is what lands in the flag file, so the conversation list
	// can render "(N)".
	if !originator {
		count := cc.increment(cc.unread, sourceHex, filepath.Join(convDir, "unread"))
		writeCountFile(filepath.Join(convDir, "unread"), count)
	} else if state := msg.State(); state == lxmf.StateFailed || state == lxmf.StateRejected {
		count := cc.increment(cc.failed, sourceHex, filepath.Join(convDir, "failed"))
		writeCountFile(filepath.Join(convDir, "failed"), count)
	}

	// Notify the UI that this conversation's on-disk content changed, so the
	// open conversation widget can reload its message list + trust banner
	// (mirrors Python's Conversation.ingest firing created_callback +
	// scan_storage firing __changed_callback, Conversation.py:111-112 + 271).
	if cc.OnChanged != nil {
		cc.OnChanged(sourceHex)
	}

	return ingestedPath, nil
}
