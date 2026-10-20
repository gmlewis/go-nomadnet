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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/gmlewis/go-reticulum/rrc"
	"github.com/gmlewis/tcell/v2"
)

// newDownloadTestDisplay builds a BrowserDisplay over a simulation screen so the
// download-completion branch can be driven without a terminal. Not parallel: it
// touches the screen.
func newDownloadTestDisplay(t *testing.T) *tui.BrowserDisplay {
	t.Helper()
	app := tui.NewApp(tui.ThemeDark, tui.GlyphUnicode, tui.ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(100, 30)
	return tui.NewBrowserDisplay(app)
}

// TestCompleteRemoteFilePath pins the /file/ branch of the remote fetch: a file
// path is written to the downloads directory and reported as "Saved <name>" in
// the footer, and a page path is left entirely alone for the caller's page path
// (cache + render) to handle.
//
// Python makes the same split in Browser.load_page: a path starting with
// "/file/" routes to download_file (Browser.py:932-940); anything else is a
// page.
func TestCompleteRemoteFilePath(t *testing.T) {
	body := []byte("download body bytes\n")

	t.Run("a file path is saved and reported", func(t *testing.T) {
		bd := newDownloadTestDisplay(t)
		dir := tempDir(t)

		if !completeRemoteFilePath(bd, dir, "/file/report.txt", body, nil, 1.5) {
			t.Fatal("completeRemoteFilePath = false for a /file/ path, want true")
		}

		saved := filepath.Join(dir, "report.txt")
		got, err := os.ReadFile(saved)
		if err != nil {
			t.Fatalf("the download was not written to %v: %v", saved, err)
		}
		if string(got) != string(body) {
			t.Errorf("saved bytes = %q, want %q", got, body)
		}
		if footer := bd.FooterText(); !strings.HasPrefix(footer, "Saved report.txt") {
			t.Errorf("footer = %q, want it to start with %q", footer, "Saved report.txt")
		}
	})

	t.Run("a nested file path keeps its basename", func(t *testing.T) {
		bd := newDownloadTestDisplay(t)
		dir := tempDir(t)

		if !completeRemoteFilePath(bd, dir, "/file/docs/readme.md", body, nil, 0.5) {
			t.Fatal("completeRemoteFilePath = false for a nested /file/ path, want true")
		}
		if _, err := os.Stat(filepath.Join(dir, "readme.md")); err != nil {
			t.Errorf("nested download did not land as readme.md: %v", err)
		}
		if footer := bd.FooterText(); !strings.HasPrefix(footer, "Saved readme.md") {
			t.Errorf("footer = %q, want it to start with %q", footer, "Saved readme.md")
		}
	})

	t.Run("the response metadata names the saved file", func(t *testing.T) {
		bd := newDownloadTestDisplay(t)
		dir := tempDir(t)

		metadata := map[string][]byte{"name": []byte("from-server.txt")}
		if !completeRemoteFilePath(bd, dir, "/file/request-name.txt", body, metadata, 0.75) {
			t.Fatal("completeRemoteFilePath = false for a /file/ path, want true")
		}
		if _, err := os.Stat(filepath.Join(dir, "from-server.txt")); err != nil {
			t.Errorf("download was not saved under the response metadata name: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "request-name.txt")); err == nil {
			t.Error("download was saved under the request path despite the response metadata name")
		}
		if footer := bd.FooterText(); !strings.HasPrefix(footer, "Saved from-server.txt") {
			t.Errorf("footer = %q, want it to start with %q", footer, "Saved from-server.txt")
		}
	})

	t.Run("a page path is not touched", func(t *testing.T) {
		bd := newDownloadTestDisplay(t)
		dir := tempDir(t)

		if completeRemoteFilePath(bd, dir, "/page/index.mu", body, nil, 1.0) {
			t.Fatal("completeRemoteFilePath = true for a page path, want false")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("a page fetch wrote %v entries into the downloads directory", len(entries))
		}
		if footer := bd.FooterText(); strings.Contains(footer, "Saved ") {
			t.Errorf("footer = %q, want no download line for a page fetch", footer)
		}
	})
}

// TestShouldRingMentionBell pins the bell's gate, mirroring Python's
// _on_rrc_message (Channels.py:2262-2271): only a message that mentions the local
// nick rings, and only when its room is not the one already on screen. The bell
// itself (debounce, the BEL byte) is covered by tui/mention-bell_test.go; this
// pins the call site's gate.
func TestShouldRingMentionBell(t *testing.T) {
	t.Parallel()

	newApp := func(t *testing.T) (*app.App, *rrc.RRCHub, *rrc.RRCHub) {
		t.Helper()
		a := app.NewApp(tempDir(t), "", false, false)
		a.RRC = rrc.NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
		hub := a.RRC.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
		other := a.RRC.AddHub([]byte("otherhash"), "rrc.chat", "OtherHub")
		return a, hub, other
	}

	tests := []struct {
		name    string
		mention bool
		// active selects which hub/room the manager reports as on screen.
		activeHub  string // "hub", "other" or "" for none
		activeRoom string
		room       string
		want       bool
	}{
		{"a mention in an unseen room rings", true, "", "", "general", true},
		{"a mention in another hub rings", true, "other", "general", "general", true},
		{"a mention in the room on screen stays silent", true, "hub", "general", "general", false},
		{"a mention in a different room of the active hub rings", true, "hub", "lounge", "general", true},
		{"the active room is matched case-insensitively", true, "hub", "GENERAL", "general", false},
		{"a message without a mention never rings", false, "hub", "lounge", "general", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, hub, other := newApp(t)
			msg := &rrc.RRCMessage{Kind: "msg", Room: tc.room, Mention: tc.mention}
			switch tc.activeHub {
			case "hub":
				a.RRC.SetActive(hub, tc.activeRoom)
			case "other":
				a.RRC.SetActive(other, tc.activeRoom)
			}

			if got := shouldRingMentionBell(a, hub, msg); got != tc.want {
				t.Errorf("shouldRingMentionBell = %v, want %v", got, tc.want)
			}
		})
	}
}
