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
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// appSendDeps is the production conversation.SendDeps adapter backed by an
// App's RNS/LXMF/directory primitives. It mirrors Python Conversation's access
// to self.send_destination, self.app.lxmf_destination, the directory, the
// message router, and Conversation.ingest (Conversation.py:194-212,297-331).
//
// One adapter is built per sent conversation (carrying the peer's LXMF source
// hash) so SendDestination can recall the peer identity and build the OUT
// destination. The app-level dependencies (router, directory, paths) are read
// live from the App each call, so an adapter stays valid across reconfig.
type appSendDeps struct {
	a          *App
	sourceHash string // peer LXMF delivery hash, hex
	dest       *rns.Destination
}

// NewSendDeps builds the production SendDeps for a conversation identified by
// its peer LXMF source hash. It is the App-side factory passed to
// Conversation.SetSendDeps so the conversation package stays decoupled from
// the app package.
func (a *App) NewSendDeps(sourceHash string) conversation.SendDeps {
	return &appSendDeps{a: a, sourceHash: sourceHash}
}

// SendDestination returns the peer's lxmf.delivery OUT destination, built from
// the recalled peer identity (Python Conversation.py:205-212). If the peer
// identity is not yet known to the transport, the destination cannot be built
// and Send/PaperOutput must abort — mirroring Python's send_destination=None.
// The result is cached for the adapter's lifetime.
func (d *appSendDeps) SendDestination() *rns.Destination {
	if d.dest != nil {
		return d.dest
	}
	if d.a.Transport == nil {
		return nil
	}
	hash, err := hex.DecodeString(d.sourceHash)
	if err != nil || len(hash) == 0 {
		return nil
	}
	id := rns.RecallIdentity(d.a.Transport, hash)
	if id == nil {
		return nil
	}
	dest, err := rns.NewDestination(d.a.Transport, id, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		return nil
	}
	d.dest = dest
	return dest
}

// LXMFSource returns the local outbound LXMF delivery destination (Python
// self.app.lxmf_destination).
func (d *appSendDeps) LXMFSource() *rns.Destination { return d.a.LXMFDest }

// PreferredDelivery returns the directory's preferred delivery mode for the
// hash (Python directory.preferred_delivery).
func (d *appSendDeps) PreferredDelivery(hash []byte) byte {
	return d.a.Dir.PreferredDelivery(hash)
}

// TrustLevel returns the directory trust level for the hash (Python
// directory.trust_level).
func (d *appSendDeps) TrustLevel(hash []byte) byte {
	return d.a.Dir.TrustLevel(hash, nil)
}

// OutboundPropagationNode returns the router's configured outbound propagation
// node hash, or nil when none is set (Python
// message_router.get_outbound_propagation_node).
func (d *appSendDeps) OutboundPropagationNode() []byte {
	if d.a.Router == nil {
		return nil
	}
	return d.a.Router.GetOutboundPropagationNode()
}

// DeliveryLinkAvailable reports whether a direct delivery link is currently
// available to the hash (Python message_router.delivery_link_available).
func (d *appSendDeps) DeliveryLinkAvailable(hash []byte) bool {
	if d.a.Router == nil {
		return false
	}
	return d.a.Router.DeliveryLinkAvailable(hash)
}

// CurrentRatchetID returns a non-nil value when a forward-secrecy ratchet is
// known for the hash (Python RNS.Identity.current_ratchet_id), enabling
// opportunistic delivery. send.go only checks the result for nil/non-nil, so
// the transport's stored ratchet public key (non-nil when known) stands in for
// the ratchet ID.
func (d *appSendDeps) CurrentRatchetID(hash []byte) []byte {
	if d.a.Transport == nil {
		return nil
	}
	return d.a.Transport.GetRatchet(hash)
}

// TryPropagationOnFail mirrors app.try_propagation_on_fail.
func (d *appSendDeps) TryPropagationOnFail() bool { return d.a.TryPropagationOnFail }

// HandleOutbound submits the message to the router for delivery (Python
// message_router.handle_outbound).
func (d *appSendDeps) HandleOutbound(lxm *lxmf.Message) error {
	if d.a.Router == nil {
		return nil
	}
	return d.a.Router.HandleOutbound(lxm)
}

// Ingest writes the outbound message into conversation storage and returns its
// file path (Python Conversation.ingest(lxm, app, originator=True)).
func (d *appSendDeps) Ingest(lxm *lxmf.Message, originator bool) (string, error) {
	return d.a.ConversationCache.Ingest(lxm, d.a.ConversationPath, originator)
}

