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

	"github.com/vmihailenco/msgpack/v5"
)

// Settings holds local peer configuration and state.
type Settings struct {
	DisplayName         string `msgpack:"display_name"`
	AnnounceInterval    int    `msgpack:"announce_interval"`
	LastAnnounce        any    `msgpack:"last_announce"`
	NodeLastAnnounce    any    `msgpack:"node_last_announce"`
	PropagationNode     any    `msgpack:"propagation_node"`
	LastLXMFSync        int    `msgpack:"last_lxmf_sync"`
	NodeConnects        int    `msgpack:"node_connects"`
	ServedPageRequests  int    `msgpack:"served_page_requests"`
	ServedFileRequests  int    `msgpack:"served_file_requests"`
}

// DefaultSettings returns a new Settings with default values.
func DefaultSettings(announceInterval int) *Settings {
	return &Settings{
		DisplayName:      "Anonymous Peer",
		AnnounceInterval: announceInterval,
		LastAnnounce:     nil,
		NodeLastAnnounce: nil,
		PropagationNode:  nil,
		LastLXMFSync:     0,
		NodeConnects:     0,
		ServedPageRequests:  0,
		ServedFileRequests:  0,
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

	s := &Settings{}
	if err := msgpack.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("decoding peer settings: %w", err)
	}

	// Ensure required fields have defaults
	if s.NodeLastAnnounce == nil {
		s.NodeLastAnnounce = nil
	}
	if s.PropagationNode == nil {
		s.PropagationNode = nil
	}

	s.AnnounceInterval = announceInterval
	return s, nil
}

// Save writes peer settings to a msgpack file atomically.
func Save(s *Settings, path string) error {
	data, err := msgpack.Marshal(s)
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
