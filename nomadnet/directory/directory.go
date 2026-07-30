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

// Package directory implements the NomadNet peer directory with trust
// levels and announce stream management.
//
// The directory tracks known peers, their trust levels, preferred
// delivery methods, and maintains an announce stream for the network
// display.
package directory

import (
	"sync"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/util"
)

// AnnounceStreamMaxLen is the maximum number of announces kept in each stream.
const AnnounceStreamMaxLen = 256

// Announce represents a single announce entry in the stream.
type Announce struct {
	Timestamp    float64
	SourceHash   []byte
	AppData      []byte
	AnnounceType string // "peer", "node", or "pn"
}

// Directory manages the peer directory and announce streams.
type Directory struct {
	entries map[string]*Entry // source_hash hex → Entry

	nodeAnnounces []Announce
	peerAnnounces []Announce
	pnAnnounces   []Announce

	mu sync.Mutex
}

// New creates a new empty Directory.
func New() *Directory {
	return &Directory{
		entries: make(map[string]*Entry),
	}
}

// Remember stores a directory entry. If an associated node entry exists
// with the same identity, its trust level is updated to match.
func (d *Directory) Remember(entry *Entry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(entry.SourceHash)
	d.entries[key] = entry
}

// Forget removes a directory entry by source hash.
func (d *Directory) Forget(sourceHash []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	delete(d.entries, key)
}

// Find returns the directory entry for the given source hash, or nil.
func (d *Directory) Find(sourceHash []byte) *Entry {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	return d.entries[key]
}

// DisplayName returns the sanitized display name for a source hash,
// or an empty string if not found.
func (d *Directory) DisplayName(sourceHash []byte) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return ""
	}
	name := util.StripModifiers(&entry.DisplayName)
	if name == nil {
		return ""
	}
	return *name
}

// TrustLevel returns the trust level for a source hash. If
// announcedDisplayName is provided and the entry is not TRUSTED,
// it checks for display name collisions with other entries.
func (d *Directory) TrustLevel(sourceHash []byte, announcedDisplayName *string) byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return TrustUnknown
	}

	if announcedDisplayName != nil && entry.TrustLevel != TrustTrusted {
		for _, e := range d.entries {
			if e.DisplayName == *announcedDisplayName {
				hashMatch := bytesEqual(e.SourceHash, sourceHash)
				if !hashMatch {
					return TrustWarning
				}
			}
		}
	}

	return entry.TrustLevel
}

// PreferredDelivery returns the preferred delivery mode for a source hash.
func (d *Directory) PreferredDelivery(sourceHash []byte) byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return DeliveryDirect
	}
	return entry.PreferredDelivery
}

// ShouldIdentifyOnConnect returns whether the entry requests identification
// on connection.
func (d *Directory) ShouldIdentifyOnConnect(sourceHash []byte) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return false
	}
	return entry.IdentifyOnConnect
}

// SetIdentifyOnConnect sets the identify-on-connect flag for an entry.
func (d *Directory) SetIdentifyOnConnect(sourceHash []byte, identify bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	if entry, ok := d.entries[key]; ok {
		entry.IdentifyOnConnect = identify
	}
}

// SimplestDisplayStr returns a sanitized display name. For WARNING and
// UNTRUSTED entries, the hex hash is appended in angle brackets.
func (d *Directory) SimplestDisplayStr(sourceHash []byte) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return "<" + hexString(sourceHash) + ">"
	}

	name := util.StripModifiers(&entry.DisplayName)
	displayName := ""
	if name != nil {
		displayName = *name
	}

	if entry.TrustLevel == TrustWarning || entry.TrustLevel == TrustUntrusted {
		return displayName + " <" + hexString(sourceHash) + ">"
	}

	if displayName == "" {
		return "<" + hexString(sourceHash) + ">"
	}

	return displayName
}

// AllegedDisplayStr returns the raw display name with modifiers stripped.
func (d *Directory) AllegedDisplayStr(sourceHash []byte) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return ""
	}
	name := util.StripModifiers(&entry.DisplayName)
	if name == nil {
		return ""
	}
	return *name
}

// KnownNodes returns all directory entries that host a node, sorted
// by sort rank (ascending), then trust level (descending), then
// display name (ascending).
func (d *Directory) KnownNodes() []*Entry {
	d.mu.Lock()
	defer d.mu.Unlock()

	var nodes []*Entry
	for _, entry := range d.entries {
		if entry.HostsNode {
			nodes = append(nodes, entry)
		}
	}

	// Sort by sort_rank (nil = last), then trust level desc, then name asc
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if shouldSwap(nodes[i], nodes[j]) {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}

	return nodes
}

// AnnounceStream returns all announces from all streams combined.
func (d *Directory) AnnounceStream() []Announce {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]Announce, 0, len(d.nodeAnnounces)+len(d.peerAnnounces)+len(d.pnAnnounces))
	result = append(result, d.nodeAnnounces...)
	result = append(result, d.peerAnnounces...)
	result = append(result, d.pnAnnounces...)
	return result
}

// PeerAnnounces returns all peer announces.
func (d *Directory) PeerAnnounces() []Announce {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]Announce, len(d.peerAnnounces))
	copy(result, d.peerAnnounces)
	return result
}

