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

package config

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

//go:embed testdata/applyconfig_parity.json
var applyCfgFS embed.FS

type applyCfgFixture struct {
	Variants map[string]string         `json:"variants"`
	Golden   map[string]map[string]any `json:"golden"`
}

// TestApplyConfigPythonParity verifies that Go's DefaultConfig + Apply produces
// the same per-field values as Python's NomadNetworkApp.applyConfig, for a
// battery of config files covering default (empty-section) and edge-case
// (clamping, out-of-range, invalid) inputs. The golden values were captured by
// running the real Python applyConfig method body (extracted from
// NomadNetworkApp.py) against a mock self pre-initialized with Python's
// __init__ defaults, so absent keys reflect the true post-applyConfig state.
//
// Python stores some fields in different units than the Go config struct:
//   - rrc_ephemeral_notices: Python seconds (value*60), Go config minutes.
//   - node_announce_interval: Python minutes, Go config seconds (n*60).
//
// The comparison applies these conversions. Only sections present in a variant
// are compared for that variant (Python leaves section-specific fields at their
// __init__ defaults when the section is absent, and several of those are not
// set at all in Python's __init__).
func TestApplyConfigPythonParity(t *testing.T) {
	t.Parallel()

	data, err := applyCfgFS.ReadFile("testdata/applyconfig_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var fx applyCfgFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for name, text := range fx.Variants {
		name := name
		text := text
		golden := fx.Golden[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "nomadnet.conf")
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatalf("write conf: %v", err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			hasSection := func(sec string) bool {
				return strings.Contains(text, "["+sec+"]")
			}

			if hasSection("client") {
				checkClient(t, cfg, golden)
			}
			if hasSection("rrc") {
				checkRRC(t, cfg, golden)
			}
			if hasSection("node") {
				checkNode(t, cfg, golden)
			}
			if hasSection("printing") {
				checkPrinting(t, cfg, golden)
			}
		})
	}
}

func checkClient(t *testing.T, cfg *Config, g map[string]any) {
	t.Helper()
	has := func(k string) bool { _, ok := cfg.Raw["client"][k]; return ok }

	if has("enable_client") {
		if got, want := cfg.Client.EnableClient, gBool(g["enable_client"]); got != want {
			t.Errorf("enable_client = %v, want %v", got, want)
		}
	}
	if has("announce_interval") {
		if got, want := cfg.Client.AnnounceInterval, gInt(g["announce_interval"]); got != want {
			t.Errorf("announce_interval = %v, want %v", got, want)
		}
	}
	if has("try_propagation_on_send_fail") {
		if got, want := cfg.Client.TryPropagationOnSendFail, gBool(g["try_propagation_on_fail"]); got != want {
			t.Errorf("try_propagation_on_send_fail = %v, want %v", got, want)
		}
	}
	if has("periodic_lxmf_sync") {
		if got, want := cfg.Client.PeriodicLXMFSync, gBool(g["periodic_lxmf_sync"]); got != want {
			t.Errorf("periodic_lxmf_sync = %v, want %v", got, want)
		}
	}
	if has("lxmf_sync_interval") {
		if got, want := cfg.Client.LXMFSyncInterval, gInt(g["lxmf_sync_interval"]); got != want {
			t.Errorf("lxmf_sync_interval = %v, want %v", got, want)
		}
	}
	if has("lxmf_sync_limit") {
		if got, want := cfg.Client.LXMFSyncLimit, gIntOrZero(g["lxmf_sync_limit"]); got != want {
			t.Errorf("lxmf_sync_limit = %v, want %v", got, want)
		}
	}
	if has("required_stamp_cost") {
		if got, want := cfg.Client.RequiredStampCost, gIntPtr(g["required_stamp_cost"]); !reflect.DeepEqual(got, want) {
			t.Errorf("required_stamp_cost = %v, want %v", got, want)
		}
	}
	if has("accept_invalid_stamps") {
		if got, want := cfg.Client.AcceptInvalidStamps, gBool(g["accept_invalid_stamps"]); got != want {
			t.Errorf("accept_invalid_stamps = %v, want %v", got, want)
		}
	}
	// max_accepted_size: Python leaves it None when the key is absent (no
	// override); only compare when the key is present.
	if has("max_accepted_size") {
		if got, want := cfg.Client.MaxAcceptedSize, gFloat(g["lxmf_max_incoming_size"]); got != want {
			t.Errorf("max_accepted_size = %v, want %v", got, want)
		}
	}
	if has("compact_announce_stream") {
		if got, want := cfg.Client.CompactAnnounceStream, gBool(g["compact_stream"]); got != want {
			t.Errorf("compact_announce_stream = %v, want %v", got, want)
		}
	}
	if has("notify_on_new_message") {
		if got, want := cfg.Client.NotifyOnNewMessage, gBool(g["notify_on_new_message"]); got != want {
			t.Errorf("notify_on_new_message = %v, want %v", got, want)
		}
	}
	if has("compose_in_markdown") {
		if got, want := cfg.Client.ComposeInMarkdown, gBool(g["compose_markdown"]); got != want {
			t.Errorf("compose_in_markdown = %v, want %v", got, want)
		}
	}
}

