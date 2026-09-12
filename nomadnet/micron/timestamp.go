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
	"fmt"
	"strconv"
	"strings"
	"time"
)

// This file adds the timestamp construct to Micron:
//
//	`T<unix-seconds>`T
//	`T<unix-seconds>|<strftime-format>`T
//
// A page stores a clock time as unix seconds, which carry no timezone, and
// names the format it wants them shown in. The client that parses the page
// renders the seconds in its own timezone, so every reader sees the same
// instant in their own local time. Python's NomadNet has no equivalent
// construct: it consumes the backtick and the marker character and renders the
// payload as ordinary text, which is why a page that wants a readable
// timestamp for every client should still include one (nomadnet pages are
// parsed client-side, so only the reader's own client can localize a time).
//
// The format language is strftime, the POSIX/C spelling that Python's
// time.strftime, the shell's date and most other tooling share, rather than
// Go's reference-time layout: "%a %b %d, %Y %-I:%M:%S%p %Z" is legible to
// anyone who has written a date format before.

// DefaultTimeFormat is the format a timestamp construct uses when the page does
// not name one: "Fri Sep 11, 2026 9:08:27PM EST".
const DefaultTimeFormat = "%a %b %d, %Y %-I:%M:%S%p %Z"

// timestampMarker opens and closes a timestamp construct.
const timestampMarker = "`T"

// FormatUnix renders unix seconds for a reader: format is a strftime format
// (empty for DefaultTimeFormat) and loc is the zone to render in, nil for the
// machine's own local zone. Unix seconds are UTC by definition, so the zone
// decides only how the same instant is spelled.
func FormatUnix(secs int64, format string, loc *time.Location) string {
	if format == "" {
		format = DefaultTimeFormat
	}
	if loc == nil {
		loc = time.Local
	}
	return formatTime(time.Unix(secs, 0).In(loc), format)
}

// formatTime expands one strftime format. A conversion it does not implement
// passes through with its percent sign, so a typo in a page's format is visible
// to the page's author instead of silently vanishing.
func formatTime(t time.Time, format string) string {
	var sb strings.Builder

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			sb.WriteByte(format[i])
			continue
		}

		start := i
		i++
		noPad := i < len(format) && format[i] == '-'
		if noPad {
			i++
		}
		if i >= len(format) {
			// A trailing percent sign is a literal one.
			sb.WriteByte('%')
			break
		}

		if expanded, ok := expandConversion(t, format[i], noPad); ok {
			sb.WriteString(expanded)
			continue
		}
		sb.WriteString(format[start : i+1])
	}

	return sb.String()
}

// expandConversion expands a single strftime conversion. Conversions that
// compose others (D, F, R, T) recurse so they inherit the same handling.
func expandConversion(t time.Time, conv byte, noPad bool) (string, bool) {
	switch conv {
	case '%':
		return "%", true
	case 'a':
		return t.Format("Mon"), true
	case 'A':
		return t.Format("Monday"), true
	case 'b', 'h': // h is the traditional synonym for b
		return t.Format("Jan"), true
	case 'B':
		return t.Format("January"), true
	case 'd':
		return padded(t.Day(), 2, false, noPad), true
	case 'D':
		return formatTime(t, "%m/%d/%y"), true
	case 'e':
		return padded(t.Day(), 2, true, noPad), true
	case 'F':
		return formatTime(t, "%Y-%m-%d"), true
	case 'H':
		return padded(t.Hour(), 2, false, noPad), true
	case 'I':
		return padded(hour12(t), 2, false, noPad), true
	case 'j':
		return padded(t.YearDay(), 3, false, noPad), true
	case 'm':
		return padded(int(t.Month()), 2, false, noPad), true
	case 'M':
		return padded(t.Minute(), 2, false, noPad), true
	case 'p':
		return t.Format("PM"), true
	case 'R':
		return formatTime(t, "%H:%M"), true
	case 's':
		return strconv.FormatInt(t.Unix(), 10), true
	case 'S':
		return padded(t.Second(), 2, false, noPad), true
	case 'T':
		return formatTime(t, "%H:%M:%S"), true
	case 'y':
		return padded(t.Year()%100, 2, false, noPad), true
	case 'Y':
		return strconv.Itoa(t.Year()), true
	case 'z':
		return t.Format("-0700"), true
	case 'Z':
		return t.Format("MST"), true
	}
	return "", false
}

// hour12 reports an hour on the 12-hour clock, where midnight and noon are 12.
func hour12(t time.Time) int {
	if h := t.Hour() % 12; h != 0 {
		return h
	}
	return 12
}

// padded renders value at width, zero-filled unless spaceFill requests the
// space padding of %e, and bare when the format used the no-pad flag.
func padded(value, width int, spaceFill, noPad bool) string {
	if noPad {
		return strconv.Itoa(value)
	}
	if spaceFill {
		return fmt.Sprintf("%*d", width, value)
	}
	return fmt.Sprintf("%0*d", width, value)
}

// parseTimestamp parses a timestamp construct whose marker starts at start. It
// returns the node holding the rendered timestamp and the number of bytes
// consumed, or nil when the construct is unusable. The marker itself is always
// consumed: it is a recognized escape, so only its payload falls back to text.
func parseTimestamp(line string, start int) (*Node, int) {
	body := start + len(timestampMarker)
	payload := line[body:]
	end := strings.Index(payload, timestampMarker)
	if end < 0 {
		return nil, len(timestampMarker)
	}

	secs, format, _ := strings.Cut(payload[:end], "|")
	seconds, err := strconv.ParseInt(strings.TrimSpace(secs), 10, 64)
	if err != nil {
		return nil, len(timestampMarker)
	}

	rendered := FormatUnix(seconds, format, nil)
	return &Node{Type: NodeText, Text: rendered}, end + 2*len(timestampMarker)
}
