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

import (
	"reflect"
	"testing"
)

func TestSortChannelMessagesChronological(t *testing.T) {
	t.Parallel()

	// The Go room buffer used to render newest-first (TODO items 1-3): the
	// render adapter must order oldest→newest like Python's RoomWidget.
	msgs := []ChannelMessage{
		{Text: "Message 6", TsMs: 6000},
		{Text: "Message 4", TsMs: 4000},
		{Text: "Message 2", TsMs: 2000},
		{Text: "Message 5", TsMs: 5000},
		{Text: "Message 1", TsMs: 1000},
		{Text: "Message 3", TsMs: 3000},
	}
	SortChannelMessages(msgs)
	var got []string
	for _, m := range msgs {
		got = append(got, m.Text)
	}
	want := []string{"Message 1", "Message 2", "Message 3", "Message 4", "Message 5", "Message 6"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted order = %v, want %v", got, want)
	}
}

func TestSortChannelMessagesStableOnTies(t *testing.T) {
	t.Parallel()

	// History replay: rrcd replays past messages on join, all carrying the
	// hub's original (or shared) timestamps. Equal timestamps must keep their
	// arrival order — the replay must not scramble same-second traffic.
	msgs := []ChannelMessage{
		{Text: "live", TsMs: 5000},
		{Text: "replay-1", TsMs: 1000},
		{Text: "replay-2", TsMs: 1000},
		{Text: "replay-3", TsMs: 1000},
	}
	SortChannelMessages(msgs)
	var got []string
	for _, m := range msgs {
		got = append(got, m.Text)
	}
	want := []string{"replay-1", "replay-2", "replay-3", "live"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted order = %v, want %v", got, want)
	}
}

func TestSortChannelMessagesEmptyAndNil(t *testing.T) {
	t.Parallel()

	var nilMsgs []ChannelMessage
	SortChannelMessages(nilMsgs) // must not panic
	empty := []ChannelMessage{}
	SortChannelMessages(empty)
	if len(empty) != 0 {
		t.Errorf("empty slice changed length: %v", len(empty))
	}
}
