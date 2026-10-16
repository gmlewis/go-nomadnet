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

//go:build integration

package app

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// propagationHub is the three-cornered topology of the fleet: two clients and a
// propagation node, each with a transport of its own, all joined to one hub that
// carries every announce, path request and link between them. That is the shape
// the two Android appliances talk over — each phone dials the hub and the
// propagation node is a client of the same hub — so a failure here is a failure
// there, with stacks and -race available to say where it went.
type propagationHub struct {
	appA *App
	appB *App
	pn   *lxmf.Router
	// pnHash is the propagation node's lxmf.propagation destination — the address
	// a client must be configured with to send or sync through it.
	pnHash []byte
	// hashA and hashB are the two clients' lxmf.delivery destination hashes. Each
	// is the key of the conversation the OTHER client holds with it: appA's
	// conversation is keyed on hashB, and appB's on hashA.
	hashA []byte
	hashB []byte
	hexA  string
	hexB  string
	// deliveredA and deliveredB count the LXMF messages each client's own
	// delivery path received. They separate "the router never handed the message
	// over" from "the app handed it over but the conversation never showed it".
	deliveredA atomic.Int64
	deliveredB atomic.Int64
	cleanup    func()
}

// newPropagationHub builds the hub, starts two clients on it, and puts a
// propagation node on it too.
func newPropagationHub(t *testing.T) *propagationHub {
	t.Helper()

	dirHub := testutils.TempDir(t, "nomadnet-prophub-hub")
	dirA := testutils.TempDir(t, "nomadnet-prophub-a")
	dirB := testutils.TempDir(t, "nomadnet-prophub-b")
	dirPN := testutils.TempDir(t, "nomadnet-prophub-pn")
	// Each client owns a private config with enable_node = no, so the suite never
	// depends on the machine's real NomadNet configuration.
	writeTestNomadNetConfig(t, dirA)
	writeTestNomadNetConfig(t, dirB)

	tsHub, hubCleanup := newStartedTSApp(t, dirHub)
	// The hub is a transport node: it is the only thing joining the three
	// corners, so a bare (non-transport) instance would validate and drop every
	// announce instead of rebroadcasting it, and no client would ever learn a
	// path or an identity. glenn-kamrui runs exactly this way
	// (enable_transport = yes).
	tsHub.SetEnabled(true)
	tsA, aCleanup := newStartedTSApp(t, dirA)
	tsB, bCleanup := newStartedTSApp(t, dirB)
	tsPN, pnCleanup := newStartedTSApp(t, dirPN)

	pipes := make([]func(), 0, 3)
	join := func(left, right *rns.TransportSystem) {
		pipeLeft, pipeRight, pipeCleanup := newAppPipes(t, left, right)
		left.RegisterInterface(pipeLeft)
		right.RegisterInterface(pipeRight)
		pipes = append(pipes, pipeCleanup)
	}
	join(tsHub, tsA)
	join(tsHub, tsB)
	join(tsHub, tsPN)

	appA := NewAppWithTransport(dirA, WithTransport(tsA), WithIdentity(tsA.Identity()))
	if err := appA.InitWithTransport(tsA, tsA.Identity()); err != nil {
		t.Fatalf("InitWithTransport A: %v", err)
	}
	appB := NewAppWithTransport(dirB, WithTransport(tsB), WithIdentity(tsB.Identity()))
	if err := appB.InitWithTransport(tsB, tsB.Identity()); err != nil {
		t.Fatalf("InitWithTransport B: %v", err)
	}
	appA.DownloadsPath = testutils.TempDir(t, "nomadnet-prophub-dl-a")
	appB.DownloadsPath = testutils.TempDir(t, "nomadnet-prophub-dl-b")

	hub := &propagationHub{}
	appA.DeliveryCallback = func(any) { hub.deliveredA.Add(1) }
	appB.DeliveryCallback = func(any) { hub.deliveredB.Add(1) }

	pnIdentity, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	pnRouter, err := lxmf.NewRouter(tsPN, pnIdentity, dirPN)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	pnDest, err := pnRouter.RegisterPropagationDestination()
	if err != nil {
		t.Fatalf("RegisterPropagationDestination: %v", err)
	}
	pnRouter.EnablePropagation()

	hub.appA = appA
	hub.appB = appB
	hub.pn = pnRouter
	hub.pnHash = pnDest.Hash
	hub.hashA = appA.LXMFDest.Hash
	hub.hashB = appB.LXMFDest.Hash
	hub.hexA = fmt.Sprintf("%x", appA.LXMFDest.Hash)
	hub.hexB = fmt.Sprintf("%x", appB.LXMFDest.Hash)
	hub.cleanup = func() {
		appA.Shutdown()
		appB.Shutdown()
		_ = pnRouter.Close()
		for _, pipeCleanup := range pipes {
			pipeCleanup()
		}
		aCleanup()
		bCleanup()
		pnCleanup()
		hubCleanup()
	}

	// Both peers are trusted BEFORE anything announces, as the two phones'
	// operators trust each other, and the propagation node is trusted too: the
	// client syncs "from the nearest trusted node", and it only records the
	// announces of peers it trusts.
	trusted := []struct {
		a    *App
		hash []byte
	}{
		{appA, appB.LXMFDest.Hash},
		{appB, appA.LXMFDest.Hash},
		{appA, pnDest.Hash},
		{appB, pnDest.Hash},
	}
	for _, pair := range trusted {
		entry := directory.NewEntry(pair.hash)
		entry.TrustLevel = directory.TrustTrusted
		pair.a.Dir.Remember(entry)
	}

	// Announce a few rounds so both clients learn the paths, then let the mesh
	// settle. The harness does not gate on a probe of its own: what is being
	// measured is whether the message reaches the recipient's conversation, and a
	// probe that is wrong would only hide that behind a failure of the probe.
	announceAll := func() {
		_ = appA.Router.Announce(appA.LXMFDest.Hash)
		_ = appB.Router.Announce(appB.LXMFDest.Hash)
		pnRouter.AnnouncePropagationNode()
	}
	for range 6 {
		announceAll()
		time.Sleep(500 * time.Millisecond)
	}

	return hub
}

