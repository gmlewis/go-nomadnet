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
	"errors"
	"fmt"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// This file holds the Go-only guards that keep the client from adopting a
// default LXMF propagation node that nothing actually serves. Python has the
// same hole and no counterpart to any of this: it accepts whatever address a
// node's Info page derives, reports nothing, and then fails every propagated
// send and sync for as long as the selection stands.
//
// A propagation node's address is its own `lxmf.propagation` destination hash
// and nothing else, so it is only real when that destination has announced
// itself. The Info page's checkbox does not derive that address: it derives the
// `lxmf.propagation` address of whichever identity announced the NODE being
// viewed (Python Network.py:629, hash_from_name_and_identity on
// Identity.recall(node_hash)). When a host's node and its propagation node run
// under different identities, the derived address belongs to the node's
// announcing identity and no process serves it. The client then shows an opaque
// <hex…> in the Conversations footer and silently fails every propagated
// delivery.
//
// See the package documentation in app.go for the overall package layout.

// ErrPropagationNodeUnproven reports that the `lxmf.propagation` address derived
// from a node's announcing identity is not backed by any announced identity, so
// nothing on the network serves it. Callers surface it to the user instead of
// adopting the address, and point at the Propagation Nodes tab, whose rows are
// `lxmf.propagation` announces and therefore carry a served address in the row's
// own hash.
var ErrPropagationNodeUnproven = errors.New("no propagation node has announced this address")

// PropagationNodeKnown reports whether an identity has been announced for the
// given `lxmf.propagation` destination hash — that is, whether anything on the
// network has claimed this propagation address. The transport keys its known-
// destination table by destination hash and fills it from every received
// announce (rns.Identity.remember), so a recalled identity here means a
// propagation node announced this exact address.
//
// Known is NOT the same as serving: NomadNet declares its own propagation
// address on every node announce (Node.py:216-220) and announces the withdrawal
// when propagation is switched off (LXMRouter.disable_propagation,
// LXMRouter.py:692-694), so an address can be announced with the payload's
// node_state element false. Use PropagationNodeServing to ask whether the
// address is backed by a node that actually serves propagation.
func (a *App) PropagationNodeKnown(pnHash []byte) bool {
	if a.Transport == nil || len(pnHash) == 0 {
		return false
	}
	return rns.RecallIdentity(a.Transport, pnHash) != nil
}

// PropagationNodeServing reports whether the identity that announced the given
// `lxmf.propagation` address declared itself as an *active* propagation node —
// the node_state element of the announce payload
// (lxmf.PropagationNodeStateFromAppData; Python LXMRouter.py:324-336 and its
// pn_announce_data_is_valid acceptance rules).
//
// This is the check that matters for adopting a propagation node. A NomadNet
// node declares its own propagation address on every node announce even when it
// serves nothing, so "announced" alone would adopt an address that answers no
// requests. An address that was never announced, or whose latest announce
// carries no readable state, is not serving.
func (a *App) PropagationNodeServing(pnHash []byte) bool {
	if a.Transport == nil || len(pnHash) == 0 {
		return false
	}
	serving, ok := lxmf.PropagationNodeStateFromAppData(a.Transport.RecallAppData(pnHash))
	return ok && serving
}

// NodePropagationNotServing reports whether the node identified by nodeHash has
// declared that it does not serve a propagation node — that is, the
// `lxmf.propagation` address derived from the node's identity has been announced
// by that same identity with node_state false.
//
// The declaration is the node's own, so it is authoritative: the node is telling
// the network "this is my propagation address, and I am not serving it" (Python's
// Node.announce declares it on every node announce, Node.py:216-220, and
// disable_propagation announces the withdrawal, LXMRouter.py:692-694). A caller
// that offers the node's propagation address for adoption — the KnownNodeInfo
// "LXMF Propagation Node Address is <hex>" line and its "Use as default
// propagation node" checkbox — must not offer it at all in that case: the address
// cannot be used as a propagation node, and presenting it (with a control that
// adopts it) is exactly the trap this file's guard exists to refuse.
//
// An address that was never declared at all is NOT covered: absence of a
// declaration means "unknown", not "not serving", so the caller keeps the
// SoT behavior (show the derived address; the guard refuses an unserved
// adoption).
func (a *App) NodePropagationNotServing(nodeHash []byte) bool {
	pnHash := a.NodePropagationHash(nodeHash)
	return len(pnHash) > 0 && a.PropagationNodeKnown(pnHash) && !a.PropagationNodeServing(pnHash)
}