// DownloadsPath returns the directory where paper-message QR/URI files are
// saved (Python self.app.downloads_path).
func (d *appSendDeps) DownloadsPath() string { return d.a.DownloadsPath }

// TmpFilesPath returns the scratch directory for transient print files
// (Python self.app.tmpfilespath).
func (d *appSendDeps) TmpFilesPath() string { return d.a.TmpFilesPath }

// PrintFile prints the file at the given path using the configured print
// command (Python self.app.print_file).
func (d *appSendDeps) PrintFile(path string) bool { return d.a.PrintFile(path) }

// SendConversation composes and dispatches an LXMF message to the peer
// identified by sourceHash, mirroring the Python Conversations.py C-d / send
// path (send_message, Conversations.py:2412-2436): load (or create) the
// conversation, wire its SendDeps once, build the LXMF fields from any
// staged attachment file paths (FIELD_FILE_ATTACHMENTS = [[name, data], ...])
// and the markdown renderer flag, then call Conversation.Send. Returns false
// when no send destination is configured (the peer identity is not yet known)
// or the router rejects the message.
//
// The optional attachments are local file paths read at send time, matching
// Python's pending_attachments loop. Unreadable files are skipped (Python
// logs and continues).
func (a *App) SendConversation(sourceHash, content, title string, attachments ...string) bool {
	if a.ConversationPath == "" {
		return false
	}
	conv := a.ConversationCache.Get(sourceHash)
	if conv == nil {
		conv = conversation.NewConversation(sourceHash, filepath.Join(a.ConversationPath, sourceHash))
		a.ConversationCache.Store(conv)
	}
	conv.SetSendDeps(a.NewSendDeps(sourceHash))
	return conv.Send(content, title, a.buildSendFields(attachments))
}

// buildSendFields constructs the LXMF fields map for an outbound message from
// the staged attachment file paths and the app's compose-markdown setting,
// mirroring Python send_message (Conversations.py:2416-2433). Returns nil when
// there are no attachments and markdown is off (so the message carries no
// fields, like Python's `fields = None` branch).
func (a *App) buildSendFields(attachments []string) map[any]any {
	var fileAttachments []any
	for _, path := range attachments {
		data, err := os.ReadFile(path)
		if err != nil {
			// Python logs and continues; skip unreadable attachments.
			continue
		}
		name := filepath.Base(path)
		fileAttachments = append(fileAttachments, []any{name, data})
	}

	var fields map[any]any
	if len(fileAttachments) > 0 {
		fields = map[any]any{lxmf.FieldFileAttachments: fileAttachments}
	}
	if a.ComposeMarkdown {
		if fields == nil {
			fields = map[any]any{}
		}
		fields[lxmf.FieldRenderer] = lxmf.RendererMarkdown
	}
	return fields
}

// PaperMessage generates an offline paper (QR/URI) message addressed to the
// peer identified by sourceHash, mirroring the Python Conversations.py C-p /
// paper_output path (paper_message, Conversations.py:2474-2503). action selects
// the output method: "PrintQR", "SaveQR", or "SaveURI". It loads (or creates)
// the conversation, wires its SendDeps once, then calls Conversation.PaperOutput.
// Returns the saved file path (for SaveQR/SaveURI) and ok=true on success; ("",
// false) when no send destination is configured (peer identity not known) or
// the output fails — matching Python's paper_output returning False.
func (a *App) PaperMessage(sourceHash, action, content, title string) (string, bool) {
	if a.ConversationPath == "" {
		return "", false
	}
	conv := a.ConversationCache.Get(sourceHash)
	if conv == nil {
		conv = conversation.NewConversation(sourceHash, filepath.Join(a.ConversationPath, sourceHash))
		a.ConversationCache.Store(conv)
	}
	conv.SetSendDeps(a.NewSendDeps(sourceHash))
	mode, ok := paperOutputMode(action)
	if !ok {
		return "", false
	}
	return conv.PaperOutput(content, title, mode)
}

// paperOutputMode maps a TUI paper-message action string to the conversation
// PaperOutputMode, mirroring Python's paper_output mode argument
// (Conversations.py:2474-2503).
func paperOutputMode(action string) (conversation.PaperOutputMode, bool) {
	switch action {
	case "PrintQR":
		return conversation.PaperPrintQR, true
	case "SaveQR":
		return conversation.PaperSaveQR, true
	case "SaveURI":
		return conversation.PaperSaveURI, true
	default:
		return 0, false
	}
}