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

// Package browser implements the nomadnet page-fetch backend: parsing nomadnet
// RNS URLs, establishing an RNS link to a remote nomadnetwork.node destination,
// requesting a Micron page, and returning the raw markup bytes for the TUI
// layer to render.
//
// This is the Go port of Python nomadnet's Browser (nomadnet/ui/textui/Browser.py,
// 1848 lines). The URL-parsing rules and request shape are pinned by golden
// values captured from the installed Python 3.14 nomadnet (see url_test.go).
// The fetch runs over go-reticulum's rns.Link.Establish + Link.Request (the
// same primitives the node side serves pages with — see nomadnet/node).
package browser

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// Constants matching Python nomadnet Browser (Browser.py:73-88).
const (
	// DefaultPath is the path requested when a URL carries no path, matching
	// Python Browser.DEFAULT_PATH = "/page/index.mu".
	DefaultPath = "/page/index.mu"
	// DefaultTimeout is the link/path establishment timeout in seconds,
	// matching Python Browser.DEFAULT_TIMEOUT = 10.
	DefaultTimeout = 10
	// DefaultCacheTime is the page-cache time-to-live in seconds, matching
	// Python Browser.DEFAULT_CACHE_TIME = 12*60*60.
	DefaultCacheTime = 12 * 60 * 60
)

// Request-lifecycle status states, matching Python Browser (Browser.py:77-88).
// The browser footer surfaces these via StatusText during a fetch.
const (
	StatusNoPath            = 0x00
	StatusPathRequested     = 0x01
	StatusEstablishingLink  = 0x02
	StatusLinkTimeout       = 0x03
	StatusLinkEstablished   = 0x04
	StatusRequesting        = 0x05
	StatusRequestSent       = 0x06
	StatusRequestFailed     = 0x07
	StatusRequestTimeout    = 0x08
	StatusReceivingResponse = 0x09
	StatusDisconnected      = 0xFE
	StatusDone              = 0xFF
)

// StatusText returns the human-readable status string for a request-lifecycle
// state, matching Python Browser.status_text (Browser.py:1756-1802). The DONE
// state returns just "Done"; Python appends a size/time/speed stats suffix
// computed from the response receipt, which the TUI layer adds (it depends on
// response_size/response_transfer_size/response_time, not the status alone).
// An unrecognized status yields "Browser Status Unknown".
func StatusText(status int) string {
	switch status {
	case StatusNoPath:
		return "No path to destination known"
	case StatusPathRequested:
		return "Path requested, waiting for path..."
	case StatusEstablishingLink:
		return "Establishing link..."
	case StatusLinkTimeout:
		return "Link establishment timed out"
	case StatusLinkEstablished:
		return "Link established"
	case StatusRequesting:
		return "Sending request..."
	case StatusRequestSent:
		return "Request sent, awaiting response..."
	case StatusRequestFailed:
		return "Request failed"
	case StatusRequestTimeout:
		return "Request timed out"
	case StatusReceivingResponse:
		return "Receiving response..."
	case StatusDone:
		return "Done"
	case StatusDisconnected:
		return "Disconnected"
	default:
		return "Browser Status Unknown"
	}
}

// ErrToStatus maps a fetch error to the request-lifecycle status Python would
// set for that failure mode (Python request_failed/request_timeout/link
// establishment timeout, Browser.py:1465-1473/1686-1734), so the TUI can surface
// the matching StatusText. A nil error maps to StatusDone; an unrecognized error
// maps to StatusRequestFailed (Python's generic failure state).
func ErrToStatus(err error) int {
	switch err {
	case nil:
		return StatusDone
	case ErrNoPath:
		return StatusNoPath
	case ErrLinkTimeout:
		return StatusLinkTimeout
	case ErrRequestFailed:
		return StatusRequestFailed
	case ErrRequestTimeout:
		return StatusRequestTimeout
	default:
		return StatusRequestFailed
	}
}

