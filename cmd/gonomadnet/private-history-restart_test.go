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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rrc"
	"github.com/gmlewis/go-reticulum/rrc/cbor"
)

// historyFilePath mirrors the rrc layer's per-hub/per-room history path for a
// hub using the default "rrc.hub" destination name: storage/rrc_history/
// <hub hash hex>/<room>_<sha256(room)[:4] hex>.log.
func historyFilePath(storageDir string, hubHash []byte, room string) string {
	sum := sha256.Sum256([]byte(room))
	return filepath.Join(storageDir, "rrc_history", hex.EncodeToString(hubHash),
		fmt.Sprintf("%v_%x.log", room, sum[:4]))
}

// writeHistoryEntry appends one CBOR history entry to a room's history file,
// exactly as the rrc layer records it.
func writeHistoryEntry(t *testing.T, path string, msg *rrc.RRCMessage) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating history dir: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("opening history file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(cbor.Encode(msg.HistoryEntry())); err != nil {
		t.Fatalf("writing history entry: %v", err)
	}
}

// TestRestartKeepsBotAndPrivateRepliesFromRoomHistory reproduces the live
// failure on the Mac mini: after gonomadnet was brought back up, the room view
// showed the user's own commands ("/msg gorrcbot help", "@gorrcbot help dn")
// but NEITHER gorrbot's private replies NOR its public reply, even though all
// of them were in the stored room history file.
//
// The rows below are the mini's real rows (hub chatter, the user's commands,
// gorrbot's private K_DST replies and its public reply), aged past the
// ephemeral-notices timeout the way a reloaded file is. The client is then
// restarted over the same storage directory and the hub's greeting arrives, so
// the room view is rebuilt from the buffer the load produced.
func TestRestartKeepsBotAndPrivateRepliesFromRoomHistory(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	hubHash, err := hex.DecodeString("a012129c10205c0b9441fcd2b755b2a7")
	if err != nil {
		t.Fatalf("decoding hub hash: %v", err)
	}
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("rns.NewIdentity: %v", err)
	}
	own := id.Hash
	botHash := bytes.Repeat([]byte{0xff}, 16)
	hubIdentity := bytes.Repeat([]byte{0x52}, 16)

	// First run: register the hub and its room so the store is persisted, then
	// record the conversation the way the live client did.
	first := rrc.NewManager(dir, nil)
	t.Cleanup(first.Shutdown)
	first.SetIdentity(id)
	first.SetHistoryConfig(500, true, 600)
	firstHub := first.AddHub(hubHash, "rrc.hub", "gonomadnet Public Hub")
	firstHub.AddRoom("general")
	firstHub.SetNickOverride("glenn")

	old := time.Now().Add(-2 * time.Hour).UnixMilli()
	path := historyFilePath(dir, hubHash, "general")
	for _, msg := range []*rrc.RRCMessage{
		{Kind: "notice", Src: hubIdentity, Text: "Welcome! JOIN #general to discuss go-nomadnet and @gorrcbot.", Ts: old},
		{Kind: "system", Text: "gonomadnet on RaspPi joined", Ts: old},
		{Kind: "msg", Src: own, Nick: "glenn", Text: "/msg gorrcbot help", Ts: old},
		{Kind: "notice", Src: hubIdentity, Text: "Direct NOTICE sent to ff2fbcf2e5e57ef710e4b3a855482390 (id=82f76b781f07ba66)", Ts: old},
		{Kind: "notice", Src: botHash, Nick: "gorrcbot", Direct: true, Dst: own, Text: "Commands: botinfo, dn, help, id, ping", Ts: old},
		{Kind: "msg", Src: own, Nick: "glenn", Text: "@gorrcbot help dn", Ts: old},
		{Kind: "notice", Src: botHash, Nick: "gorrcbot", Text: "dn — send a direct NOTICE to one client.", Ts: old},
	} {
		writeHistoryEntry(t, path, msg)
	}

	// Restart: a fresh manager over the same storage directory loads the hub,
	// its room, and the stored history.
	second := rrc.NewManager(dir, nil)
	t.Cleanup(second.Shutdown)
	second.SetIdentity(id)
	second.SetHistoryConfig(500, true, 600)
	if err := second.Load(); err != nil {
		t.Fatalf("loading RRC hubs: %v", err)
	}
	hub := second.HubsSnapshot()
	if len(hub) != 1 {
		t.Fatalf("loaded %v hubs, want 1", len(hub))
	}

	// The hub's greeting arrives right after the reconnect; recording it is
	// what triggers the first ephemeral-notice cleanup, which used to erase
	// the replies the load had just restored.
	feedEnvelope(t, hub[0], rrc.MakeClientEnvelope(rrc.TypeNotice, hubIdentity, []byte("general"), nil,
		"Welcome! JOIN #general to discuss go-nomadnet and @gorrcbot.", make([]byte, 8), rrc.NowMs()))

	msgs := rrcRoomMessages(hub[0], "general")
	texts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		texts = append(texts, m.Text)
	}
	joined := strings.Join(texts, "\n")

	for _, want := range []string{
		"/msg gorrcbot help",
		"Commands: botinfo, dn, help, id, ping",
		"@gorrcbot help dn",
		"dn — send a direct NOTICE to one client.",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("room view after restart is missing %q; rows:\n%v", want, joined)
		}
	}
	for _, unwanted := range []string{"gonomadnet on RaspPi joined", "Direct NOTICE sent to"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("room view after restart kept ephemeral chatter %q; rows:\n%v", unwanted, joined)
		}
	}

	// The private reply must still read as private from gorrbot; the public
	// reply must render as ordinary peer traffic.
	var privateRow, publicRow *tui.ChannelMessage
	for i := range msgs {
		switch msgs[i].Text {
		case "Commands: botinfo, dn, help, id, ping":
			privateRow = &msgs[i]
		case "dn — send a direct NOTICE to one client.":
			publicRow = &msgs[i]
		}
	}
	if privateRow == nil || !privateRow.IsPrivate || privateRow.IsSelf {
		t.Errorf("private reply row = %+v, want a private row from gorrbot", privateRow)
	} else if privateRow.Nick != "gorrcbot" {
		t.Errorf("private reply nick = %q, want gorrbot (it renders as \"private from <gorrcbot>\")", privateRow.Nick)
	}
	if publicRow == nil || publicRow.IsPrivate {
		t.Errorf("public reply row = %+v, want an unmarked peer notice", publicRow)
	}
}
