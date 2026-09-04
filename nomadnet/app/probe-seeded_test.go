// Copyright 2026 Glenn Lewis. All rights reserved.
// Throwaway diagnostic probe (integration tag, deleted after use).
//go:build integration

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

func TestProbeIsKnown(t *testing.T) {
	t.Parallel()
	seed := os.Getenv("PROBE_SEED")
	if seed == "" {
		t.Skip("PROBE_SEED not set")
	}
	logger := rns.NewLogger()
	ts := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulumWithLogger(ts, filepath.Join(seed, "rns"), logger); err != nil {
		t.Fatalf("reticulum init: %v", err)
	}
	id, err := rns.FromFile(filepath.Join(seed, "storage", "identity"), logger)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	a := NewAppWithTransport(seed, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	defer a.Shutdown()

	entries, _ := os.ReadDir(filepath.Join(seed, "storage", "conversations"))
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 32 {
			continue
		}
		var hb []byte
		for i := 0; i < len(e.Name()); i += 2 {
			var b byte
			_, err := fmt.Sscanf(e.Name()[i:i+2], "%02x", &b)
			if err != nil {
				continue
			}
			hb = append(hb, b)
		}
		if len(hb) != 16 {
			continue
		}
		t.Logf("peer %s: IsKnown=%v recalled=%v", e.Name(), a.Dir.IsKnown(hb), ts.Recall(hb) != nil)
	}
}
