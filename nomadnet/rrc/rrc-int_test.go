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

package rrc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

func TestIntegrationHubConnectEstablishesLink(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	// Server: create RRC destination
	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverConnected := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case serverConnected <- l:
		default:
		}
	})

	// Client: create hub and connect
	mgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	mgr.SetNickname("TestUser")
	hub := mgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	hub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := hub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
		if hub.Status != StatusConnected {
			t.Errorf("hub status = %d, want StatusConnected", hub.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverConnected:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}
}

func newStartedTS(t *testing.T) (*rns.TransportSystem, func()) {
	t.Helper()
	dir, cleanup := testutils.TempDir(t, "nomadnet-rrc-int-ts")
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfigRRC(t, cfgDir)
	ts := rns.NewTransportSystem(nil)
	_, err := rns.NewReticulum(ts, cfgDir)
	if err != nil {
		cleanup()
		t.Fatalf("NewReticulum error: %v", err)
	}
	return ts, cleanup
}

func writeRNSConfigRRC(t *testing.T, configDir string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `[reticulum]
share_instance = No

[logging]
loglevel = 4
`
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newRRCPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
	t.Helper()
	pipeA := interfaces.NewPipeInterface("a", func(data []byte, iface interfaces.Interface) {
		tsA.Inbound(data, iface)
	})
	pipeB := interfaces.NewPipeInterface("b", func(data []byte, iface interfaces.Interface) {
		tsB.Inbound(data, iface)
	})
	pipeA.SetOther(pipeB)
	pipeB.SetOther(pipeA)
	cleanup := func() {
		_ = pipeA.Detach()
		_ = pipeB.Detach()
	}
	return pipeA, pipeB, cleanup
}

func TestIntegrationHubExchangeMessages(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverLinkEstablished := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case serverLinkEstablished <- l:
		default:
		}
	})

	mgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	mgr.SetNickname("TestUser")
	hub := mgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	hub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := hub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case serverLink := <-serverLinkEstablished:
		t.Cleanup(serverLink.Teardown)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	hub.AddRoom("general")
	hub.SendMessage("general", "Hello from test!")

	msgReceived := make(chan string, 1)
	mgr.SetMessageCallback(func(h *RRCHub, m *RRCMessage) {
		select {
		case msgReceived <- m.Text:
		default:
		}
	})

	// The server-side doesn't have an RRC hub, so messages sent from the client
	// go through the link but aren't processed by a server hub.
	// This test verifies the link and _sendEnv work without errors.
	// A full round-trip message exchange requires the server to also run an RRCHub.

	msgs := hub.GetMessages("general")
	if len(msgs) == 0 {
		t.Error("expected at least one message (local echo)")
	}
}

func TestIntegrationHubAnnounce(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	// Create RRC destination on side A
	dest, err := rns.NewDestination(tsA, tsA.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("dest error: %v", err)
	}

	// Side B registers announce handler for rrc.chat
	announceReceived := make(chan []byte, 1)
	tsB.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter: "rrc.chat",
		ReceivedAnnounceWithContext: func(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
			select {
			case announceReceived <- destHash:
			default:
			}
		},
	})

	// Side A announces
	if err := dest.Announce(nil); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	select {
	case hash := <-announceReceived:
		if len(hash) == 0 {
			t.Error("received empty hash")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for rrc.chat announce on peer")
	}
}

func tempDirRRC(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-rrc-int-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestIntegrationHelloWelcomeHandshake(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return tsServer.Identity().Hash })
	serverMgr.SetNickname("ServerHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "ServerHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			serverHub.HandleData(data)
		})
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverLinkCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		welcomed := clientHub.Welcomed
		clientHub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.lock.Lock()
	welcomed := clientHub.Welcomed
	clientHub.lock.Unlock()
	if !welcomed {
		t.Fatal("client hub should be welcomed after handshake")
	}
}

func TestIntegrationJoinRoomSeesJoinedNotification(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return tsServer.Identity().Hash })
	serverMgr.SetNickname("ServerHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "ServerHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			serverHub.HandleData(data)
		})
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverLinkCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		welcomed := clientHub.Welcomed
		clientHub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.JoinRoom("general", false)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		hasMembers := len(clientHub.Members["general"]) > 0
		clientHub.lock.Unlock()
		if hasMembers {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.lock.Lock()
	membersBefore := len(clientHub.Members["general"])
	clientHub.lock.Unlock()
	if membersBefore == 0 {
		t.Fatal("expected members in 'general' room after join")
	}

	serverHub.lock.Lock()
	serverMembersBefore := len(serverHub.Members["general"])
	serverHub.lock.Unlock()
	if serverMembersBefore == 0 {
		t.Error("server should also have members in 'general' room after join")
	}
}

func TestIntegrationSendMessageReceivedByServer(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return tsServer.Identity().Hash })
	serverMgr.SetNickname("ServerHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "ServerHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			serverHub.HandleData(data)
		})
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverLinkCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		welcomed := clientHub.Welcomed
		clientHub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.AddRoom("general")
	clientHub.SendMessage("general", "Hello from client!")

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		serverMsgs := serverHub.GetMessages("general")
		if len(serverMsgs) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	serverMsgs := serverHub.GetMessages("general")
	if len(serverMsgs) == 0 {
		t.Fatal("server hub should have recorded the message")
	}
	found := false
	for _, m := range serverMsgs {
		if m.Text == "Hello from client!" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("server hub messages don't contain expected text; got %v", serverMsgs)
	}
}