// destHashHexLen is the hex-character length of an RNS destination (truncated)
// hash: RNS.Reticulum.TRUNCATED_HASHLENGTH//8 = 16 bytes → 32 hex chars in the
// installed RNS version (128-bit truncated hashes).
const destHashHexLen = 32

// ErrMalformedURL is returned by ParseURL for a URL that does not match the
// nomadnet address grammar, matching Python's `raise ValueError("Malformed URL")`.
var ErrMalformedURL = errors.New("Malformed URL")

// Fetch errors, mirroring the Python Browser status states that surface as
// failures during page retrieval (Browser.py __load / request_timeout).
var (
	ErrNoPath         = errors.New("no path to destination")
	ErrLinkTimeout    = errors.New("link establishment timed out")
	ErrRequestFailed  = errors.New("page request failed")
	ErrRequestTimeout = errors.New("page request timed out")
)

// nodeAspect is the RNS application/aspect a nomadnetwork node serves pages on.
// Python constructs the destination as RNS.Destination(identity, OUT, SINGLE,
// "nomadnetwork", "node") (Browser.py:1416-1420).
var nodeAspect = []string{"nomadnetwork", "node"}

// MarkedLinkTarget builds the marked-link target string the way Python
// Browser.marked_link does (Browser.py:173-176): when fields is a non-empty
// slice, the fields are joined with "|" and appended to target after a backtick
// ("<target>`<f1|f2>"). A nil/empty fields slice leaves the target unchanged
// (Python `if link_fields:` is falsy for None and []).
func MarkedLinkTarget(target string, fields []string) string {
	if len(fields) > 0 {
		target = target + "`" + strings.Join(fields, "|")
	}
	return target
}

// NormalizeEnteredURL applies Python Browser.url_dialog's input normalization
// (Browser.py:1144-1146): when the entered text contains no backtick and
// contains a colon, the FIRST "|" (at pos>0) is rewritten to a backtick so a
// user can type "hash:path|x=1|y=2" instead of "hash:path`x=1|y=2". The
// remaining "|" separators stay (the backtick splits the URL from its
// field-list, "|" separates the fields). Inputs already containing a backtick
// (or no colon) are returned unchanged.
func NormalizeEnteredURL(url string) string {
	if !strings.Contains(url, "`") && strings.Contains(url, ":") {
		if pos := strings.Index(url, "|"); pos > 0 {
			url = url[:pos] + "`" + url[pos+1:]
		}
	}
	return url
}

// ParseURL parses a nomadnet RNS URL into its destination hash, page path, and
// request-data map. It is the Go port of Python Browser.retrieve_url's inline
// parse (Browser.py:884-945), which first strips an optional backtick
// field-suffix (merging `var_<k>=<v>` entries into request_data) and then splits
// on ':' to recover (destination_hash, path) via the same logic as
// Browser.parse_url (Browser.py:631-657).
//
// currentDest is the currently connected destination hash; it is used for
// relative URLs of the form ":<path>" (empty hash means "reuse current"). A
// relative URL with no current destination yields ErrMalformedURL.
//
// requestData is the incoming request-data map (or nil). When a non-empty
// backtick-fields suffix is present, nil/empty incoming maps are replaced by a
// fresh map and the var_* entries are merged in; a non-empty incoming map is
// preserved and extended — matching Python's `if not request_data:
// request_data = {}`. A non-empty fields suffix with no valid entries yields an
// empty (non-nil) map; an empty fields suffix (or no backtick) yields nil.
//
// destHash is the 16-byte destination hash (nil only on error). path is the
// page path (e.g. "/page/index.mu"); it may start with "/file/" — routing that
// to a download is the caller's responsibility, matching retrieve_url.
func ParseURL(url string, currentDest []byte, requestData map[string]string) (destHash []byte, path string, outRequestData map[string]string, err error) {
	// 1) Optional backtick field-suffix: split off "<rest>`<fields>".
	if parts := strings.Split(url, "`"); len(parts) == 2 {
		url = parts[0]
		linkFieldsStr := parts[1]
		if linkFieldsStr != "" {
			if len(requestData) == 0 { // nil or empty → fresh map (Python: `if not request_data`)
				requestData = map[string]string{}
			}
			for _, e := range strings.Split(linkFieldsStr, "|") {
				if !strings.Contains(e, "=") {
					continue
				}
				c := strings.Split(e, "=")
				if len(c) == 2 {
					requestData["var_"+c[0]] = c[1]
				}
			}
		}
	}

	// 2) Colon split → (destination_hash, path). Mirrors Browser.parse_url.
	components := strings.Split(url, ":")
	switch len(components) {
	case 1:
		if len(components[0]) != destHashHexLen {
			return nil, "", nil, ErrMalformedURL
		}
		h, decErr := hex.DecodeString(components[0])
		if decErr != nil {
			return nil, "", nil, ErrMalformedURL
		}
		return h, DefaultPath, requestData, nil
	case 2:
		if len(components[0]) == destHashHexLen {
			h, decErr := hex.DecodeString(components[0])
			if decErr != nil {
				return nil, "", nil, ErrMalformedURL
			}
			p := components[1]
			if p == "" {
				p = DefaultPath
			}
			return h, p, requestData, nil
		}
		if len(components[0]) == 0 {
			if currentDest != nil {
				p := components[1]
				if p == "" {
					p = DefaultPath
				}
				return currentDest, p, requestData, nil
			}
			return nil, "", nil, ErrMalformedURL
		}
		return nil, "", nil, ErrMalformedURL
	default:
		return nil, "", nil, ErrMalformedURL
	}
}

