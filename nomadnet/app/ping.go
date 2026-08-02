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

package app

import (
	"fmt"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// FormatPongResult formats the ping-outcome string matching Python's
// _ping_peer_from_dialog (Conversations.py:734-744):
//
//	hops_str = f" ({hops} hop{'s' if hops != 1 else '')})"  when hops < PATHFINDER_M
//	f"Pong in {elapsed_ms} ms{hops_str}"
//
// The hop suffix is omitted when the hop count is unknown: hops < 0 (the
// sentinel PingPeer uses when HopsTo returns PathfinderM, matching Python's
// `hops is None or hops >= RNS.Transport.PATHFINDER_M` branch) or hops >=
// PathfinderM. The plural is "s" for every count except 1 (so "0 hops",
// "1 hop", "2 hops").
func FormatPongResult(elapsedMs int, hops int) string {
	hopsStr := ""
	if hops >= 0 && hops < rns.PathfinderM {
		plural := "s"
		if hops == 1 {
			plural = ""
		}
		hopsStr = fmt.Sprintf(" (%d hop%s)", hops, plural)
	}
	return fmt.Sprintf("Pong in %d ms%s", elapsedMs, hopsStr)
}

// PingPeer pings the peer whose LXMF delivery hash is sourceHash (hex) and
// reports the outcome via setStatus, mirroring Python's _ping_peer_from_dialog
// (Conversations.py:705-768). It recalls the peer's identity, optionally
// requests a path when none is known, opens an outbound lxmf.delivery link, and
// on establishment reports "Pong in N ms (M hops)" (then tears the link down); if
// the link closes without ever activating it reports "Ping failed (no link)".
//
// setStatus may be invoked from a background goroutine (the link's established /
// closed callbacks run on the RNS worker), so the caller MUST marshal setStatus
// onto the UI thread (e.g. via QueueUpdateDraw). The synchronous preamble
// statuses ("Invalid address", "Identity unknown; query first", "No path;
// requesting…", "Pinging…", "Ping init failed: …") are set before the goroutine
// would race, but marshaling them is harmless and matches Python's schedule_ui.
func (a *App) PingPeer(sourceHash string, setStatus func(string)) {
	if a.Transport == nil || setStatus == nil {
		return
	}

	destHash, ok := SourceHashFromHex(sourceHash)
	if !ok {
		setStatus("Invalid address")
		return
	}

	// Recall the peer's identity from its LXMF delivery hash (Python
	// RNS.Identity.recall(dest)). Without it we cannot open a link.
	identity := rns.RecallIdentity(a.Transport, destHash)
	if identity == nil {
		setStatus("Identity unknown; query first")
		return
	}

	// Request a path if none is known (Python RNS.Transport.has_path /
	// request_path). A link cannot establish without a path.
	if !a.Transport.HasPath(destHash) {
		setStatus("No path; requesting…")
		_ = a.Transport.RequestPath(destHash)
	}

	setStatus("Pinging…")
	startedAt := time.Now()

	// Build the outbound lxmf.delivery destination for the recalled identity
	// (Python RNS.Destination(identity, OUT, SINGLE, "lxmf", "delivery")).
	dest, err := rns.NewDestination(a.Transport, identity, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		setStatus(fmt.Sprintf("Ping init failed: %v", err))
		return
	}

	link, err := rns.NewLink(a.Transport, dest)
	if err != nil {
		setStatus(fmt.Sprintf("Ping init failed: %v", err))
		return
	}

	// Whether the link ever activated — the closed callback reports "no link"
	// only when the link closed without activating (Python on_closed checks
	// `link.status == RNS.Link.ACTIVE` and returns early).
	var activated bool

	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		activated = true
		elapsedMs := int(time.Since(startedAt).Milliseconds())
		// Python reads hops_to(dest) at establishment time; HopsTo returns
		// PathfinderM when unknown, which FormatPongResult renders as no
		// suffix (the `hops is None or hops >= PATHFINDER_M` branch).
		hops := a.Transport.HopsTo(destHash)
		setStatus(FormatPongResult(elapsedMs, hops))
		// Tear the link down once the ping is reported (Python link.teardown()).
		go func() {
			defer func() { _ = recover() }()
			l.Teardown()
		}()
	})

	link.SetLinkClosedCallback(func(l *rns.Link) {
		// If the link activated, the established callback already reported the
		// pong; the subsequent teardown-close is a no-op (Python on_closed
		// returns when status == ACTIVE).
		if activated || l.GetStatus() == rns.LinkActive {
			return
		}
		setStatus("Ping failed (no link)")
	})

	// Initiate the handshake (Go's NewLink does not auto-establish; Python's
	// RNS.Link(...) constructor does).
	if err := link.Establish(); err != nil {
		setStatus(fmt.Sprintf("Ping init failed: %v", err))
		return
	}
}
