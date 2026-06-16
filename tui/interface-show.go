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
	"fmt"
	"sort"
	"strings"
)

// ParamCategories holds interface parameters grouped by category for
// the ShowInterface detail view. Matches Python's parameter sorting
// in ShowInterface.__init__() at Interfaces.py:2208.
type ParamCategories struct {
	Connection map[string]any
	Radio      map[string]any
	Network    map[string]any
	IFAC       map[string]any
	Other      map[string]any
}

// connectionKeys are interface config keys in the "Connection" category.
var connectionKeys = map[string]bool{
	"port": true, "listen_ip": true, "listen_port": true,
	"target_host": true, "target_port": true, "device": true,
}

// radioKeys are interface config keys in the "Radio" category.
var radioKeys = map[string]bool{
	"frequency": true, "bandwidth": true,
	"spreadingfactor": true, "codingrate": true, "txpower": true,
}

// networkKeys are interface config keys in the "Network" category.
var networkKeys = map[string]bool{
	"network_name": true, "bitrate": true, "peers": true,
	"group_id": true, "multicast_address_type": true,
	"discovery_scope": true, "announce_cap": true, "mode": true,
}

// ifacKeys are interface config keys in the "IFAC" category.
var ifacKeys = map[string]bool{
	"passphrase": true, "ifac_size": true, "ifac_netname": true, "ifac_netkey": true,
}

// skipKeys are config keys that should not be displayed in the detail view.
var skipKeys = map[string]bool{
	"type": true, "interface_enabled": true, "enabled": true,
	"selected_interface_mode": true, "name": true,
}

// CategorizeInterfaceParams groups interface config parameters into
// connection, radio, network, IFAC, and other categories. Empty and
// nil values are excluded. Keys in skipKeys are omitted.
// Matches Python's ShowInterface parameter sorting at Interfaces.py:2368.
func CategorizeInterfaceParams(config map[string]any) ParamCategories {
	cats := ParamCategories{
		Connection: make(map[string]any),
		Radio:      make(map[string]any),
		Network:    make(map[string]any),
		IFAC:       make(map[string]any),
		Other:      make(map[string]any),
	}

	for key, value := range config {
		if skipKeys[key] {
			continue
		}
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok && s == "" {
			continue
		}

		switch {
		case connectionKeys[key]:
			cats.Connection[key] = value
		case radioKeys[key]:
			cats.Radio[key] = value
		case networkKeys[key]:
			cats.Network[key] = value
		case ifacKeys[key]:
			cats.IFAC[key] = value
		default:
			cats.Other[key] = value
		}
	}

	return cats
}

// SortedKeys returns the keys of a map sorted alphabetically.
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatParamValue formats an interface parameter value for display.
// Special-cases: frequency (Hz → MHz), bandwidth (Hz → kHz),
// passphrase (masked), boolean (Yes/No).
// Matches Python's create_param_row() at Interfaces.py:2413.
func FormatParamValue(key string, value any) string {
	switch key {
	case "frequency":
		return formatRadioFrequency(value)
	case "bandwidth":
		return formatRadioBandwidth(value)
	case "passphrase":
		if s, ok := value.(string); ok {
			return strings.Repeat("*", len(s))
		}
		return "***"
	}

	switch v := value.(type) {
	case bool:
		if v {
			return "Yes"
		}
		return "No"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// FormatParamKey converts a snake_case config key to a display label.
// For example, "listen_port" becomes "Listen Port".
// Matches Python's key.replace('_', ' ').title() at Interfaces.py:2427.
// Python's .title() treats digit-letter boundaries as word breaks,
// so "i2p" becomes "I2P". We replicate this behavior.
func FormatParamKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = titleWord(p)
		}
	}
	return strings.Join(parts, " ")
}

// titleWord mimics Python's str.title() behavior for a single
// word. Python's title() capitalizes a character if the preceding
// character is not a letter or digit (i.e., after digits, the next
// letter is also uppercased). For example: "i2p" → "I2P".
func titleWord(word string) string {
	if len(word) == 0 {
		return word
	}
	var sb strings.Builder
	prevIsLetter := false
	for i, r := range word {
		if i == 0 || !prevIsLetter {
			sb.WriteRune(toUpper(r))
		} else {
			sb.WriteRune(toLower(r))
		}
		prevIsLetter = isLetter(r)
	}
	return sb.String()
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}

// formatRadioFrequency converts a Hz value to MHz display string.
func formatRadioFrequency(value any) string {
	var hz float64
	switch v := value.(type) {
	case float64:
		hz = v
	case int64:
		hz = float64(v)
	case int:
		hz = float64(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	mhz := hz / 1000000.0
	return fmt.Sprintf("%.3f MHz", mhz)
}

// formatRadioBandwidth converts a Hz value to kHz display string.
func formatRadioBandwidth(value any) string {
	var hz float64
	switch v := value.(type) {
	case float64:
		hz = v
	case int64:
		hz = float64(v)
	case int:
		hz = float64(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	khz := hz / 1000.0
	return fmt.Sprintf("%.1f kHz", khz)
}
