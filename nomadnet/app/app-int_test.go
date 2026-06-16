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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// setupTwoNodeApps creates two App instances connected via PipeInterface.
// Each app is initialized with InitWithTransport and connected via in-memory
// pipes. Returns both apps and a cleanup function.
func setupTwoNodeApps(t *testing.T) (*App, *App, func()) {
	t.Helper()

	dirA, cleanupA := testutils.TempDir(t, "nomadnet-int-a")
	dirB, cleanupB := testutils.TempDir(t, "nomadnet-int-b")

	tsA, rnsCleanupA := newStartedTSApp(t, dirA)
	tsB, rnsCleanupB := newStartedTSApp(t, dirB)

	pipeA, pipeB, pipeCleanup := newAppPipes(t, tsA, tsB)
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	appA := NewAppWithTransport(dirA, WithTransport(tsA), WithIdentity(tsA.Identity()))
	if err := appA.InitWithTransport(tsA, tsA.Identity()); err != nil {
		t.Fatalf("InitWithTransport A: %v", err)
	}

	appB := NewAppWithTransport(dirB, WithTransport(tsB), WithIdentity(tsB.Identity()))
	if err := appB.InitWithTransport(tsB, tsB.Identity()); err != nil {
		t.Fatalf("InitWithTransport B: %v", err)
	}

	cleanup := func() {
		appA.Shutdown()
		appB.Shutdown()
		pipeCleanup()
		rnsCleanupA()
		rnsCleanupB()
		cleanupA()
		cleanupB()
	}

	return appA, appB, cleanup
}

// waitForAnnounce polls app.Announces until an announce of the given
// type appears, or times out.
func waitForAnnounce(t *testing.T, app *App, announceType string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range app.GetAnnounces() {
			if ev.AnnounceType == announceType {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %v announce", announceType)
}

// waitForLXMFMessage waits for a message on the given channel or times out.
func waitForLXMFMessage(t *testing.T, ch <-chan *lxmf.Message, timeout time.Duration) *lxmf.Message {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for LXMF message")
		return nil
	}
}

func newStartedTSApp(t *testing.T, storageDir string) (*rns.TransportSystem, func()) {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	if err := ts.Start(filepath.Join(storageDir, "rns-storage")); err != nil {
		t.Fatalf("TransportSystem.Start error: %v", err)
	}
	return ts, func() {}
}

func writeAppRNSConfig(t *testing.T, configDir string) {
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

func newAppPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
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

func init() {
	_ = fmt.Sprintf
}