// setPreferredDelivery records how appA would like to reach appB: direct over the
// mesh, or carried by a propagation node. It writes the same directory field the
// client's node form writes.
func (h *propagationHub) setPreferredDelivery(t *testing.T, delivery byte) {
	t.Helper()
	hash, ok := SourceHashFromHex(h.hexB)
	if !ok {
		t.Fatalf("SourceHashFromHex(%q) failed", h.hexB)
	}
	entry := directory.NewEntry(hash)
	entry.PreferredDelivery = delivery
	h.appA.Dir.Remember(entry)
}

// waitForConversationMessage waits until the named conversation holds a message
// whose body contains body, and fails the test with what it did find when the
// deadline passes. It reads the conversation the way the widget does — through
// App.ConversationMessages — so a message that survived the wire but never
// reached the conversation is a failure, not a pass.
//
// diag, when non-nil, is appended to the failure message so a missing message
// says which stage dropped it rather than only that it is missing.
func waitForConversationMessage(t *testing.T, a *App, hexConv, body string, timeout time.Duration, diag func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var found []string
	for time.Now().Before(deadline) {
		found = found[:0]
		for _, msg := range a.ConversationMessages(hexConv) {
			if msg.Content == body {
				return
			}
			found = append(found, msg.Content)
		}
		time.Sleep(100 * time.Millisecond)
	}
	extra := ""
	if diag != nil {
		extra = "; " + diag()
	}
	t.Fatalf("conversation %v never received %q; it holds %q%v", hexConv, body, found, extra)
}

