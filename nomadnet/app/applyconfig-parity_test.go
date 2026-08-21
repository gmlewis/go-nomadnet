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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/testutils"
)

// applyCfgVariants is the input battery for the live cross-implementation
// applyConfig parity check, mirroring the config-package variant battery. The
// Go test owns these config-file variants; the expected post-apply field
// values are derived FRESH on every run by executing the real Python nomadnet
// NomadNetworkApp.applyConfig method (see applyCfgPython).
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

// TestApplyConfigAppFieldsParity verifies that app.applyConfig maps the loaded
// config.Config onto App fields with the same values Python's
// NomadNetworkApp.applyConfig produces. It focuses on the app-level fields
// whose conversion differs from the raw config struct:
//   - RRCEphemeralNotices: Python seconds (value*60); Go config holds minutes,
//     so applyConfig must multiply by 60.
//   - DisablePropagation: must honor the [node] disable_propagation key rather
//     than being hardcoded true.
//   - NodeAnnounceInterval: Python minutes; Go config holds seconds, so
//     applyConfig must divide by 60.
//
// Expected values are derived FRESH on every run by execing the real Python
// applyConfig against a mock self pre-initialized with Python's __init__
// defaults.
func TestApplyConfigAppFieldsParity(t *testing.T) {
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
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			a := &App{}
			a.applyConfig(cfg)

			hasSection := func(sec string) bool {
				return strings.Contains(text, "["+sec+"]")
			}
			hasKey := func(sec, key string) bool {
				_, ok := cfg.Raw[sec][key]
				return ok
			}

			if hasSection("rrc") && hasKey("rrc", "ephemeral_notices") {
				// Python stores ephemeral_notices in seconds; the App field is
				// documented in seconds.
				want := gIntApp(g["rrc_ephemeral_notices"])
				if a.RRCEphemeralNotices != want {
					t.Errorf("RRCEphemeralNotices = %v (sec), want %v (sec)", a.RRCEphemeralNotices, want)
				}
			}

			if hasSection("node") {
				if hasKey("node", "disable_propagation") {
					want := gBoolApp(g["disable_propagation"])
					if a.DisablePropagation != want {
						t.Errorf("DisablePropagation = %v, want %v", a.DisablePropagation, want)
					}
				}
				if hasKey("node", "announce_interval") {
					// Python stores node_announce_interval in minutes; the App
					// field is documented in minutes.
					want := gIntApp(g["node_announce_interval"])
					if a.NodeAnnounceInterval != want {
						t.Errorf("NodeAnnounceInterval = %v (min), want %v (min)", a.NodeAnnounceInterval, want)
					}
				}
			}
		})
	}
}

func gBoolApp(v any) bool {
	if v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

func gIntApp(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
