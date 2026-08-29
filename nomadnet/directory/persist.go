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

package directory

import (
	"os"

	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// SaveToDisk serializes the directory entries and announce stream to path in
// the msgpack format used by the Python NomadNet save_to_disk: an
// insertion-ordered map with an "entry_list" of per-entry tuples and an
// "announce_stream" of announce tuples. The output is byte-identical to
// Python's save_to_disk: entries are emitted in insertion order (via
// entryOrder, matching Python's insertion-ordered dict), the top-level map keys
// are written "entry_list" then "announce_stream" (via OrderedMap, since a Go
// map iterates in random order), and the integer fields (trust_level,
// preferred_delivery, sort_rank) are packed as unsigned so non-negative values
// use the same fixint/uint8/uint16/uint32 encodings Python's msgpack picks
// (notably TrustTrusted=0xFF -> uint8 0xcc 0xff, not int16).
func (d *Directory) SaveToDisk(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entryList := make([]any, 0, len(d.entryOrder))
	for _, key := range d.entryOrder {
		if e, ok := d.entries[key]; ok {
			entryList = append(entryList, entryToTuple(e))
		}
	}

	stream := d.nodeAnnounces
	stream = append(stream, d.peerAnnounces...)
	stream = append(stream, d.pnAnnounces...)
	announceList := make([]any, 0, len(stream))
	for _, a := range stream {
		announceList = append(announceList, []any{a.Timestamp, a.SourceHash, a.AppData, a.AnnounceType})
	}

	directory := rnsmsgpack.OrderedMap{
		{Key: "entry_list", Value: entryList},
		{Key: "announce_stream", Value: announceList},
	}

	data, err := rnsmsgpack.Pack(directory)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// entryToTuple builds the msgpack tuple for a directory entry, matching the
// Python NomadNet save order: source_hash, display_name, trust_level,
// hosts_node, preferred_delivery, identify, sort_rank, notes. trust_level and
// preferred_delivery are widened to uint so their msgpack encoding matches
// Python's unsigned encoding for non-negative values (a Go int would pack as
// signed int8/int16/int32 and diverge, e.g. TrustTrusted=0xFF).
func entryToTuple(e *Entry) []any {
	var sortRank any
	if e.SortRank != nil {
		sortRank = uint(*e.SortRank)
	}
	return []any{
		e.SourceHash,
		e.DisplayName,
		uint(e.TrustLevel),
		e.HostsNode,
		uint(e.PreferredDelivery),
		e.IdentifyOnConnect,
		sortRank,
		e.Notes,
	}
}

// LoadFromDisk reads a directory file written by SaveToDisk (or by the Python
// NomadNet) and restores entries and the announce stream, mirroring the Python
// NomadNet load_from_disk. entryOrder is rebuilt from the file's entry_list
// order so a subsequent SaveToDisk re-emits the entries in the same order
// (matching Python, whose insertion-ordered dict preserves file order across a
// load/save cycle).
func (d *Directory) LoadFromDisk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	raw, err := rnsmsgpack.Unpack(data)
	if err != nil {
		return err
	}
	m, ok := raw.(map[any]any)
	if !ok {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.entries = make(map[string]*Entry)
	d.entryOrder = d.entryOrder[:0]
	if entryList, ok := m["entry_list"].([]any); ok {
		for _, item := range entryList {
			tuple, ok := item.([]any)
			if !ok || len(tuple) < 3 {
				continue
			}
			sourceHash, _ := tuple[0].([]byte)
			if len(sourceHash) == 0 {
				continue
			}
			// Python's load_from_disk (Directory.py:116-117) substitutes
			// "Undefined" ONLY for a nil display_name (a saved Python None):
			// an empty-string display name is kept as-is, so the announced
			// app-data name fallback (Conversation.py:151-152) still applies
			// to it after a restart instead of the row reading "Undefined".
			displayName := "Undefined"
			if s, ok := tuple[1].(string); ok {
				displayName = s
			}
			trustLevel := byte(TrustUnknown)
			if v, ok := toInt(tuple[2]); ok {
				trustLevel = byte(v)
			}
			entry := &Entry{
				SourceHash:  sourceHash,
				DisplayName: displayName,
				TrustLevel:  trustLevel,
			}
			if len(tuple) > 3 {
				entry.HostsNode, _ = tuple[3].(bool)
			}
			if len(tuple) > 4 {
				if v, ok := toInt(tuple[4]); ok {
					entry.PreferredDelivery = byte(v)
				} else {
					entry.PreferredDelivery = DeliveryDirect
				}
			} else {
				entry.PreferredDelivery = DeliveryDirect
			}
			if len(tuple) > 5 {
				entry.IdentifyOnConnect, _ = tuple[5].(bool)
			}
			if len(tuple) > 6 {
				if v, ok := toInt(tuple[6]); ok {
					r := v
					entry.SortRank = &r
				}
			}
			if len(tuple) > 7 {
				if n, ok := tuple[7].(string); ok {
					entry.Notes = n
				}
			}
			key := hexKey(sourceHash)
			d.entries[key] = entry
			d.entryOrder = append(d.entryOrder, key)
		}
	}

	d.nodeAnnounces = nil
	d.peerAnnounces = nil
	d.pnAnnounces = nil
	if stream, ok := m["announce_stream"].([]any); ok {
		for _, item := range stream {
			tuple, ok := item.([]any)
			if !ok || len(tuple) < 4 {
				continue
			}
			a := Announce{}
			if f, ok := tuple[0].(float64); ok {
				a.Timestamp = f
			}
			if b, ok := tuple[1].([]byte); ok {
				a.SourceHash = b
			}
			if b, ok := tuple[2].([]byte); ok {
				a.AppData = b
			}
			if s, ok := tuple[3].(string); ok {
				a.AnnounceType = s
			}
			switch a.AnnounceType {
			case "node":
				d.nodeAnnounces = append(d.nodeAnnounces, a)
			case "peer":
				d.peerAnnounces = append(d.peerAnnounces, a)
			case "pn":
				d.pnAnnounces = append(d.pnAnnounces, a)
			}
		}
	}

	return nil
}

// toInt extracts an int from a msgpack-decoded value (int, int64, uint64,
// float64). Returns false when the value is not numeric.
func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int8:
		return int(val), true
	case int16:
		return int(val), true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case uint:
		return int(val), true
	case uint8:
		return int(val), true
	case uint16:
		return int(val), true
	case uint32:
		return int(val), true
	case uint64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}
