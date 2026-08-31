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
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gmlewis/go-reticulum/rns"
	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
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

	// Transport, when set, lets ScanStorage-created messages parse their LXMF
	// envelope from disk (recalling the sender identity for signature
	// verification). It mirrors Python's implicit access to the shared app
	// transport via NomadNetworkApp.get_shared_instance(). Stamp it with
	// SetTransport before ScanStorage/DisplayMessages so loaded messages
	// carry title/content/state/source-hash for display.
	Transport rns.Transport

	// PendingChecker, when set, is stamped onto ScanStorage-created messages
	// so Load can mark interrupted pending messages FAILED, mirroring
	// Python's ConversationMessage.load (Conversation.py:451-460). The app
	// wires it to the LXMF router's pending-outbound / pending-deferred-stamps
	// lookup. nil skips the check.
	PendingChecker func(hash []byte) bool

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

// SetTransport stamps the RNS transport onto the conversation and all of its
// already-scanned messages so their LXMF envelopes can be parsed on Load. It
// must be called before ScanStorage/DisplayMessages for the parsed fields
// (title, content, state, source hash) to be available.
func (c *Conversation) SetTransport(t rns.Transport) {
	c.Transport = t
	for _, m := range c.Messages {
		m.Transport = t
	}
}

// SetPendingChecker stamps the pending-outbound lookup onto the conversation
// and all of its already-scanned messages so Load can mark interrupted pending
// messages FAILED (Python Conversation.py:451-460). It must be called before
// ScanStorage/DisplayMessages for the check to apply to freshly loaded
// messages.
func (c *Conversation) SetPendingChecker(fn func(hash []byte) bool) {
	c.PendingChecker = fn
	for _, m := range c.Messages {
		m.PendingChecker = fn
	}
}

// RecallPeer mirrors Python Conversation.__init__ (Conversation.py:204-208):
// recall the peer's identity with the default use-marking recall so the peer's
// known-destinations entry is recorded as in use and survives the transport's
// pathless/never-used cleanup, and request a path from the network when the
// identity has not been recalled yet. It is a no-op when no transport is
// stamped or the source hash is malformed.
func (c *Conversation) RecallPeer() {
	if c.Transport == nil {
		return
	}
	hash, err := hex.DecodeString(c.SourceHash)
	if err != nil {
		log.Printf("Warning: RecallPeer could not decode source hash: %v", err)
		return
	}
	if c.Transport.Recall(hash) == nil {
		if err := c.Transport.RequestPath(hash); err != nil {
			log.Printf("Warning: RequestPath failed: %v", err)
		}
	}
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
		msg.Transport = c.Transport
		msg.PendingChecker = c.PendingChecker

		// Restore cached fields from the index whenever an entry exists,
		// exactly like Python's scan_storage for newly-discovered message
		// files (Conversation.py:2240-2246: `if filename in index:
		// msg.restore_from_index(index[filename])` — no mtime comparison).
		// Adding an mtime guard here diverged from Python: index-restored
		// messages skipped the lazy disk load and re-verification, so
		// gonomadnet rendered a fresh-verified header where nomadnet renders
		// the cached one (e.g. "✓ ←" vs the indexed "← Unknown Origin").
		if ie, ok := index.Get(name); ok {
			if om, ok := ie.(rnsmsgpack.OrderedMap); ok {
				msg.RestoreFromIndex(om)
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
		log.Printf("Warning: could not write index: %v\n", err)
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
	// SortRank carries the pinned-conversation rank from the directory entry
	// (nil = not pinned). Python renders a pin glyph prefix and sorts pinned
	// conversations first when this is set.
	SortRank *int
}

// ConversationList returns a sorted list of all conversations.
//
// LastActivity mirrors Python's conversation_list() (Conversation.py), which
// uses os.path.getmtime of the per-conversation DIRECTORY — updated whenever a
// message file or flag is created/renamed there — not the max mtime of the
// message files inside (message mtimes stay at write time and miss later
// state-only changes like marking a conversation read).
func ConversationList(conversationsPath string, displayNames map[string]string, trustLevels map[string]byte, sortRanks map[string]*int) []ConversationInfo {
	entries, err := os.ReadDir(conversationsPath)
	if err != nil {
		log.Printf("Warning: ConversationList error: %v", err)
		return nil
	}

	var list []ConversationInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		hash := entry.Name()

		// Python's conversation_list only accepts directories whose name is a
		// truncated identity hash in hex: len == Identity.TRUNCATED_HASHLENGTH//
		// 8*2 == 32 AND bytes.fromhex succeeds (fromhex lives inside the
		// per-entry try, so a 32-char non-hex name raises and the directory is
		// skipped). Filter everything else out here too.
		if len(hash) != 32 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}

		convPath := filepath.Join(conversationsPath, hash)

		// Check unread/failed flags. Python (Conversation.py:127-148) reads the
		// flag file *content* as the count (defaulting to 1 when the file exists
		// but the content is empty/unparseable), not just file existence. The
		// flag is Python truthiness of the parsed int (used as `if failed:` /
		// `elif unread:`), so any non-zero count — including a malformed
		// negative one — counts as set.
		unreadCount := readCountFile(filepath.Join(convPath, "unread"))
		failedCount := readCountFile(filepath.Join(convPath, "failed"))
		unread := unreadCount != 0
		failed := failedCount != 0

		// Last activity = the conversation directory's mtime, as in Python.
		lastActivity := float64(0)
		if info, err := os.Stat(convPath); err == nil {
			lastActivity = float64(info.ModTime().UnixNano()) / 1e9
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
			SortRank:     sortRanks[hash],
		})
	}

	// Sort by last activity descending (stable, as Python's list.sort is)
	sort.SliceStable(list, func(i, j int) bool {
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
// int yields that count (kept verbatim — Python does not clamp negatives, it
// relies on int truthiness); a present but empty/unparseable file yields 1 (the
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
	n, ok := parsePyInt(s)
	if !ok {
		return 1
	}
	return n
}

// parsePyInt mirrors Python's int(string) for the ASCII decimal subset —
// optional sign, digits with single underscores allowed BETWEEN digits only
// (int("1_0") == 10, but int("1_"), int("_1"), int("1__0") all raise). Go's
// strconv.Atoi accepts none of the underscore forms. Python additionally
// accepts non-ASCII unicode digits (int("١٢") == 12); those still fall through
// to unparseable here, which the caller maps to count 1, matching how Python
// would treat any other non-ASCII punctuation.
func parsePyInt(s string) (int, bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := s[i:]
	if digits == "" {
		return 0, false
	}
	prevUnderscore := false
	for j := 0; j < len(digits); j++ {
		c := digits[j]
		switch {
		case c >= '0' && c <= '9':
			prevUnderscore = false
		case c == '_':
			if prevUnderscore || j == 0 || j == len(digits)-1 {
				return 0, false
			}
			prevUnderscore = true
		default:
			return 0, false
		}
	}
	n, err := strconv.Atoi(strings.ReplaceAll(digits, "_", ""))
	if err != nil {
		return 0, false
	}
	if i > 0 && s[0] == '-' {
		n = -n
	}
	return n, true
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
