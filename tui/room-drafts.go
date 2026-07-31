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

package tui

// draftKey is the composite key for a room draft, matching Python's
// _draft_key tuple (hub_hash, dest_name, room) at Channels.py:1500. Hubs that
// share a hub_hash but differ in dest_name are distinct keys.
type draftKey struct {
	hubHash  string
	destName string
	room     string
}

// RoomDrafts stores unsent message drafts per (hub, room), matching Python's
// _room_drafts dict with _draft_key(hub, room) (Channels.py:1506,1519).
type RoomDrafts struct {
	drafts map[draftKey]string
}

// NewRoomDrafts creates a new empty drafts store.
func NewRoomDrafts() *RoomDrafts {
	return &RoomDrafts{drafts: make(map[draftKey]string)}
}

// Save stores a draft for the given (hubHash, destName, room). Empty text
// removes the draft, matching Python's _save_room_draft which pops the key
// when the editor text is falsy.
func (rd *RoomDrafts) Save(hubHash, destName, room, text string) {
	key := draftKey{hubHash, destName, room}
	if text == "" {
		delete(rd.drafts, key)
		return
	}
	rd.drafts[key] = text
}

// Restore returns the saved draft for (hubHash, destName, room), or empty
// string, matching Python's _room_drafts.get(key) (None -> empty).
func (rd *RoomDrafts) Restore(hubHash, destName, room string) string {
	return rd.drafts[draftKey{hubHash, destName, room}]
}

// Remove deletes the draft for (hubHash, destName, room).
func (rd *RoomDrafts) Remove(hubHash, destName, room string) {
	delete(rd.drafts, draftKey{hubHash, destName, room})
}
