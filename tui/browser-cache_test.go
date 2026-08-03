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

// expiresAfter is the epoch-seconds float for now+d, matching Python's
// time.time()+cache_time used to stamp cache entries.
func expiresAfter(d time.Duration) float64 {
	return float64(time.Now().Add(d).UnixNano()) / 1e9
}

// TestURLHashGolden pins URLHash against values captured from the installed
// Python nomadnet (Browser.py:165, RNS.hexrep(RNS.Identity.full_hash(url)) =
// SHA-256 → 64 hex).
func TestURLHashGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"abc123 path", "abc123:/page/index.mu", "39a7dbe816ee2a2f39ff47421c7154242914cd07b54c33bfcb72f06a08552035"},
		{"deadbeef path", "deadbeef:/page/x.mu", "45ebf2f1c947d2f76cc99606bdcd9a3bba15a756eca8587679f380b3f6f066d4"},
		{"empty", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"relative", ":/page/p.mu", "630980d0446b9ae434cf5055151f24df1bf313d291288cf8bdf5e5d7f57c5324"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := URLHash(c.url)
			if got != c.want {
				t.Errorf("URLHash(%q) = %q, want %q", c.url, got, c.want)
			}
			// Cross-check against a fresh SHA-256 of the URL.
			raw := sha256.Sum256([]byte(c.url))
			if got != hex.EncodeToString(raw[:]) {
				t.Errorf("URLHash(%q) = %q, want sha256 %q", c.url, got, hex.EncodeToString(raw[:]))
			}
		})
	}
}

// TestCacheTimeFromMarkupGolden pins the #!c= cache-time extraction against the
// values captured from the installed Python nomadnet load_page
// (Browser.py:1524-1531). The directive is recognized only when it is the very
// first 4 chars; the value runs to the next newline (or end of markup); int()
// strips whitespace; a parse failure keeps DEFAULT_CACHE_TIME (43200).
func TestCacheTimeFromMarkupGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		markup string
		want   int
	}{
		{"no header keeps default", "no cache header here", 43200},
		{"seconds", "#!c=3600\nbody", 3600},
		{"zero disables cache", "#!c=0\nbody", 0},
		{"large", "#!c=999999\nbody", 999999},
		{"parse failure keeps default", "#!c=abc\nbody", 43200},
		{"no newline still parses", "#!c=3600", 3600},
		{"whitespace stripped", "#!c= 100 \nbody", 100},
		{"negative", "#!c=-5\nbody", -5},
		{"#!bg first means no #!c header", "#!bg=fff\n#!c=7200\nbody", 43200},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CacheTimeFromMarkup(c.markup); got != c.want {
				t.Errorf("CacheTimeFromMarkup(%q) = %v, want %v", c.markup, got, c.want)
			}
		})
	}
}

func TestCachePageAndGetCached(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/index"
	pageData := []byte("Hello Micron Page!")

	bc.CachePage(url, pageData, expiresAfter(5*time.Minute))

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

	bc.CachePage(url, pageData, expiresAfter(-1*time.Second))

	got := bc.GetCached(url)
	if got != nil {
		t.Errorf("GetCached = %q, want nil for expired entry", got)
	}

	files, _ := os.ReadDir(cacheDir)
	if len(files) != 0 {
		t.Errorf("cache dir has %v files after expired read, want 0", len(files))
	}
}

// TestGetCachedExpiredFractional pins the float (not int-truncated) expiry
// comparison: an entry expiring at now+0.2s must be gone after a 0.5s sleep,
// which int-second truncation (the original bug) would have left stale.
func TestGetCachedExpiredFractional(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/frac"
	bc.CachePage(url, []byte("frac"), expiresAfter(200*time.Millisecond))

	if got := bc.GetCached(url); got == nil {
		t.Fatal("entry should be fresh immediately")
	}
	time.Sleep(500 * time.Millisecond)
	if got := bc.GetCached(url); got != nil {
		t.Errorf("GetCached after fractional expiry = %q, want nil", got)
	}
}

func TestUncachePage(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/about"
	pageData := []byte("About page")

	bc.CachePage(url, pageData, expiresAfter(5*time.Minute))

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
	expires := expiresAfter(5 * time.Minute)

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
		t.Errorf("cache dir has %v files after overwrite, want 1", len(files))
	}
}

func TestCleanCache(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url1 := "aabb1122aabb1122aabb1122aabb1122:/fresh"
	url2 := "ccdd4433ccdd4433ccdd4433ccdd4433:/stale"

	bc.CachePage(url1, []byte("fresh"), expiresAfter(5*time.Minute))
	bc.CachePage(url2, []byte("stale"), expiresAfter(-1*time.Second))

	bc.CleanCache()

	got := bc.GetCached(url1)
	if got == nil || string(got) != "fresh" {
		t.Errorf("fresh page after CleanCache = %s, want 'fresh'", got)
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
		t.Errorf("GetCached on missing dir = %s, want nil", got)
	}
}

func TestCachePageEmptyURL(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	bc.CachePage("", []byte("data"), expiresAfter(5*time.Minute))

	got := bc.GetCached("")
	if got == nil {
		t.Error("GetCached for empty URL returned nil, want data")
	}
}

// TestCacheFilenameMatchesPython pins the on-disk filename layout
// ("<urlhash>_<expires>") and the str(float) expiry formatting so a Go-written
// entry round-trips and matches the Python filename convention (whole numbers
// carry ".0").
func TestCacheFilenameMatchesPython(t *testing.T) {
	cacheDir := bcTempDir(t)
	bc := NewBrowserCache(cacheDir)

	url := "aabb1122aabb1122aabb1122aabb1122:/index"
	bc.CachePage(url, []byte("x"), 1700000000.0)

	entries, _ := os.ReadDir(cacheDir)
	if len(entries) != 1 {
		t.Fatalf("want 1 cache file, got %v", len(entries))
	}
	name := entries[0].Name()
	prefix := URLHash(url) + "_"
	if name[:len(prefix)] != prefix {
		t.Fatalf("filename %q does not start with %q", name, prefix)
	}
	if want := "1700000000.0"; name[len(prefix):] != want {
		t.Errorf("filename expiry = %q, want %q (str(float) form)", name[len(prefix):], want)
	}
}
