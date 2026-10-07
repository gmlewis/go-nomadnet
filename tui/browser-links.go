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
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// truncatedHashHexLen is the number of hex characters for a truncated
// RNS destination hash: TRUNCATED_HASHLENGTH/8*2 = 128/8*2 = 32.
const truncatedHashHexLen = 32

// ValidateLXMFLink validates that linkTarget is a valid LXMF link
// destination hash (32 hex characters). Matches Python's
// handle_lxmf_link() validation at Browser.py:383.
func ValidateLXMFLink(linkTarget string) error {
	if len(linkTarget) != truncatedHashHexLen {
		return fmt.Errorf("invalid length for LXMF link: got %v, want %v", len(linkTarget), truncatedHashHexLen)
	}
	if _, err := hex.DecodeString(linkTarget); err != nil {
		return errors.New("could not decode destination hash from LXMF link")
	}
	return nil
}

// ParseRRCLink parses an RRC link target into hub hash hex, room,
// and optional destination name. Input format:
// "hexhash/room" or "hexhash:dest/room".
// Matches Python's handle_rrc_link() at Browser.py:426.
func ParseRRCLink(linkTarget string) (hubHex, room, dest string, err error) {
	rest := strings.TrimSpace(linkTarget)
	rest = strings.TrimPrefix(rest, "/")

	hubAndRoom := strings.SplitN(rest, "/", 2)
	hubPart := hubAndRoom[0]
	roomVal := ""
	if len(hubAndRoom) > 1 {
		roomVal = hubAndRoom[1]
	}

	hubAndDest := strings.SplitN(hubPart, ":", 2)
	hexPart := strings.TrimSpace(hubAndDest[0])
	destVal := ""
	if len(hubAndDest) > 1 {
		destVal = strings.TrimSpace(hubAndDest[1])
	}

	hubBytes, decodeErr := hex.DecodeString(hexPart)
	if decodeErr != nil {
		return "", "", "", errors.New("invalid hub hash")
	}
	if len(hubBytes) != truncatedHashHexLen/2 {
		return "", "", "", fmt.Errorf("hub hash must be %v bytes", truncatedHashHexLen/2)
	}

	// Normalize the room exactly as Python's handle_rrc_link does:
	// room.strip().lstrip("#").strip(), lowercased, with an empty room
	// becoming "" (Python uses None; Go callers treat "" as no room).
	roomVal = strings.TrimSpace(roomVal)
	roomVal = strings.TrimLeft(roomVal, "#")
	roomVal = strings.TrimSpace(roomVal)
	roomVal = strings.ToLower(roomVal)

	return hexPart, roomVal, destVal, nil
}

// HandleLXMFLink validates and dispatches an LXMF link.
// On success calls OnOpenLXMF; on failure calls OnBrowserError.
// Matches Python's handle_lxmf_link() at Browser.py:383.
func (bd *BrowserDisplay) HandleLXMFLink(linkTarget string) {
	if err := ValidateLXMFLink(linkTarget); err != nil {
		if bd.OnBrowserError != nil {
			bd.OnBrowserError(fmt.Sprintf("Could not open LXMF link: %v", err))
		}
		return
	}
	if bd.OnOpenLXMF != nil {
		bd.OnOpenLXMF(linkTarget)
	}
}

// HandleRRCLink parses and dispatches an RRC link.
// On success calls OnOpenRRC; on failure calls OnBrowserError.
// Matches Python's handle_rrc_link() at Browser.py:426.
func (bd *BrowserDisplay) HandleRRCLink(linkTarget string) {
	hubHex, room, _, err := ParseRRCLink(linkTarget)
	if err != nil {
		if bd.OnBrowserError != nil {
			bd.OnBrowserError(fmt.Sprintf("Could not open RRC link: %v", err))
		}
		return
	}
	if bd.OnOpenRRC != nil {
		bd.OnOpenRRC(hubHex, room)
	}
}

