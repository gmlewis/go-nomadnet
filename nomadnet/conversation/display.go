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

package conversation

import "sort"

// MessageDisplayData is the pure-data view of a single conversation message
// for the UI layer: the LXMF wire fields the LXMessageWidget header needs
// (raw LXMF state int, method, sender source hash, transport encryption,
// signature status, attachments) plus the title/content/timestamp for the
// body. It carries no RNS/lxmf types so the TUI wiring layer can map it onto
// its own ConversationMessage without importing the conversation package's
// dependencies.
//
// The State field is the RAW lxmf state int (lxm.State, e.g. 0x04 SENT,
// 0x08 DELIVERED, 0xFF FAILED) — NOT the mapped conversation.MessageState —
// because the Python LXMessageWidget compares against the raw LXMF constants
// (Conversations.py:2609-2626). The wiring layer passes it straight through.
type MessageDisplayData struct {
	Content              string
	Title                string
	Timestamp            float64 // message timestamp (get_timestamp)
	SortTimestamp        float64 // file-mtime-derived sort key
	State                int     // raw LXMF state int (lxm.State)
	Method               int     // raw LXMF method int (lxm.Method)
	SourceHash           []byte  // sender LXMF hash (lxm.SourceHash)
	Hash                 []byte  // LXMF message hash (lxm.Hash) — locates the extracted attachment dir
	TransportEncrypted   bool
	SignatureValidated   bool
	SignatureDescription string
	HasAttachments       bool
	AttachmentTypes      []string
	AttachmentNames      []string
}

// DisplayMessages returns display data for every message in the conversation,
// sorted ascending by sort_timestamp (oldest first). This mirrors Python
// ConversationWidget.update_message_widgets
// (Conversations.py:2281-2284), which sorts message_widgets by
// sort_timestamp with reverse=False so the IndicativeListBox shows oldest at
// the top and the newest (focus position = len-1) at the bottom.
//
// Each message is Loaded (parsing its LXMF envelope via the conversation's
// transport when set) so the parsed fields are populated. Messages are left
// loaded; callers that want to release memory may Unload individually.
func (c *Conversation) DisplayMessages() []MessageDisplayData {
	msgs := make([]*Message, len(c.Messages))
	copy(msgs, c.Messages)
	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].SortTimestamp < msgs[j].SortTimestamp
	})

	out := make([]MessageDisplayData, 0, len(msgs))
	for _, m := range msgs {
		// Match Python's lazy-load: when the index already supplied the
		// cached state (restore_from_index set _cached_state), Python's
		// get_state/get_title/get_content return the cached values WITHOUT
		// loading from disk (Conversation.py:592-597, 2281-2284). Only load
		// from disk when the index lacks the state. Loading unconditionally
		// would overwrite the index-restored raw state with the on-disk
		// container state, which can differ (the index state is the value
		// captured at first load; the on-disk state may have been updated by
		// the router since) — causing header glyph divergence from nomadnet.
		if m.CachedState == nil {
			m.Load()
		}
		d := MessageDisplayData{
			Content:              m.GetContent(),
			Title:                m.GetTitle(),
			Timestamp:            m.GetTimestamp(),
			SortTimestamp:        m.SortTimestamp,
			State:                m.CachedRawState,
			Method:               m.CachedMethod,
			SourceHash:           m.CachedSourceHash,
			Hash:                 m.GetHash(),
			TransportEncrypted:   m.GetTransportEncrypted(),
			SignatureValidated:   m.SignatureValidated(),
			SignatureDescription: m.GetSignatureDescription(),
			HasAttachments:       m.HasAttachments(),
		}
		for _, a := range m.CachedAttachmentNames {
			d.AttachmentTypes = append(d.AttachmentTypes, a.Type)
			d.AttachmentNames = append(d.AttachmentNames, a.Name)
		}
		out = append(out, d)
	}
	return out
}
