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
	"reflect"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// TestApplyConfigWiresFields verifies that applyConfig maps the config.Config
// fields that Python NomadNetworkApp.applyConfig assigns onto the App struct.
// The config package performs unit conversion and clamping during Load, so
// applyConfig only needs to copy each field across. These mappings were
// previously missing (hardcoded or unassigned), diverging from the Python
// ~420-line applyConfig body (NomadNetworkApp.py:800-1218).
func TestApplyConfigWiresFields(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Client: config.ClientConfig{
			AttachmentSavePath: "/tmp/attachments",
			AnnounceAtStart:    true,
			UserInterface:      "text",
			MaxAcceptedSize:    512,
		},
		RRC: config.RRCConfig{
			NickColorsTheme: []string{"ff0000", "00ff00", "0000ff"},
		},
		Node: config.NodeConfig{
			PageRefreshInterval:    7,
			FileRefreshInterval:    9,
			StaticPeers:            []string{"abcdef0123456789abcdef0123456789"},
			MaxTransferSize:        256,
			MaxSyncSize:            10240,
			MessageStorageLimit:    2000,
			PrioritiseDestinations: []string{"fedcba9876543210fedcba9876543210"},
		},
	}
	maxPeers := 10
	cfg.Node.MaxPeers = &maxPeers

	a := &App{}
	a.applyConfig(cfg)

	if a.AttachmentSavePath != cfg.Client.AttachmentSavePath {
		t.Errorf("AttachmentSavePath = %q, want %q", a.AttachmentSavePath, cfg.Client.AttachmentSavePath)
	}
	if a.PeerAnnounceAtStart != cfg.Client.AnnounceAtStart {
		t.Errorf("PeerAnnounceAtStart = %v, want %v", a.PeerAnnounceAtStart, cfg.Client.AnnounceAtStart)
	}
	if a.UIMode != UIText {
		t.Errorf("UIMode = %d, want %d (UIText)", a.UIMode, UIText)
	}
	if !reflect.DeepEqual(a.RRCNickColorsTheme, cfg.RRC.NickColorsTheme) {
		t.Errorf("RRCNickColorsTheme = %v, want %v", a.RRCNickColorsTheme, cfg.RRC.NickColorsTheme)
	}
	if a.PageRefreshInterval != cfg.Node.PageRefreshInterval {
		t.Errorf("PageRefreshInterval = %d, want %d", a.PageRefreshInterval, cfg.Node.PageRefreshInterval)
	}
	if a.FileRefreshInterval != cfg.Node.FileRefreshInterval {
		t.Errorf("FileRefreshInterval = %d, want %d", a.FileRefreshInterval, cfg.Node.FileRefreshInterval)
	}
	if !reflect.DeepEqual(a.StaticPeers, cfg.Node.StaticPeers) {
		t.Errorf("StaticPeers = %v, want %v", a.StaticPeers, cfg.Node.StaticPeers)
	}
	if a.LXMFMaxIncomingSize == nil || *a.LXMFMaxIncomingSize != 512 {
		t.Errorf("LXMFMaxIncomingSize = %v, want 512", a.LXMFMaxIncomingSize)
	}
	if a.LXMFMaxPropagationSize == nil || *a.LXMFMaxPropagationSize != 256 {
		t.Errorf("LXMFMaxPropagationSize = %v, want 256", a.LXMFMaxPropagationSize)
	}
	if a.LXMFMaxSyncSize == nil || *a.LXMFMaxSyncSize != 10240 {
		t.Errorf("LXMFMaxSyncSize = %v, want 10240", a.LXMFMaxSyncSize)
	}
	if a.MaxPeers == nil || *a.MaxPeers != 10 {
		t.Errorf("MaxPeers = %v, want 10", a.MaxPeers)
	}
	if a.MessageStorageLimit != 2000 {
		t.Errorf("MessageStorageLimit = %v, want 2000", a.MessageStorageLimit)
	}
	if !reflect.DeepEqual(a.PrioritisedLXMF, cfg.Node.PrioritiseDestinations) {
		t.Errorf("PrioritisedLXMF = %v, want %v", a.PrioritisedLXMF, cfg.Node.PrioritiseDestinations)
	}
}

// TestApplyConfigUIModeMapping verifies the user_interface string maps to the
// correct App.UIMode constant, matching Python applyConfig's uimode selection.
func TestApplyConfigUIModeMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ui   string
		want int
	}{
		{"none", UINone},
		{"menu", UIMenu},
		{"text", UIText},
		{"graphical", UIGraphical},
		{"web", UIWeb},
	}
	for _, c := range cases {
		c := c
		t.Run(c.ui, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Client: config.ClientConfig{UserInterface: c.ui}}
			a := &App{}
			a.applyConfig(cfg)
			if a.UIMode != c.want {
				t.Errorf("UIMode(%q) = %d, want %d", c.ui, a.UIMode, c.want)
			}
		})
	}
}
