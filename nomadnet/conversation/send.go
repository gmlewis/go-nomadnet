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
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// SendDeps supplies the app-level dependencies that Conversation.Send,
// Conversation.MessageNotification, and Conversation.PaperOutput require,
// mirroring Python Conversation's access to self.send_destination,
// self.app.lxmf_destination, the directory, the message router, the app's
// downloads/tmp paths, the print command, and Conversation.ingest. Injecting
// these as an interface keeps the conversation package decoupled from the app
// package (avoiding an import cycle) and lets tests substitute fakes.
type SendDeps interface {
	// SendDestination returns the peer LXMF destination to deliver to (Python
	// self.send_destination). A nil return means the destination is unknown and
	// Send/PaperOutput abort with false.
	SendDestination() *rns.Destination
	// LXMFSource returns the local outbound LXMF destination (Python
	// self.app.lxmf_destination), used as the message source.
	LXMFSource() *rns.Destination
	// PreferredDelivery returns the directory's preferred delivery mode for the
	// destination hash (Python directory.preferred_delivery).
	PreferredDelivery(hash []byte) byte
	// TrustLevel returns the directory trust level for the hash (Python
	// directory.trust_level). Compared against directory.TrustTrusted to decide
	// whether to include a reply ticket.
	TrustLevel(hash []byte) byte
	// OutboundPropagationNode returns the configured propagation node hash, or
	// nil when none is set (Python message_router.get_outbound_propagation_node).
	OutboundPropagationNode() []byte
	// DeliveryLinkAvailable reports whether a direct delivery link is currently
	// available to the hash (Python message_router.delivery_link_available).
	DeliveryLinkAvailable(hash []byte) bool
	// CurrentRatchetID returns a non-nil value when an ratchet identity is known
	// for the hash (Python RNS.Identity.current_ratchet_id), enabling
	// opportunistic delivery.
	CurrentRatchetID(hash []byte) []byte
	// TryPropagationOnFail mirrors app.try_propagation_on_fail.
	TryPropagationOnFail() bool
	// HandleOutbound submits the message to the router for delivery (Python
	// message_router.handle_outbound).
	HandleOutbound(lxm *lxmf.Message) error
	// Ingest writes the outbound message into conversation storage and returns
	// its file path (Python Conversation.ingest(lxm, app, originator=True)).
	Ingest(lxm *lxmf.Message, originator bool) (string, error)
	// DownloadsPath returns the directory where paper-message QR/URI files are
	// saved (Python self.app.downloads_path).
	DownloadsPath() string
	// TmpFilesPath returns the scratch directory for transient print files
	// (Python self.app.tmpfilespath).
	TmpFilesPath() string
	// PrintFile prints the file at the given path using the configured print
	// command (Python self.app.print_file). Returns true on success.
	PrintFile(path string) bool
}

// SetSendDeps wires the app-level dependencies used by Send and
// MessageNotification. It must be called before Send.
func (c *Conversation) SetSendDeps(deps SendDeps) {
	c.sendDeps = deps
}

// Send composes and dispatches an LXMF message to the conversation peer,
// mirroring Python Conversation.send. It chooses the delivery method
// (DIRECT/PROPAGATED/OPPORTUNISTIC) from the directory preference and link
// availability, registers the delivery/failed callback, requests a reply
// ticket when the peer is trusted, submits the message to the router, then
// ingests the outbound message. It returns false when no send destination is
// configured or the router rejects the message.
func (c *Conversation) Send(content, title string, fields map[any]any) bool {
	deps := c.sendDeps
	if deps == nil {
		return false
	}
	dest := deps.SendDestination()
	if dest == nil {
		return false
	}
	source := deps.LXMFSource()
	if source == nil {
		return false
	}

	desired := lxmf.MethodDirect
	if deps.PreferredDelivery(dest.Hash) == directory.DeliveryPropagated {
		if deps.OutboundPropagationNode() != nil {
			desired = lxmf.MethodPropagated
		}
	} else if !deps.DeliveryLinkAvailable(dest.Hash) && deps.CurrentRatchetID(dest.Hash) != nil {
		desired = lxmf.MethodOpportunistic
	}

	trusted := deps.TrustLevel(dest.Hash) == directory.TrustTrusted

	lxm, err := lxmf.NewMessage(dest, source, content, title, fields)
	if err != nil {
		return false
	}
	lxm.DesiredMethod = desired
	lxm.IncludeTicket = trusted
	lxm.DeliveryCallback = c.MessageNotification
	lxm.FailedCallback = c.MessageNotification
	if deps.OutboundPropagationNode() != nil {
		lxm.TryPropagationOnFail = deps.TryPropagationOnFail()
	}

	if err := deps.HandleOutbound(lxm); err != nil {
		return false
	}

	path, err := deps.Ingest(lxm, true)
	if err != nil {
		return false
	}
	c.Messages = append(c.Messages, NewMessage(path))
	return true
}

// MessageNotification is the delivery/failure callback registered on outbound
// messages, mirroring Python Conversation.message_notification. On a permanent
// failure with try-propagation-on-fail still set, it retries the message as
// propagated; otherwise it ingests the (now state-updated) message into the
// conversation so the local view reflects the final state.
//
// When driven by the go-reticulum router, the router itself performs the
// try-propagation-on-fail retry (resetting the flag before invoking this
// callback), so this callback ingests in that case — matching the observable
// state transition of the Python flow.
func (c *Conversation) MessageNotification(message *lxmf.Message) {
	deps := c.sendDeps
	if message.State == lxmf.StateFailed && message.TryPropagationOnFail {
		message.TryPropagationOnFail = false
		message.DesiredMethod = lxmf.MethodPropagated
		if deps != nil {
			_ = deps.HandleOutbound(message)
		}
		return
	}
	if deps != nil {
		_, _ = deps.Ingest(message, true)
	}
}
