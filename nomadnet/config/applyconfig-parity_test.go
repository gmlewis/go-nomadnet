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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// applyCfgVariants is the input battery for the live cross-implementation
// applyConfig parity check. The Go test owns these config-file variants; the
// expected post-apply field values are derived FRESH on every run by executing
// the real Python nomadnet NomadNetworkApp.applyConfig method (see
// applyCfgPython). The battery covers default (empty-section) and edge-case
// (clamping, out-of-range, invalid) inputs across the client, rrc, node, and
// printing sections.
var applyCfgVariants = map[string]string{
	"all_defaults":   "[logging]\n[client]\n[rrc]\n[node]\n[printing]\n",
	"client_edges":   "[client]\nenable_client = yes\nuser_interface = menu\ndownloads_path = ~/dl\nattachment_save_path = ~/atts\nnotify_on_new_message = true\nannounce_at_start = yes\nannounce_interval = 15\ntry_propagation_on_send_fail = true\nperiodic_lxmf_sync = yes\nlxmf_sync_interval = 2\nlxmf_sync_limit = 0\nrequired_stamp_cost = none\naccept_invalid_stamps = false\nmax_accepted_size = 0\ncompact_announce_stream = true\ncompose_in_markdown = yes\n",
	"client_edges2":  "[client]\nenable_client = no\nuser_interface = none\nannounce_interval = 100\nlxmf_sync_interval = 120\nlxmf_sync_limit = 25\nrequired_stamp_cost = 300\nmax_accepted_size = 1000\n",
	"client_stamp0":  "[client]\nrequired_stamp_cost = 0\nlxmf_sync_limit = 5\nlxmf_sync_interval = 0\n",
	"node_edges":     "[node]\nenable_node = yes\nnode_name = none\ndisable_propagation = false\nmax_transfer_size = 0\nmax_sync_size = 100\nannounce_at_start = yes\nannounce_interval = 0\npropagation_cost = 5\npage_refresh_interval = -5\nfile_refresh_interval = 12\nprioritise_destinations = abcd,ef12\nstatic_peers = 00112233\nmax_peers = -1\nmessage_storage_limit = 0\n",
	"node_edges2":    "[node]\nnode_name = My Node\nmax_transfer_size = 512\nmax_sync_size = 2000\nannounce_interval = 720\npropagation_cost = 20\nmax_peers = 8\nmessage_storage_limit = 0.001\n",
	"printing_edges": "[printing]\nprint_messages = yes\nprint_command = lpr\nprint_from = trusted\nmessage_template = ~/tmpl.txt\n",
	"rrc_edges":      "[rrc]\nhistory_per_room_cap = 5\nfilter_loaded_history = false\nephemeral_notices = 10\ncolor_mention_timestamps = false\nrender_markdown = false\nrender_micron = false\nnick_colors = false\njustify_msgs = false\nspace_msgs = true\nshow_gutters = true\nmention_color = ff00ff\nnick_colors_theme = ff00ff,xyz,00ff00\nenable_esoterics = true\n",
	"rrc_edges2":     "[rrc]\nhistory_per_room_cap = 0\nephemeral_notices = 3\nmention_color = xyz\nnick_colors_theme = abcdef\n",
}

