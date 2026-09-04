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
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// Wire-encoding compliance against doc 3-RRC: the envelope field keys 0-7,
// the message type numbers, the protocol version, the 8-byte random message
// id, the TEXT-vs-BYTE-string split, and the forward-compatibility rules
// (unknown envelope keys, unknown body keys and unknown message types must be
// ignored without error).

// TestEnvelopeFieldKeys pins doc 3-RRC's envelope field keys 0-7.
func TestEnvelopeFieldKeys(t *testing.T) {
	t.Parallel()

	want := map[string]int{
		"KeyVersion":   0,
		"KeyType":      1,
		"KeyMessageID": 2,
		"KeyTimestamp": 3,
		"KeySource":    4,
		"KeyRoom":      5,
		"KeyBody":      6,
		"KeyNick":      7,
	}
	got := map[string]int{
		"KeyVersion":   KeyVersion,
		"KeyType":      KeyType,
		"KeyMessageID": KeyMessageID,
		"KeyTimestamp": KeyTimestamp,
		"KeySource":    KeySource,
		"KeyRoom":      KeyRoom,
		"KeyBody":      KeyBody,
		"KeyNick":      KeyNick,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%v = %v, want %v (doc 3-RRC field keys)", name, got[name], w)
		}
	}
}

// TestMessageTypeAssignments pins doc 3-RRC's message type numbers.
func TestMessageTypeAssignments(t *testing.T) {
	t.Parallel()

	want := map[string]int{
		"TypeHello":   1,
		"TypeWelcome": 2,
		"TypeJoin":    10,
		"TypeJoined":  11,
		"TypePart":    12,
		"TypeParted":  13,
		"TypeMsg":     20,
		"TypeNotice":  21,
		"TypeAction":  22,
		"TypePing":    30,
		"TypePong":    31,
		"TypeError":   40,
	}
	got := map[string]int{
		"TypeHello":   TypeHello,
		"TypeWelcome": TypeWelcome,
		"TypeJoin":    TypeJoin,
		"TypeJoined":  TypeJoined,
		"TypePart":    TypePart,
		"TypeParted":  TypeParted,
		"TypeMsg":     TypeMsg,
		"TypeNotice":  TypeNotice,
		"TypeAction":  TypeAction,
		"TypePing":    TypePing,
		"TypePong":    TypePong,
		"TypeError":   TypeError,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%v = %v, want %v (doc 3-RRC message types)", name, got[name], w)
		}
	}
}

// TestProtocolVersionAndResourceEnvelope pins RRCVersion=1 (doc 3-RRC Field
// 0) and the resource-envelope extension type at the Python SOT's value 50:
// doc 3-RRC reserves 0-63 for core types and assigns extensions 64+, but the
// Python nomadnet SOT (T_RESOURCE_ENVELOPE, RRC.py:77) and the rrcd hub both
// use 50, and wire parity with the SOT wins.
func TestProtocolVersionAndResourceEnvelope(t *testing.T) {
	t.Parallel()

	if RRCVersion != 1 {
		t.Errorf("RRCVersion = %v, want 1 (doc 3-RRC Field 0)", RRCVersion)
	}
	if TypeResourceEnvelope != 50 {
		t.Errorf("TypeResourceEnvelope = %v, want 50 (Python SOT T_RESOURCE_ENVELOPE)", TypeResourceEnvelope)
	}
}

// TestMsgIDIs8RandomBytes pins doc 3-RRC Field 2: an 8-byte random message
// id, distinct across calls.
func TestMsgIDIs8RandomBytes(t *testing.T) {
	t.Parallel()

	a, b := MsgID(), MsgID()
	if len(a) != 8 || len(b) != 8 {
		t.Fatalf("MsgID lengths = %v, %v, want 8, 8", len(a), len(b))
	}
	if string(a) == string(b) {
		t.Error("two MsgID calls returned identical bytes, want random distinct ids")
	}
}