// HandleLink dispatches a browser link target to the appropriate
// handler based on its format. Matches Python's handle_link() at
// Browser.py:216-268.
//
// linkFields is the link's pipe-separated field-name component (the 3rd
// backtick segment of a micron submit link, e.g. `[Search](page`query)`).
// For a nomadnetwork.node link, a non-empty linkFields collects the live
// values of the named form fields from the rendered page (Python
// recurse_down) into request_data before fetching the target — the fetch
// backend (fetchBytes → link.Request(path, requestData)) already forwards
// request_data over the wire, so this is the sole collection point. "*"
// collects every field on the page. Other link types (rrc, lxmf, partial)
// ignore linkFields.
//
// Link formats:
//   - "#name"       → anchor jump (OnJumpAnchor)
//   - "rrc://..."   → RRC link (HandleRRCLink)
//   - "type@target" → typed dispatch (node, lxmf, rrc, partial)
//   - "p:ids"       → partial update (OnPartialUpdate)
//   - plain hash    → nomadnetwork.node (OnRetrieveURL)
func (bd *BrowserDisplay) HandleLink(linkTarget, linkFields string) {
	if strings.HasPrefix(linkTarget, "#") {
		if bd.OnJumpAnchor != nil {
			bd.OnJumpAnchor(linkTarget[1:])
		}
		return
	}

	if strings.HasPrefix(linkTarget, "rrc://") {
		bd.HandleRRCLink(linkTarget[6:])
		return
	}

	// Location links (the rendered `L construct's clickable text) are handled
	// locally: activating one opens the client's location actions — the code,
	// the coordinate, and the distance and bearing when a position is set —
	// rather than loading a page. The target carries the Plus Code.
	if code, ok := micron.ParseLocationLink(linkTarget); ok {
		bd.app.ShowLocationActions(code)
		return
	}

	// Split on @ for a type prefix. Python's handle_link uses an unbounded
	// split("@") and only treats the link as typed when exactly one "@" is
	// present (len(components) == 2); a target with two or more "@" falls
	// through to the bare-address (nomadnetwork.node) branch.
	components := strings.Split(linkTarget, "@")
	var destType, target string
	var partialIDs []string

	if len(components) == 2 {
		destType = ExpandShorthands(components[0])
		target = components[1]
	} else if strings.HasPrefix(linkTarget, "p:") {
		comps := strings.Split(linkTarget, ":")
		if len(comps) > 1 {
			partialIDs = comps[1:]
		}
		destType = "partial"
		target = linkTarget
	} else {
		destType = "nomadnetwork.node"
		target = components[0]
	}

	switch destType {
	case "nomadnetwork.node":
		// A link may ask the target page to open at one of its anchors: the
		// documentation generator emits "page.md#section" links as an
		// "anchor=<section>" field, because a Micron link has nowhere to carry a
		// fragment. Peel it out here so it drives the post-load jump instead of
		// being collected as a form field (see splitLinkAnchor).
		anchor, rest := splitLinkAnchor(linkFields)
		// Go-only enhancement (see BrowserDisplay.OnBlockedConnectCheck): a
		// link click targeting a blocked destination defers the fetch behind
		// the "Blocked node" warning modal; the unguarded link flow below runs
		// only via the modal's explicit Connect.
		if bd.interceptBlockedConnect(target, func() { bd.loadLinkDirect(target, rest, anchor) }) {
			return
		}
		bd.loadLinkDirect(target, rest, anchor)
	case "lxmf.delivery":
		bd.HandleLXMFLink(target)
	case "rrc.hub.session":
		bd.HandleRRCLink(target)
	case "partial":
		if len(partialIDs) > 0 && bd.OnPartialUpdate != nil {
			bd.OnPartialUpdate(partialIDs)
		}
	default:
		if bd.OnBrowserError != nil {
			bd.OnBrowserError(fmt.Sprintf("No known handler for destination type %v", destType))
		}
	}
}