// applyCfgParityScript imports the real nomadnet NomadNetworkApp reference,
// binds the real applyConfig method to a mock self pre-initialized with
// Python's __init__ defaults, and runs it for each variant supplied as JSON on
// stdin (variant name → config text). For each variant it collects the
// post-apply self.<field> values into a dict and emits the combined result as
// JSON on stdout. None values become JSON null. File-I/O side effects from the
// printing template path are intercepted so no files are created or read.
//
// Python stores some fields in different units than the Go config struct:
//   - rrc_ephemeral_notices: Python seconds (value*60), Go config minutes.
//   - node_announce_interval: Python minutes, Go config seconds (n*60).
//   - announce_interval: Python seconds (value*60), Go config seconds.
//
// The Go test applies the same unit conversions it documents in the check
// functions below; the script emits the raw Python values.
const applyCfgParityScript = `
import types, json, sys, os, io, builtins
import nomadnet.NomadNetworkApp as cls
import RNS, LXMF
from RNS.vendor.configobj import ConfigObj

# Intercept file I/O for the printing template path so applyConfig does not
# create or read real files when message_template is set.
_real_open = builtins.open
_real_isfile = os.path.isfile
def _safe_isfile(p):
    if "tmpl.txt" in str(p):
        return True
    return _real_isfile(p)
def _safe_open(p, mode="r", *a, **kw):
    if "tmpl.txt" in str(p):
        return io.BytesIO(b"TEMPLATE")
    return _real_open(p, mode, *a, **kw)
os.path.isfile = _safe_isfile
builtins.open = _safe_open

FIELDS = [
    "enable_client", "announce_interval", "try_propagation_on_fail",
    "periodic_lxmf_sync", "lxmf_sync_interval", "lxmf_sync_limit",
    "required_stamp_cost", "accept_invalid_stamps", "lxmf_max_incoming_size",
    "compact_stream", "notify_on_new_message", "compose_markdown",
    "rrc_history_per_room_cap", "rrc_filter_loaded_history",
    "rrc_ephemeral_notices", "rrc_nick_colors", "rrc_mention_color",
    "rrc_nick_colors_theme", "rrc_color_mention_timestamps",
    "rrc_ui_justify_msgs", "rrc_ui_space_msgs", "rrc_ui_render_markdown",
    "rrc_ui_render_micron", "rrc_show_gutters", "rrc_enable_esoterics",
    "enable_node", "node_name", "disable_propagation",
    "lxmf_max_propagation_size", "lxmf_max_sync_size",
    "node_announce_at_start", "node_announce_interval",
    "node_propagation_cost", "page_refresh_interval",
    "file_refresh_interval", "prioritised_lxmf_destinations",
    "static_peers", "max_peers", "message_storage_limit",
    "print_messages", "print_command",
]

def make_self():
    s = types.SimpleNamespace()
    s.configdir = "/tmp/parity_cfg"
    s.logfilepath = s.configdir + "/logfile"
    s.pagespath = s.configdir + "/storage/pages"
    s.filespath = s.configdir + "/storage/files"
    s.downloads_path = os.path.expanduser("~/Downloads")
    s.attachment_save_path = None
    s.page_refresh_interval = 0
    s.file_refresh_interval = 0
    s.static_peers = []
    s.peer_announce_at_start = True
    s.try_propagation_on_fail = True
    s.disable_propagation = True
    s.notify_on_new_message = True
    s.compose_markdown = True
    s.lxmf_max_propagation_size = None
    s.lxmf_max_sync_size = None
    s.lxmf_max_incoming_size = None
    s.node_propagation_cost = LXMF.LXMRouter.PROPAGATION_COST
    s.periodic_lxmf_sync = True
    s.lxmf_sync_interval = 360 * 60
    s.lxmf_sync_limit = 8
    s.compact_stream = False
    s.required_stamp_cost = None
    s.accept_invalid_stamps = False
    s.rrc_history_per_room_cap = 500
    s.rrc_filter_loaded_history = True
    s.rrc_ephemeral_notices = 600
    s.rrc_nick_colors = True
    s.rrc_nick_colors_theme = None
    s.rrc_mention_color = None
    s.rrc_color_mention_timestamps = True
    s.rrc_ui_justify_msgs = True
    s.rrc_ui_space_msgs = False
    s.rrc_ui_render_markdown = True
    s.rrc_ui_render_micron = True
    s.rrc_show_gutters = False
    s.rrc_enable_esoterics = False
    s.enable_client = False
    s.enable_node = False
    s.uimode = None
    s.announce_interval = 6 * 60 * 60
    s.node_name = None
    s.node_announce_at_start = None
    s.node_announce_interval = None
    s.prioritised_lxmf_destinations = None
    s.max_peers = None
    s.message_storage_limit = None
    s.print_command = "lp"
    s.print_messages = False
    s.print_all_messages = False
    s.print_trusted_messages = False
    s.allowed_message_print_destinations = None
    s.printing_template_msg = "TEMPLATE"
    s.force_console_log = False
    return s

variants = json.loads(sys.stdin.read() or "{}")
out = {}
for name, text in variants.items():
    s = make_self()
    s.applyConfig = types.MethodType(cls.applyConfig, s)
    s.config = ConfigObj(text.splitlines())
    s.applyConfig()
    fields = {}
    for f in FIELDS:
        fields[f] = getattr(s, f, None)
    out[name] = fields
print(json.dumps(out, ensure_ascii=False, default=str))
`

// applyCfgPythonOnce caches the single live Python run that derives fresh
// expected post-apply field values for every variant, so the per-variant
// sub-tests below share one python3 exec instead of one each.
var (
	applyCfgPythonOnce sync.Once
	applyCfgPythonOut  map[string]map[string]any
)

// applyCfgPython execs the real Python nomadnet applyConfig reference for the
// full variant battery and returns the fresh post-apply field values keyed by
// variant name. It is skipped (not failed) when the Python nomadnet reference
// is not importable.
func applyCfgPython(t *testing.T) map[string]map[string]any {
	t.Helper()
	applyCfgPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, applyCfgVariants, applyCfgParityScript, &applyCfgPythonOut)
	})
	return applyCfgPythonOut
}

// TestApplyConfigPythonParity verifies that Go's DefaultConfig + Apply produces
// the same per-field values as Python's NomadNetworkApp.applyConfig, for a
// battery of config files covering default (empty-section) and edge-case
// (clamping, out-of-range, invalid) inputs. The expected values are derived
// FRESH on every run by execing the real Python applyConfig against a mock self
// pre-initialized with Python's __init__ defaults, so absent keys reflect the
// true post-applyConfig state.
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

	golden := applyCfgPython(t)

	for name, text := range applyCfgVariants {
		g := golden[name]
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
				checkClient(t, cfg, g)
			}
			if hasSection("rrc") {
				checkRRC(t, cfg, g)
			}
			if hasSection("node") {
				checkNode(t, cfg, g)
			}
			if hasSection("printing") {
				checkPrinting(t, cfg, g)
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
