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
	"errors"
	"strings"
	"testing"
)

// newMsgWidget returns a connected room widget whose private-command callback
// records what the /msg command forwarded.
func newMsgWidget(t *testing.T) (*RoomWidget, *[]string) {
	t.Helper()
	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")
	rw.hubConnected = true
	rw.SetOwnNick("glenn")
	forwarded := &[]string{}
	rw.OnPrivateCommand = func(arg string) error {
		*forwarded = append(*forwarded, arg)
		return nil
	}
	return rw, forwarded
}

// renderedRow returns the rendered text of one chat row, so an assertion can
// check what the user actually sees.
func renderedRow(t *testing.T, rw *RoomWidget, idx int) string {
	t.Helper()
	msgs := rw.ChatMessages()
	if idx < 0 || idx >= len(msgs) {
		t.Fatalf("row %v does not exist (%v rows)", idx, len(msgs))
	}
	return formatRRCMessage(msgs[idx], rw.renderOpts())
}

// rowTexts flattens the widget's rows for assertions.
func rowTexts(rw *RoomWidget) []string {
	var out []string
	for _, msg := range rw.ChatMessages() {
		out = append(out, msg.Text)
	}
	return out
}

// TestMsgCommandEchoesTheTypedLine asserts /msg forwards the argument verbatim
// and echoes the line the user typed, exactly as typed — the way an IRC client
// echoes its own input — rather than a rewritten paraphrase of it.
func TestMsgCommandEchoesTheTypedLine(t *testing.T) {
	t.Parallel()

	const line = "/msg minipc yo dude, whazzup?"
	rw, forwarded := newMsgWidget(t)
	rw.handleSlashCommand(line)

	if len(*forwarded) != 1 || (*forwarded)[0] != "minipc yo dude, whazzup?" {
		t.Fatalf("forwarded = %v, want one line with the target and text", *forwarded)
	}
	msgs := rw.ChatMessages()
	if len(msgs) != 1 {
		t.Fatalf("rows = %v, want one echo", rowTexts(rw))
	}
	if !msgs[0].IsSelf {
		t.Errorf("echo row = %+v, want a self row", msgs[0])
	}
	if msgs[0].IsPrivate {
		t.Error("the echo is marked private; the typed line already carries the command")
	}
	if msgs[0].Text != line {
		t.Errorf("echo text = %q, want the unaltered typed line %q", msgs[0].Text, line)
	}
	if msgs[0].Nick != "glenn" {
		t.Errorf("echo nick = %q, want the local nick", msgs[0].Nick)
	}
	if rw.editor.GetText() != "" {
		t.Errorf("composer still holds %q, want it cleared", rw.editor.GetText())
	}
	if got := renderedRow(t, rw, 0); !strings.Contains(got, line) {
		t.Errorf("rendered echo = %q, want the typed line", got)
	}
}

// TestMsgCommandEchoIsRecordedOnTheHub asserts the echo is also recorded in the
// room buffer: the room view is rebuilt from that buffer on every hub refresh,
// so an echo kept only in the widget would vanish as soon as the private reply
// arrived.
func TestMsgCommandEchoIsRecordedOnTheHub(t *testing.T) {
	t.Parallel()

	rw, _ := newMsgWidget(t)
	recorded := &[]string{}
	rw.OnLocalMessage = func(kind, text string) error {
		*recorded = append(*recorded, kind+"|"+text)
		return nil
	}
	rw.handleSlashCommand("/msg gorrcbot help")

	if len(*recorded) != 1 || (*recorded)[0] != "msg|/msg gorrcbot help" {
		t.Errorf("recorded = %v, want one msg row holding the typed line", *recorded)
	}
}

