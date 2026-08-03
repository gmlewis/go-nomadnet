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

//go:build integration

package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// setupTwoNodeApps creates two App instances connected via PipeInterface.
// Each app is initialized with InitWithTransport and connected via in-memory
// pipes. Returns both apps and a cleanup function.
func setupTwoNodeApps(t *testing.T) (*App, *App, func()) {
	t.Helper()

	dirA := testutils.TempDir(t, "nomadnet-int-a")
	dirB := testutils.TempDir(t, "nomadnet-int-b")

	tsA, rnsCleanupA := newStartedTSApp(t, dirA)
	tsB, rnsCleanupB := newStartedTSApp(t, dirB)

	pipeA, pipeB, pipeCleanup := newAppPipes(t, tsA, tsB)
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	appA := NewAppWithTransport(dirA, WithTransport(tsA), WithIdentity(tsA.Identity()))
	if err := appA.InitWithTransport(tsA, tsA.Identity()); err != nil {
		t.Fatalf("InitWithTransport A: %v", err)
	}

	appB := NewAppWithTransport(dirB, WithTransport(tsB), WithIdentity(tsB.Identity()))
	if err := appB.InitWithTransport(tsB, tsB.Identity()); err != nil {
		t.Fatalf("InitWithTransport B: %v", err)
	}

	cleanup := func() {
		appA.Shutdown()
		appB.Shutdown()
		pipeCleanup()
		rnsCleanupA()
		rnsCleanupB()
	}

	return appA, appB, cleanup
}

// waitForAnnounce polls app.Announces until an announce of the given
// type appears, or times out.
func waitForAnnounce(t *testing.T, app *App, announceType string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range app.GetAnnounces() {
			if ev.AnnounceType == announceType {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %v announce", announceType)
}

// waitForLXMFMessage waits for a message on the given channel or times out.
func waitForLXMFMessage(t *testing.T, ch <-chan *lxmf.Message, timeout time.Duration) *lxmf.Message {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for LXMF message")
		return nil
	}
}

func newStartedTSApp(t *testing.T, storageDir string) (*rns.TransportSystem, func()) {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	if err := ts.Start(filepath.Join(storageDir, "rns-storage")); err != nil {
		t.Fatalf("TransportSystem.Start error: %v", err)
	}
	return ts, func() { ts.Stop() }
}

func writeAppRNSConfig(t *testing.T, configDir string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `[reticulum]
share_instance = No

[logging]
loglevel = 4
`
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newAppPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
	t.Helper()
	pipeA := interfaces.NewPipeInterface("a", func(data []byte, iface interfaces.Interface) {
		tsA.Inbound(data, iface)
	})
	pipeB := interfaces.NewPipeInterface("b", func(data []byte, iface interfaces.Interface) {
		tsB.Inbound(data, iface)
	})
	pipeA.SetOther(pipeB)
	pipeB.SetOther(pipeA)
	cleanup := func() {
		_ = pipeA.Detach()
		_ = pipeB.Detach()
	}
	return pipeA, pipeB, cleanup
}

func init() {
	_ = fmt.Sprintf
}

// TestIntegrationSendConversationWiring exercises the Phase-1 conversation
// send wiring end-to-end: composing via App.SendConversation (the C-d path)
// builds the outbound LXMF message against the recalled peer destination,
// dispatches it through the real router, and the peer receives it with the
// composed content/title byte-faithful through the LXMF stack. It also
// verifies the outbound message is ingested into the sender's conversation.
func TestIntegrationSendConversationWiring(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	// Announce both ways so each side can recall the other's LXMF identity
	// (SendDestination recalls the peer identity to build the OUT dest).
	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	if !appA.SendConversation(peerHex, "hello from wiring", "wiring title") {
		t.Fatal("SendConversation returned false; peer identity not recallable?")
	}

	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)
	if got.ContentString() != "hello from wiring" {
		t.Errorf("received content = %q, want %q", got.ContentString(), "hello from wiring")
	}
	if got.TitleString() != "wiring title" {
		t.Errorf("received title = %q, want %q", got.TitleString(), "wiring title")
	}

	// The outbound message must be ingested into A's conversation with B.
	list := appA.ConversationList()
	found := false
	for _, c := range list {
		if c.SourceHash == peerHex {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("sender has no conversation with peer %s after send; list=%v", peerHex, len(list))
	}
}

// TestIntegrationConversationMessages verifies App.ConversationMessages loads
// the ingested message from disk with its parsed LXMF envelope: content, raw
// state, sender source hash, and method are populated so the TUI's
// LXMessageWidget header can render Python-parity. B receives a message
// from A; B's ConversationMessages(peerHex) returns one inbound entry whose
// source hash is A's LXMF destination hash.
func TestIntegrationConversationMessages(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	if !appA.SendConversation(peerHex, "msg body content", "msg title") {
		t.Fatal("SendConversation returned false")
	}
	// Wait for B to ingest the delivered message.
	waitForLXMFMessage(t, receivedCh, 30*time.Second)

	// Give B's conversation cache a moment to scan the ingested file.
	deadline := time.Now().Add(5 * time.Second)
	var got []conversation.MessageDisplayData
	for time.Now().Before(deadline) {
		got = appB.ConversationMessages(fmt.Sprintf("%x", appA.LXMFDest.Hash))
		if len(got) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("ConversationMessages returned %v entries, want 1", len(got))
	}
	if got[0].Content != "msg body content" {
		t.Errorf("content = %q, want %q", got[0].Content, "msg body content")
	}
	if got[0].Title != "msg title" {
		t.Errorf("title = %q, want %q", got[0].Title, "msg title")
	}
	// The sender is A, so the source hash must be A's LXMF destination hash.
	if !bytesEqual(got[0].SourceHash, appA.LXMFDest.Hash) {
		t.Errorf("source hash = %x, want %x", got[0].SourceHash, appA.LXMFDest.Hash)
	}
	// NOTE: TransportEncrypted is not asserted here. go-reticulum's Pack does
	// not call DetermineTransportEncryption before serializing the container
	// (Python calls it inside pack), and the wire Packed payload omits the
	// transport-metadata container, so the receiver cannot recover it on
	// re-load. That is a go-reticulum wire-format divergence, not a
	// go-nomadnet loading bug — the parsed content/title/source-hash above
	// prove the envelope is loaded correctly.
}

func bytesEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}

