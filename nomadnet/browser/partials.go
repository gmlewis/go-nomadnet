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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/rns"
)

// Partial is a page partial reference extracted from Micron markup, mirroring
// the metadata Python attaches to the parsed partial's urwid Pile
// (MicronParser.parse_partial, MicronParser.py:185-192). The browser fetches
// each partial's URL on a refresh schedule and substitutes the rendered content
// into the page (Browser.__load_partial / partial_received).
type Partial struct {
	// Hash is the 64-hex SHA-256 of Descriptor, matching Python's
	// pile.partial_hash = RNS.hexrep(RNS.Identity.full_hash(descriptor), delimit=False).
	Hash string
	// URL is the partial's request URL (partial_url); may be relative (":<path>")
	// and is resolved against the page's destination hash via ParseURL.
	URL string
	// Fields is the pipe-split partial_fields list ([""] when none declared).
	Fields []string
	// ID is the partial_id from a "pid=<id>" field entry, "" when absent.
	ID string
	// Refresh is the partial_refresh interval in seconds (0 when no refresh or
	// refresh < 1, matching Python's `if partial_refresh < 1: partial_refresh = None`).
	Refresh float64
	// Descriptor is the "|"-joined raw components — the SHA-256 hash input
	// (Python: "|".join(partial_components)).
	Descriptor string
	// Raw is the full directive as it appeared in the markup ("`{...}"). The
	// browser substitutes the rendered partial content for this substring on
	// refresh (Python replaces the partial's urwid Pile slot instead).
	Raw string
}

// PartialHash returns the 64-hex lowercase SHA-256 of descriptor, matching
// Python RNS.hexrep(RNS.Identity.full_hash(descriptor.encode("utf-8")), delimit=False)
// (MicronParser.py:189). RNS.Identity.full_hash is SHA-256.
func PartialHash(descriptor string) string {
	sum := sha256.Sum256([]byte(descriptor))
	return hex.EncodeToString(sum[:])
}

// ExtractPartials parses Micron markup and returns the partial references it
// declares, mirroring Python MicronParser.parse_partial (each "`{...}" line →
// one partial). The URL/fields/ID/refresh/descriptor are taken from the Go
// micron parser's NodePartial (which already pins Python parse_partial); the
// hash is computed via PartialHash. A partial with an empty URL yields no entry
// (Python returns None — e.g. the 4-component else branch, or an empty url).
func ExtractPartials(markup string) []Partial {
	nodes := micron.Parse(markup)
	var out []Partial
	for _, n := range nodes {
		if n.Type != micron.NodePartial || n.PartialURL == "" {
			continue
		}
		out = append(out, Partial{
			Hash:       PartialHash(n.PartialDescriptor),
			URL:        n.PartialURL,
			Fields:     n.PartialFields,
			ID:         n.PartialID,
			Refresh:    n.PartialRefresh,
			Descriptor: n.PartialDescriptor,
			Raw:        n.PartialRaw,
		})
	}
	return out
}

// PartialRequestData builds the request-data map for a partial fetch from its
// fields, mirroring the pure-logic part of Python Browser.__get_partial_request_data
// (Browser.py:763-777): each "k=v" field becomes requestData["var_k"]="v"
// (split on the first "=" with exactly 2 parts), and a field without "=" is a
// linkField — a form-field name whose live value the TUI must resolve into a
// "field_<name>" entry (the urwid form-widget walk at Browser.py:779-812 is
// TUI glue, not pure logic, and is NOT done here).
//
// Mirroring Python, requestData is always a non-nil map ({} when fields has no
// "=" entries) because Python sets request_data = {} whenever partial["fields"]
// is non-None (always, since fields defaults to [""]). The returned linkFields
// is the list of non-"=" entries (form-field names the TUI should resolve).
func PartialRequestData(fields []string) (requestData map[string]string, linkFields []string) {
	requestData = map[string]string{}
	for _, e := range fields {
		if strings.Contains(e, "=") {
			c := strings.Split(e, "=")
			if len(c) == 2 {
				requestData["var_"+c[0]] = c[1]
			}
			continue
		}
		linkFields = append(linkFields, e)
	}
	return requestData, linkFields
}

// FetchPartial fetches a single partial's Micron markup over RNS, mirroring
// Python Browser.__load_partial (Browser.py:707-761). It resolves the partial's
// URL (relative ":<path>" URLs resolve against currentDest, the page's
// destination hash), builds the request data via PartialRequestData, and reuses
// the shared fetchBytes link-establish + request core. The returned bytes are
// the partial's raw Micron markup (Python partial_received stores
// request_receipt.response.decode("utf-8").rstrip()).
//
// The form-widget field_* collection (linkFields → live form values) is TUI
// glue and is NOT applied here — only the var_* entries from PartialRequestData
// are sent. For static node pages (which ignore request_data) this is a no-op
// difference; for dynamic pages it matches Python's var_* behavior.
//
// onProgress (may be nil) reports transfer progress; timeout bounds the fetch
// (DefaultTimeout when <= 0). onLinkEstablished (may be nil) lets the caller
// identify to the remote node when the directory requests it (see fetchBytes).
// The link is torn down after the fetch (one-shot).
func FetchPartial(ts *rns.TransportSystem, partial Partial, currentDest []byte, timeout time.Duration, onProgress func(float64), onLinkEstablished func(*rns.Link)) ([]byte, error) {
	rd, _ := PartialRequestData(partial.Fields)
	// PartialRequestData always returns a non-nil map (matching Python, which
	// sets request_data = {} whenever fields is non-None). go-reticulum's
	// server-side handleRequest decodes the request-data element as []byte only,
	// so a non-nil map — even an empty {} — is dropped and the request yields no
	// response (see memory: go-reticulum-transport-encrypted-wire-gap /
	// browser-fetch-backend). An empty map carries no data, so pass nil for it
	// (works against Go and Python static pages, which ignore request_data). A
	// populated map is sent verbatim (correct wire parity with Python); the
	// receiving node's handling of map data is go-reticulum's concern.
	if len(rd) == 0 {
		rd = nil
	}
	dest, path, _, err := ParseURL(partial.URL, currentDest, rd)
	if err != nil {
		return nil, err
	}
	data, link, err := fetchBytes(ts, dest, path, rd, timeout, onProgress, onLinkEstablished)
	if err != nil {
		return nil, err
	}
	link.Teardown()
	return data, nil
}
