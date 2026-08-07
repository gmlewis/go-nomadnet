// Copyright 2026 Glenn Lewis. All rights reserved.

package browser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestServeLocalPage verifies the loopback page-serving path that lets a
// nomadnet instance browse its own node's served pages from disk (mirroring
// Python Browser.load_page's loopback branch, Browser.py:1300-1320) instead
// of establishing an RNS link to itself.
func TestServeLocalPage(t *testing.T) {
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "index.mu"), []byte("= Index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pages, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "sub", "page.mu"), []byte("= Sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"top-level index", "/page/index.mu", "= Index\n"},
		{"nested page", "/page/sub/page.mu", "= Sub\n"},
		{"missing page", "/page/none.mu", string(LocalPageNotFound)},
		{"directory target", "/page/sub", string(LocalPageNotFound)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ServeLocalPage(pages, tc.path)
			if string(got) != tc.want {
				t.Errorf("ServeLocalPage(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestServeLocalPageTraversal verifies the traversal guard: a path that
// escapes pagesPath returns the not-found body rather than reading an
// arbitrary file. Python concatenates unsanitized; the guard is a defensive
// addition.
func TestServeLocalPageTraversal(t *testing.T) {
	pages := t.TempDir()
	secret := t.TempDir()
	// Drop a secret file outside pagesPath.
	secretFile := filepath.Join(secret, "secret.mu")
	if err := os.WriteFile(secretFile, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "/page/../../<secretbase>/secret.mu" must not escape.
	rel, err := filepath.Rel(pages, secretFile)
	if err != nil {
		t.Fatal(err)
	}
	// Build a traversal URL.
	escaped := filepath.ToSlash(rel)
	got := ServeLocalPage(pages, "/page/../../"+escaped)
	if string(got) != string(LocalPageNotFound) {
		t.Errorf("traversal returned %q, want not-found", got)
	}
}

// TestServeLocalFile verifies the loopback file download (Python
// Browser.download_local_file, Browser.py:964-984) copies a served file into
// the downloads dir under a unique basename.
func TestServeLocalFile(t *testing.T) {
	files := t.TempDir()
	if err := os.MkdirAll(filepath.Join(files, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(files, "docs", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	dl := t.TempDir()

	name, size, err := ServeLocalFile(files, "/file/docs/readme.txt", dl)
	if err != nil {
		t.Fatalf("ServeLocalFile: %v", err)
	}
	if name != "readme.txt" {
		t.Errorf("savedName = %q, want readme.txt", name)
	}
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
	if _, err := os.Stat(filepath.Join(dl, name)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}

	// Missing file -> ErrNotExist.
	if _, _, err := ServeLocalFile(files, "/file/missing.bin", dl); err == nil {
		t.Errorf("missing file did not error")
	}
}