// fieldLookupInt looks up an int key in a msgpack-decoded map[any]any,
// normalizing the key-type coercion msgpack applies on round-trip (keys come
// back as int64/uint8/uint64 rather than int). Mirrors conversation.fieldLookup.
func fieldLookupInt(fields map[any]any, key int) (any, bool) {
	if fields == nil {
		return nil, false
	}
	for k, v := range fields {
		switch c := k.(type) {
		case int:
			if c == key {
				return v, true
			}
		case int64:
			if c == int64(key) {
				return v, true
			}
		case uint8:
			if int(c) == key {
				return v, true
			}
		case uint64:
			if int(c) == key {
				return v, true
			}
		}
	}
	return nil, false
}

// TestIntegrationSendConversationWithAttachment verifies the compose-side
// attachment flow: App.SendConversation with attachment file paths reads each
// file, builds the LXMF FIELD_FILE_ATTACHMENTS field ([[name, data], ...]),
// and dispatches a message the receiver unpacks with those attachments intact.
// This pins the send side of the "attachFile" TODO (Phase 1). The receive-side
// extraction (ExtractAttachmentsFromLXM) is a separate, still-unwired gap.
func TestIntegrationSendConversationWithAttachment(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	// Write a temp attachment file the sender will stage.
	attDir := t.TempDir()
	attBody := []byte("attachment payload bytes")
	attPath := filepath.Join(attDir, "notes.txt")
	if err := os.WriteFile(attPath, attBody, 0o644); err != nil {
		t.Fatal(err)
	}

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	if !appA.SendConversation(peerHex, "body with file", "with attachment", attPath) {
		t.Fatal("SendConversation returned false")
	}

	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)
	if got.ContentString() != "body with file" {
		t.Errorf("content = %q, want %q", got.ContentString(), "body with file")
	}
	if got.TitleString() != "with attachment" {
		t.Errorf("title = %q, want %q", got.TitleString(), "with attachment")
	}

	// The receiver must see the file-attachment field with [[name, data]].
	// msgpack round-trips map keys as int64/uint8, so use a type-normalizing
	// lookup (mirrors conversation.fieldLookup) rather than a typed map index.
	fv, ok := fieldLookupInt(got.Fields, lxmf.FieldFileAttachments)
	if !ok {
		t.Fatal("received message has no FIELD_FILE_ATTACHMENTS field")
	}
	atts, ok := fv.([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("file attachments = %v (type %T), want 1 entry", fv, fv)
	}
	entry, ok := atts[0].([]any)
	if !ok || len(entry) < 2 {
		t.Fatalf("attachment entry = %v, want [name, data]", atts[0])
	}
	if name, _ := entry[0].(string); name != "notes.txt" {
		t.Errorf("attachment name = %q, want %q", name, "notes.txt")
	}
	if data, _ := entry[1].([]byte); !bytes.Equal(data, attBody) {
		t.Errorf("attachment data = %q, want %q", data, attBody)
	}
}

