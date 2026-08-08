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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
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

	return slices.Contains(a.AllowedMessagePrintDestinations, sourceHashText)
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

// SaveConnectedNode remembers a directory entry for the node the browser is
// currently connected to, mirroring Python Browser.save_node_dialog's confirmed
// (Browser.py:1196-1200): DirectoryEntry(destination_hash, display_name=…,
// hosts_node=True). Unlike SaveNode (which records an announced peer that may
// or may not host a node), this marks HostsNode true because the user reached
// it by browsing a served page. Returns the remembered entry.
func (a *App) SaveConnectedNode(sourceHash []byte, displayName string) *directory.Entry {
	if a.Dir == nil {
		a.Dir = directory.New()
	}
	entry := directory.NewEntry(sourceHash)
	if displayName != "" {
		entry.DisplayName = displayName
	}
	entry.HostsNode = true
	a.Dir.Remember(entry)
	return entry
}

// truncatedHashHexLen is the number of hex characters for a truncated RNS
// destination hash: rns.TruncatedHashLength/8*2 = 128/8*2 = 32.
const truncatedHashHexLen = rns.TruncatedHashLength / 8 * 2

// OpenLXMFLink mirrors Python Browser.handle_lxmf_link (Browser.py:383-423):
// it validates the LXMF destination hash (32 hex chars, decodable), recalls
// the announced display name from the identity's app_data when a transport is
// available, and — for a source not already in the conversation list — creates
// a directory entry (so the peer shows up in the directory) and an on-disk
// conversation directory (so the conversation persists and appears in
// conversation_list, matching Python's Conversation(source_hash, initiator=True)
// creating the directory). It returns whether a new conversation was created.
// The TUI wiring refreshes the conversation list, displays the conversation, and
// switches to the Conversations page on success.
func (a *App) OpenLXMFLink(sourceHashHex string) (isNew bool, err error) {
	if len(sourceHashHex) != truncatedHashHexLen {
		return false, fmt.Errorf("invalid length for LXMF link: got %v, want %v", len(sourceHashHex), truncatedHashHexLen)
	}
	hash, err := hex.DecodeString(sourceHashHex)
	if err != nil {
		return false, errors.New("could not decode destination hash from LXMF link")
	}

	// An existing conversation directory means this is a known peer.
	for _, c := range a.ConversationList() {
		if c.SourceHash == sourceHashHex {
			isNew = false
			return isNew, nil
		}
	}

	// Recall the announced display name (Python: Identity.recall_app_data +
	// LXMF.display_name_from_app_data). Best-effort: with no transport wired
	// (e.g. in tests) the name is simply empty.
	displayName := ""
	if a.Transport != nil {
		if id := a.Transport.Recall(hash); id != nil && len(id.AppData) > 0 {
			displayName, _ = lxmf.DisplayNameFromAppData(id.AppData)
		}
	}

	a.CreateDirectoryEntry(hash, displayName)

	if a.ConversationPath != "" {
		if err := os.MkdirAll(filepath.Join(a.ConversationPath, sourceHashHex), 0o700); err != nil {
			return false, err
		}
	}

	isNew = true
	return isNew, nil
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

// SetPeerTrustLevel updates the trust level of the directory entry for
// sourceHash, preserving any existing display name and preferred delivery,
// mirroring Python's ConversationWidget _on_trust_click (Conversations.py:
// 1989-2003): it looks up the existing entry, re-creates it as trusted, and
// remembers it.
func (a *App) SetPeerTrustLevel(sourceHash []byte, trustLevel byte) {
	if a.Dir == nil {
		a.Dir = directory.New()
	}
	entry := a.Dir.Find(sourceHash)
	if entry == nil {
		entry = directory.NewEntry(sourceHash)
	}
	entry.TrustLevel = trustLevel
	a.Dir.Remember(entry)
}

// PeerInfoData holds the editable peer-directory fields the Conversations
// "Peer Info" dialog (Python edit_selected_in_directory, Conversations.py:821)
// reads and writes. It is the Go counterpart to the subset of DirectoryEntry
// fields the dialog touches.
type PeerInfoData struct {
	DisplayName       string
	TrustLevel        byte
	PreferredDelivery byte
	Pinned            bool
	Notes             string
	Known             bool
}

// PeerInfoLoad reads the current directory entry for sourceHash, returning the
// editable fields the Peer Info dialog should pre-fill, mirroring Python's
// existing_entry lookup in edit_selected_in_directory (Conversations.py:844-
// 876). A missing entry yields the Python defaults: Unknown trust, direct
// delivery, unpinned, empty notes. Known reflects directory.is_known.
func (a *App) PeerInfoLoad(sourceHash []byte) PeerInfoData {
	d := PeerInfoData{
		TrustLevel:        directory.TrustUnknown,
		PreferredDelivery: directory.DeliveryDirect,
	}
	if a.Dir == nil {
		return d
	}
	d.Known = a.Dir.IsKnown(sourceHash)
	if entry := a.Dir.Find(sourceHash); entry != nil {
		d.DisplayName = entry.DisplayName
		d.TrustLevel = entry.TrustLevel
		d.PreferredDelivery = entry.PreferredDelivery
		d.Pinned = entry.SortRank != nil
		d.Notes = entry.Notes
	}
	return d
}

// RememberPeerInfo writes the full set of peer-directory fields edited by the
// Peer Info dialog, mirroring Python's confirmed() (Conversations.py:901-929):
// it builds a DirectoryEntry from the dialog values and remembers it,
// preserving any existing HostsNode/IdentifyOnConnect flags. Pinned maps to a
// sort_rank of 0; unpinned maps to a nil sort_rank (sorted below pinned
// entries). Returns the remembered entry.
func (a *App) RememberPeerInfo(sourceHash []byte, data PeerInfoData) *directory.Entry {
	if a.Dir == nil {
		a.Dir = directory.New()
	}
	entry := a.Dir.Find(sourceHash)
	if entry == nil {
		entry = directory.NewEntry(sourceHash)
	}
	entry.SourceHash = sourceHash
	entry.DisplayName = data.DisplayName
	entry.TrustLevel = data.TrustLevel
	entry.PreferredDelivery = data.PreferredDelivery
	if data.Pinned {
		zero := 0
		entry.SortRank = &zero
	} else {
		entry.SortRank = nil
	}
	entry.Notes = data.Notes
	a.Dir.Remember(entry)
	return entry
}

// QueryForPeer requests the network for the given peer's identity/path,
// mirroring Python's Conversation.query_for_peer (Conversation.py:49-52) which
// calls RNS.Transport.request_path. It is a no-op when no transport is
// available. Used by the Peer Info dialog's "Query network for keys" action
// (Conversations.py:962-979).
func (a *App) QueryForPeer(destHash []byte) {
	if a.Transport == nil || destHash == nil {
		return
	}
	_ = a.Transport.RequestPath(destHash)
}

// BlockPeer blocks the given destination: it blackholes the peer's identity on
// the transport, adds the hash to the ignored list, and instructs the LXMF
// router to ignore it, mirroring Python's block_destination as invoked from the
// Peer Info dialog's Block action (_block_peer_from_dialog, Conversations.py:
// 769-800). The reason is recorded with the block.
func (a *App) BlockPeer(destHash []byte, reason string) bool {
	return a.BlockDestination(destHash, reason)
}

// PeerStampCost returns the outbound stamp cost for the peer, mirroring Python
// _update_peer_info's stamp_cost resolution (Conversations.py:2103-2105): the
// LXMF router's get_outbound_stamp_cost, falling back to
// LXMF.stamp_cost_from_app_data on the recalled identity's app_data when the
// router has no value. Returns nil when no stamp cost is known (the dialog
// then omits the "Stamp: N" segment, matching Python stamp_cost is None).
func (a *App) PeerStampCost(destHash []byte) *int {
	if destHash == nil {
		return nil
	}
	if a.Router != nil {
		if sc, ok := a.Router.OutboundStampCost(destHash); ok {
			return &sc
		}
	}
	// Fallback: derive the stamp cost from the peer's recalled announce
	// app_data, matching Python's LXMF.stamp_cost_from_app_data branch.
	if a.Transport != nil {
		if id := rns.RecallIdentity(a.Transport, destHash); id != nil && len(id.AppData) > 0 {
			if sc, ok, err := lxmf.StampCostFromAppData(id.AppData); err == nil && ok {
				return &sc
			}
		}
	}
	return nil
}

// PeerHops returns the transport hop count to the peer, or nil when the path
// is unknown, mirroring Python _update_peer_info (Conversations.py:2107-2112):
// RNS.Transport.hops_to returns >= PATHFINDER_M for unknown paths, which the
// dialog renders as "unknown".
func (a *App) PeerHops(destHash []byte) *int {
	if a.Transport == nil || destHash == nil {
		return nil
	}
	h := a.Transport.HopsTo(destHash)
	if h >= rns.PathfinderM {
		return nil
	}
	return &h
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
