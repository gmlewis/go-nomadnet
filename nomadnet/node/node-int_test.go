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

package node

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

func TestIntegrationNodeAnnounceReceivedByPeer(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	n := NewNode("IntegrationNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	announceReceived := make(chan []byte, 1)
	tsB.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter: "nomadnetwork.node",
		ReceivedAnnounceWithContext: func(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
			select {
			case announceReceived <- appData:
			default:
			}
		},
	})

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	select {
	case appData := <-announceReceived:
		if string(appData) != "IntegrationNode" {
			t.Errorf("appData = %q, want %q", string(appData), "IntegrationNode")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for node announce on peer")
	}
}

func newStartedTS(t *testing.T) (*rns.TransportSystem, func()) {
	t.Helper()
	dir, cleanup := testutils.TempDir(t, "nomadnet-node-int-ts")
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfig(t, cfgDir)
	ts := rns.NewTransportSystem(nil)
	_, err := rns.NewReticulum(ts, cfgDir)
	if err != nil {
		cleanup()
		t.Fatalf("NewReticulum error: %v", err)
	}
	return ts, cleanup
}

func writeRNSConfig(t *testing.T, configDir string) {
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

func newTestPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
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

func tempDirInt(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-node-int-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func init() {
	_ = fmt.Sprintf
}
