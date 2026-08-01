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
	"bytes"
	"testing"
	"time"
)

// TestLXMessageHeaderPythonParity checks LXMessageHeader against golden title
// strings + styles captured from the live Python nomadnet LXMessageWidget
// (Conversations.py:2596-2670) via tooling/tui-parity-style capture. The
// capture stubs urwid (GLib is broken on this host) and instantiates
// LXMessageWidget with a fake app + fake message at a fixed now (UTC) so
// relative_time and the strftime timestamp are deterministic.
//
// Fixed now = 2023-11-14 22:13:20 UTC (unix 1700000000); timestamp = 1h prior
// (unix 1699996400) so relative_time == "1h ago" and the strftime timestamp
// (default app.time_format "%Y-%m-%d %H:%M:%S") == "2023-11-14 21:13:20".
func TestLXMessageHeaderPythonParity(t *testing.T) {
	t.Parallel()

	const (
		nowUnix = 1700000000
		tsUnix  = nowUnix - 3600
	)
	now := time.Unix(nowUnix, 0).UTC()
	ts := time.Unix(tsUnix, 0).UTC()
	timeFormat := "%Y-%m-%d %H:%M:%S"
	g := GetGlyphSet(GlyphUnicode)

	ownHash := bytes.Repeat([]byte{0x11}, 32)
	peerHash := bytes.Repeat([]byte{0xAA}, 32)

	cases := []struct {
		name  string
		in    MessageHeaderInputs
		want  string
		style string
	}{
		{
			"outbound_sent",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"↑ → 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_propagated",
		},
		{
			"outbound_delivered",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateDelivered, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"✓ → 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_delivered",
		},
		{
			"outbound_failed",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateFailed, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"✕ → 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_failed",
		},
		{
			"outbound_rejected",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateRejected, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"✕ → Rejected 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_failed",
		},
		{
			"outbound_paper",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStatePaper, Method: lxmfMethodPaper, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"▤ → 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_propagated",
		},
		{
			"outbound_pending",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: 0x02, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"→ 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_sent",
		},
		{
			"inbound_trusted_sig",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: peerHash, OwnHash: ownHash, TransportEncrypted: true, SignatureValidated: true, TimeFormat: timeFormat, Glyphs: g},
			"✓ ← 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_ok",
		},
		{
			"inbound_untrusted_sig",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: peerHash, OwnHash: ownHash, TransportEncrypted: true, SignatureValidated: false, SignatureDescription: "Signature could not be verified", TimeFormat: timeFormat, Glyphs: g},
			"⚠ ← Signature could not be verified\n  1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_caution",
		},
		{
			"failed_no_source",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateFailed, Method: lxmfMethodPropagated, SourceHash: nil, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
			"⚠ 1h ago | 2023-11-14 21:13:20 ⚿", "msg_header_failed",
		},
		{
			"with_title",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, Title: "My Subject", TimeFormat: timeFormat, Glyphs: g},
			"↑ → 1h ago | 2023-11-14 21:13:20 ⚿ | My Subject", "msg_header_propagated",
		},
		{
			"plaintext",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: false, TimeFormat: timeFormat, Glyphs: g},
			"↑ → 1h ago | 2023-11-14 21:13:20 !", "msg_header_propagated",
		},
		{
			"with_attachment",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, AttachmentTypes: []string{"file"}, AttachmentNames: []string{"report.pdf"}, TimeFormat: timeFormat, Glyphs: g},
			"↑ → 1h ago | 2023-11-14 21:13:20 ⚿ | ▤ report.pdf", "msg_header_propagated",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, gotStyle := LXMessageHeader(tc.in)
			if got != tc.want {
				t.Errorf("title = %q\nwant   %q", got, tc.want)
			}
			if gotStyle != tc.style {
				t.Errorf("style = %q, want %q", gotStyle, tc.style)
			}
		})
	}
}

// TestStrftimeLayout verifies the strftime→Go-layout conversion for the default
// app.time_format and a few common variants.
func TestStrftimeLayout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"%Y-%m-%d %H:%M:%S", "2006-01-02 15:04:05"},
		{"%H:%M:%S", "15:04:05"},
		{"%Y/%m/%d", "2006/01/02"},
		{"%I:%M %p", "03:04 PM"},
		{"100%% done", "100% done"},
		{"plain text", "plain text"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := strftimeLayout(tc.in); got != tc.want {
				t.Errorf("strftimeLayout(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRelativeTimeAtParity checks the time-injected relative_time against the
// captured Python relative_time buckets at a fixed now.
func TestRelativeTimeAtParity(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0).UTC()
	cases := []struct {
		delta int
		want  string
	}{
		{30, "just now"},
		{120, "2m ago"},
		{3600, "1h ago"},
		{5400, "1h ago"},
		{90000, "yesterday"}, // 25h
		{3 * 86400, "3d ago"},
		{10 * 86400, "1w ago"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			ts := now.Add(-time.Duration(tc.delta) * time.Second)
			if got := relativeTimeAt(ts, now); got != tc.want {
				t.Errorf("relativeTimeAt(-%ds) = %q, want %q", tc.delta, got, tc.want)
			}
		})
	}
}