// CurrentURL reconstructs the canonical URL string for a loaded page, matching
// Python Browser.current_url (Browser.py:146-163): "<hex>:<path>" optionally
// followed by "`k=v|k=v" for the var_* request-data entries (field_* and other
// keys are NOT round-tripped into the URL). destHash is the 16-byte destination
// hash; path may be empty. Python dict iteration is insertion-ordered; Go maps
// are not, so the var_* keys are emitted in sorted key order for a deterministic
// result (the order is not semantically meaningful — the var_* keys are
// re-merged into request_data on the next parse regardless of order).
func CurrentURL(destHash []byte, path string, requestData map[string]string) string {
	var b strings.Builder
	b.WriteString(hex.EncodeToString(destHash))
	b.WriteByte(':')
	b.WriteString(path)
	if len(requestData) > 0 {
		var keys []string
		for k := range requestData {
			if strings.HasPrefix(k, "var_") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, k[4:]+"="+requestData[k])
		}
		if len(parts) > 0 {
			b.WriteByte('`')
			b.WriteString(strings.Join(parts, "|"))
		}
	}
	return b.String()
}

// fetchBytes establishes an RNS link to the nomadnetwork.node destination for
// destHash and issues a request for path, returning the raw response bytes and
// the (open) link. It is the shared link-establish + request core for both
// FetchPage (page fetch, Python Browser.__load) and DownloadFile (file fetch,
// Python Browser.download_file — the two differ only in how the response is
// handled, not in how the link/request is set up).
//
//  1. Ensure a path to the destination exists (Transport.HasPath → RequestPath →
//     poll, mirroring Python's has_path/request_path busy-wait up to timeout).
//  2. Recall the destination identity and build the outbound SINGLE
//     "nomadnetwork","node" destination.
//  3. Establish an RNS link and wait for it to become ACTIVE.
//  4. Issue link.Request(path, requestData, ...) and wait for the response.
//
// requestData (nil or a map of "var_*"/"field_*" → string) is passed verbatim as
// the request data — matching Python's `link.request(path, data=request_data)`.
// The wire encoding (msgpack map) matches Python; the receiving node's handling
// of map request data is go-reticulum's concern, not the browser's.
//
// onProgress (may be nil) reports the transfer progress as a 0..1 fraction,
// mirroring response_progressed. timeout bounds path resolution, link
// establishment, and the request itself (DefaultTimeout when <= 0).
//
// onLinkEstablished (may be nil) is invoked once the link becomes ACTIVE, before
// the request is issued — mirroring Python Browser.link_established
// (Browser.py:1454-1459), which identifies to the remote node when the directory
// entry requests it (should_identify_on_connect). The caller owns the
// identify/no-identify decision so this package need not import the directory.
//
// The returned link is OPEN on success (the caller decides whether to retain
// or tear it down); it is torn down on every failure/timeout path.
func fetchBytes(ctx context.Context, ts *rns.TransportSystem, destHash []byte, path string, requestData map[string]string, timeout time.Duration, onProgress func(float64), onLinkEstablished func(*rns.Link)) ([]byte, *rns.Link, error) {
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeout) * time.Second
	}
	if ts != nil {
		if hops := ts.HopsTo(destHash); hops > 0 && hops < 128 {
			timeout += time.Duration(hops*3) * time.Second
		}
	}
	// Cancellation guard: a superseding Connect cancels this fetch's ctx before
	// it reaches the network; bail out immediately rather than rendering a stale
	// page over the one the user is now viewing. Normalize a nil ctx to
	// context.Background() so the select arms below can call ctx.Done()
	// unconditionally (a nil context.Context interface would panic on Done()).
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	// 1. Path resolution.
	if !ts.HasPath(destHash) {
		_ = ts.RequestPath(destHash)
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) && !ts.HasPath(destHash) {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !ts.HasPath(destHash) {
			return nil, nil, ErrNoPath
		}
	}

	// 2. Recall identity + build outbound destination.
	identity := ts.Recall(destHash)
	if identity == nil {
		return nil, nil, ErrNoPath
	}
	dest, err := rns.NewDestination(ts, identity, rns.DestinationOut, rns.DestinationSingle, nodeAspect[0], nodeAspect[1:]...)
	if err != nil {
		return nil, nil, err
	}

	// 3. Establish the link (non-blocking Establish + wait for ACTIVE).
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		return nil, nil, err
	}
	established := make(chan struct{}, 1)
	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	})
	if err := link.Establish(); err != nil {
		return nil, nil, err
	}
	select {
	case <-established:
	case <-time.After(timeout):
		link.Teardown()
		return nil, nil, ErrLinkTimeout
	case <-ctx.Done():
		link.Teardown()
		return nil, nil, ctx.Err()
	}

	// 3b. Link is ACTIVE — give the caller a chance to identify to the remote
	// node (Python link_established, Browser.py:1454-1459) before the request.
	if onLinkEstablished != nil {
		onLinkEstablished(link)
	}

	// 4. Issue the request and wait for the response.
	var dataArg any
	if requestData != nil {
		dataArg = requestData
	}
	type result struct {
		data []byte
		err  error
	}
	resCh := make(chan result, 1)
	_, err = link.Request(path, dataArg, func(rr *rns.RequestReceipt) {
		if rr.Status != rns.RequestReady {
			resCh <- result{err: ErrRequestFailed}
			return
		}
		resCh <- result{data: rr.GetResponse()}
	}, func(rr *rns.RequestReceipt) {
		resCh <- result{err: ErrRequestFailed}
	}, func(rr *rns.RequestReceipt) {
		if onProgress != nil {
			onProgress(rr.GetProgress())
		}
	}, timeout)
	if err != nil {
		link.Teardown()
		return nil, nil, err
	}
	select {
	case r := <-resCh:
		if r.err != nil {
			link.Teardown()
			return nil, nil, r.err
		}
		return r.data, link, nil
	case <-time.After(timeout):
		link.Teardown()
		return nil, nil, ErrRequestTimeout
	case <-ctx.Done():
		link.Teardown()
		return nil, nil, ctx.Err()
	}
}