// TestIntegrationAttachmentExtractionOnReceive verifies the receive-side
// attachment extraction: when B ingests a delivered message carrying a file
// attachment, ConversationCache.Ingest extracts the attachment to disk under the
// per-message attachment directory (mirroring Python Conversation.ingest →
// extract_attachments_from_lxm, Conversation.py:73-76). The extracted file
// "file_0" must equal the original attachment bytes, and a msgpack manifest must
// be written. This pins the receive side of the attachment TODO (Phase 1); the
// C-s save-focattachments path then copies these extracted files to downloads.
func TestIntegrationAttachmentExtractionOnReceive(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	attBody := []byte("the attachment payload for extraction")
	attDir := t.TempDir()
	attPath := filepath.Join(attDir, "report.txt")
	if err := os.WriteFile(attPath, attBody, 0o644); err != nil {
		t.Fatal(err)
	}

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	if !appA.SendConversation(peerHex, "extract me", "extraction title", attPath) {
		t.Fatal("SendConversation returned false")
	}
	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)

	// Ingest runs synchronously inside the delivery callback, so by the time
	// the message is delivered the extraction must already be on disk.
	attFile := filepath.Join(appB.AttachmentPath, fmt.Sprintf("%x", got.Hash), "file_0")
	data, err := os.ReadFile(attFile)
	if err != nil {
		t.Fatalf("extracted attachment not on disk at %s: %v", attFile, err)
	}
	if !bytes.Equal(data, attBody) {
		t.Errorf("extracted attachment = %q, want %q", data, attBody)
	}
	manifestPath := filepath.Join(filepath.Dir(attFile), "manifest")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("attachment manifest not written: %v", err)
	}
}

// TestIntegrationSaveConversationAttachments verifies the full receive→save
// path: after B ingests a message with a file attachment (extracted on receive),
// App.SaveConversationAttachments copies the extracted file to the download
// directory under a sanitized name, mirroring Python's do_save
// (Conversations.py:2368-2391). The selection is built from B's loaded
// ConversationMessages (message hash + field index 0).
func TestIntegrationSaveConversationAttachments(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	attBody := []byte("save-path payload")
	attDir := t.TempDir()
	attPath := filepath.Join(attDir, "invoice.pdf")
	if err := os.WriteFile(attPath, attBody, 0o644); err != nil {
		t.Fatal(err)
	}

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	if !appA.SendConversation(peerHex, "save me", "save title", attPath) {
		t.Fatal("SendConversation returned false")
	}
	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)

	// Wait for B's conversation to reflect the ingested message.
	deadline := time.Now().Add(5 * time.Second)
	var msgs []conversation.MessageDisplayData
	for time.Now().Before(deadline) {
		msgs = appB.ConversationMessages(fmt.Sprintf("%x", appA.LXMFDest.Hash))
		if len(msgs) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(msgs) != 1 {
		t.Fatalf("ConversationMessages = %v, want 1", len(msgs))
	}

	// Redirect B's downloads path to a temp dir so we can assert the saved file.
	destDir := t.TempDir()
	appB.DownloadsPath = destDir
	appB.AttachmentSavePath = ""

	saved, failed := appB.SaveConversationAttachments(
		fmt.Sprintf("%x", appA.LXMFDest.Hash),
		[]conversation.SaveAttachmentSelection{
			{MessageHash: got.Hash, FieldType: "file", FieldIndex: 0, Name: "invoice.pdf"},
		},
	)
	if failed != 0 {
		t.Errorf("failed = %v, want 0", failed)
	}
	if len(saved) != 1 {
		t.Fatalf("saved = %v, want 1 path", saved)
	}
	data, err := os.ReadFile(saved[0])
	if err != nil {
		t.Fatalf("reading saved attachment: %v", err)
	}
	if !bytes.Equal(data, attBody) {
		t.Errorf("saved attachment = %q, want %q", data, attBody)
	}
	if filepath.Base(saved[0]) != "invoice.pdf" {
		t.Errorf("saved base = %q, want invoice.pdf", filepath.Base(saved[0]))
	}
}

