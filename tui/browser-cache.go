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

// CachePage writes page data to the cache directory. The cache file
// is named "urlHash_expiresEpoch". Any existing cache entry for the
// same URL is removed first (uncache_page semantics).
// Matches Python's cache_page() at Browser.py:1615.
func (bc *BrowserCache) CachePage(url string, data []byte, expires int64) {
	urlHash := URLHash(url)
	bc.UncachePage(url)

	filename := fmt.Sprintf("%s_%v", urlHash, expires)
	cachefile := filepath.Join(bc.cachePath, filename)
	if err := os.WriteFile(cachefile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Could not write cache file: %v\n", err)
	}
}

// GetCached returns cached page data for url if a fresh entry exists.
// Expired entries are removed. Returns nil if no valid cache entry found.
// Matches Python's get_cached() at Browser.py:1564.
func (bc *BrowserCache) GetCached(url string) []byte {
	urlHash := URLHash(url)

	entries, err := os.ReadDir(bc.cachePath)
	if err != nil {
		return nil
	}

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

		if time.Now().Unix() > int64(expires) {
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

		if time.Now().Unix() > int64(expires) {
			_ = os.Remove(cachefile)
		}
	}
}
