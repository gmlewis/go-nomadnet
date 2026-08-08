// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package browser

import "testing"

// TestStatusTextGolden pins Python Browser.status_text (Browser.py:1756-1802):
// each request-lifecycle status state maps to a fixed human-readable string.
// The DONE state's base is "Done" (the size/time stats suffix is appended by the
// TUI from response data, not part of this pure mapping). Golden strings
// captured verbatim from the installed Python nomadnet source.
func TestStatusTextGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"no path", StatusNoPath, "No path to destination known"},
		{"path requested", StatusPathRequested, "Path requested, waiting for path..."},
		{"establishing link", StatusEstablishingLink, "Establishing link..."},
		{"link timeout", StatusLinkTimeout, "Link establishment timed out"},
		{"link established", StatusLinkEstablished, "Link established"},
		{"requesting", StatusRequesting, "Sending request..."},
		{"request sent", StatusRequestSent, "Request sent, awaiting response..."},
		{"request failed", StatusRequestFailed, "Request failed"},
		{"request timeout", StatusRequestTimeout, "Request timed out"},
		{"receiving response", StatusReceivingResponse, "Receiving response..."},
		{"done", StatusDone, "Done"},
		{"disconnected", StatusDisconnected, "Disconnected"},
		{"unknown", 0x42, "Browser Status Unknown"},
		{"unknown zero-ish", 0x10, "Browser Status Unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := StatusText(c.status); got != c.want {
				t.Errorf("StatusText(%#x) = %q, want %q", c.status, got, c.want)
			}
		})
	}
}

// TestErrToStatus pins the fetch-error → status-state mapping so the TUI can
// surface Python's status strings for the terminal failure modes (Python sets
// these status values in request_failed/request_timeout/link_establishment
// timeout, Browser.py:1465-1473/1686-1734).
func TestErrToStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"no path", ErrNoPath, StatusNoPath},
		{"link timeout", ErrLinkTimeout, StatusLinkTimeout},
		{"request failed", ErrRequestFailed, StatusRequestFailed},
		{"request timeout", ErrRequestTimeout, StatusRequestTimeout},
		{"nil", nil, StatusDone},
		{"other error", errOther, StatusRequestFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := ErrToStatus(c.err); got != c.want {
				t.Errorf("ErrToStatus(%v) = %#x, want %#x", c.err, got, c.want)
			}
		})
	}
}

var errOther = newSentinelErr("something else")

type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

func newSentinelErr(s string) error { return sentinelErr(s) }