func TestIntegrationPartRoomUpdatesMembers(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return tsServer.Identity().Hash })
	serverMgr.SetNickname("ServerHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "ServerHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			serverHub.HandleData(data)
		})
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverLinkCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		welcomed := clientHub.Welcomed
		clientHub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.JoinRoom("general", false)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		hasMembers := len(clientHub.Members["general"]) > 0
		clientHub.lock.Unlock()
		if hasMembers {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.lock.Lock()
	membersBefore := len(clientHub.Members["general"])
	clientHub.lock.Unlock()
	if membersBefore == 0 {
		t.Fatal("expected members in 'general' room before part")
	}

	clientHub.PartRoom("general")

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		serverHub.lock.Lock()
		serverEmpty := len(serverHub.Members["general"]) == 0
		serverHub.lock.Unlock()
		if serverEmpty {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	serverHub.lock.Lock()
	serverMembersAfter := len(serverHub.Members["general"])
	serverHub.lock.Unlock()
	if serverMembersAfter > 0 {
		t.Errorf("server should have removed member after part, got %d members", serverMembersAfter)
	}

	clientHub.lock.Lock()
	clientMembersAfter := len(clientHub.Members["general"])
	clientHub.lock.Unlock()
	if clientMembersAfter > 0 {
		t.Errorf("client should have removed member after PARTED notification, got %d members", clientMembersAfter)
	}
}

func TestIntegrationHubDisconnectUpdatesStatus(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	clientClosed := make(chan struct{}, 1)
	clientHub.SetOnLinkClosed(func() {
		select {
		case clientClosed <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	clientHub.lock.Lock()
	status := clientHub.Status
	clientHub.lock.Unlock()
	if status != StatusConnected {
		t.Fatalf("expected StatusConnected, got %d", status)
	}

	clientHub.Disconnect()

	select {
	case <-clientClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for link closed callback")
	}

	clientHub.lock.Lock()
	status = clientHub.Status
	clientHub.lock.Unlock()
	if status != StatusDisconnected {
		t.Errorf("expected StatusDisconnected after disconnect, got %d", status)
	}
}

func TestIntegrationMultipleRoomsIsolated(t *testing.T) {
	tsClient, clientCleanup := newStartedTS(t)
	defer clientCleanup()
	tsServer, serverCleanup := newStartedTS(t)
	defer serverCleanup()

	pipeA, pipeB, pipeCleanup := newRRCPipes(t, tsClient, tsServer)
	defer pipeCleanup()
	tsClient.RegisterInterface(pipeA)
	tsServer.RegisterInterface(pipeB)

	serverDest, err := rns.NewDestination(tsServer, tsServer.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return tsServer.Identity().Hash })
	serverMgr.SetNickname("ServerHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "ServerHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, packet *rns.Packet) {
			serverHub.HandleData(data)
		})
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	clientMgr := NewManager(tempDirRRC(t), func() []byte { return tsClient.Identity().Hash })
	clientMgr.SetNickname("TestClient")
	clientHub := clientMgr.AddHub(serverDest.Hash, "rrc.chat", "TestHub")

	clientEstablished := make(chan struct{}, 1)
	clientHub.SetOnLinkEstablished(func() {
		select {
		case clientEstablished <- struct{}{}:
		default:
		}
	})

	if err := clientHub.Connect(tsClient, serverDest); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	select {
	case <-clientEstablished:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client link establishment")
	}

	select {
	case <-serverLinkCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server link establishment")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		clientHub.lock.Lock()
		welcomed := clientHub.Welcomed
		clientHub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	clientHub.AddRoom("room-a")
	clientHub.AddRoom("room-b")
	clientHub.SendMessage("room-a", "message for room A")
	clientHub.SendMessage("room-b", "message for room B")

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		serverMsgsA := serverHub.GetMessages("room-a")
		serverMsgsB := serverHub.GetMessages("room-b")
		if len(serverMsgsA) > 0 && len(serverMsgsB) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	serverMsgsA := serverHub.GetMessages("room-a")
	serverMsgsB := serverHub.GetMessages("room-b")

	if len(serverMsgsA) == 0 {
		t.Fatal("expected messages in room-a")
	}
	if len(serverMsgsB) == 0 {
		t.Fatal("expected messages in room-b")
	}

	for _, m := range serverMsgsA {
		if m.Room != "room-a" {
			t.Errorf("room-a message has wrong room: %q", m.Room)
		}
	}
	for _, m := range serverMsgsB {
		if m.Room != "room-b" {
			t.Errorf("room-b message has wrong room: %q", m.Room)
		}
	}

	foundA := false
	for _, m := range serverMsgsA {
		if m.Text == "message for room A" {
			foundA = true
		}
	}
	if !foundA {
		t.Error("room-a should contain 'message for room A'")
	}

	foundB := false
	for _, m := range serverMsgsB {
		if m.Text == "message for room B" {
			foundB = true
		}
	}
	if !foundB {
		t.Error("room-b should contain 'message for room B'")
	}
}