// NodeAnnounces returns all node announces.
func (d *Directory) NodeAnnounces() []Announce {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]Announce, len(d.nodeAnnounces))
	copy(result, d.nodeAnnounces)
	return result
}

// PNAnnounces returns all propagation node announces.
func (d *Directory) PNAnnounces() []Announce {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]Announce, len(d.pnAnnounces))
	copy(result, d.pnAnnounces)
	return result
}

// RemoveAnnounceWithTimestamp removes the first announce with the given
// timestamp from any stream.
func (d *Directory) RemoveAnnounceWithTimestamp(timestamp float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.removeAnnounceFromList(&d.nodeAnnounces, timestamp)
	d.removeAnnounceFromList(&d.peerAnnounces, timestamp)
	d.removeAnnounceFromList(&d.pnAnnounces, timestamp)
}

// PeerAnnounceReceived adds a peer announce to the stream. If compact
// mode is enabled, removes prior announces from the same source.
func (d *Directory) PeerAnnounceReceived(announce Announce, compact bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if compact {
		d.compactList(&d.peerAnnounces, announce.SourceHash)
	}

	d.peerAnnounces = append([]Announce{announce}, d.peerAnnounces...)
	d.cleanList(&d.peerAnnounces)
}

// NodeAnnounceReceived adds a node announce to the stream. If compact
// mode is enabled, removes prior announces from the same source.
func (d *Directory) NodeAnnounceReceived(announce Announce, compact bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if compact {
		d.compactList(&d.nodeAnnounces, announce.SourceHash)
	}

	d.nodeAnnounces = append([]Announce{announce}, d.nodeAnnounces...)
	d.cleanList(&d.nodeAnnounces)
}

// PNAnnounceReceived adds a propagation node announce to the stream.
// If compact mode is enabled, removes prior announces from the same source.
func (d *Directory) PNAnnounceReceived(announce Announce, compact bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if compact {
		d.compactList(&d.pnAnnounces, announce.SourceHash)
	}

	d.pnAnnounces = append([]Announce{announce}, d.pnAnnounces...)
	d.cleanList(&d.pnAnnounces)
}

// Entries returns all directory entries.
func (d *Directory) Entries() []*Entry {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]*Entry, 0, len(d.entries))
	for _, entry := range d.entries {
		result = append(result, entry)
	}
	return result
}

// Len returns the number of entries in the directory.
func (d *Directory) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}

func (d *Directory) compactList(list *[]Announce, sourceHash []byte) {
	compacted := (*list)[:0]
	for _, a := range *list {
		if !bytesEqual(a.SourceHash, sourceHash) {
			compacted = append(compacted, a)
		}
	}
	*list = compacted
}

func (d *Directory) cleanList(list *[]Announce) {
	for len(*list) > AnnounceStreamMaxLen {
		*list = (*list)[:len(*list)-1]
	}
}

func (d *Directory) removeAnnounceFromList(list *[]Announce, timestamp float64) {
	for i, a := range *list {
		if a.Timestamp == timestamp {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return
		}
	}
}

func shouldSwap(a, b *Entry) bool {
	// Sort rank: nil goes to the end
	if a.SortRank == nil && b.SortRank != nil {
		return false
	}
	if a.SortRank != nil && b.SortRank == nil {
		return true
	}
	if a.SortRank != nil && b.SortRank != nil {
		if *a.SortRank != *b.SortRank {
			return *a.SortRank > *b.SortRank
		}
	}

	// Trust level descending
	if a.TrustLevel != b.TrustLevel {
		return a.TrustLevel < b.TrustLevel
	}

	// Display name ascending
	return a.DisplayName > b.DisplayName
}

func hexKey(hash []byte) string {
	return hexString(hash)
}

func hexString(hash []byte) string {
	const hexDigits = "0123456789abcdef"
	buf := make([]byte, len(hash)*2)
	for i, b := range hash {
		buf[i*2] = hexDigits[b>>4]
		buf[i*2+1] = hexDigits[b&0x0f]
	}
	return string(buf)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// NumberOfKnownNodes returns the count of directory entries that host a node.
func (d *Directory) NumberOfKnownNodes() int {
	return len(d.KnownNodes())
}

// NumberOfKnownPeers returns the count of unique source hashes seen in the
// announce stream. When lookbackSeconds is non-nil, only announces newer than
// (now - lookbackSeconds) are counted, matching the Python
// number_of_known_peers lookback behavior.
func (d *Directory) NumberOfKnownPeers(lookbackSeconds *float64) int {
	stream := d.AnnounceStream()
	seen := make(map[string]bool, len(stream))
	now := float64(0)
	if lookbackSeconds != nil {
		now = float64(time.Now().UnixNano()) / 1e9
	}
	for _, a := range stream {
		if lookbackSeconds != nil {
			cutoff := now - *lookbackSeconds
			if a.Timestamp <= cutoff {
				continue
			}
		}
		seen[hexKey(a.SourceHash)] = true
	}
	return len(seen)
}

// SortRank returns the configured sort rank for a source hash, or nil when
// the entry is absent or has no sort rank set.
func (d *Directory) SortRank(sourceHash []byte) *int {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := hexKey(sourceHash)
	entry, ok := d.entries[key]
	if !ok {
		return nil
	}
	return entry.SortRank
}