// TestMsgCommandAcceptsTheHubAliases asserts /dn and /dnotice reach the hub
// exactly as /msg does, so a user who reads the hub's command list is not met
// with "Unknown command".
func TestMsgCommandAcceptsTheHubAliases(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"/msg minipc hello", "/dn minipc hello", "/dnotice minipc hello"} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			rw, forwarded := newMsgWidget(t)
			rw.handleSlashCommand(line)
			if len(*forwarded) != 1 || (*forwarded)[0] != "minipc hello" {
				t.Fatalf("%q forwarded %v, want the target and text", line, *forwarded)
			}
			msgs := rw.ChatMessages()
			if len(msgs) != 1 || msgs[0].Text != line {
				t.Errorf("%q rows = %v, want the typed line echoed unchanged", line, rowTexts(rw))
			}
		})
	}
}

// TestMsgCommandQuotedTargetKeepsItsSpaces asserts a quoted nick is forwarded
// unchanged, because the hub parses the line and a nick is never guessed at.
func TestMsgCommandQuotedTargetKeepsItsSpaces(t *testing.T) {
	t.Parallel()

	rw, forwarded := newMsgWidget(t)
	rw.handleSlashCommand(`/msg 'gonomadnet on MiniPC' yo dude`)

	if len(*forwarded) != 1 || (*forwarded)[0] != `'gonomadnet on MiniPC' yo dude` {
		t.Fatalf("forwarded = %v, want the quoted target unchanged", *forwarded)
	}
	msgs := rw.ChatMessages()
	const line = `/msg 'gonomadnet on MiniPC' yo dude`
	if len(msgs) != 1 || msgs[0].Text != line {
		t.Fatalf("echo rows = %v, want the typed line echoed unchanged", rowTexts(rw))
	}
}

// TestMsgCommandUsageAndFailureRows asserts the two local error paths: a
// missing target or text never reaches the hub, and a hub error is reported
// without an echo claiming the message was sent.
func TestMsgCommandUsageAndFailureRows(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		line    string
		wantRow string
	}{
		{name: "no argument", line: "/msg", wantRow: "Usage: /msg <nick|hash> <text>"},
		{name: "target only", line: "/msg minipc", wantRow: "Usage: /msg <nick|hash> <text>"},
		{name: "unterminated quote", line: "/msg 'minipc yo", wantRow: "Usage: /msg <nick|hash> <text>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rw, forwarded := newMsgWidget(t)
			rw.handleSlashCommand(tc.line)
			if len(*forwarded) != 0 {
				t.Errorf("%q forwarded %v, want nothing", tc.line, *forwarded)
			}
			texts := rowTexts(rw)
			if len(texts) != 1 || texts[0] != tc.wantRow {
				t.Errorf("%q rows = %v, want the usage notice", tc.line, texts)
			}
		})
	}

	t.Run("hub reports an error", func(t *testing.T) {
		t.Parallel()
		rw, _ := newMsgWidget(t)
		rw.OnPrivateCommand = func(string) error {
			return errors.New("hub does not support private commands")
		}
		rw.handleSlashCommand("/msg minipc hi")
		texts := rowTexts(rw)
		if len(texts) != 1 || !strings.Contains(texts[0], "hub does not support private commands") {
			t.Errorf("rows = %v, want the hub error reported", texts)
		}
		if len(rw.ChatMessages()) != 1 {
			t.Errorf("rows = %v, want no echo after a failed send", texts)
		}
	})
}

// TestMsgCommandRequiresAConnectedHub asserts /msg never fires while the hub
// link is down.
func TestMsgCommandRequiresAConnectedHub(t *testing.T) {
	t.Parallel()

	rw, forwarded := newMsgWidget(t)
	rw.hubConnected = false
	rw.handleSlashCommand("/msg minipc hi")

	if len(*forwarded) != 0 {
		t.Errorf("forwarded %v while disconnected, want nothing", *forwarded)
	}
	texts := rowTexts(rw)
	if len(texts) != 1 || !strings.Contains(texts[0], "Not connected to hub") {
		t.Errorf("rows = %v, want the disconnected notice", texts)
	}
}
