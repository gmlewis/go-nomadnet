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

// TestLXMessageHeaderPythonParity is a LIVE cross-implementation check: it
// runs the stubbed-urwid golden capture script tooling/tui-parity/
// lxmsg_title_golden.py fresh on every run. That script stubs gi/urwid (GLib
// is broken on this host), instantiates the real Python nomadnet
// LXMessageWidget (Conversations.py:2596-2670) with a fake app + fake message
// at a fixed now (unix 1700000000) and 1h-prior timestamp, and prints the
// title string + header style for each named case. The Go test reproduces each
// case through LXMessageHeader and compares title+style to the freshly
// captured Python values, matched by case name.
//
// Both sides render the strftime timestamp in the machine's LOCAL zone — Go via
// time.Unix(ts,0) (matching production, which feeds msg.Timestamp straight
// through without forcing UTC) and Python via datetime.fromtimestamp — so the
// timestamp portion is zone-consistent across implementations rather than a
// UTC-vs-local artifact. relative_time ("1h ago") is a delta and zone-
// independent. The test SKIPs, not fails, when the Python reference is not
// importable or the script file is not accessible.
func TestLXMessageHeaderPythonParity(t *testing.T) {
	t.Parallel()

	const (
		nowUnix = 1700000000
		tsUnix  = nowUnix - 3600
	)
	// Local zone, matching production (msg.Timestamp flows through unchanged)
	// and Python's datetime.fromtimestamp — no forced UTC.
	now := time.Unix(nowUnix, 0)
	ts := time.Unix(tsUnix, 0)
	timeFormat := "%Y-%m-%d %H:%M:%S"
	g := GetGlyphSet(GlyphUnicode)

	ownHash := bytes.Repeat([]byte{0x11}, 32)
	peerHash := bytes.Repeat([]byte{0xAA}, 32)

	cases := []struct {
		name string
		in   MessageHeaderInputs
	}{
		{
			"outbound_sent",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"outbound_delivered",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateDelivered, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"outbound_failed",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateFailed, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"outbound_rejected",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateRejected, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"outbound_paper",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStatePaper, Method: lxmfMethodPaper, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"outbound_pending",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: 0x02, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"inbound_trusted_sig",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: peerHash, OwnHash: ownHash, TransportEncrypted: true, SignatureValidated: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"inbound_untrusted_sig",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: peerHash, OwnHash: ownHash, TransportEncrypted: true, SignatureValidated: false, SignatureDescription: "Signature could not be verified", TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"failed_no_source",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateFailed, Method: lxmfMethodPropagated, SourceHash: nil, OwnHash: ownHash, TransportEncrypted: true, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"with_title",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, Title: "My Subject", TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"plaintext",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: false, TimeFormat: timeFormat, Glyphs: g},
		},
		{
			"with_attachment",
			MessageHeaderInputs{Timestamp: ts, Now: now, State: lxmfStateSent, Method: lxmfMethodPropagated, SourceHash: ownHash, OwnHash: ownHash, TransportEncrypted: true, AttachmentTypes: []string{"file"}, AttachmentNames: []string{"report.pdf"}, TimeFormat: timeFormat, Glyphs: g},
		},
	}

	type goldenEntry struct {
		Name  string `json:"name"`
		Title string `json:"title"`
		Style string `json:"style"`
	}
	var entries []goldenEntry
	runPythonNomadnetScript(t, "../tooling/tui-parity/lxmsg_title_golden.py", &entries)
	want := make(map[string]goldenEntry, len(entries))
	for _, e := range entries {
		want[e.Name] = e
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, ok := want[tc.name]
			if !ok {
				t.Fatalf("no golden entry named %q in script output", tc.name)
			}
			got, gotStyle := LXMessageHeader(tc.in)
			if got != w.Title {
				t.Errorf("title = %q\nwant   %q (Python)", got, w.Title)
			}
			if gotStyle != w.Style {
				t.Errorf("style = %q, want %q (Python)", gotStyle, w.Style)
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
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			ts := now.Add(-time.Duration(tc.delta) * time.Second)
			if got := relativeTimeAt(ts, now); got != tc.want {
				t.Errorf("relativeTimeAt(-%ds) = %q, want %q", tc.delta, got, tc.want)
			}
		})
	}
}