// FetchPage establishes an RNS link to the nomadnetwork.node destination for
// destHash and issues a request for path, returning the raw response bytes. It
// is the Go port of Python Browser.__load (Browser.py:1375-1451). It is a
// one-shot fetch: the link is torn down after the response (the Go BrowserDisplay
// does not yet retain a per-destination link the way Python retains self.link;
// link reuse is a TUI-ownership concern). See fetchBytes for the per-step
// behavior and the requestData/onProgress/onLinkEstablished/timeout semantics.
func FetchPage(ctx context.Context, ts *rns.TransportSystem, destHash []byte, path string, requestData map[string]string, timeout time.Duration, onProgress func(float64), onLinkEstablished func(*rns.Link)) ([]byte, error) {
	data, link, err := fetchBytes(ctx, ts, destHash, path, requestData, timeout, onProgress, onLinkEstablished)
	if err != nil {
		return nil, err
	}
	link.Teardown()
	return data, nil
}

// DownloadFile fetches a /file/<name> path from the nomadnetwork.node
// destination and saves the response to downloadsDir, mirroring Python
// Browser.download_file (Browser.py:999-1068) + file_received
// (Browser.py:1635-1690). The link-establish + request path is shared with
// FetchPage (Python download_file reuses self.link exactly as __load does);
// the difference is the response is saved to disk with a unique basename rather
// than rendered. onLinkEstablished lets the caller identify to the remote node
// when the directory requests it (see fetchBytes).
//
// It returns the saved file's name (relative to downloadsDir — the basename
// used, with a ".N" suffix appended on collision, matching Python's
// saved_file_name) and its size in bytes.
func DownloadFile(ctx context.Context, ts *rns.TransportSystem, destHash []byte, path string, requestData map[string]string, downloadsDir string, timeout time.Duration, onProgress func(float64), onLinkEstablished func(*rns.Link)) (savedName string, savedSize int, err error) {
	data, link, err := fetchBytes(ctx, ts, destHash, path, requestData, timeout, onProgress, onLinkEstablished)
	if err != nil {
		return "", 0, err
	}
	link.Teardown()
	return SaveDownload(downloadsDir, path, data)
}

