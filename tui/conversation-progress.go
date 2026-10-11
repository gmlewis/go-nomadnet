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
	"bytes"
	"strconv"
	"strings"
)

// lxmfRepresentationResource is LXMF's RESOURCE transport representation wire
// constant (LXMessage.RESOURCE = 0x02). Python's LXMessageWidget only shows a
// transfer progress row for a pending outbound message whose representation is
// RESOURCE (Conversations.py:2712-2717).
const lxmfRepresentationResource = 0x02

// lxmfRepresentationPacket is LXMF's PACKET transport representation wire
// constant (LXMessage.PACKET = 0x01): the message fits in a single Reticulum
// packet, so it has no transfer progress to show.
const lxmfRepresentationPacket = 0x01

// lxmfStateOutbound is LXMF's OUTBOUND lifecycle state (LXMessage.OUTBOUND =
// 0x01): the message is queued for delivery but not yet on the wire.
const lxmfStateOutbound = 0x01

// lxmfStateSending is LXMF's SENDING lifecycle state (LXMessage.SENDING =
// 0x02): the message is actively being transmitted.
const lxmfStateSending = 0x02

// progressBarWidth is the LXMessageWidget progress bar's cell width
// (Conversations.py:2721, "bar_width = 20").
const progressBarWidth = 20

// LiveTransfer is the router's in-flight state for one outbound LXMF message:
// the subset of Python's app.message_router.pending_outbound entry that the
// LXMessageWidget progress row reads (Conversations.py:2711-2725).
type LiveTransfer struct {
	// Progress is the transfer progress in the range 0.0-1.0 (LXMessage.progress).
	Progress float64
	// State is the raw LXMF lifecycle state (LXMessage.state).
	State int
	// Representation is the raw LXMF transport representation
	// (LXMessage.representation); only RESOURCE yields a progress row.
	Representation int
}

// OnLiveTransfer, when set, returns the router's in-flight state for the
// outbound message with the given LXMF hash, or ok=false when no such message
// is pending. It mirrors Python's scan of app.message_router.pending_outbound
// (Conversations.py:2711-2720): the UI layer never touches the router directly,
// and the wiring layer maps a pending message onto this small value. Nil ⇒ no
// progress row is ever rendered.
type OnLiveTransferFunc func(hash []byte) (LiveTransfer, bool)

// liveProgressRowText returns the progress row string for a live transfer,
// mirroring Python's LXMessageWidget progress widget text — the __init__ branch
// (Conversations.py:2721-2727) for an in-flight transfer and _poll_progress
// (Conversations.py:2766-2790) for the settled states. A transfer that has
// reached SENT yields "" (no row). colorMode selects the bar glyphs: U+2588 /
// U+2591 at >= 256 colors, ASCII "#" / "-" below that (Conversations.py:2723,
// 2786).
func liveProgressRowText(lt LiveTransfer, glyphs GlyphSet, colorMode int) string {
	g := glyphs
	if g == nil {
		g = glyphsUnicode
	}
	switch {
	case lt.State == lxmfStateFailed:
		return "  " + g["cross"] + " Transfer failed"
	case lt.State == lxmfStateRejected:
		return "  " + g["cross"] + " Rejected: too large or not accepted"
	case lt.State >= lxmfStateSent:
		return ""
	}
	filled := int(float64(progressBarWidth) * lt.Progress)
	filled = min(max(filled, 0), progressBarWidth)
	full, empty := "#", "-"
	if colorMode >= 256 {
		full, empty = "█", "░"
	}
	bar := strings.Repeat(full, filled) + strings.Repeat(empty, progressBarWidth-filled)
	return "  [" + bar + "] " + strconv.Itoa(int(lt.Progress*100)) + "%"
}

// colorMode returns the app's configured color depth, defaulting to true color
// when no app is attached (the shipped 24-bit default).
func (cw *ConversationWidget) colorMode() int {
	if cw.app != nil {
		return cw.app.ColorMode
	}
	return ColorModeTrue
}

// resolvedOwnHash returns this app's LXMF destination hash, preferring the live
// OnOwnHash hook over the cached OwnHash (Python reads app.lxmf_destination.hash
// fresh on every widget build, Conversations.py:2607).
func (cw *ConversationWidget) resolvedOwnHash() []byte {
	if cw.OnOwnHash != nil {
		if h := cw.OnOwnHash(); len(h) > 0 {
			return h
		}
	}
	return cw.OwnHash
}

// liveTransferFor returns the live transfer for msg when a progress row should
// be shown: the message must be outbound, its cached state below SENT, and the
// router must report a matching pending message whose representation is
// RESOURCE. This mirrors Python's gate exactly — "is_outbound and msg_state is
// not None and msg_state < LXMF.LXMessage.SENT and msg_hash is not None", then
// a scan of pending_outbound for the same hash, then the RESOURCE check
// (Conversations.py:2709-2719).
func (cw *ConversationWidget) liveTransferFor(msg ConversationMessage) (LiveTransfer, bool) {
	if cw == nil || cw.OnLiveTransfer == nil || len(msg.Hash) == 0 {
		return LiveTransfer{}, false
	}
	if !bytes.Equal(msg.SourceHash, cw.resolvedOwnHash()) {
		return LiveTransfer{}, false // inbound
	}
	if msg.State >= lxmfStateSent {
		return LiveTransfer{}, false
	}
	lt, ok := cw.OnLiveTransfer(msg.Hash)
	if !ok || lt.Representation != lxmfRepresentationResource {
		return LiveTransfer{}, false
	}
	return lt, true
}

// ProgressPollNeeded reports whether the open conversation has a message whose
// progress row would advance, so the UI ticker can run its fast poll only
// during an active transfer (Python re-arms its 0.3 s alarm only while
// _live_lxm is set, Conversations.py:2783-2790).
func (cw *ConversationWidget) ProgressPollNeeded() bool {
	if cw == nil {
		return false
	}
	for _, msg := range cw.messages {
		if _, ok := cw.liveTransferFor(msg); ok {
			return true
		}
	}
	return false
}

// RefreshProgress re-renders the message list in place (no disk reload) so a
// live transfer's progress row advances, mirroring Python's _poll_progress
// redraw (Conversations.py:2786-2790). Must run on the UI thread.
func (cw *ConversationWidget) RefreshProgress() {
	if cw == nil {
		return
	}
	cw.renderMessages()
}
