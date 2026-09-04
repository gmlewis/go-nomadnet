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

package rrc

import (
	"strings"
	"testing"
)

// Hub limits enforcement: real hubs silently drop envelopes whose nickname
// exceeds the WELCOME max_nick_bytes, so over-long-named Go clients
// registered as bare hashes in the hub's member list (observed live). The
// effective nick is truncated rune-safely to the active limit (defaulting to
// 32 bytes before the first WELCOME) in every outgoing envelope.

// TestTruncateUTF8 pins the rune-safe byte truncation helper.
func TestTruncateUTF8(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{"short ascii untouched", "nick", 32, "nick"},
		{"exact fit", strings.Repeat("a", 32), 32, strings.Repeat("a", 32)},
		{"ascii cut", strings.Repeat("a", 40), 32, strings.Repeat("a", 32)},
		{"partial emoji dropped", strings.Repeat("a", 31) + "🎉", 32, strings.Repeat("a", 31)},
		{"whole emoji kept", "a🎉b", 32, "a🎉b"},
		{"zero limit", "nick", 0, ""},
		{"negative limit", "nick", -1, ""},
		{"limit over length", "nick", 32, "nick"},
		{"invalid bytes dropped", "ok\x80\x81tail", 32, "oktail"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := truncateUTF8(tc.in, tc.maxBytes)
			if got != tc.want {
				t.Errorf("truncateUTF8(%q, %v) = %q, want %q", tc.in, tc.maxBytes, got, tc.want)
			}
			if len(got) > tc.maxBytes && tc.maxBytes >= 0 {
				t.Errorf("result %q is %v bytes, want <= %v", got, len(got), tc.maxBytes)
			}
		})
	}
}

// TestSendHelloTruncatesOverLongNick pins the live bug: a display name longer
// than the 32-byte default limit must leave the HELLO envelope within the
// limit, or the hub registers the client as a bare hash.
func TestSendHelloTruncatesOverLongNick(t *testing.T) {
	t.Parallel()

	longNick := strings.Repeat("n", 40)
	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname(longNick)
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	captured := &[]map[any]any{}
	hub.onSend = func(env map[any]any) { *captured = append(*captured, env) }

	hub.sendHello(nil)

	hellos := filterByType(*captured, TypeHello)
	if len(hellos) != 1 {
		t.Fatalf("sent %v hello envelopes, want 1", len(hellos))
	}
	nick := byteVal(hellos[0], KeyNick)
	if len(nick) != 32 {
		t.Errorf("HELLO nick = %v bytes, want truncated to 32 (the default limit before WELCOME)", len(nick))
	}
	if string(nick) != strings.Repeat("n", 32) {
		t.Errorf("HELLO nick = %q, want the 32-byte prefix", string(nick))
	}
}

// TestWelcomeLimitTruncatesNickToReceivedValue pins the local enforcement of
// the WELCOME-provided limit: a hub advertising 48 bytes truncates to 48.
func TestWelcomeLimitTruncatesNickToReceivedValue(t *testing.T) {
	t.Parallel()

	longNick := strings.Repeat("w", 60)
	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname(longNick)
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	hub.handleWelcome(map[any]any{
		BWelcomeLimits: map[any]any{LMaxNickBytes: 48},
	})

	captured := &[]map[any]any{}
	hub.onSend = func(env map[any]any) { *captured = append(*captured, env) }

	hub.SendMessage("general", "hello")

	msgs := filterByType(*captured, TypeMsg)
	if len(msgs) != 1 {
		t.Fatalf("sent %v msg envelopes, want 1", len(msgs))
	}
	nick := byteVal(msgs[0], KeyNick)
	if len(nick) != 48 {
		t.Errorf("MSG nick = %v bytes, want truncated to the hub's 48-byte limit", len(nick))
	}
}

// TestOverLongNickWireEnvelope pins the requirement directly on the wire
// encoding: the CBOR envelope of a >32-byte-nick send carries a <=32-byte
// nick, valid UTF-8.
func TestOverLongNickWireEnvelope(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	// A 40-byte nick with a multi-byte rune straddling the 32-byte boundary.
	mgr.SetNickname(strings.Repeat("é", 20)) // 40 UTF-8 bytes
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	captured := &[]map[any]any{}
	hub.onSend = func(env map[any]any) { *captured = append(*captured, env) }

	hub.JoinRoom("general", false)

	joins := filterByType(*captured, TypeJoin)
	if len(joins) != 1 {
		t.Fatalf("sent %v join envelopes, want 1", len(joins))
	}
	nick := byteVal(joins[0], KeyNick)
	if len(nick) > 32 {
		t.Errorf("JOIN nick = %v bytes, want <= 32", len(nick))
	}
	if !isWellFormedUTF8(string(nick)) {
		t.Errorf("JOIN nick %q is not valid UTF-8 (rune-safe truncation required)", string(nick))
	}
}

// TestSendCommandTruncatesNick pins the truncation on the command path too.
func TestSendCommandTruncatesNick(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname(strings.Repeat("c", 40))
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	captured := &[]map[any]any{}
	hub.onSend = func(env map[any]any) { *captured = append(*captured, env) }

	if err := hub.SendCommand("/who general", "general"); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}

	msgs := filterByType(*captured, TypeMsg)
	if len(msgs) != 1 {
		t.Fatalf("sent %v msg envelopes, want 1", len(msgs))
	}
	nick := byteVal(msgs[0], KeyNick)
	if len(nick) != 32 {
		t.Errorf("command nick = %v bytes, want truncated to 32", len(nick))
	}
}

// TestSendActionTruncatesNick pins the truncation on the action path.
func TestSendActionTruncatesNick(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname(strings.Repeat("a", 40))
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	captured := &[]map[any]any{}
	hub.onSend = func(env map[any]any) { *captured = append(*captured, env) }

	hub.SendAction("general", "waves")

	actions := filterByType(*captured, TypeAction)
	if len(actions) != 1 {
		t.Fatalf("sent %v action envelopes, want 1", len(actions))
	}
	nick := byteVal(actions[0], KeyNick)
	if len(nick) != 32 {
		t.Errorf("action nick = %v bytes, want truncated to 32", len(nick))
	}
}

// TestNickLimitFallsBackTo32OnZero pins Python's `max_nick_bytes or 32`
// fallback: a hub advertising a non-positive limit keeps the 32-byte default.
func TestNickLimitFallsBackTo32OnZero(t *testing.T) {
	t.Parallel()

	_, hub := fanoutFixture(t)

	hub.handleWelcome(map[any]any{BWelcomeLimits: map[any]any{LMaxNickBytes: 0}})
	if got := hub.effectiveNickLimit(); got != 32 {
		t.Errorf("effective nick limit = %v, want the 32-byte default", got)
	}

	hub.handleWelcome(map[any]any{BWelcomeLimits: map[any]any{LMaxNickBytes: 48}})
	if got := hub.effectiveNickLimit(); got != 48 {
		t.Errorf("effective nick limit after WELCOME = %v, want 48", got)
	}
}

// isWellFormedUTF8 reports whether every rune of s decodes cleanly.
func isWellFormedUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}
