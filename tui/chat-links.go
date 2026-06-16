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
	"strings"
)

// ChatLinkHandler dispatches links clicked in RRC chat messages.
// Matches Python's _ChatLinkDelegate.handle_link() at Channels.py:1151.
type ChatLinkHandler struct {
	onOpenRoom func(room string)
	onOpenLXMF func(hash string)
	onOpenPage func(url string)
}

// NewChatLinkHandler creates a link handler with the given callbacks.
func NewChatLinkHandler(
	onOpenRoom func(room string),
	onOpenLXMF func(hash string),
	onOpenPage func(url string),
) *ChatLinkHandler {
	return &ChatLinkHandler{
		onOpenRoom: onOpenRoom,
		onOpenLXMF: onOpenLXMF,
		onOpenPage: onOpenPage,
	}
}

// HandleLink parses and dispatches a link target from chat messages.
// Supported formats: room://#name, lxmf://lxmf@hash, page://hash.
// Matches Python's _ChatLinkDelegate.handle_link() at Channels.py:1151.
func (h *ChatLinkHandler) HandleLink(target string, fields ...string) {
	if target == "" {
		return
	}
	parts := strings.SplitN(target, "://", 2)
	if len(parts) < 2 {
		return
	}
	kind := parts[0]
	payload := parts[1]

	switch kind {
	case "room":
		room := strings.TrimLeft(payload, "#")
		if h.onOpenRoom != nil && room != "" {
			h.onOpenRoom(room)
		}
	case "lxmf":
		hash := strings.TrimLeft(payload, "lxmf@")
		if h.onOpenLXMF != nil && hash != "" {
			h.onOpenLXMF(hash)
		}
	case "page":
		url := payload
		if len(fields) > 0 && fields[0] != "" {
			url += "`" + fields[0]
		}
		if h.onOpenPage != nil {
			h.onOpenPage(url)
		}
	}
}