func checkRRC(t *testing.T, cfg *Config, g map[string]any) {
	t.Helper()
	has := func(k string) bool { _, ok := cfg.Raw["rrc"][k]; return ok }

	if has("history_per_room_cap") {
		if got, want := cfg.RRC.HistoryPerRoomCap, gInt(g["rrc_history_per_room_cap"]); got != want {
			t.Errorf("rrc_history_per_room_cap = %v, want %v", got, want)
		}
	}
	if has("filter_loaded_history") {
		if got, want := cfg.RRC.FilterLoadedHistory, gBool(g["rrc_filter_loaded_history"]); got != want {
			t.Errorf("rrc_filter_loaded_history = %v, want %v", got, want)
		}
	}
	if has("ephemeral_notices") {
		// Python stores ephemeral_notices in seconds (value*60); Go config
		// stores minutes, so expect golden_seconds / 60.
		if got, want := cfg.RRC.EphemeralNotices, gFloat(g["rrc_ephemeral_notices"])/60; got != want {
			t.Errorf("ephemeral_notices = %v (min), want %v (min)", got, want)
		}
	}
	if has("nick_colors") {
		if got, want := cfg.RRC.NickColors, gBool(g["rrc_nick_colors"]); got != want {
			t.Errorf("rrc_nick_colors = %v, want %v", got, want)
		}
	}
	if has("mention_color") {
		if got, want := cfg.RRC.MentionColor, gStr(g["rrc_mention_color"]); got != want {
			t.Errorf("rrc_mention_color = %q, want %q", got, want)
		}
	}
	if has("nick_colors_theme") {
		if got, want := cfg.RRC.NickColorsTheme, gStrList(g["rrc_nick_colors_theme"]); !reflect.DeepEqual(got, want) {
			t.Errorf("rrc_nick_colors_theme = %v, want %v", got, want)
		}
	}
	if has("color_mention_timestamps") {
		if got, want := cfg.RRC.ColorMentionTimestamps, gBool(g["rrc_color_mention_timestamps"]); got != want {
			t.Errorf("rrc_color_mention_timestamps = %v, want %v", got, want)
		}
	}
	if has("justify_msgs") {
		if got, want := cfg.RRC.JustifyMsgs, gBool(g["rrc_ui_justify_msgs"]); got != want {
			t.Errorf("rrc_ui_justify_msgs = %v, want %v", got, want)
		}
	}
	if has("space_msgs") {
		if got, want := cfg.RRC.SpaceMsgs, gBool(g["rrc_ui_space_msgs"]); got != want {
			t.Errorf("rrc_ui_space_msgs = %v, want %v", got, want)
		}
	}
	if has("render_markdown") {
		if got, want := cfg.RRC.RenderMarkdown, gBool(g["rrc_ui_render_markdown"]); got != want {
			t.Errorf("rrc_ui_render_markdown = %v, want %v", got, want)
		}
	}
	if has("render_micron") {
		if got, want := cfg.RRC.RenderMicron, gBool(g["rrc_ui_render_micron"]); got != want {
			t.Errorf("rrc_ui_render_micron = %v, want %v", got, want)
		}
	}
	if has("show_gutters") {
		if got, want := cfg.RRC.ShowGutters, gBool(g["rrc_show_gutters"]); got != want {
			t.Errorf("rrc_show_gutters = %v, want %v", got, want)
		}
	}
	if has("enable_esoterics") {
		if got, want := cfg.RRC.EnableEsoterics, gBool(g["rrc_enable_esoterics"]); got != want {
			t.Errorf("rrc_enable_esoterics = %v, want %v", got, want)
		}
	}
}

