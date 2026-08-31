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

package app

import (
	"bytes"
	"encoding/hex"
	"os"

	"github.com/gmlewis/go-reticulum/rns"
)

// loadIgnoredList reads the ignored-destinations file from IgnoredPath, parsing
// one hex hash per line. Malformed lines are skipped. This mirrors the Python
// NomadNetworkApp ignored-list loading.
func (a *App) loadIgnoredList() {
	a.IgnoredList = nil
	data, err := os.ReadFile(a.IgnoredPath)
	if err != nil {
		return
	}
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		h, err := hex.DecodeString(string(line))
		if err != nil {
			continue
		}
		a.IgnoredList = append(a.IgnoredList, h)
	}
}

// PersistIgnoredList writes the current ignored-destinations list to disk, one
// hex hash per line, mirroring the Python NomadNetworkApp _persist_ignored_list.
func (a *App) PersistIgnoredList() {
	var buf []byte
	for _, h := range a.IgnoredList {
		buf = append(buf, []byte(hex.EncodeToString(h))...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(a.IgnoredPath, buf, 0o644); err != nil {
		if a.Logger != nil {
			a.Logger.Error("Could not persist ignored list: %v", err)
		}
	}
}

// IsIgnored reports whether the given source hash is in the ignored list.
func (a *App) IsIgnored(sourceHash []byte) bool {
	for _, h := range a.IgnoredList {
		if bytes.Equal(h, sourceHash) {
			return true
		}
	}
	return false
}

// BlockDestination adds a destination hash to the ignored list, persists it,
// and instructs the LXMF router (if available) to ignore messages from it. When
// the destination's identity is known it is also blackholed on the transport.
// This mirrors the Python NomadNetworkApp block_destination.
func (a *App) BlockDestination(destHash []byte, reason string) bool {
	if destHash == nil {
		return false
	}
	if a.Transport != nil {
		if id := rns.RecallIdentity(a.Transport, destHash); id != nil {
			a.Transport.BlackholeIdentity(id.Hash, nil, reason)
		}
	}
	if !a.IsIgnored(destHash) {
		a.IgnoredList = append(a.IgnoredList, destHash)
		a.PersistIgnoredList()
	}
	if a.Router != nil {
		a.Router.IgnoreDestination(destHash)
	}
	return true
}

// UnblockDestination removes a destination hash from the ignored list, persists
// the change, lifts any transport blackhole, and tells the LXMF router (if
// available) to stop ignoring it. This mirrors the Python NomadNet
// unblock_destination.
func (a *App) UnblockDestination(destHash []byte) bool {
	if destHash == nil {
		return false
	}
	if a.Transport != nil {
		if id := rns.RecallIdentity(a.Transport, destHash); id != nil {
			a.Transport.UnblackholeIdentity(id.Hash)
		}
	}
	for i, h := range a.IgnoredList {
		if bytes.Equal(h, destHash) {
			a.IgnoredList = append(a.IgnoredList[:i], a.IgnoredList[i+1:]...)
			a.PersistIgnoredList()
			break
		}
	}
	if a.Router != nil {
		a.Router.UnignoreDestination(destHash)
	}
	return true
}

// applyIgnoredDestinations replays every loaded ignored destination hash into
// the LXMF router's ignored list, mirroring the Python NomadNetworkApp startup
// loop (NomadNetworkApp.py:351-352: for destination_hash in self.ignored_list:
// message_router.ignore_destination(destination_hash)). Must be called after
// the router is created; it is a safe no-op when no router exists yet.
func (a *App) applyIgnoredDestinations() {
	if a.Router == nil {
		return
	}
	for _, h := range a.IgnoredList {
		a.Router.IgnoreDestination(h)
	}
}

// splitLines splits data on newline bytes, dropping the trailing empty element that
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
