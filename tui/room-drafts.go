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

// RoomDrafts stores unsent message drafts per hub+room.
// Matches Python's _room_drafts dict with _draft_key(hub, room).
type RoomDrafts struct {
	drafts map[string]string
}

// NewRoomDrafts creates a new empty drafts store.
func NewRoomDrafts() *RoomDrafts {
	return &RoomDrafts{drafts: make(map[string]string)}
}

// draftKey builds the map key from hub identifier and room name.
func draftKey(hub, room string) string {
	return hub + "/" + room
}

// Save stores a draft for the given hub+room. Empty text removes the draft.
func (rd *RoomDrafts) Save(hub, room, text string) {
	key := draftKey(hub, room)
	if text == "" {
		delete(rd.drafts, key)
		return
	}
	rd.drafts[key] = text
}

// Restore returns the saved draft for the given hub+room, or empty string.
func (rd *RoomDrafts) Restore(hub, room string) string {
	return rd.drafts[draftKey(hub, room)]
}

// Remove deletes the draft for the given hub+room.
func (rd *RoomDrafts) Remove(hub, room string) {
	delete(rd.drafts, draftKey(hub, room))
}