// assertPropagatedDelivery runs one leg of a conversation through the
// propagation node and asserts both halves of it, in the order the two failures
// can be told apart.
//
// First the sender's own copy must reach the Sent state, which is what handing
// the message to the propagation node means — the lightning glyph the tablet
// showed on its "cldtest2" and "cldtest3". Then the recipient's sync must put
// the message in its conversation with the sender, which is what never happened
// on the Pixel: its sync reported "Downloaded 1 new messages" and its
// conversation stayed empty.
func (h *propagationHub) assertPropagatedDelivery(t *testing.T, sender, recipient *App, peerHex, recipientConvHex, body string, recipientDelivered *atomic.Int64) {
	t.Helper()

	// The production send path: the one Ctrl-D takes.
	if !sender.SendConversation(peerHex, body, "") {
		t.Fatalf("SendConversation(%q) returned false", body)
	}

	// The sender's copy must reach Sent: the propagation node accepted it.
	deadline := time.Now().Add(20 * time.Second)
	sent := false
	for time.Now().Before(deadline) && !sent {
		for _, msg := range sender.ConversationMessages(peerHex) {
			if msg.Content == body && msg.State >= lxmf.StateSent {
				sent = true
			}
		}
		if !sent {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !sent {
		t.Fatalf("the propagation node never accepted %q: the sender's copy never reached Sent", body)
	}

	// The recipient syncs from the propagation node, as its Ctrl-R → Sync Now does.
	recipient.RequestLXMFSync(0)

	waitForConversationMessage(t, recipient, recipientConvHex, body, 30*time.Second, func() string {
		return fmt.Sprintf("sync=%q deliveryCallback=%v", recipient.GetSyncStatus(), recipientDelivered.Load())
	})
}

// TestIntegrationPropagatedConversationReachesRecipient is the fleet's failing
// case, reduced to one process: two clients that cannot reach each other
// directly, a propagation node both are configured to use, and conversation
// messages that have to cross it — in both directions, because a one-way
// exchange would not tell a broken recipient path from a broken sender path.
func TestIntegrationPropagatedConversationReachesRecipient(t *testing.T) {
	t.Parallel()
	hub := newPropagationHub(t)
	defer hub.cleanup()

	// Both clients use the propagation node, as both phones are configured to.
	hub.appA.SetUserSelectedPropagationNode(hub.pnHash)
	hub.appB.SetUserSelectedPropagationNode(hub.pnHash)
	hub.setPreferredDelivery(t, directory.DeliveryPropagated)

	hub.assertPropagatedDelivery(t, hub.appA, hub.appB, hub.hexB, hub.hexA, "propagated hello", &hub.deliveredB)
	hub.assertPropagatedDelivery(t, hub.appB, hub.appA, hub.hexA, hub.hexB, "propagated reply", &hub.deliveredA)
}

// TestIntegrationDirectConversationReachesRecipient is the same exchange without
// the propagation node: the messages go straight over the mesh, which is what
// the client's default delivery preference asks for and what has to keep working
// when no propagation node is configured.
func TestIntegrationDirectConversationReachesRecipient(t *testing.T) {
	t.Parallel()
	hub := newPropagationHub(t)
	defer hub.cleanup()

	hub.setPreferredDelivery(t, directory.DeliveryDirect)

	const first = "direct hello"
	if !hub.appA.SendConversation(hub.hexB, first, "") {
		t.Fatalf("SendConversation returned false")
	}
	waitForConversationMessage(t, hub.appB, hub.hexA, first, 30*time.Second, func() string {
		return fmt.Sprintf("deliveryCallback=%v", hub.deliveredB.Load())
	})

	const reply = "direct reply"
	if !hub.appB.SendConversation(hub.hexA, reply, "") {
		t.Fatalf("SendConversation(reply) returned false")
	}
	waitForConversationMessage(t, hub.appA, hub.hexB, reply, 30*time.Second, func() string {
		return fmt.Sprintf("deliveryCallback=%v", hub.deliveredA.Load())
	})
}
