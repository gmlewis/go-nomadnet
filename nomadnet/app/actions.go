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

package app

import (
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
)

// Conversations returns the list of known conversations, mirroring the
// Python NomadNetworkApp.conversations accessor.
func (a *App) Conversations() []conversation.ConversationInfo {
	return conversation.ConversationList(a.ConversationPath, nil, nil)
}

// HasUnreadConversations reports whether any conversation is flagged unread
// or failed, mirroring the Python NomadNet has_unread_conversations.
func (a *App) HasUnreadConversations() bool {
	return conversation.HasUnreadConversations(a.ConversationPath)
}

// ConversationIsUnread reports whether the conversation identified by the hex
// source hash is unread or failed, mirroring the Python NomadNet
// conversation_is_unread.
func (a *App) ConversationIsUnread(sourceHash string) bool {
	return conversation.ConversationIsUnread(sourceHash, a.ConversationPath)
}

// MarkConversationRead clears the unread and failed flags for a conversation,
// mirroring the Python NomadNet mark_conversation_read.
func (a *App) MarkConversationRead(sourceHash string) {
	a.ConversationCache.MarkRead(sourceHash, a.ConversationPath)
}

// ClearTmpDir removes all files from the temporary files directory, mirroring
// the Python NomadNet clear_tmp_dir.
func (a *App) ClearTmpDir() {
	entries, err := os.ReadDir(a.TmpFilesPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.Remove(filepath.Join(a.TmpFilesPath, entry.Name()))
	}
}

// ShouldPrint reports whether an incoming message should be printed according
// to the configured printing rules, mirroring the Python NomadNet should_print.
// When printing is disabled it returns false; when printing all messages is
// enabled it returns true; otherwise it returns true only for trusted senders
// (when print-trusted is enabled) or for explicitly allowed source hashes.
func (a *App) ShouldPrint(msg *lxmf.Message) bool {
	if !a.PrintMessages {
		return false
	}
	if a.PrintAllMessages {
		return true
	}
	sourceHashText := hex.EncodeToString(msg.SourceHash)

	if a.PrintTrustedMessages && a.Dir != nil {
		if a.Dir.TrustLevel(msg.SourceHash, nil) == directory.TrustTrusted {
			return true
		}
	}

	for _, allowed := range a.AllowedMessagePrintDestinations {
		if allowed == sourceHashText {
			return true
		}
	}
	return false
}

// DeleteConversation removes the conversation directory for the given hex
// source hash and clears the in-memory unread/failed caches, mirroring the
// Python Conversation.delete_conversation.
func (a *App) DeleteConversation(sourceHash string) {
	if err := a.ConversationCache.Delete(sourceHash, a.ConversationPath); err != nil {
		if a.Logger != nil {
			a.Logger.Error("Could not remove conversation at %v: %v", sourceHash, err)
		}
	}
}

// CreateDirectoryEntry creates and remembers a directory entry for a new peer
// with the given source hash and optional display name, mirroring the Python
// NomadNet create_directory_entry flow. The new entry defaults to the unknown
// trust level and direct delivery.
func (a *App) CreateDirectoryEntry(sourceHash []byte, displayName string) *directory.Entry {
	if a.Dir == nil {
		a.Dir = directory.New()
	}
	entry := directory.NewEntry(sourceHash)
	if displayName != "" {
		entry.DisplayName = displayName
	}
	a.Dir.Remember(entry)
	return entry
}

// RemoveAnnounce removes the announce with the given float64 timestamp from
// every announce stream, mirroring the Python Directory.remove_announce path
// triggered by the Network page's C-x "remove entry" action.
func (a *App) RemoveAnnounce(timestamp float64) {
	if a.Dir == nil {
		return
	}
	a.Dir.RemoveAnnounceWithTimestamp(timestamp)
}

// SaveNode remembers a directory entry for a node/peer at sourceHash with the
// given display name, mirroring the Python Network "save node" (C-s/C-b) flow.
// The entry defaults to the unknown trust level and direct delivery.
func (a *App) SaveNode(sourceHash []byte, displayName string) *directory.Entry {
	return a.CreateDirectoryEntry(sourceHash, displayName)
}

// ForgetNode removes the directory entry for sourceHash, mirroring the Python
// Network "remove entry" (C-x) action on a saved node.
func (a *App) ForgetNode(sourceHash []byte) {
	if a.Dir == nil {
		return
	}
	a.Dir.Forget(sourceHash)
}

// SetPeerDisplayName updates the display name of the directory entry for
// sourceHash, creating it if absent. Mirrors the Python Conversations
// "edit peer info" flow that updates the directory entry's name.
func (a *App) SetPeerDisplayName(sourceHash []byte, displayName string) {
	if a.Dir == nil {
		a.Dir = directory.New()
	}
	entry := a.Dir.Find(sourceHash)
	if entry == nil {
		entry = directory.NewEntry(sourceHash)
	}
	entry.DisplayName = displayName
	a.Dir.Remember(entry)
}

// PeerDisplayName returns the directory display name for sourceHash, or "" if
// the peer is unknown.
func (a *App) PeerDisplayName(sourceHash []byte) string {
	if a.Dir == nil {
		return ""
	}
	return a.Dir.DisplayName(sourceHash)
}

// LXMFAddressHex returns the user's LXMF delivery address as a lowercase hex
// string, or "" if the LXMF destination is not ready. Used by the Conversations
// "show my LXMF/QR" (C-p) action.
func (a *App) LXMFAddressHex() string {
	if a.LXMFDest == nil {
		return ""
	}
	return hex.EncodeToString(a.LXMFDest.Hash)
}

// SourceHashFromHex decodes a hex source-hash string (as shown in the UI) to
// bytes. It returns nil (and ok=false) for an invalid hex string.
func SourceHashFromHex(s string) ([]byte, bool) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}
