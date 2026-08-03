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
	"time"
)

// TestFormatLXMFPeerEntry pins the peer_info_str produced by Python's
// LXMFPeerEntry (Network.py:1875-1928) for a set of representative peers. The
// expected strings are assembled from the Python template plus golden helper
// outputs captured from RNS (prettysize, prettyspeed, prettyhexrep) and the
// pretty_date port (tui.PrettyDate). pretty_date is made deterministic by
// injecting `now` (Python's pretty_date uses datetime.now, so the original is
// non-deterministic; the values here pin the time-injected core).
//
// Captured helper outputs (python3 -c "import RNS; ..."):
//
//	RNS.prettyhexrep(0x0123456789abcdef0123456789abcdef)
//	  -> '<0123456789abcdef0123456789abcdef>'
//	RNS.prettysize(500*1000)  -> '500.00 KB'
//	RNS.prettysize(256*1000)  -> '256.00 KB'
//	RNS.prettysize(100*1000)  -> '100.00 KB'
//	RNS.prettyspeed(12345.6)  -> '12.35 Kbps'
//	RNS.prettyspeed(678.9)    -> '679 bps'
//	RNS.prettyspeed(0)        -> '0 bps'
//	round(0.875*100, 2) -> 87.5  -> f"{ar}" -> '87.5'
//	round(0.0*100, 2)   -> 0.0   -> f"{ar}" -> '0.0'
func TestFormatLXMFPeerEntry(t *testing.T) {
	// 2023-01-01 00:00:00 UTC == epoch 1672531200. Use fixed ints so the
	// pretty_date day/second math is exact.
	const baseEpoch = 1672531200
	lastHeard := time.Unix(baseEpoch, 0)
	now := time.Unix(baseEpoch+30, 0) // 30 s later -> "30 seconds ago"
	yesterdayNow := time.Unix(baseEpoch+86400+100, 0)

	sent := "↑" // unicode "sent" glyph (nerdfont is \U000f0cd8 in production)
	hexrep := "<0123456789abcdef0123456789abcdef>"

	tests := []struct {
		name string
		data PeerEntryData
		want string
	}{
		{
			name: "alive trusted full limits",
			data: PeerEntryData{
				Sym:           sent,
				DisplayStr:    hexrep,
				AliveStr:      "Available",
				LastHeard:     lastHeard,
				SyncLimit:     "256.00 KB",
				TxferLimit:    "500.00 KB",
				STR:           "12.35 Kbps",
				LER:           "679 bps",
				StampCost:     "12",
				StampFlex:     " (flex 4)",
				Unhandled:     3,
				AcceptancePct: "87.5",
			},
			want: joinLines(
				sent+" "+hexrep,
				"  Available, last heard 30 seconds ago",
				"  256.00 KB sync limit, 500.00 KB msg limit",
				"  12.35 Kbps STR, 679 bps LER",
				"  Propagation cost 12 (flex 4)",
				"  3 unhandled LXMs, 87.5% AR",
			),
		},
		{
			name: "unresponsive no limits",
			data: PeerEntryData{
				Sym:           sent,
				DisplayStr:    hexrep,
				AliveStr:      "Unresponsive",
				LastHeard:     lastHeard,
				SyncLimit:     "Unknown",
				TxferLimit:    "No",
				STR:           "0 bps",
				LER:           "0 bps",
				StampCost:     "Unknown",
				StampFlex:     "",
				Unhandled:     0,
				AcceptancePct: "0.0",
			},
			want: joinLines(
				sent+" "+hexrep,
				"  Unresponsive, last heard 30 seconds ago",
				"  Unknown sync limit, No msg limit",
				"  0 bps STR, 0 bps LER",
				"  Propagation cost Unknown",
				"  0 unhandled LXMs, 0.0% AR",
			),
		},
		{
			name: "alive unknown-trust no flex full AR",
			data: PeerEntryData{
				Sym:           sent,
				DisplayStr:    "Operator X\n  " + hexrep,
				AliveStr:      "Available",
				LastHeard:     lastHeard,
				SyncLimit:     "100.00 KB",
				TxferLimit:    "No",
				STR:           "12.35 Kbps",
				LER:           "0 bps",
				StampCost:     "8",
				StampFlex:     "",
				Unhandled:     12,
				AcceptancePct: "100.0",
			},
			want: joinLines(
				sent+" Operator X\n  "+hexrep,
				"  Available, last heard 30 seconds ago",
				"  100.00 KB sync limit, No msg limit",
				"  12.35 Kbps STR, 0 bps LER",
				"  Propagation cost 8",
				"  12 unhandled LXMs, 100.0% AR",
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLXMFPeerEntry(tc.data, now)
			if got != tc.want {
				t.Errorf("FormatLXMFPeerEntry (now=%v):\ngot:\n%v\nwant:\n%v", now, got, tc.want)
			}
		})
	}

	// Verify the "Yesterday" case uses the injected now, not time.Now.
	yesterdayWant := joinLines(
		sent+" "+hexrep,
		"  Unresponsive, last heard Yesterday",
		"  Unknown sync limit, No msg limit",
		"  0 bps STR, 0 bps LER",
		"  Propagation cost Unknown",
		"  0 unhandled LXMs, 0.0% AR",
	)
	got := FormatLXMFPeerEntry(tests[1].data, yesterdayNow)
	if got != yesterdayWant {
		t.Errorf("yesterday-now variant:\ngot:\n%v\nwant:\n%v", got, yesterdayWant)
	}
}

// joinLines joins lines with "\n" and no trailing newline, matching the
// peer_info_str template (Python builds it with "\n" inter-line, no trailing).
func joinLines(lines ...string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
