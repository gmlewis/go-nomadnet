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

	"github.com/gmlewis/go-nomadnet/testutils"
)

// relativeTimeParityScript calls the REAL Python relative_time() (nomadnet
// 1.2.8, ui/textui/Conversations.py:28-50) on every (timestamp, now) pair, with
// time.time patched to the pair's `now` so the reference is fully deterministic.
// The Go side runs relativeTimeAt on exactly the same pairs.
const relativeTimeParityScript = `
import json, sys, time
from nomadnet.ui.textui.Conversations import relative_time
pairs = json.load(sys.stdin)
out = []
for ts, now in pairs:
    real_time = time.time
    time.time = lambda n=now: n
    try:
        out.append(relative_time(ts))
    finally:
        time.time = real_time
json.dump(out, sys.stdout)
`

// TestRelativeTimePythonParity diffs relativeTimeAt against the live Python
// relative_time() over an extreme range of (timestamp, now) pairs: every
// bucket boundary, minute/hour/day/week edges, midnight and calendar-day
// crossings, DST spring-forward and fall-back days, leap days, the year
// rollover, the epoch and pre-epoch dates, and far-future timestamps.
func TestRelativeTimePythonParity(t *testing.T) {
	t.Parallel()
	testutils.SkipIfNoPythonNomadnet(t)
	testutils.SkipShortIntegration(t)

	base := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.Local) // a Sunday
	at := func(m time.Month, d, h, min, s int) time.Time {
		return time.Date(2026, m, d, h, min, s, 0, time.Local)
	}

	pairs := [][2]time.Time{
		{base, base},                         // the future is "just now"
		{base.Add(30 * time.Second), base},   // future
		{base.Add(36 * time.Hour), base},     // far future
		{at(time.January, 1, 0, 0, 0), base}, // months out
		{base, base},                         // now
		{base.Add(-59 * time.Second), base},  // last second of the minute bucket
		{base.Add(-60 * time.Second), base},  // exact minute boundary
		{base.Add(-60 * time.Second), base},  // first second of the minute bucket
		{base.Add(-5*time.Minute - 30*time.Second), base},
		{base.Add(-59 * time.Minute), base}, // last minute of the hour bucket
		{base.Add(-60 * time.Minute), base}, // exact hour boundary
		{base.Add(-61 * time.Minute), base}, // first minute of the hour bucket
		{base.Add(-23*time.Hour - 59*time.Minute), base},
		{base.Add(-24 * time.Hour), base},             // exactly one day before
		{base.Add(-24*time.Hour - time.Second), base}, // first second in the calendar-day ranges
		// Calendar-day edges (NOT duration edges): timestamps crossing local
		// midnight.
		{at(time.August, 30, 0, 0, 0), base}, // same day → hours bucket
		{at(time.August, 30, 0, 1, 0), base},
		{at(time.August, 29, 23, 59, 59), base}, // 1 calendar day, 12h ago → "12h ago"
		{at(time.August, 29, 12, 0, 0), base},   // exactly 24h → "yesterday"
		{at(time.August, 29, 0, 0, 0), base},    // 36h but 1 calendar day → "yesterday"
		{at(time.August, 28, 23, 59, 0), base},  // 2 calendar days → "2d ago"
		{at(time.August, 28, 12, 0, 0), base},
		{at(time.August, 27, 12, 0, 0), base},
		{at(time.August, 26, 12, 0, 0), base},
		{at(time.August, 25, 12, 0, 0), base},
		{at(time.August, 24, 12, 0, 0), base}, // 6 days → "6d ago"
		{at(time.August, 23, 12, 0, 0), base}, // 7 days → "1w ago"
		{at(time.August, 22, 12, 0, 0), base}, // 8 days → "1w ago"
		{at(time.August, 17, 12, 0, 0), base}, // 13 days → "1w ago"
		{at(time.August, 16, 12, 0, 0), base}, // 14 days → "2w ago"
		{at(time.August, 3, 12, 0, 0), base},  // 27 days → "3w ago"
		{at(time.August, 2, 12, 0, 0), base},  // 28 days → "4w ago"
		{at(time.August, 1, 12, 0, 0), base},  // 29 days → "4w ago"
		{at(time.July, 31, 12, 0, 0), base},   // 30 days → absolute date
		// Year rollover: Dec 31 → Jan 1 is a 1-calendar-day diff.
		{at(time.January, 1, 0, 0, 0), at(time.December, 31, 23, 59, 59)},
		{at(time.December, 31, 12, 0, 0), at(time.December, 31, 23, 59, 59)},
		// Leap day (2024-02-29) as the timestamp.
		{time.Date(2024, time.February, 29, 12, 0, 0, 0, time.Local), base},
		// DST spring-forward 2026-03-08 (23-hour local day): 47 wall-hours
		// across the jump is 2 calendar days.
		{time.Date(2026, time.March, 7, 12, 0, 0, 0, time.Local), time.Date(2026, time.March, 9, 12, 0, 0, 0, time.Local)},
		{time.Date(2026, time.March, 8, 1, 30, 0, 0, time.Local), time.Date(2026, time.March, 8, 12, 0, 0, 0, time.Local)},
		// DST fall-back 2026-11-01 (25-hour local day): a 25-hour age within
		// one calendar day.
		{time.Date(2026, time.November, 1, 12, 0, 0, 0, time.Local), time.Date(2026, time.November, 2, 12, 0, 0, 0, time.Local)},
		{time.Date(2026, time.November, 1, 1, 30, 0, 0, time.Local), time.Date(2026, time.November, 2, 12, 0, 0, 0, time.Local)},
		// The epoch and pre-epoch dates print absolute dates.
		{time.Unix(0, 0), base},
		{time.Unix(-86400, 0), base},
		// Century-old timestamp.
		{time.Date(1900, time.January, 1, 12, 0, 0, 0, time.Local), base},
	}

	type pair [2]float64
	inputs := make([]pair, len(pairs))
	for i, p := range pairs {
		inputs[i] = pair{float64(p[0].UnixMicro()) / 1e6, float64(p[1].UnixMicro()) / 1e6}
	}

	var want []string
	runPythonNomadnet(t, inputs, relativeTimeParityScript, &want)

	if len(want) != len(pairs) {
		t.Fatalf("python returned %v labels, want %v", len(want), len(pairs))
	}
	for i, p := range pairs {
		if got := relativeTimeAt(p[0], p[1]); got != want[i] {
			t.Errorf("relativeTimeAt(%v, now=%v) = %q, Python says %q", p[0], p[1], got, want[i])
		}
	}
}
