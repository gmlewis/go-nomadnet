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

import "sort"

// SortChannelMessages orders a room's message list oldest→newest by the
// message timestamp, mirroring the chronological rendering Python's
// RoomWidget.update_messages produces (Channels.py:758-793 — the hub buffer is
// arrival-ordered and rrcd replays the join history before any live traffic).
// The sort is STABLE: messages whose timestamps tie (history replayed with a
// shared hub timestamp, messages stamped on arrival) keep their buffer order,
// so a late-arriving replay interleaves without scrambling same-second
// traffic. Zero timestamps (locally appended notices without a hub stamp)
// stay in place relative to their neighbors only when every timestamp is
// zero; callers that mix them should stamp them with the local time first.
func SortChannelMessages(msgs []ChannelMessage) {
	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].TsMs < msgs[j].TsMs
	})
}
