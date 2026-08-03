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

package conversation

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// newPaperDest creates an outbound LXMF delivery destination suitable for
// paper-message encryption (the destination must be announceable so its public
// key is available to encrypt to).
func newPaperDest(t *testing.T) *rns.Destination {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

// paperDeps builds a fakeSendDeps wired for paper output: outbound
// destinations, a downloads/tmp directory, and a configurable PrintFile.
func paperDeps(t *testing.T) (*fakeSendDeps, *rns.Destination, string, string) {
	t.Helper()
	downloads := t.TempDir()
	tmpFiles := t.TempDir()
	dest := newPaperDest(t)
	source := newPaperDest(t)
	deps := &fakeSendDeps{
		dest:          dest,
		source:        source,
		convPath:      tempDir(t),
		downloadsPath: downloads,
		tmpFilesPath:  tmpFiles,
		printResult:   true,
	}
	return deps, dest, downloads, tmpFiles
}

// wantPaperPath builds the expected save path for the ingested message hash.
func wantPaperPath(t *testing.T, downloads string, deps *fakeSendDeps, ext string) string {
	t.Helper()
	if deps.lastIngest == nil {
		t.Fatal("no message ingested")
	}
	return filepath.Join(downloads, "LXM_"+hex.EncodeToString(deps.lastIngest.Hash)+"."+ext)
}

// TestPaperOutputSaveURI verifies save_uri mode writes a .txt file containing
// the paper-message URI followed by a newline to downloads_path, ingests the
// outbound message, and returns the save path.
func TestPaperOutputSaveURI(t *testing.T) {
	t.Parallel()

	deps, dest, downloads, _ := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	path, ok := c.PaperOutput("hello paper", "a title", PaperSaveURI)
	if !ok {
		t.Fatal("PaperOutput save_uri returned !ok")
	}
	if want := wantPaperPath(t, downloads, deps, "txt"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved URI file: %v", err)
	}
	if !strings.HasSuffix(string(got), "\n") {
		t.Errorf("saved URI missing trailing newline: %q", string(got))
	}
	uri := strings.TrimSuffix(string(got), "\n")
	if !strings.HasPrefix(uri, lxmf.URISchema+"://") {
		t.Errorf("URI %q does not start with %q", uri, lxmf.URISchema+"://")
	}
	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %v times, want 1", deps.ingestCount)
	}
	if len(deps.ingestOrigin) != 1 || !deps.ingestOrigin[0] {
		t.Error("Ingest not called with originator=true")
	}
}

// TestPaperOutputReturnURI verifies return_uri mode returns the paper URI
// without writing a file or ingesting.
func TestPaperOutputReturnURI(t *testing.T) {
	t.Parallel()

	deps, dest, _, _ := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	uri, ok := c.PaperOutput("hello paper", "", PaperReturnURI)
	if !ok {
		t.Fatal("PaperOutput return_uri returned !ok")
	}
	if !strings.HasPrefix(uri, lxmf.URISchema+"://") {
		t.Errorf("URI %q does not start with %q", uri, lxmf.URISchema+"://")
	}
	if deps.ingestCount != 0 {
		t.Errorf("Ingest called %v times, want 0", deps.ingestCount)
	}
}

// TestPaperOutputSaveQR verifies save_qr mode writes a valid PNG file whose
// name embeds the message hash to downloads_path and ingests the message.
func TestPaperOutputSaveQR(t *testing.T) {
	t.Parallel()

	deps, dest, downloads, _ := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	path, ok := c.PaperOutput("qr body", "title", PaperSaveQR)
	if !ok {
		t.Fatal("PaperOutput save_qr returned !ok")
	}
	if want := wantPaperPath(t, downloads, deps, "png"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved QR file: %v", err)
	}
	pngSig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(got) < len(pngSig) || string(got[:len(pngSig)]) != string(pngSig) {
		t.Errorf("saved QR file is not a PNG: % x", got[:min(len(got), 8)])
	}
	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %v times, want 1", deps.ingestCount)
	}
}

// TestPaperOutputPrintQRSuccess verifies print_qr mode writes a QR PNG to the
// tmp dir, invokes PrintFile, removes the tmp file, and ingests only when
// printing succeeds.
func TestPaperOutputPrintQRSuccess(t *testing.T) {
	t.Parallel()

	deps, dest, _, tmpFiles := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	_, ok := c.PaperOutput("print body", "", PaperPrintQR)
	if !ok {
		t.Fatal("PaperOutput print_qr returned !ok with PrintFile succeeding")
	}
	if deps.printCalls != 1 {
		t.Errorf("PrintFile called %v times, want 1", deps.printCalls)
	}
	if deps.ingestCount != 1 {
		t.Errorf("Ingest called %v times, want 1 after successful print", deps.ingestCount)
	}
	entries, err := os.ReadDir(tmpFiles)
	if err != nil {
		t.Fatalf("reading tmp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("tmp dir not cleaned up: %v entries remain", len(entries))
	}
}

// TestPaperOutputPrintQRFailure verifies that when PrintFile fails, print_qr
// returns false and does not ingest the message.
func TestPaperOutputPrintQRFailure(t *testing.T) {
	t.Parallel()

	deps, dest, _, _ := paperDeps(t)
	deps.printResult = false
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	_, ok := c.PaperOutput("print body", "", PaperPrintQR)
	if ok {
		t.Error("PaperOutput print_qr returned ok with PrintFile failing")
	}
	if deps.ingestCount != 0 {
		t.Errorf("Ingest called %v times, want 0 after failed print", deps.ingestCount)
	}
}

// TestPaperOutputNoDestination verifies PaperOutput returns false without
// touching the router when no send destination is configured.
func TestPaperOutputNoDestination(t *testing.T) {
	t.Parallel()

	deps, _, _, _ := paperDeps(t)
	deps.dest = nil
	c := NewConversation("deadbeef", deps.convPath)
	c.SetSendDeps(deps)

	_, ok := c.PaperOutput("body", "", PaperSaveURI)
	if ok {
		t.Error("PaperOutput returned ok with no destination")
	}
	if deps.ingestCount != 0 {
		t.Errorf("Ingest called %v times, want 0", deps.ingestCount)
	}
}

// TestPaperOutputEmptyContent verifies empty content is still accepted (Python
// guards on content at the TUI layer, not in paper_output).
func TestPaperOutputEmptyContent(t *testing.T) {
	t.Parallel()

	deps, dest, _, _ := paperDeps(t)
	c := NewConversation(hex.EncodeToString(dest.Hash), deps.convPath)
	c.SetSendDeps(deps)

	_, ok := c.PaperOutput("", "", PaperReturnURI)
	if !ok {
		t.Error("PaperOutput return_uri with empty content returned !ok")
	}
}
