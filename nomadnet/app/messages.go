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
	"path/filepath"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
)

// ConversationMessages returns display data for every message in the
// conversation identified by the peer's hex source hash, mirroring the data
// Python's ConversationWidget loads from nomadnet.Conversation(source_hash)
// (Conversations.py:1888-1894). The conversation is loaded (or created) from
// the conversation cache, stamped with the app's RNS transport so its
// messages' LXMF envelopes are parsed, rescanned from disk, and returned as
// display data sorted oldest-first (Python message_widgets reverse=False).
//
// Returns nil when no conversation path is configured or the conversation has
// no messages. The TUI wiring layer (cmd/gonomadnet/textui.go) maps each
// entry onto its ConversationMessage for the LXMessageWidget header/body.
func (a *App) ConversationMessages(sourceHash string) []conversation.MessageDisplayData {
	if a.ConversationPath == "" {
		return nil
	}
	conv := a.ConversationCache.Get(sourceHash)
	if conv == nil {
		conv = conversation.NewConversation(sourceHash, filepath.Join(a.ConversationPath, sourceHash))
		a.ConversationCache.Store(conv)
	}
	if a.Transport != nil {
		conv.SetTransport(a.Transport)
	}
	if err := conv.ScanStorage(); err != nil {
		return nil
	}
	return conv.DisplayMessages()
}