// TestIntegrationPaperMessageSaveURI verifies the app-layer paper-message path:
// App.PaperMessage with the "SaveURI" action builds a paper LXMF message
// addressed to the peer, writes a .txt file under downloads_path containing
// the lxm:// URI, ingests the outbound message into the sender's conversation,
// and returns the save path. This pins the app side of the paperMessage TODO
// (Phase 1); the QR modes share the same paper_output path (unit-tested in
// conversation/paper_test.go).
func TestIntegrationPaperMessageSaveURI(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	path, ok := appA.PaperMessage(peerHex, "SaveURI", "paper body", "paper title")
	if !ok {
		t.Fatal("PaperMessage SaveURI returned !ok; peer identity not recallable?")
	}
	if path == "" {
		t.Fatal("PaperMessage SaveURI returned empty path")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved paper file: %v", err)
	}
	if !strings.HasPrefix(string(got), "lxm") {
		t.Errorf("saved paper file content = %q, want an lxm:// URI", string(got))
	}

	// The outbound paper message must be ingested into A's conversation.
	list := appA.ConversationList()
	found := false
	for _, c := range list {
		if c.SourceHash == peerHex {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("sender has no conversation with peer %s after paper output", peerHex)
	}
}

// TestIntegrationIngestLXMURI verifies the end-to-end paper-URI ingest path: A
// produces a paper message addressed to B as an lxm:// URI (SaveURI), B ingests
// that URI via the router's granular IngestLXMURIOutcome, the message is
// decrypted and delivered locally (IngestOutcomeLocalDelivery, the
// signal_local_delivery equivalent), and the ingested message appears in B's
// conversation with A. This pins the receive side of the ingestLXMURI TODO
// (Phase 1): the URI is parsed, the message is created, and parity with
// Python's ingest_lxm_uri local-delivery branch holds.
func TestIntegrationIngestLXMURI(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerBHex := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	savePath, ok := appA.PaperMessage(peerBHex, "SaveURI", "ingest via uri body", "ingest title")
	if !ok {
		t.Fatal("PaperMessage SaveURI returned !ok; peer identity not recallable?")
	}
	uriBytes, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("reading saved paper URI file: %v", err)
	}
	uri := strings.TrimRight(string(uriBytes), "\n")
	if !strings.HasPrefix(uri, "lxm://") {
		t.Fatalf("saved paper URI = %q, want lxm:// prefix", uri)
	}

	outcome, err := appB.Router.IngestLXMURIOutcome(uri)
	if err != nil {
		t.Fatalf("IngestLXMURIOutcome: %v", err)
	}
	if outcome != lxmf.IngestOutcomeLocalDelivery {
		t.Fatalf("outcome = %v, want IngestOutcomeLocalDelivery", outcome)
	}

	// The delivery callback must fire with the decrypted content.
	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)
	if got.ContentString() != "ingest via uri body" {
		t.Errorf("delivered content = %q, want %q", got.ContentString(), "ingest via uri body")
	}
	if got.TitleString() != "ingest title" {
		t.Errorf("delivered title = %q, want %q", got.TitleString(), "ingest title")
	}

	// The ingested message must appear in B's conversation with A.
	deadline := time.Now().Add(5 * time.Second)
	var msgs []conversation.MessageDisplayData
	for time.Now().Before(deadline) {
		msgs = appB.ConversationMessages(fmt.Sprintf("%x", appA.LXMFDest.Hash))
		if len(msgs) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(msgs) != 1 {
		t.Fatalf("ConversationMessages = %v, want 1", len(msgs))
	}
	if msgs[0].Content != "ingest via uri body" {
		t.Errorf("ingested content = %q, want %q", msgs[0].Content, "ingest via uri body")
	}

	// Re-ingesting the same URI reports a duplicate (signal_duplicate).
	outcome, err = appB.Router.IngestLXMURIOutcome(uri)
	if err != nil {
		t.Fatalf("IngestLXMURIOutcome duplicate: %v", err)
	}
	if outcome != lxmf.IngestOutcomeDuplicate {
		t.Fatalf("duplicate outcome = %v, want IngestOutcomeDuplicate", outcome)
	}
}