// SaveDownload writes data to a uniquely-named file under downloadsDir,
// mirroring Python Browser.file_received's non-BufferedReader branch
// (Browser.py:1655-1672) + download_local_file's collision logic
// (Browser.py:964-984). name is the request path or supplied file name; its
// basename is taken (os.path.basename) so a path like "/file/docs/report.pdf"
// saves as "report.pdf". On collision with an existing file, ".N" is appended
// (report.pdf → report.pdf.1 → report.pdf.2), matching Python's counter loop.
// Returns the saved basename (the relative name Python stores as
// saved_file_name) and the written size in bytes.
func SaveDownload(downloadsDir, name string, data []byte) (savedName string, savedSize int, err error) {
	dest := UniqueDownloadPath(downloadsDir, name)
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		return "", 0, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", 0, err
	}
	rel, err := filepath.Rel(downloadsDir, dest)
	if err != nil {
		rel = filepath.Base(dest)
	}
	return rel, len(data), nil
}

// UniqueDownloadPath returns the full path for a downloaded file under
// downloadsDir, appending ".N" for collisions, mirroring Python
// Browser.file_received (Browser.py:1641-1646) and download_local_file
// (Browser.py:969-974):
//
//	counter = 0
//	while os.path.isfile(file_destination):
//	    counter += 1
//	    file_destination = downloads_path+"/"+file_name+"."+str(counter)
//
// name is the request path or supplied file name; its basename is taken
// (os.path.basename) so a path like "/file/docs/report.pdf" → "report.pdf".
// The first existing colliding name gets ".1", then ".2", etc.
func UniqueDownloadPath(downloadsDir, name string) string {
	base := filepath.Base(name)
	dest := filepath.Join(downloadsDir, base)
	counter := 0
	for fileExists(dest) {
		counter++
		dest = filepath.Join(downloadsDir, base+"."+strconv.Itoa(counter))
	}
	return dest
}

// fileExists reports whether path exists (file or otherwise), mirroring
// Python os.path.isfile for collision detection. Python uses isfile, which
// excludes directories; a directory at the target name would NOT collide in
// Python (isfile returns False), so it would overwrite — matching that, we
// only treat a non-directory file as a collision.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