// splitLinkAnchor peels an "anchor=<name>" entry out of a pipe-separated link
// field list, returning that anchor name and the remaining field list.
//
// Micron gives a link no place to carry a URL fragment, so a Markdown
// "page.md#section" link has to become a request field, which is what
// tools/bin/sync-go-reticulum-docs emits: `:/page/page.mu plus
// "anchor=<section>". Python's handle_link folds every k=v field into
// request_data["var_k"], so it sends var_anchor to the page and the reader lands
// at the top of the target instead of the section — the generated
// documentation's cross-page section links do not work there. The Go port keeps
// the k=v convention for every other key and reads this one as the navigation
// request it was meant to be: the entry is removed from the collected request
// data and handed to the caller, which records it for the post-load jump.
//
// Only an exact "anchor=" prefix matches, so an unrelated key such as
// "myanchor=x" is left alone. The last non-empty anchor wins, matching the way a
// repeated k=v entry overwrites the earlier one in request_data.
func splitLinkAnchor(linkFields string) (anchor, rest string) {
	if linkFields == "" || !strings.Contains(linkFields, "anchor=") {
		return "", linkFields
	}
	var kept []string
	for e := range strings.SplitSeq(linkFields, "|") {
		name, ok := strings.CutPrefix(e, "anchor=")
		if !ok {
			kept = append(kept, e)
			continue
		}
		if name != "" {
			anchor = name
		}
	}
	return anchor, strings.Join(kept, "|")
}

// loadLinkDirect performs the unguarded nomadnetwork.node link-click flow:
// eager history push (marking pendingLinkHist so a failure rolls it back)
// followed by the OnRetrieveURL fetch.
//
// anchor is the section this link asked the target page to open at ("" for a
// link that named none), recorded after the history push has dropped whatever a
// superseded click may have left behind.
func (bd *BrowserDisplay) loadLinkDirect(target, linkFields, anchor string) {
	// Push the target onto history eagerly (mirroring LoadURL) so Ctrl-d
	// (GoBack) returns to the page the link was on. Python's retrieve_url
	// appends to history only on success (Browser.py:131-145, 216-268); the
	// Go fetch resolves via an async app-layer callback that calls RenderPage
	// with markup (not the URL), so the tui layer cannot push on success.
	// Instead HandleLink pushes now and the failure paths roll it back:
	// NotifyLinkError (a malformed/dispatch-error link) and SetContent (a
	// fetch-fatal timeout/no-path) pop the just-pushed entry, and RenderPage
	// clears the pending flag on success (keeping it). pendingLinkHist marks
	// the push so only a link click (not a typed-URL LoadURL, which displayURL
	// clears) is rolled back — see the pendingLinkHist field doc.
	bd.rollbackPendingLink()
	bd.pushHistory(target)
	// Record the requested anchor only now: pushHistory rolls a superseded
	// click back itself (that is what clears a stale pendingAnchor), so setting
	// it before this call would have the same click wipe it again.
	bd.pendingAnchor = anchor
	bd.pendingLinkHist = true
	// Move the URL bar to the clicked target, exactly as the typed-URL / Back /
	// Forward path does in displayURL. Without this the header kept the
	// PREVIOUS page's URL: a click on the wasm demo (or any link) rendered the
	// new page under a URL bar still naming the page the link was on, because
	// refreshURLHeader only re-truncates bd.currentURLDisp rather than deriving
	// it from the page. Python updates the control widget on every
	// retrieve_url, clicked or typed (Browser.py:216-268 → 505-524).
	//
	// The centered "Retrieving" body that displayURL also shows is deliberately
	// NOT swapped in here: HandleLink already leaves the old page's content up
	// until the fetch lands, and swapping it changes the focused widget under a
	// mouse click (it blurred the content pane and hid the cursor, breaking the
	// click-focus contract that the mouse tests pin).
	bd.setURLHeader(target)
	if bd.OnRetrieveURL != nil {
		// A submit link names the form fields to send (linkFields). Collect
		// their live values from the rendered page (recurse_down analog) and
		// forward as request_data; a plain link has no fields → nil request
		// data (the cache + var_*-suffix path is unchanged).
		bd.OnRetrieveURL(target, bd.collectFields(linkFields))
	}
}
