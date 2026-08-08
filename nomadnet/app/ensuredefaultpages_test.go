// Copyright 2026 Glenn Lewis. All rights reserved.

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureDefaultPagesOnFreshInstall verifies the starter index.mu is copied
// into storage/pages on startup even when node hosting is disabled — the
// fresh-install case. The default config written on first run has
// enable_node = no, so startNode is a no-op; ensureDefaultPages must therefore
// run independently of enable_node (from Init) or the starter page is never
// created. See node-hosting.go:ensureDefaultPages.
func TestEnsureDefaultPagesOnFreshInstall(t *testing.T) {
	dir := tempDir(t)
	rnsDir := writeTestRNSConfig(t)
	// No config file => first run => CreateDefaultConfig writes the default
	// config (enable_node = no) and applies it, exactly like a real fresh
	// install.
	a := NewApp(dir, rnsDir, false, false)
	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer a.Shutdown()

	// The fresh-install default has node hosting disabled, so startNode is a
	// no-op; the node must NOT be started.
	if a.EnableNode {
		t.Fatalf("EnableNode = true on fresh install, want false (default config has enable_node = no)")
	}
	if a.Node != nil {
		t.Errorf("Node started on fresh install, want nil (enable_node = no)")
	}

	// Despite no node hosting, the starter index.mu must be present.
	indexPath := filepath.Join(a.PagesPath, "index.mu")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("index.mu not copied to %v: %v", indexPath, err)
	}
	if len(data) == 0 {
		t.Errorf("index.mu is empty")
	}
}

// TestEnsureDefaultPagesDoesNotOverwriteExisting verifies ensureDefaultPages
// never clobbers an operator's existing index.mu (the write is gated on
// os.IsNotExist).
func TestEnsureDefaultPagesDoesNotOverwriteExisting(t *testing.T) {
	dir := tempDir(t)
	rnsDir := writeTestRNSConfig(t)
	if err := os.MkdirAll(filepath.Join(dir, "storage", "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("> My Custom Home Page\n\nCustom content.\n")
	if err := os.WriteFile(filepath.Join(dir, "storage", "pages", "index.mu"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp(dir, rnsDir, false, false)
	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer a.Shutdown()

	got, err := os.ReadFile(filepath.Join(a.PagesPath, "index.mu"))
	if err != nil {
		t.Fatalf("index.mu: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("index.mu overwritten:\n got  %q\n want %q", string(got), string(custom))
	}
}