// TestMakeEnvelopeTextVsByteFields pins the doc 3-RRC encoding split: room
// (Field 5) and nick (Field 7) ride as CBOR TEXT strings, while the source
// identity (Field 4) and message id (Field 2) ride as CBOR byte strings.
func TestMakeEnvelopeTextVsByteFields(t *testing.T) {
	t.Parallel()

	env := MakeEnvelope(TypeMsg, []byte("ownhash"), []byte("general"), []byte("alice"), "hi", []byte("mid1"), 1000)
	data, err := cbor.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[any]any
	if err := cbor.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	// fxamacker/cbor decodes integer map keys as uint64; the helpers
	// normalize both key forms.
	for _, key := range []int{KeyRoom, KeyNick} {
		if v, ok := decoded[uint64(key)]; ok {
			if _, isStr := v.(string); !isStr {
				t.Errorf("field %v decoded as %T, want a CBOR text string", key, v)
			}
			continue
		}
		t.Errorf("field %v missing from the decoded envelope", key)
	}
	for _, key := range []int{KeySource, KeyMessageID} {
		if v, ok := decoded[uint64(key)]; ok {
			if _, isBytes := v.([]byte); !isBytes {
				t.Errorf("field %v decoded as %T, want a CBOR byte string", key, v)
			}
			continue
		}
		t.Errorf("field %v missing from the decoded envelope", key)
	}
	if got := intVal(decoded, KeyVersion); got != 1 {
		t.Errorf("field %v = %v, want 1 (protocol version)", KeyVersion, got)
	}
}

// TestUnknownMessageTypeIgnored pins doc 3-RRC's forward-compatibility rule:
// a message with an unknown type must be ignored without error.
func TestUnknownMessageTypeIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		typ  int
	}{
		{"reserved zero", 0},
		{"core gap", 55},
		{"extension", 64},
		{"far future", 999},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr, hub := fanoutFixture(t)
			mgr.SetActive(hub, "test")

			env := MakeEnvelope(tc.typ, []byte("peer"), []byte("test"), []byte("nick"), "should be ignored", []byte("mid-x"), NowMs())
			hub.HandleData(mustEncode(t, env))

			if msgs := hub.GetMessages("test"); len(msgs) != 0 {
				t.Errorf("unknown type %v recorded %v, want nothing", tc.typ, textsOf(msgs))
			}
			if hub.MOTD != "" {
				t.Error("unknown message type touched the MOTD")
			}
		})
	}
}

// TestUnknownEnvelopeKeyIgnored pins doc 3-RRC: unknown top-level keys must
// be ignored — the envelope still processes normally.
func TestUnknownEnvelopeKeyIgnored(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	env := MakeEnvelope(TypeMsg, []byte("peer"), []byte("test"), []byte("Nick"), "hello", []byte("mid-u1"), NowMs())
	env[100] = "extension field"
	env[uint64(101)] = []byte("another extension")

	hub.HandleData(mustEncode(t, env))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("message with unknown envelope keys = %v, want the normal MSG record", textsOf(msgs))
	}
}

// TestUnknownBodyKeyIgnored pins doc 3-RRC's body rule: unknown body keys are
// ignored (exercised on the WELCOME limits map).
func TestUnknownBodyKeyIgnored(t *testing.T) {
	t.Parallel()

	_, hub := fanoutFixture(t)

	body := map[any]any{
		BWelcomeHub:    []byte("PyHub"),
		BWelcomeLimits: map[any]any{LMaxNickBytes: 40, uint64(99): "unknown limit"},
		uint64(200):    "unknown top-level body key",
	}
	hub.handleWelcome(body)

	if got := hub.GetServerName(); got != "PyHub" {
		t.Errorf("server name = %q, want PyHub (unknown body keys ignored)", got)
	}
	if got := hub.effectiveNickLimit(); got != 40 {
		t.Errorf("nick limit = %v, want 40 (unknown keys must not break limits)", got)
	}
}

// TestNonStringMsgBodyIgnored pins Python's isinstance(body, str) guard
// (RRC.py:1060): a MSG whose body is a structured payload is opaque and not
// recorded.
func TestNonStringMsgBodyIgnored(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	env := MakeEnvelope(TypeMsg, []byte("peer"), []byte("test"), []byte("Nick"),
		map[any]any{0: "structured"}, []byte("mid-b1"), NowMs())
	hub.HandleData(mustEncode(t, env))

	if msgs := hub.GetMessages("test"); len(msgs) != 0 {
		t.Errorf("structured-body MSG recorded %v, want ignored", textsOf(msgs))
	}
}
