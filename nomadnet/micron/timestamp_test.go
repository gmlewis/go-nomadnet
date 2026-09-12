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

package micron

import (
	"strings"
	"testing"
	"time"
)

// estZone is a fixed UTC-5 zone, so timestamp expectations do not depend on
// the machine running the tests.
var estZone = time.FixedZone("EST", -5*3600)

// The instants below were captured from Python's own time.strftime under
// TZ=EST5EDT, which is the reference implementation for this format language.
const (
	fridayEvening  = 1789178907 // 2026-09-11 21:08:27 EST
	tuesdayMorning = 1788271507 // 2026-09-01 09:05:07 EST
)

// TestFormatUnixStrftime pins the strftime conversions this port supports,
// against values captured from Python's time.strftime.
func TestFormatUnixStrftime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		secs   int64
		format string
		want   string
	}{
		{"default format matches the documented example", fridayEvening, "", "Fri Sep 11, 2026 9:08:27PM EST"},
		{"explicit default format", fridayEvening, "%a %b %d, %Y %-I:%M:%S%p %Z", "Fri Sep 11, 2026 9:08:27PM EST"},
		{"12-hour is zero padded without the no-pad flag", tuesdayMorning, "%I:%M:%S %p", "09:05:07 AM"},
		{"no-pad flag strips the leading zero", tuesdayMorning, "%-I:%-M:%-S %-d/%-m", "9:5:7 1/9"},
		{"day of month", tuesdayMorning, "%d|%e|%-d", "01| 1|1"},
		{"month and year", tuesdayMorning, "%b %B %y %Y", "Sep September 26 2026"},
		{"weekday", fridayEvening, "%a %A", "Fri Friday"},
		{"24-hour clock", fridayEvening, "%H:%M:%S", "21:08:27"},
		{"iso shorthands", fridayEvening, "%F %T", "2026-09-11 21:08:27"},
		{"us shorthand", fridayEvening, "%D", "09/11/26"},
		{"unix seconds round-trip", fridayEvening, "%s", "1789178907"},
		{"numeric zone offset", fridayEvening, "%z", "-0500"},
		{"day of year", tuesdayMorning, "%j", "244"},
		{"literal percent", fridayEvening, "100%%", "100%"},
		{"trailing percent is literal", fridayEvening, "50%", "50%"},
		{"unknown specifier passes through so a typo is visible", fridayEvening, "%q", "%q"},
		{"text around the conversions is preserved", fridayEvening, "at %H:%M on %F", "at 21:08 on 2026-09-11"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := FormatUnix(tc.secs, tc.format, estZone); got != tc.want {
				t.Errorf("FormatUnix(%v, %q) = %q, want %q", tc.secs, tc.format, got, tc.want)
			}
		})
	}
}

// TestFormatUnixUsesTheViewersZone pins the whole point of the construct: the
// same instant renders differently depending on the zone it is viewed in.
func TestFormatUnixUsesTheViewersZone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		zone *time.Location
		want string
	}{
		{"east coast", estZone, "Fri Sep 11, 2026 9:08:27PM EST"},
		{"utc", time.UTC, "Sat Sep 12, 2026 2:08:27AM UTC"},
		{"tokyo", time.FixedZone("JST", 9*3600), "Sat Sep 12, 2026 11:08:27AM JST"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := FormatUnix(fridayEvening, "", tc.zone); got != tc.want {
				t.Errorf("FormatUnix in %v = %q, want %q", tc.zone, got, tc.want)
			}
		})
	}
}

// TestFormatUnixNilZoneFallsBackToLocal pins that a caller with no opinion gets
// the viewer's own zone.
func TestFormatUnixNilZoneFallsBackToLocal(t *testing.T) {
	t.Parallel()

	got := FormatUnix(fridayEvening, "%s", nil)
	if want := "1789178907"; got != want {
		t.Errorf("FormatUnix with a nil zone = %q, want %q", got, want)
	}
}

// TestParseTimestampConstruct pins the `t<seconds>`<format>t` construct: the
// page emits unix seconds, and the client that parses the page renders them in
// its own zone.
func TestParseTimestampConstruct(t *testing.T) {
	t.Parallel()

	local := FormatUnix(fridayEvening, "", time.Local)
	tests := []struct {
		name   string
		markup string
		want   string
	}{
		{"seconds alone use the default format", "`T1789178907`T", local},
		{"explicit format overrides the default", "`T1789178907|%F %T`T",
			FormatUnix(fridayEvening, "%F %T", time.Local)},
		{"zone-independent format is stable everywhere", "`T1789178907|%s`T", "1789178907"},
		{"zone-dependent format uses the viewer's zone",
			"`T1789178907|%a %b %d, %Y %-I:%M:%S%p %Z`T", FormatUnix(fridayEvening, "%a %b %d, %Y %-I:%M:%S%p %Z", time.Local)},
		{"surrounding text is preserved", "posted `T1789178907|%s`T by Glenn", "posted 1789178907 by Glenn"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := stripText(Parse(tc.markup))
			if got != tc.want {
				t.Errorf("Parse(%q) rendered %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// TestParseTimestampConstructLineInitial pins that a line starting with the
// construct is not mistaken for a partial include, which owns the line-initial
// `{ spelling.
func TestParseTimestampConstructLineInitial(t *testing.T) {
	t.Parallel()

	got := stripText(Parse("`T1789178907|%s`T Glenn: hello"))
	if want := "1789178907 Glenn: hello"; got != want {
		t.Errorf("line-initial construct rendered %q, want %q", got, want)
	}
	if strings.Contains(got, "partial") || strings.Contains(got, "{") {
		t.Errorf("line-initial construct rendered %q, want plain text", got)
	}
}

// TestParseTimestampConstructMalformedStaysText pins the fallback for a
// construct the parser cannot use: only the marker character is consumed and
// the payload stays ordinary text, which is what Python's parser does with the
// same backtick escape.
func TestParseTimestampConstructMalformedStaysText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		markup string
		want   string
	}{
		{"no closing marker", "`T1789178907", "1789178907"},
		{"no digits", "`Tsoon`T", "soon"},
		{"empty", "`T`T", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := stripText(Parse(tc.markup)); got != tc.want {
				t.Errorf("Parse(%q) rendered %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}

// stripText renders parsed nodes down to their visible text, so a construct's
// expansion can be compared as a string.
func stripText(nodes []*Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Text != "" {
			sb.WriteString(n.Text)
		}
	}
	return sb.String()
}
