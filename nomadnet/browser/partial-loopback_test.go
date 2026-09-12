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

package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-reticulum/testutils"
)

// TestFetchPartialServesLocalNodeLocally pins that a partial whose destination
// is the local node is served from the local pages directory rather than over
// an RNS link, mirroring the loopback branch Python Browser.load_page applies
// to pages (Browser.py:1300-1320). RNS has no self-loopback, so a node's own
// browser can otherwise never render a partial of its own page.
//
// A nil transport is passed deliberately: it proves the loopback branch
// short-circuits before any transport use (fetchBytes with a nil transport
// returns ErrNoPath).
func TestFetchPartialServesLocalNodeLocally(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "browser-partial-loopback")
	body := ">Partial heading\nPartial body.\n"
	if err := os.WriteFile(filepath.Join(pages, "part.mu"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	localHash := []byte{
		0xc7, 0xd0, 0xe7, 0xbb, 0xd8, 0x83, 0xe5, 0x95,
		0xf5, 0x3e, 0x14, 0xfa, 0x69, 0x86, 0x18, 0x8c,
	}
	data, err := FetchPartial(context.Background(), nil, Partial{URL: ":/page/part.mu", Fields: []string{""}},
		PartialFetch{CurrentDest: localHash, LoopbackDest: localHash, PagesPath: pages})
	if err != nil {
		t.Fatalf("FetchPartial: %v", err)
	}
	if got := string(data); got != body {
		t.Errorf("partial = %q, want %q", got, body)
	}
}

// TestFetchPartialLoopbackMissingPageErrors pins that a local partial which
// does not exist reports an error rather than rendering the not-found body as
// if it were the partial's markup (Python reports the failure too, and the
// browser renders "Could not load partial <url>").
func TestFetchPartialLoopbackMissingPageErrors(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "browser-partial-missing")
	localHash := []byte{0x01, 0x02}
	_, err := FetchPartial(context.Background(), nil, Partial{URL: ":/page/absent.mu", Fields: []string{""}},
		PartialFetch{CurrentDest: localHash, LoopbackDest: localHash, PagesPath: pages})
	if err == nil {
		t.Fatal("FetchPartial error = nil, want a not-found error")
	}
	if !strings.Contains(err.Error(), "absent.mu") {
		t.Errorf("error = %v, want it to name the missing page", err)
	}
}

// TestFetchPartialRemoteDestinationUsesTransport pins the other side of the
// decision: when the partial's destination is NOT the local node, the fetch
// goes to the network. A nil transport then fails with ErrNoPath, which is what
// proves the remote path was taken rather than a local read.
func TestFetchPartialRemoteDestinationUsesTransport(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "browser-partial-remote")
	if err := os.WriteFile(filepath.Join(pages, "part.mu"), []byte("local copy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	remoteHash := []byte{0xaa}
	localHash := []byte{0xbb}
	_, err := FetchPartial(context.Background(), nil, Partial{URL: ":/page/part.mu", Fields: []string{""}},
		PartialFetch{CurrentDest: remoteHash, LoopbackDest: localHash, PagesPath: pages})
	if err == nil {
		t.Fatal("FetchPartial error = nil, want ErrNoPath from the network path")
	}
	if !strings.Contains(err.Error(), ErrNoPath.Error()) {
		t.Errorf("error = %v, want it to carry %v", err, ErrNoPath)
	}
}

// TestFetchPartialLoopbackCarriesRequestData pins that a page can address its
// own counter: the partial's "k=v" field reaches the locally served page as
// var_k=v, exactly as it would over a remote link, so a loopback browse shows
// the same per-page count a visitor sees.
func TestFetchPartialLoopbackCarriesRequestData(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "browser-partial-loopback-data")
	if err := os.WriteFile(filepath.Join(pages, "part.mu"), []byte("static\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	localHash := []byte{0x77}
	partial := Partial{URL: ":/page/part.mu", Fields: []string{"page=index.mu"}}
	rd, linkFields := PartialRequestData(partial.Fields)
	if len(linkFields) != 0 {
		t.Fatalf("linkFields = %v, want none", linkFields)
	}
	if got := rd["var_page"]; got != "index.mu" {
		t.Fatalf("request data var_page = %q, want index.mu", got)
	}

	data, err := FetchPartial(context.Background(), nil, partial,
		PartialFetch{CurrentDest: localHash, LoopbackDest: localHash, PagesPath: pages})
	if err != nil {
		t.Fatalf("FetchPartial: %v", err)
	}
	if got := string(data); got != "static\n" {
		t.Errorf("partial = %q, want %q", got, "static\n")
	}
}