// propagationNodeMarker classifies a default propagation node from the two
// facts the transport can supply, as a short ASCII annotation for the
// Conversations footer: the empty string when the node looks usable, otherwise
// the reason it does not. The three failure states are kept apart deliberately —
// "no identity" means no process has announced that address at all (a dead
// selection); "stopped" means the address is declared but its node_state says it
// is not serving (a NomadNet node's own propagation address, which it declares
// even with propagation switched off); "no path" means the address is real and
// serving but not currently routable (a transient condition that resolves itself,
// e.g. once the node re-announces).
//
// The wording is short on purpose. The footer is one line in a 52-column pane
// and the marker is appended to the node label, so a longer phrase would push the
// *reason* past the right edge — leaving the reader with "unreachable: " and no
// idea why, which is worse than saying nothing.
func propagationNodeMarker(identityKnown, serving, hasPath bool) string {
	switch {
	case !identityKnown:
		return "no identity"
	case !serving:
		return "stopped"
	case !hasPath:
		return "no path"
	}
	return ""
}

// propagationNodeMarkerFor returns the reachability marker for a specific
// propagation address, or "" when there is no address to judge or no transport
// to judge it with.
func (a *App) propagationNodeMarkerFor(pnHash []byte) string {
	if a.Transport == nil || len(pnHash) == 0 {
		return ""
	}
	return propagationNodeMarker(
		a.PropagationNodeKnown(pnHash),
		a.PropagationNodeServing(pnHash),
		a.Transport.HasPath(pnHash),
	)
}

// PropagationNodeMarker returns the reachability marker for the default
// propagation node currently in use, or "" when no node is selected or it
// looks usable. See propagationNodeMarker for the marker's vocabulary.
func (a *App) PropagationNodeMarker() string {
	return a.propagationNodeMarkerFor(a.GetDefaultPropagationNode())
}

// SetPropagationNodeFromNodeIdentity adopts the default propagation node the
// KnownNodeInfo "Use as default propagation node" checkbox implies: the
// `lxmf.propagation` address derived from the identity that announced nodeHash
// (Python save_node, Network.py:730-734). It refuses with
// ErrPropagationNodeUnproven unless a propagation node is actually *serving* at
// the derived address — announced with node_state true
// (PropagationNodeServing) — which is the case whenever the host's node and its
// propagation node run under different identities, and also for a NomadNet node
// that declares its own address with propagation switched off. This guard is
// Go-only; see the file comment above.
//
// The caller is expected to show the returned error to the user rather than
// dropping it, because the whole point of the guard is that a dead address must
// never be accepted silently.
func (a *App) SetPropagationNodeFromNodeIdentity(nodeHash []byte) error {
	pnHash := a.NodePropagationHash(nodeHash)
	if !a.PropagationNodeServing(pnHash) {
		return ErrPropagationNodeUnproven
	}
	a.SetUserSelectedPropagationNode(pnHash)
	return nil
}

// PropagationNodeUnprovenNotice is the notice the TUI shows when a selection is
// refused. It names the address that was refused and points at the Propagation
// Nodes tab, whose rows are `lxmf.propagation` announces and therefore carry a
// served address in the row's own hash — without that, a refusal leaves the
// operator with no working way to pick a node.
//
// pnHash may be nil, when the node's own identity could not be recalled and no
// address could be derived at all.
func PropagationNodeUnprovenNotice(pnHash []byte) string {
	addr := "this address"
	if len(pnHash) >= 4 {
		addr = "<" + fmt.Sprintf("%x", pnHash)[:8] + "…>"
	}
	return "Could not set the\ndefault propagation node\n\n" + addr +
		"\nis not served by any\npropagation node\n\nUse the Propagation Nodes\ntab (Ctrl-L) instead."
}
