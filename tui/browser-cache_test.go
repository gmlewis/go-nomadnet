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

package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bcTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "browser-cache-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestURLHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"simple", "abc123"},
		{"with path", "aabb1122aabb1122aabb1122aabb1122:/index"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := URLHash(tt.input)
			if len(got) != 64 {
				t.Errorf("URLHash(%q) len = %d, want 64", tt.input, len(got))
			}
			raw := sha256.Sum256([]byte(tt.input))
			expected := hex.EncodeToString(raw[:])
			if got != expected {
				t.Errorf("URLHash(%q) = %q, want %q", tt.input, got, expected)
			}
		})
	}
}

func TestCachePageAndGetCached(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/index"
	pageData := []byte("Hello Micron Page!")

	expires := time.Now().Add(5 * time.Minute).Unix()
	bc.CachePage(url, pageData, expires)

	got := bc.GetCached(url)
	if got == nil {
		t.Fatal("GetCached returned nil, want page data")
	}
	if string(got) != string(pageData) {
		t.Errorf("GetCached = %q, want %q", got, pageData)
	}
}

func TestGetCachedExpired(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/index"
	pageData := []byte("Expired page")

	expires := time.Now().Add(-1 * time.Second).Unix()
	bc.CachePage(url, pageData, expires)

	got := bc.GetCached(url)
	if got != nil {
		t.Errorf("GetCached = %q, want nil for expired entry", got)
	}

	files, _ := os.ReadDir(cacheDir)
	if len(files) != 0 {
		t.Errorf("cache dir has %d files after expired read, want 0", len(files))
	}
}

func TestUncachePage(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/about"
	pageData := []byte("About page")

	expires := time.Now().Add(5 * time.Minute).Unix()
	bc.CachePage(url, pageData, expires)

	bc.UncachePage(url)

	got := bc.GetCached(url)
	if got != nil {
		t.Errorf("GetCached after uncache = %q, want nil", got)
	}
}

func TestCachePageOverwrites(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/index"
	expires := time.Now().Add(5 * time.Minute).Unix()

	bc.CachePage(url, []byte("first"), expires)
	bc.CachePage(url, []byte("second"), expires)

	got := bc.GetCached(url)
	if got == nil {
		t.Fatal("GetCached returned nil")
	}
	if string(got) != "second" {
		t.Errorf("GetCached = %q, want %q", got, "second")
	}

	files, _ := os.ReadDir(cacheDir)
	if len(files) != 1 {
		t.Errorf("cache dir has %d files after overwrite, want 1", len(files))
	}
}

func TestCleanCache(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url1 := "aabb1122aabb1122aabb1122aabb1122:/fresh"
	url2 := "ccdd4433ccdd4433ccdd4433ccdd4433:/stale"

	freshExpires := time.Now().Add(5 * time.Minute).Unix()
	staleExpires := time.Now().Add(-1 * time.Second).Unix()

	bc.CachePage(url1, []byte("fresh"), freshExpires)
	bc.CachePage(url2, []byte("stale"), staleExpires)

	bc.CleanCache()

	got := bc.GetCached(url1)
	if got == nil || string(got) != "fresh" {
		t.Errorf("fresh page after CleanCache = %v, want 'fresh'", got)
	}

	got = bc.GetCached(url2)
	if got != nil {
		t.Errorf("stale page after CleanCache = %q, want nil", got)
	}
}

func TestCleanCacheRemovesMalformed(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	malformedFile := filepath.Join(cacheDir, "not_a_valid_cache_file")
	if err := os.WriteFile(malformedFile, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	bc.CleanCache()

	if _, err := os.Stat(malformedFile); !os.IsNotExist(err) {
		t.Error("malformed cache file should be removed by CleanCache")
	}
}

func TestGetCachedNoCacheDir(t *testing.T) {
	bc := NewBrowserCache("/nonexistent/path/that/does/not/exist")

	got := bc.GetCached("anyurl")
	if got != nil {
		t.Errorf("GetCached on missing dir = %v, want nil", got)
	}
}

func TestCachePageEmptyURL(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	bc.CachePage("", []byte("data"), time.Now().Add(5*time.Minute).Unix())

	got := bc.GetCached("")
	if got == nil {
		t.Error("GetCached for empty URL returned nil, want data")
	}
}
