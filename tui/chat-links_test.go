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
	"testing"
)

func TestChatLinkHandleRoom(t *testing.T) {
	t.Parallel()

	var openedRoom string
	handler := NewChatLinkHandler(
		func(room string) { openedRoom = room },
		nil,
		nil,
	)

	handler.HandleLink("room://general")

	if openedRoom != "general" {
		t.Errorf("openedRoom = %q, want %q", openedRoom, "general")
	}
}

func TestChatLinkHandleRoomWithHash(t *testing.T) {
	t.Parallel()

	var openedRoom string
	handler := NewChatLinkHandler(
		func(room string) { openedRoom = room },
		nil,
		nil,
	)

	handler.HandleLink("room://#random")

	if openedRoom != "random" {
		t.Errorf("openedRoom = %q, want %q", openedRoom, "random")
	}
}

func TestChatLinkHandleLXMF(t *testing.T) {
	t.Parallel()

	var openedLXMF string
	handler := NewChatLinkHandler(
		nil,
		func(hash string) { openedLXMF = hash },
		nil,
	)

	handler.HandleLink("lxmf://lxmf@aabb1122ccdd")

	if openedLXMF != "aabb1122ccdd" {
		t.Errorf("openedLXMF = %q, want %q", openedLXMF, "aabb1122ccdd")
	}
}

func TestChatLinkHandlePage(t *testing.T) {
	t.Parallel()

	var openedPage string
	handler := NewChatLinkHandler(
		nil,
		nil,
		func(url string) { openedPage = url },
	)

	handler.HandleLink("page://abc123")

	if openedPage != "abc123" {
		t.Errorf("openedPage = %q, want %q", openedPage, "abc123")
	}
}

func TestChatLinkHandlePageWithFields(t *testing.T) {
	t.Parallel()

	var openedPage string
	handler := NewChatLinkHandler(
		nil,
		nil,
		func(url string) { openedPage = url },
	)

	handler.HandleLink("page://abc123", "field1|field2")

	if openedPage == "" {
		t.Error("page link with fields should be opened")
	}
}

func TestChatLinkHandleNil(t *testing.T) {
	t.Parallel()

	handler := NewChatLinkHandler(nil, nil, nil)
	handler.HandleLink("")
}

func TestChatLinkHandleInvalid(t *testing.T) {
	t.Parallel()

	handler := NewChatLinkHandler(nil, nil, nil)
	handler.HandleLink("invalid://something")
}
