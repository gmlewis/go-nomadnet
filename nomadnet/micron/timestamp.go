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
// anyone who has written a date format before. Every conversion C strftime
// defines is implemented, along with the glibc and BSD extensions, and the
// -/_/0 padding flags and the E/O modifiers. The conversions that strftime
// resolves through the locale are pinned to their C locale forms, so a page
// renders the same text on every client rather than following the reader's
// region or language.

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
		// An optional padding flag and E/O modifier, as in %-d and %Ec. POSIX
		// defines %E and %O to change nothing in the C locale, which is the
		// locale this renders in.
		var flag byte
		if i < len(format) && (format[i] == '-' || format[i] == '_' || format[i] == '0') {
			flag = format[i]
			i++
		}
		if i+1 < len(format) && (format[i] == 'E' || format[i] == 'O') {
			i++
		}
		if i >= len(format) {
			// A trailing percent sign is a literal one.
			sb.WriteByte('%')
			break
		}

		if expanded, ok := expandConversion(t, format[i], flag); ok {
			sb.WriteString(expanded)
			continue
		}
		sb.WriteString(format[start : i+1])
	}

	return sb.String()
}

// expandConversion expands a single strftime conversion. Conversions that
// compose others (c, D, F, r, R, T, v, x, X, +) recurse so they inherit the same
// handling. The flag is the padding flag that preceded the conversion, if any.
func expandConversion(t time.Time, conv, flag byte) (string, bool) {
	switch conv {
	case '%':
		return "%", true
	case '+': // the date(1) default format
		return formatTime(t, "%a %b %e %H:%M:%S %Z %Y"), true
	case 'a':
		return t.Format("Mon"), true
	case 'A':
		return t.Format("Monday"), true
	case 'b', 'h': // h is the traditional synonym for b
		return t.Format("Jan"), true
	case 'B':
		return t.Format("January"), true
	case 'c':
		return formatTime(t, "%a %b %e %H:%M:%S %Y"), true
	case 'C':
		return pad(t.Year()/100, 2, flag, false), true
	case 'd':
		return pad(t.Day(), 2, flag, false), true
	case 'D':
		return formatTime(t, "%m/%d/%y"), true
	case 'e':
		return pad(t.Day(), 2, flag, true), true
	case 'F':
		return formatTime(t, "%Y-%m-%d"), true
	case 'g':
		isoYear, _ := t.ISOWeek()
		return pad(isoYear%100, 2, flag, false), true
	case 'G':
		isoYear, _ := t.ISOWeek()
		return strconv.Itoa(isoYear), true
	case 'H':
		return pad(t.Hour(), 2, flag, false), true
	case 'I':
		return pad(hour12(t), 2, flag, false), true
	case 'j':
		return pad(t.YearDay(), 3, flag, false), true
	case 'k':
		return pad(t.Hour(), 2, flag, true), true
	case 'l':
		return pad(hour12(t), 2, flag, true), true
	case 'm':
		return pad(int(t.Month()), 2, flag, false), true
	case 'M':
		return pad(t.Minute(), 2, flag, false), true
	case 'n':
		return "\n", true
	case 'p':
		return t.Format("PM"), true
	case 'P':
		return strings.ToLower(t.Format("PM")), true
	case 'r':
		return formatTime(t, "%I:%M:%S %p"), true
	case 'R':
		return formatTime(t, "%H:%M"), true
	case 's':
		return strconv.FormatInt(t.Unix(), 10), true
	case 'S':
		return pad(t.Second(), 2, flag, false), true
	case 't':
		return "\t", true
	case 'T':
		return formatTime(t, "%H:%M:%S"), true
	case 'u':
		return strconv.Itoa(int(t.Weekday()+6)%7 + 1), true
	case 'U':
		return pad((t.YearDay()+7-int(t.Weekday()))/7, 2, flag, false), true
	case 'v':
		return formatTime(t, "%e-%b-%Y"), true
	case 'V':
		_, isoWeek := t.ISOWeek()
		return pad(isoWeek, 2, flag, false), true
	case 'w':
		return strconv.Itoa(int(t.Weekday())), true
	case 'W':
		return pad((t.YearDay()+7-int((t.Weekday()+6)%7))/7, 2, flag, false), true
	case 'x':
		return formatTime(t, "%m/%d/%y"), true
	case 'X':
		return formatTime(t, "%H:%M:%S"), true
	case 'y':
		return pad(t.Year()%100, 2, flag, false), true
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

// pad renders value at width. The flag is the padding flag that preceded the
// conversion: - leaves it bare, _ pads with spaces, 0 pads with zeros. With no
// flag, spaceFill selects the space padding that %e, %k and %l have by default.
func pad(value, width int, flag byte, spaceFill bool) string {
	switch flag {
	case '-':
		return strconv.Itoa(value)
	case '_':
		return fmt.Sprintf("%*d", width, value)
	case '0':
		return fmt.Sprintf("%0*d", width, value)
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
