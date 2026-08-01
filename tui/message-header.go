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
	"strings"
	"time"
)

// LXMF state/method wire constants (Python LXMessage.*), reproduced locally so
// the tui package need not depend on the lxmf core. Values match
// go-reticulum/lxmf/constants.go and the Python LXMF module.
const (
	lxmfStateSent        = 0x04
	lxmfStateDelivered   = 0x08
	lxmfStateRejected    = 0xFD
	lxmfStateFailed      = 0xFF
	lxmfStatePaper       = 0x05
	lxmfMethodPropagated = 0x03
	lxmfMethodPaper      = 0x05
)

// MessageHeaderInputs holds the fields needed to compute an LXMessageWidget
// header (title string + urwid style name), mirroring Python's
// Conversations.py:2596-2670. The timestamp + Now drive relative_time; the
// LXMF state/method + source-hash comparison drive the prefix glyph and style.
type MessageHeaderInputs struct {
	Timestamp            time.Time // message timestamp (Python message.get_timestamp)
	Now                  time.Time // reference "now" for relative_time
	State                int       // LXMF state wire constant
	Method               int       // LXMF method wire constant
	SourceHash           []byte    // nil => failed/no-source header
	OwnHash              []byte    // app.lxmf_destination.hash
	TransportEncrypted   bool
	Title                string   // message.get_title()
	SignatureValidated   bool     // inbound: message.signature_validated()
	SignatureDescription string   // inbound unvalidated: message.get_signature_description()
	AttachmentTypes      []string // glyph key per attachment ("file","image","audio",…)
	AttachmentNames      []string // display name per attachment
	TimeFormat           string   // strftime format (Python app.time_format)
	Glyphs               GlyphSet
}

// LXMessageHeader computes the title string and header style name for a single
// conversation message, matching Python's LXMessageWidget.__init__ title
// construction (Conversations.py:2596-2670). It returns the assembled title
// (which may contain a trailing "\n  " continuation for unvalidated inbound
// signatures) and the urwid style name (e.g. "msg_header_sent") used to color
// the header row.
func LXMessageHeader(in MessageHeaderInputs) (title, style string) {
	g := in.Glyphs
	if g == nil {
		g = glyphsUnicode
	}

	encryption := " " + g["plaintext"]
	if in.TransportEncrypted {
		encryption = " " + g["encrypted"]
	}

	title = relativeTimeAt(in.Timestamp, in.Now) + " | " +
		in.Timestamp.Format(strftimeLayout(in.TimeFormat)) + encryption

	isOutbound := false
	switch {
	case in.SourceHash == nil:
		style = "msg_header_failed"
		title = g["warning"] + " " + title
	case bytes.Equal(in.SourceHash, in.OwnHash):
		isOutbound = true
		switch {
		case in.State == lxmfStateDelivered:
			style = "msg_header_delivered"
			title = g["check"] + " " + g["arrow_r"] + " " + title
		case in.State == lxmfStateFailed:
			style = "msg_header_failed"
			title = g["cross"] + " " + g["arrow_r"] + " " + title
		case in.State == lxmfStateRejected:
			style = "msg_header_failed"
			title = g["cross"] + " " + g["arrow_r"] + " Rejected " + title
		case in.Method == lxmfMethodPropagated && in.State == lxmfStateSent:
			style = "msg_header_propagated"
			title = g["sent"] + " " + g["arrow_r"] + " " + title
		case in.Method == lxmfMethodPaper && in.State == lxmfStatePaper:
			style = "msg_header_propagated"
			title = g["papermsg"] + " " + g["arrow_r"] + " " + title
		case in.State == lxmfStateSent:
			style = "msg_header_sent"
			title = g["sent"] + " " + g["arrow_r"] + " " + title
		default:
			style = "msg_header_sent"
			title = g["arrow_r"] + " " + title
		}
	default:
		// inbound
		if in.SignatureValidated {
			style = "msg_header_ok"
			title = g["check"] + " " + g["arrow_l"] + " " + title
		} else {
			style = "msg_header_caution"
			title = g["warning"] + " " + g["arrow_l"] + " " + in.SignatureDescription + "\n  " + title
		}
	}

	if in.Title != "" {
		title += " | " + in.Title
	}

	if len(in.AttachmentTypes) > 0 && len(in.AttachmentNames) > 0 {
		parts := make([]string, 0, len(in.AttachmentTypes))
		for i, atype := range in.AttachmentTypes {
			name := ""
			if i < len(in.AttachmentNames) {
				name = in.AttachmentNames[i]
			}
			parts = append(parts, g[atype]+" "+name)
		}
		title += " | " + strings.Join(parts, " ")
	}

	_ = isOutbound // retained for parity clarity; inbound branch above is the alternative
	return title, style
}

// strftimeLayout converts a C/Python strftime format string into the equivalent
// Go time package reference-layout string, covering the specifiers nomadnet's
// time_format uses (%Y-%m-%d %H:%M:%S by default) plus the common subset.
// Unknown specifiers are passed through verbatim with their leading '%'.
func strftimeLayout(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'Y':
			b.WriteString("2006")
		case 'y':
			b.WriteString("06")
		case 'm':
			b.WriteString("01")
		case 'd':
			b.WriteString("02")
		case 'e':
			b.WriteString("_2")
		case 'H':
			b.WriteString("15")
		case 'I':
			b.WriteString("03")
		case 'M':
			b.WriteString("04")
		case 'S':
			b.WriteString("05")
		case 'p':
			b.WriteString("PM")
		case 'A':
			b.WriteString("Monday")
		case 'a':
			b.WriteString("Mon")
		case 'B':
			b.WriteString("January")
		case 'b', 'h':
			b.WriteString("Jan")
		case 'j':
			b.WriteString("002")
		case 'f':
			b.WriteString("000000")
		case 'Z':
			b.WriteString("MST")
		case 'z':
			b.WriteString("-0700")
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
