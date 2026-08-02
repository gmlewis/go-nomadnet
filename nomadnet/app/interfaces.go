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
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// InterfaceStat is a transport-agnostic snapshot of one configured RNS interface
// as the Interfaces page renders it. It mirrors Python's merge of the config
// (Interfaces.py:2840-2897) with app.rns.get_interface_stats(): Name/Type/
// Enabled come from the RNS config (so disabled-in-config interfaces appear,
// in config-file order); Connected/TX/RX/Bitrate come from the live transport,
// looked up by interface name (false/0 when the interface is not running).
type InterfaceStat struct {
	Name      string
	Type      string
	Enabled   bool
	Connected bool
	TX        int64
	RX        int64
	Bitrate   int
}

// interfaceConfigEntry is one [[Name]] subsection under [interfaces] in the RNS
// config, in file order, with the properties the list view needs.
type interfaceConfigEntry struct {
	name    string // the [[...]] section key
	iface   string // display name: the "name" property, else the section key
	typeStr string // the "type" property
	enabled bool   // interface_enabled AND enabled both non-false (default true)
}

// RNSConfigPath returns the path to the RNS config file in use, or "" if it is
// not available yet (initRNS runs asynchronously). When an explicit RNS config
// dir was given it lives there; otherwise ensureStandaloneRNSConfig copied the
// system config into a standalone temp dir. The Interfaces "Open Text Editor"
// (C-w) action edits this file, matching Python's open_config_editor
// (Interfaces.py:3160) which edits self.app.rns.configpath.
func (a *App) RNSConfigPath() string {
	if a.RNSConfigDir != "" {
		return filepath.Join(a.RNSConfigDir, "config")
	}
	a.mu.Lock()
	dir := a.standaloneRNSDir
	a.mu.Unlock()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config")
}

// parseInterfaceConfig scans the RNS config and returns the [[Name]] subsections
// under [interfaces] in file order (configobj preserves order; the rns.LoadConfig
// map does not, so this scans the file directly). It captures type, name and the
// enabled state, matching Python's is_enabled rule (Interfaces.py:2871): enabled
// defaults to true and is false only when "enabled" or "interface_enabled" is one
// of false/off/no/0. RNodeMultiInterface sub-interface expansion (Interfaces.py:
// 2843-2856) is not handled here — a deferred gap.
func parseInterfaceConfig(path string) []interfaceConfigEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []interfaceConfigEntry
	section := "" // current top-level [section]
	var cur *interfaceConfigEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "[[[") && strings.HasSuffix(line, "]]]"):
			subName := strings.Trim(line, "[] ")
			if cur != nil {
				// Record sub-interface for RNodeMultiInterface
				_ = subName
			}
		case strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]"):
			name := strings.Trim(line, "[] ")
			if section == "interfaces" {
				out = append(out, interfaceConfigEntry{name: name, iface: name, enabled: true})
				cur = &out[len(out)-1]
			} else {
				cur = nil
			}
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.Trim(line, "[] ")
			cur = nil
		default:
			if cur == nil {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "type":
				cur.typeStr = value
			case "name":
				cur.iface = value
			case "enabled", "interface_enabled":
				if isFalseyConfigBool(value) {
					cur.enabled = false
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil
	}
	return out
}

// isFalseyConfigBool reports whether a config bool value means "false", matching
// Python's ('false','off','no','0') set (Interfaces.py:2871).
func isFalseyConfigBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "off", "no", "0":
		return true
	}
	return false
}

// InterfaceStats returns a snapshot of every interface configured in the RNS
// config, in config-file order, with live status/traffic merged from the
// transport — matching Python's Interfaces page list (Interfaces.py:2840-2897).
// Returns nil if the config is not available yet (initRNS runs asynchronously
// after Init, so the standalone config dir may not exist on the first poll; the
// 1 s ticker picks it up once initRNS completes).
//
// The config is the source of truth for the list (so disabled interfaces appear
// and the order matches the file); the transport supplies Connected/TX/RX/
// Bitrate by name. A transport read guarded by a.mu establishes a happens-before
// edge with the write in initRNS; GetInterfaces and the per-interface accessors
// perform their own internal locking.
func (a *App) InterfaceStats() []InterfaceStat {
	path := a.RNSConfigPath()
	entries := parseInterfaceConfig(path)
	if entries == nil {
		// Config not yet available (async init) — no interfaces to show.
		return nil
	}

	// Build a live-stats lookup by interface name from the transport.
	statsByName := map[string]InterfaceStat{}
	a.mu.Lock()
	ts := a.Transport
	a.mu.Unlock()
	if ts != nil {
		for _, iface := range ts.GetInterfaces() {
			statsByName[iface.Name()] = InterfaceStat{
				Connected: iface.Status(),
				TX:        int64(iface.BytesSent()),
				RX:        int64(iface.BytesReceived()),
				Bitrate:   iface.Bitrate(),
			}
		}
	}

	out := make([]InterfaceStat, 0, len(entries))
	for _, e := range entries {
		stat := InterfaceStat{
			Name:    e.iface,
			Type:    e.typeStr,
			Enabled: e.enabled,
		}
		if live, ok := statsByName[e.name]; ok {
			stat.Connected = live.Connected
			stat.TX = live.TX
			stat.RX = live.RX
			stat.Bitrate = live.Bitrate
		}
		out = append(out, stat)
	}
	return out
}
