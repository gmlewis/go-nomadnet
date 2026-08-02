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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// defaultCacheTime is the default page-cache lifetime in seconds (Python
// DEFAULT_CACHE_TIME = 12*60*60, Browser.py:1591).
const defaultCacheTime = 12 * 60 * 60

// BrowserCache manages on-disk page caching for the browser.
// Matches Python's cache_page/get_cached/clean_cache at Browser.py:1564-1614.
type BrowserCache struct {
	cachePath string
}

// NewBrowserCache creates a new cache manager rooted at cachePath.
func NewBrowserCache(cachePath string) *BrowserCache {
	return &BrowserCache{cachePath: cachePath}
}

// URLHash returns the SHA-256 full hash of url as a 64-character
// lowercase hex string. Matches Python's url_hash() at Browser.py:165.
func URLHash(url string) string {
	raw := sha256.Sum256([]byte(url))
	return hex.EncodeToString(raw[:])
}

// formatExpires formats an epoch-seconds float the way Python's str(float) does,
// so the cache filename matches the Python layout ("<hash>_<expires>"): a whole
// number renders with a trailing ".0" (str(123.0) == "123.0"), a fractional one
// without scientific notation. Round-trips through strconv.ParseFloat.
func formatExpires(expires float64) string {
	s := strconv.FormatFloat(expires, 'f', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}

// nowFloat returns the current time as epoch-seconds float, matching Python's
// time.time() used by get_cached/clean_cache for expiry comparison.
func nowFloat() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

// CachePage writes page data to the cache directory. The cache file is named
// "urlHash_expiresEpoch" where expiresEpoch is the Python str(float) form. Any
// existing cache entry for the same URL is removed first (uncache_page
// semantics). Matches Python's cache_page() at Browser.py:1615.
func (bc *BrowserCache) CachePage(url string, data []byte, expires float64) {
	urlHash := URLHash(url)
	bc.UncachePage(url)

	filename := urlHash + "_" + formatExpires(expires)
	cachefile := filepath.Join(bc.cachePath, filename)
	if err := os.WriteFile(cachefile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Could not write cache file: %v\n", err)
	}
}

// GetCached returns cached page data for url if a fresh entry exists. Expired
// entries are removed using a float epoch comparison (Python time.time() >
// expires, NOT int-truncated seconds — truncation diverges near the boundary).
// Returns nil if no valid cache entry found.
// Matches Python's get_cached() at Browser.py:1564.
func (bc *BrowserCache) GetCached(url string) []byte {
	urlHash := URLHash(url)

	entries, err := os.ReadDir(bc.cachePath)
	if err != nil {
		return nil
	}

	now := nowFloat()
	for _, entry := range entries {
		cachefile := filepath.Join(bc.cachePath, entry.Name())
		components := strings.SplitN(entry.Name(), "_", 2)
		if len(components) != 2 || len(components[0]) != 64 || len(components[1]) == 0 {
			continue
		}

		expires, parseErr := strconv.ParseFloat(components[1], 64)
		if parseErr != nil {
			_ = os.Remove(cachefile)
			continue
		}

		if now > expires {
			_ = os.Remove(cachefile)
			continue
		}

		if strings.HasPrefix(entry.Name(), urlHash) {
			data, readErr := os.ReadFile(cachefile)
			if readErr != nil {
				_ = os.Remove(cachefile)
				continue
			}
			return data
		}
	}

	return nil
}

// CacheTimeFromMarkup extracts the #!c=<int> cache-time directive from the head
// of a page, matching Python load_page (Browser.py:1524-1531). The directive is
// recognized ONLY when it is the very first 4 characters of the markup
// (markup[:4]=="#!c=") — a leading #!bg= line means no cache header. The value
// extends to the next newline, or the end of the markup if there is no newline
// (unlike #!bg/#!fg, which require a newline). The trimmed value is parsed as an
// int (Python int() strips surrounding whitespace); on any parse failure the
// default DEFAULT_CACHE_TIME is kept. A cache_time of 0 means "do not cache".
func CacheTimeFromMarkup(markup string) int {
	cacheTime := defaultCacheTime
	if len(markup) < 4 || markup[:4] != "#!c=" {
		return cacheTime
	}
	endpos := strings.IndexByte(markup, '\n')
	if endpos < 0 {
		endpos = len(markup)
	}
	n, err := strconv.Atoi(strings.TrimSpace(markup[4:endpos]))
	if err != nil {
		return cacheTime
	}
	return n
}

// UncachePage removes any cache entry for the given url.
// Matches Python's uncache_page() at Browser.py:1555.
func (bc *BrowserCache) UncachePage(url string) {
	urlHash := URLHash(url)

	entries, err := os.ReadDir(bc.cachePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), urlHash) {
			_ = os.Remove(filepath.Join(bc.cachePath, entry.Name()))
		}
	}
}

// CleanCache removes all expired cache entries and malformed files.
// Matches Python's clean_cache() at Browser.py:1598.
func (bc *BrowserCache) CleanCache() {
	entries, err := os.ReadDir(bc.cachePath)
	if err != nil {
		return
	}

	now := nowFloat()
	for _, entry := range entries {
		cachefile := filepath.Join(bc.cachePath, entry.Name())
		components := strings.SplitN(entry.Name(), "_", 2)
		if len(components) != 2 || len(components[0]) != 64 || len(components[1]) == 0 {
			_ = os.Remove(cachefile)
			continue
		}

		expires, parseErr := strconv.ParseFloat(components[1], 64)
		if parseErr != nil {
			_ = os.Remove(cachefile)
			continue
		}

		if now > expires {
			_ = os.Remove(cachefile)
		}
	}
}
