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
	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
)

// loadPeerSettings loads (or creates) the local peer settings file, mirroring
// the Python NomadNetworkApp peer_settings initialization. The announce
// interval is always reconciled with the configured value.
func (a *App) loadPeerSettings() {
	ps, err := peersettings.Load(a.PeerSettingsPath, a.AnnounceInterval)
	if err != nil {
		a.Logger.Error("Could not load peer settings: %v", err)
		ps = peersettings.DefaultSettings(a.AnnounceInterval)
	}
	a.PeerSettings = ps
}

// savePeerSettings persists the local peer settings to disk.
func (a *App) SavePeerSettings() {
	if a.PeerSettings == nil {
		return
	}
	if err := peersettings.Save(a.PeerSettings, a.PeerSettingsPath); err != nil {
		a.Logger.Error("Could not save peer settings: %v", err)
	}
}

// SetDisplayName updates the local peer's display name, propagates it to the
// LXMF delivery destination, and persists peer settings. This mirrors the
// Python NomadNetworkApp.set_display_name.
func (a *App) SetDisplayName(displayName string) {
	if a.PeerSettings == nil {
		a.PeerSettings = peersettings.DefaultSettings(a.AnnounceInterval)
	}
	a.PeerSettings.DisplayName = displayName
	if a.LXMFDest != nil && a.Router != nil {
		a.Router.SetDisplayName(a.LXMFDest.Hash, displayName)
	}
	a.SavePeerSettings()
}

// GetDisplayName returns the local peer's configured display name.
func (a *App) GetDisplayName() string {
	if a.PeerSettings == nil {
		return ""
	}
	return a.PeerSettings.DisplayName
}

// GetDisplayNameBytes returns the local peer's display name as UTF-8 bytes.
func (a *App) GetDisplayNameBytes() []byte {
	return []byte(a.GetDisplayName())
}
