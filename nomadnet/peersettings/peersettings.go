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

// Package peersettings manages NomadNet peer settings stored in msgpack format.
//
// Peer settings include the display name, announce interval, propagation
// node selection, sync state, and serving statistics.
package peersettings

import (
	"fmt"
	"os"

	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// Settings holds local peer configuration and state.
//
// The msgpack key for each field (the Python NomadNet save_peer_settings dict
// key) is shown in the doc comment; the on-disk map is written in this
// declaration order so the bytes match Python's insertion-ordered dict
// serialization exactly.
type Settings struct {
	// display_name
	DisplayName string
	// announce_interval
	AnnounceInterval int
	// last_announce (a float timestamp, or nil)
	LastAnnounce any
	// node_last_announce (a 16-byte announce hash, or nil)
	NodeLastAnnounce any
	// propagation_node (a 16-byte destination hash, or nil)
	PropagationNode any
	// last_lxmf_sync
	LastLXMFSync int
	// node_connects
	NodeConnects int
	// served_page_requests
	ServedPageRequests int
	// served_file_requests
	ServedFileRequests int
}

// DefaultSettings returns a new Settings with default values.
func DefaultSettings(announceInterval int) *Settings {
	return &Settings{
		DisplayName:        "Anonymous Peer",
		AnnounceInterval:   announceInterval,
		LastAnnounce:       nil,
		NodeLastAnnounce:   nil,
		PropagationNode:    nil,
		LastLXMFSync:       0,
		NodeConnects:       0,
		ServedPageRequests: 0,
		ServedFileRequests: 0,
	}
}

// Load reads peer settings from a msgpack file.
// If the file doesn't exist, it returns default settings.
func Load(path string, announceInterval int) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(announceInterval), nil
		}
		return nil, fmt.Errorf("reading peer settings: %w", err)
	}

	raw, err := rnsmsgpack.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("decoding peer settings: %w", err)
	}
	m, ok := raw.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("decoding peer settings: expected msgpack map, got %T", raw)
	}

	s := &Settings{
		DisplayName:        peerStr(m, "display_name"),
		LastLXMFSync:       peerInt(m, "last_lxmf_sync"),
		NodeConnects:       peerInt(m, "node_connects"),
		ServedPageRequests: peerInt(m, "served_page_requests"),
		ServedFileRequests: peerInt(m, "served_file_requests"),
		LastAnnounce:       m["last_announce"],
		NodeLastAnnounce:   m["node_last_announce"],
		PropagationNode:    m["propagation_node"],
	}

	// announce_interval is always driven by the loaded config, not the file.
	s.AnnounceInterval = announceInterval
	return s, nil
}

// Save writes peer settings to a msgpack file atomically. The output is
// byte-identical to Python NomadNet's save_peer_settings: an insertion-ordered
// msgpack map (via OrderedMap.MarshalMsgpack) whose integer fields use the
// unsigned encodings umsgpack picks for non-negative values (fixint / uint16 /
// uint32), so the int fields are packed as uint.
func Save(s *Settings, path string) error {
	m := rnsmsgpack.OrderedMap{
		{Key: "display_name", Value: s.DisplayName},
		{Key: "announce_interval", Value: uint(s.AnnounceInterval)},
		{Key: "last_announce", Value: s.LastAnnounce},
		{Key: "node_last_announce", Value: s.NodeLastAnnounce},
		{Key: "propagation_node", Value: s.PropagationNode},
		{Key: "last_lxmf_sync", Value: uint(s.LastLXMFSync)},
		{Key: "node_connects", Value: uint(s.NodeConnects)},
		{Key: "served_page_requests", Value: uint(s.ServedPageRequests)},
		{Key: "served_file_requests", Value: uint(s.ServedFileRequests)},
	}
	data, err := rnsmsgpack.Pack(m)
	if err != nil {
		return fmt.Errorf("encoding peer settings: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing peer settings: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming peer settings: %w", err)
	}

	return nil
}

// peerStr returns the string value of a msgpack map key, or "" if absent.
func peerStr(m map[any]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// peerInt returns the int value of a msgpack map key. umsgpack round-trips
// small positive ints as int64 (fixint/signed encodings) and larger
// non-negative ints as uint64 (uint16/uint32 encodings), so both are handled.
func peerInt(m map[any]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case uint64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