func checkNode(t *testing.T, cfg *Config, g map[string]any) {
	t.Helper()
	has := func(k string) bool { _, ok := cfg.Raw["node"][k]; return ok }

	if has("enable_node") {
		if got, want := cfg.Node.EnableNode, gBool(g["enable_node"]); got != want {
			t.Errorf("enable_node = %v, want %v", got, want)
		}
	}
	if has("node_name") {
		if got, want := cfg.Node.NodeName, gStr(g["node_name"]); got != want {
			t.Errorf("node_name = %q, want %q", got, want)
		}
	}
	if has("disable_propagation") {
		if got, want := cfg.Node.DisablePropagation, gBool(g["disable_propagation"]); got != want {
			t.Errorf("disable_propagation = %v, want %v", got, want)
		}
	}
	if has("max_transfer_size") {
		if got, want := cfg.Node.MaxTransferSize, gFloat(g["lxmf_max_propagation_size"]); got != want {
			t.Errorf("max_transfer_size = %v, want %v", got, want)
		}
	}
	if has("max_sync_size") {
		if got, want := cfg.Node.MaxSyncSize, gFloat(g["lxmf_max_sync_size"]); got != want {
			t.Errorf("max_sync_size = %v, want %v", got, want)
		}
	}
	if has("announce_at_start") {
		if got, want := cfg.Node.AnnounceAtStart, gBool(g["node_announce_at_start"]); got != want {
			t.Errorf("node_announce_at_start = %v, want %v", got, want)
		}
	}
	if has("announce_interval") {
		// Python stores node_announce_interval in minutes; Go config stores seconds.
		if got, want := cfg.Node.AnnounceInterval, gInt(g["node_announce_interval"])*60; got != want {
			t.Errorf("node_announce_interval = %v (sec), want %v (sec)", got, want)
		}
	}
	if has("propagation_cost") {
		if got, want := cfg.Node.PropagationCost, gInt(g["node_propagation_cost"]); got != want {
			t.Errorf("node_propagation_cost = %v, want %v", got, want)
		}
	}
	if has("page_refresh_interval") {
		if got, want := cfg.Node.PageRefreshInterval, gInt(g["page_refresh_interval"]); got != want {
			t.Errorf("page_refresh_interval = %v, want %v", got, want)
		}
	}
	if has("file_refresh_interval") {
		if got, want := cfg.Node.FileRefreshInterval, gInt(g["file_refresh_interval"]); got != want {
			t.Errorf("file_refresh_interval = %v, want %v", got, want)
		}
	}
	if has("prioritise_destinations") {
		if got, want := cfg.Node.PrioritiseDestinations, gStrList(g["prioritised_lxmf_destinations"]); !reflect.DeepEqual(got, want) {
			t.Errorf("prioritise_destinations = %v, want %v", got, want)
		}
	}
	if has("static_peers") {
		if got, want := cfg.Node.StaticPeers, gStrList(g["static_peers"]); !reflect.DeepEqual(got, want) {
			t.Errorf("static_peers = %v, want %v", got, want)
		}
	}
	if has("max_peers") {
		if got, want := cfg.Node.MaxPeers, gIntPtr(g["max_peers"]); !reflect.DeepEqual(got, want) {
			t.Errorf("max_peers = %v, want %v", got, want)
		}
	}
	if has("message_storage_limit") {
		if got, want := cfg.Node.MessageStorageLimit, gFloat(g["message_storage_limit"]); got != want {
			t.Errorf("message_storage_limit = %v, want %v", got, want)
		}
	}
}

func checkPrinting(t *testing.T, cfg *Config, g map[string]any) {
	t.Helper()
	has := func(k string) bool { _, ok := cfg.Raw["printing"][k]; return ok }
	if has("print_messages") {
		if got, want := cfg.Printing.PrintMessages, gBool(g["print_messages"]); got != want {
			t.Errorf("print_messages = %v, want %v", got, want)
		}
	}
	if has("print_command") {
		if got, want := cfg.Printing.PrintCommand, gStr(g["print_command"]); got != want {
			t.Errorf("print_command = %q, want %q", got, want)
		}
	}
}

// ---- golden value helpers ----

func gBool(v any) bool {
	if v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

func gInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// gIntOrZero maps a Python value to an int, treating nil (Python None) as 0,
// which is Go's zero-value representation for "no limit" integer fields.
func gIntOrZero(v any) int {
	if v == nil {
		return 0
	}
	return gInt(v)
}

func gFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func gStr(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func gStrList(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func gIntPtr(v any) *int {
	if v == nil {
		return nil
	}
	n := gInt(v)
	return &n
}
