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

// TestChatLinkHandleNil pins the nil-callback contract: a handler built with no
// callbacks wired accepts every link form — the ones it knows and the ones it does not
// — without panicking. Completing is the assertion here: a nil callback invoked through
// a missing guard crashes the test.
func TestChatLinkHandleNil(t *testing.T) {
	t.Parallel()

	handler := NewChatLinkHandler(nil, nil, nil)
	for _, target := range []string{
		"",
		"room://#general",
		"lxmf://lxmf@abcdef",
		"page://abcdef",
		"rrc://abcdef/#general",
		"invalid://something",
		"notalink",
	} {
		handler.HandleLink(target)
	}
}

// TestChatLinkHandleInvalid pins that a target this handler does not recognise opens
// nothing. Python's _ChatLinkDelegate.handle_link switches on the scheme it parsed and
// has no branch for anything else (Channels.py:1151+), so an unknown scheme, a
// scheme-less string, an empty scheme and a non-chat scheme are all inert. Before this
// test asserted nothing, a regression that opened the first thing it saw would have
// gone unnoticed.
func TestChatLinkHandleInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
	}{
		{"empty target", ""},
		{"no scheme separator", "notalink"},
		{"empty scheme", "://payload"},
		{"unknown scheme", "invalid://something"},
		{"a scheme that is not part of the chat grammar", "http://example.com"},
		{"a scheme-prefixed empty payload", "room://"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var opened []string
			handler := NewChatLinkHandler(
				func(room string) { opened = append(opened, "room:"+room) },
				func(hash string) { opened = append(opened, "lxmf:"+hash) },
				func(url string) { opened = append(opened, "page:"+url) },
			)
			// Every callback this handler can dispatch through is wired, so a target
			// that reached ANY branch would be recorded — including the rrc extension,
			// whose callback is an exported field rather than a constructor argument.
			handler.OnOpenRRC = func(payload string) { opened = append(opened, "rrc:"+payload) }
			handler.HandleLink(tc.target)

			if len(opened) != 0 {
				t.Errorf("HandleLink(%q) opened %v, want nothing", tc.target, opened)
			}
		})
	}
}

// TestChatLinkHandleKnownFormsStillOpen is the complement of the rejection test: the
// schemes the handler does know must still open, so a fix that made the rejection
// blanket would not pass as "more correct".
func TestChatLinkHandleKnownFormsStillOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"room link", "room://#general", "room:general"},
		{"lxmf link", "lxmf://lxmf@abcdef", "lxmf:abcdef"},
		{"page link", "page://abcdef", "page:abcdef"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var opened []string
			handler := NewChatLinkHandler(
				func(room string) { opened = append(opened, "room:"+room) },
				func(hash string) { opened = append(opened, "lxmf:"+hash) },
				func(url string) { opened = append(opened, "page:"+url) },
			)
			handler.OnOpenRRC = func(payload string) { opened = append(opened, "rrc:"+payload) }
			handler.HandleLink(tc.target)

			if len(opened) != 1 || opened[0] != tc.want {
				t.Errorf("HandleLink(%q) opened %v, want [%v]", tc.target, opened, tc.want)
			}
		})
	}
}

// TestChatLinkHandleRRC pins the one form beyond Python's chat grammar: an rrc://
// link hands its payload on unparsed, because the browser's own ParseRRCLink →
// HandleRRCLink already own that link and one parser serves both.
func TestChatLinkHandleRRC(t *testing.T) {
	t.Parallel()

	var openedRRC string
	handler := NewChatLinkHandler(nil, nil, nil)
	handler.OnOpenRRC = func(payload string) { openedRRC = payload }

	const hub = "a012129c10205c0b9441fcd2b755b2a7"
	handler.HandleLink("rrc://" + hub + "/general")
	if want := hub + "/general"; openedRRC != want {
		t.Errorf("rrc link opened %q, want %q", openedRRC, want)
	}

	// A handler without the callback leaves the link inert rather than panicking.
	inert := NewChatLinkHandler(nil, nil, nil)
	inert.HandleLink("rrc://" + hub)
}
