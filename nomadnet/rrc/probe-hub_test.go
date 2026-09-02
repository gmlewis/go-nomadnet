// Copyright 2026 Glenn Lewis. All rights reserved.
// Throwaway live-hub diagnostic (deleted after use).
//go:build integration

package rrc

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

func TestProbeRealHubConnect(t *testing.T) {
	home, _ := os.UserHomeDir()
	rnsDir := filepath.Join(home, ".reticulum")
	logger := rns.NewLogger()
	ts := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(ts, rnsDir); err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}
	// Load the app identity so the hello carries it.
	idPath := filepath.Join(home, ".nomadnetwork", "storage", "identity")
	id, err := rns.FromFile(idPath, logger)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	storagePath := filepath.Join(home, ".nomadnetwork", "storage")
	m := NewManager(storagePath, nil)
	m.SetIdentity(id)
	m.SetChangeCallback(func() { t.Logf("change callback fired") })
	m.Load()

	hb, _ := hex.DecodeString("28c7c1a68c735693aa8e6b8193ed44b2")
	m.SetTransport(ts)
	hub := m.AddHub(hb, "rrc.hub", "RNS Community")

	hub.ConnectAsync()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		// Read hub state through Snapshot: the connect worker and inbound-link
		// callbacks mutate these fields under hub.lock, so unlocked reads race.
		status, statusText, motd, rooms := hub.Snapshot()
		t.Logf("status=%v text=%q rooms=%v motd=%q", status, statusText, rooms, motd)
		if status == StatusConnected || status == StatusFailed {
			break
		}
		time.Sleep(2 * time.Second)
	}
	status, statusText, motd, rooms := hub.Snapshot()
	if status != StatusConnected {
		t.Fatalf("hub did not connect: status=%v text=%q", status, statusText)
	}
	t.Logf("CONNECTED! motd=%q rooms=%v", motd, rooms)
	time.Sleep(5 * time.Second)
}
