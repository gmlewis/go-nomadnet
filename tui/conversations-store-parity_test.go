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
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/testutils"
)

// This file holds the disk-level conversation-store parity tests: each test
// synthesizes a ~/.nomadnetwork/storage/conversations tree ON DISK (message
// files, unread/failed flag files, directory mtimes, hostile names), then asks
// BOTH implementations to enumerate and render it:
//
//   - Python: nomadnet.Conversation.conversation_list() runs for real over the
//     synthesized tree, the pinned-first partition from update_listbox
//     (ui/textui/Conversations.py:450-457) is applied, and each row is
//     rendered through the ACTUAL conversation_list_widget source, extracted
//     with inspect+ast exactly like TestConversationRowMainPythonParity.
//   - Go: conversation.ConversationList + SortConversations +
//     conversationRowMain/conversationRowSecondary over the same tree.
//
// Every expected value is computed FRESH by the live Python reference; nothing
// here is a hardcoded golden file. Fixture mtimes are anchored relative to the
// test's own start time (sub-day offsets) or to fixed-noon calendar dates
// (multi-day offsets) so both sides — which read the wall clock seconds apart —
// land in the same relative_time bucket every time.

// parityFile is one extra file placed inside a conversation directory.
type parityFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// parityConv describes one synthesized conversation directory.
type parityConv struct {
	Hash     string       `json:"hash"` // 32 hex chars of a truncated identity hash
	DirMtime float64      `json:"dirMtime"`
	Name     *string      `json:"name"` // nil = no display name (Python None)
	Trust    byte         `json:"trust"`
	Pinned   bool         `json:"pinned"`
	Unread   *string      `json:"unread"` // nil = no unread file; else its content
	Failed   *string      `json:"failed"` // nil = no failed file; else its content
	Files    []parityFile `json:"files"`
}

// parityCollection is one fully synthesized conversations store.
type parityCollection struct {
	Name  string       `json:"name"`
	Convs []parityConv `json:"convs"`
}

// pyRow is Python's rendered result for one conversation.
type pyRow struct {
	Hash      string  `json:"hash"`
	Unread    int     `json:"unread"`
	Failed    int     `json:"failed"`
	Last      float64 `json:"last"`
	Label     string  `json:"label"`
	Main      string  `json:"main"`
	Secondary string  `json:"secondary"`
}

// goRow is the Go-side rendered result for one conversation.
type goRow struct {
	Hash      string
	Unread    int
	Failed    int
	Last      float64
	Main      string
	Secondary string
}

// conversationStoreParityScript builds each synthesized tree on disk, runs the
// REAL conversation_list() over it, applies the pinned-first partition from
// update_listbox verbatim, and renders each row through the ACTUAL
// conversation_list_widget source. relative_time runs on the real wall clock —
// the same clock the Go side uses, seconds apart, with fixtures chosen deep
// inside their buckets.
const conversationStoreParityScript = `
import ast, inspect, json, logging, os, shutil, sys, tempfile, textwrap, types
import RNS, nomadnet.ui.textui.Conversations as C

# RNS.log prints to stdout by default (its own logging, not the logging module),
# which would corrupt the JSON channel — silence it.
RNS.loglevel = RNS.LOG_NONE

def redirect_logs_to_stderr():
    # Any logging-module handlers (root or named) would also corrupt the JSON
    # channel — repoint every existing handler at stderr.
    for lg in [logging.getLogger()] + [logging.getLogger(n) for n in list(logging.root.manager.loggerDict)]:
        for h in list(lg.handlers):
            h.stream = sys.stderr

redirect_logs_to_stderr()
from nomadnet.ui import TextUI as T
from nomadnet.Directory import DirectoryEntry
from nomadnet.Conversation import Conversation

def glyph_dict(gs):
    g = {}; idx = T.GLYPHSETS[gs]
    for tup in T.GLYPHS: g[tup[0]] = tup[idx]
    return g
G = glyph_dict("unicode")

def widget_snippet():
    src = textwrap.dedent(inspect.getsource(C.ConversationsDisplay.conversation_list_widget))
    fn = ast.parse(src).body[0]
    body = []
    for s in fn.body:
        if isinstance(s, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "widget" for t in s.targets):
            break
        body.append(s)
    mod = ast.Module(body=body, type_ignores=[]); ast.fix_missing_locations(mod)
    return compile(mod, "<convrow>", "exec")
code = widget_snippet()

class Dir:
    def __init__(self, entries):
        self._e = {}
        for e in entries:
            try:
                self._e[bytes.fromhex(e["hash"])] = e
            except Exception:
                pass  # hostile names never make it into the directory
    def display_name(self, h):
        e = self._e.get(h)
        return e["name"] if e else None
    def trust_level(self, h, dn):
        e = self._e.get(h)
        return e["trust"] if e else DirectoryEntry.UNKNOWN
    def find(self, h):
        e = self._e.get(h)
        if e is None:
            return None
        return types.SimpleNamespace(sort_rank=(0 if e["pinned"] else None))

def build(root, entries):
    for e in entries:
        p = os.path.join(root, e["hash"])
        os.mkdir(p)
        for f in e.get("files") or []:
            fp = os.path.join(p, f["name"])
            with open(fp, "w") as fh:
                fh.write(f.get("content") or "")
            # Message files are deliberately NEWER than the directory: Python
            # takes the directory mtime, so the decoy must not matter.
            fmt = e["dirMtime"] + 10
            os.utime(fp, (fmt, fmt))
        for key in ("unread", "failed"):
            v = e.get(key)
            if v is not None:
                with open(os.path.join(p, key), "w") as fh:
                    fh.write(v)
        mt = e["dirMtime"]
        os.utime(p, (mt, mt))

collections = json.load(sys.stdin)
out = []
for coll in collections:
    root = tempfile.mkdtemp(dir="/tmp")
    try:
        entries = coll["convs"] or []
        build(root, entries)
        directory = Dir(entries)
        app = types.SimpleNamespace(conversationpath=root, directory=directory)
        convs = Conversation.conversation_list(app)

        # update_listbox's pinned-first stable partition
        # (ui/textui/Conversations.py:450-457), verbatim.
        def _is_pinned(c):
            try:
                entry = directory.find(bytes.fromhex(c[0]))
                return entry is not None and entry.sort_rank is not None
            except Exception:
                return False
        convs = sorted(convs, key=lambda c: 0 if _is_pinned(c) else 1)

        rows = []
        for c in convs:
            s = types.SimpleNamespace()
            s.app = types.SimpleNamespace(ui=types.SimpleNamespace(glyphs=G), directory=directory)
            s.currently_displayed_conversation = ""
            conv = (c[0], c[1], c[2], "", c[4], c[5], (c[6] if len(c) > 6 else 0))
            ns = {"__builtins__": __builtins__, "self": s, "conversation": conv,
                  "DirectoryEntry": DirectoryEntry, "relative_time": C.relative_time}
            exec(code, ns)
            markup = ns["markup"]
            main = "".join(m for m in markup if not (isinstance(m, str) and m.startswith("\n")))
            sec = ""
            for m in markup:
                if isinstance(m, str) and m.startswith("\n"):
                    sec = m[1:]; break
            rows.append({"hash": c[0], "unread": c[4], "failed": (c[6] if len(c) > 6 else 0),
                         "last": c[5], "label": C.relative_time(c[5]), "main": main, "secondary": sec})
        out.append({"name": coll["name"], "rows": rows})
    finally:
        shutil.rmtree(root, ignore_errors=True)
redirect_logs_to_stderr()
json.dump(out, sys.stdout)
`

// buildGoStore synthesizes the same tree for the Go side and returns the
// conversations root plus the lookup maps the app would pass to
// ConversationList.
func buildGoStore(t *testing.T, coll parityCollection) (string, map[string]string, map[string]byte, map[string]*int) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "storage", "conversations")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	trusts := map[string]byte{}
	pinned := map[string]*int{}
	for _, c := range coll.Convs {
		p := filepath.Join(path, c.Hash)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range c.Files {
			fp := filepath.Join(p, f.Name)
			if err := os.WriteFile(fp, []byte(f.Content), 0o644); err != nil {
				t.Fatal(err)
			}
			ft := mtimeOf(c.DirMtime + 10)
			if err := os.Chtimes(fp, ft, ft); err != nil {
				t.Fatal(err)
			}
		}
		for key, v := range map[string]*string{"unread": c.Unread, "failed": c.Failed} {
			if v != nil {
				if err := os.WriteFile(filepath.Join(p, key), []byte(*v), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		mt := mtimeOf(c.DirMtime)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		if c.Name != nil {
			names[c.Hash] = *c.Name
		}
		trusts[c.Hash] = c.Trust
		if c.Pinned {
			zero := 0
			pinned[c.Hash] = &zero
		}
	}
	return path, names, trusts, pinned
}

// mtimeOf converts a fixture unix-mtime float to a time.Time.
func mtimeOf(m float64) time.Time {
	sec := int64(math.Floor(m))
	nsec := int64(math.Round((m - math.Floor(m)) * 1e9))
	return time.Unix(sec, nsec)
}

// noonOf returns 12:00 local k days before the date of base — a calendar
// anchor that stays in the same calendar-day-diff bucket all day.
func noonOf(base time.Time, k int) time.Time {
	today := base.In(time.Local)
	mid := time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, time.Local)
	return mid.AddDate(0, 0, -k)
}

// unixOf converts a time to the fixture float form.
func unixOf(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

// runStoreParityCollections builds the synthesized-disk battery: empty stores,
// missing messages, every unread/failed file-content shape Python's int()
// accepts or rejects, directory-mtime-vs-message-mtime decoys, fractional
// mtimes, future mtimes, pinned ordering, hostile directory names, and epoch /
// far-future activity times.
func runStoreParityCollections() []parityCollection {
	base := time.Now()
	// minute-offset helper: k minutes + 30 s inside the minute bucket
	min := func(k int) float64 { return unixOf(base.Add(-time.Duration(k)*time.Minute - 30*time.Second)) }
	return []parityCollection{
		{
			Name: "empty store",
		},
		{
			Name: "single conversation, no messages",
			Convs: []parityConv{
				{Hash: "aaaaaaaaaaaa0001aaaaaaaaaaaa0001", DirMtime: min(30), Name: new("Alice"), Trust: 0xFF},
			},
		},
		{
			Name: "unread and failed file content shapes",
			Convs: []parityConv{
				{Hash: "b0000000000000010000000000000000", DirMtime: min(1), Name: new("EmptyUnread"), Trust: 0xFF, Unread: new("")},
				{Hash: "b0000000000000020000000000000000", DirMtime: min(2), Name: new("Three"), Trust: 0xFF, Unread: new("3")},
				{Hash: "b0000000000000030000000000000000", DirMtime: min(3), Name: new("Zero"), Trust: 0xFF, Unread: new("0")},
				{Hash: "b0000000000000040000000000000000", DirMtime: min(4), Name: new("NegUnread"), Trust: 0xFF, Unread: new("-2")},
				{Hash: "b0000000000000050000000000000000", DirMtime: min(5), Name: new("Garbage"), Trust: 0xFF, Unread: new("abc")},
				{Hash: "b0000000000000060000000000000000", DirMtime: min(6), Name: new("Padded"), Trust: 0xFF, Unread: new(" 7\n")},
				{Hash: "b0000000000000070000000000000000", DirMtime: min(7), Name: new("Underscore"), Trust: 0xFF, Unread: new("1_0")},
				{Hash: "b0000000000000080000000000000000", DirMtime: min(8), Name: new("TrailUnderscore"), Trust: 0xFF, Unread: new("1_")},
				{Hash: "b0000000000000090000000000000000", DirMtime: min(9), Name: new("LeadUnderscore"), Trust: 0xFF, Unread: new("_1")},
				{Hash: "b00000000000000a0000000000000000", DirMtime: min(10), Name: new("HexPrefix"), Trust: 0xFF, Unread: new("0x2")},
				{Hash: "b00000000000000b0000000000000000", DirMtime: min(11), Name: new("PlusSign"), Trust: 0xFF, Unread: new("+3")},
				{Hash: "b00000000000000c0000000000000000", DirMtime: min(12), Name: new("BothFlags"), Trust: 0xFF, Unread: new("4"), Failed: new("2")},
				{Hash: "b00000000000000d0000000000000000", DirMtime: min(13), Name: new("FailedOnly"), Trust: 0xFF, Failed: new("-1")},
				{Hash: "b00000000000000e0000000000000000", DirMtime: min(14), Name: new("FailedEmpty"), Trust: 0xFF, Failed: new("")},
			},
		},
		{
			Name: "directory mtime wins over newer message files",
			Convs: []parityConv{
				{Hash: "c0000000000000010000000000000000", DirMtime: min(15), Name: new("OldDirNewMsg"), Trust: 0xFF,
					Files: []parityFile{{Name: "d000000000000000000000000000000000000000000000000000000000000000", Content: "x"}}},
				{Hash: "c0000000000000020000000000000000", DirMtime: min(4), Name: new("NewerByMtime"), Trust: 0xFF},
			},
		},
		{
			Name: "directory index and state files never count as messages",
			Convs: []parityConv{
				{Hash: "d0000000000000010000000000000000", DirMtime: min(25), Name: new("OnlyStateFiles"), Trust: 0xFF,
					Files: []parityFile{
						{Name: ".index", Content: "\x80."},
						{Name: "read", Content: "read"},
					}},
			},
		},
		{
			Name: "fractional-second mtimes",
			Convs: []parityConv{
				{Hash: "e0000000000000010000000000000000", DirMtime: unixOf(base.Add(-305*time.Second - 500*time.Millisecond)), Name: new("FracA"), Trust: 0xFF},
				{Hash: "e0000000000000020000000000000000", DirMtime: unixOf(base.Add(-302*time.Second - 250*time.Millisecond)), Name: new("FracB"), Trust: 0xFF},
				{Hash: "e0000000000000030000000000000000", DirMtime: unixOf(base.Add(-309*time.Second - 750*time.Millisecond)), Name: new("FracC"), Trust: 0xFF},
			},
		},
		{
			Name: "future mtime reads just now",
			Convs: []parityConv{
				{Hash: "f0000000000000010000000000000000", DirMtime: unixOf(base.Add(time.Hour)), Name: new("Future"), Trust: 0xFF},
			},
		},
		{
			Name: "pinned conversations sort first, ordered by mtime inside the group",
			Convs: []parityConv{
				{Hash: "10000000000000010000000000000000", DirMtime: unixOf(base.AddDate(0, 0, -30)), Name: new("UnpinnedOldest"), Trust: 0xFF},
				{Hash: "10000000000000020000000000000000", DirMtime: min(4), Name: new("UnpinnedNewest"), Trust: 0xFF},
				{Hash: "10000000000000030000000000000000", DirMtime: unixOf(noonOf(base, 20)), Name: new("PinnedNewest"), Trust: 0xFF, Pinned: true},
				{Hash: "10000000000000040000000000000000", DirMtime: unixOf(noonOf(base, 40)), Name: new("PinnedOldest"), Trust: 0xFF, Pinned: true},
			},
		},
		{
			Name: "hostile directory names are skipped like Python skips them",
			Convs: []parityConv{
				{Hash: "20000000000000010000000000000000", DirMtime: min(5), Name: new("Legit"), Trust: 0xFF},
				// The three hostile names never reach the fake directory (both
				// implementations skip them inside the enumeration), so their
				// display/trust stubs are placeholders.
				{Hash: "zzzz000000000000zzzz000000000000", DirMtime: min(1), Name: new("SHOULD-NOT-APPEAR"), Trust: 0xFF},
				{Hash: "3000000000000001000000000000", DirMtime: min(1), Name: new("SHOULD-NOT-APPEAR"), Trust: 0xFF},
				{Hash: "4000000000000001000000000000000000000000000000000000000000000000", DirMtime: min(1), Name: new("SHOULD-NOT-APPEAR"), Trust: 0xFF},
				{Hash: "AAAAAAAAAAAA0001AAAAAAAAAAAA0001", DirMtime: min(3), Name: new("UpperHex"), Trust: 0xFF},
			},
		},
		{
			Name: "epoch mtime and year-2044 mtime",
			Convs: []parityConv{
				{Hash: "50000000000000010000000000000000", DirMtime: 0, Name: new("Epoch"), Trust: 0xFF},
				{Hash: "50000000000000020000000000000000", DirMtime: unixOf(time.Unix(2340000000, 0)), Name: new("FarFuture"), Trust: 0xFF},
			},
		},
		{
			Name: "junk sibling files are simply ignored",
			Convs: []parityConv{
				{Hash: "60000000000000010000000000000000", DirMtime: min(7), Name: new("JunkSib"), Trust: 0xFF,
					Files: []parityFile{{Name: "notes.txt", Content: "hello"}}},
			},
		},
	}
}

// runGoStoreSide renders the synthesized store through the Go pipeline exactly
// the way cmd/gonomadnet wires it: ConversationList → tui infos →
// SortConversations → row strings.
func runGoStoreSide(t *testing.T, coll parityCollection) []goRow {
	t.Helper()
	path, names, trusts, pinned := buildGoStore(t, coll)
	convs := conversation.ConversationList(path, names, trusts, pinned)
	infos := make([]ConversationInfo, len(convs))
	for i, c := range convs {
		trustStr := "unknown"
		switch c.TrustLevel {
		case 0xFF:
			trustStr = "trusted"
		case 0x01:
			trustStr = "untrusted"
		case 0x00:
			trustStr = "warning"
		}
		var lastTime time.Time
		if c.LastActivity > 0 {
			lastTime = time.Unix(int64(c.LastActivity), 0)
		}
		infos[i] = ConversationInfo{
			SourceHash:  c.SourceHash,
			DisplayName: c.DisplayName,
			TrustLevel:  trustStr,
			LastTime:    lastTime,
			Unread:      c.Unread,
			UnreadCount: c.UnreadCount,
			Failed:      c.Failed,
			FailedCount: c.FailedCount,
			Pinned:      c.SortRank != nil,
		}
	}
	SortConversations(infos, SortRecent)

	glyphs := glyphsUnicode
	rows := make([]goRow, len(infos))
	for i, info := range infos {
		rows[i] = goRow{
			Hash:      info.SourceHash,
			Unread:    info.UnreadCount,
			Failed:    info.FailedCount,
			Last:      float64(info.LastTime.Unix()),
			Secondary: conversationRowSecondary(info),
			Main:      conversationRowMain(info, glyphs, ""),
		}
	}
	return rows
}

func TestConversationStoreDiskParity(t *testing.T) {
	t.Parallel()
	testutils.SkipIfNoPythonNomadnet(t)
	testutils.SkipShortIntegration(t)

	collections := runStoreParityCollections()
	var want []struct {
		Name string  `json:"name"`
		Rows []pyRow `json:"rows"`
	}
	runPythonNomadnet(t, collections, conversationStoreParityScript, &want)

	if len(want) != len(collections) {
		t.Fatalf("python returned %v collections, want %v", len(want), len(collections))
	}

	rendered := make([][]goRow, len(collections))
	for i, coll := range collections {
		rendered[i] = runGoStoreSide(t, coll)
	}

	for ci, coll := range collections {
		t.Run(coll.Name, func(t *testing.T) {
			t.Parallel()
			got, py := rendered[ci], want[ci].Rows
			if len(got) != len(py) {
				t.Fatalf("Go listed %v conversations, Python listed %v\nGo:  %+v\nPy:  %+v", len(got), len(py), got, py)
			}
			for i := range got {
				g, p := got[i], py[i]
				if g.Hash != p.Hash {
					t.Fatalf("row %v: Go hash %v, Python hash %v (ordering diverged)\nGo:  %+v\nPy:  %+v", i, g.Hash, p.Hash, got, py)
				}
				if g.Unread != p.Unread || g.Failed != p.Failed {
					t.Errorf("%v: counts Go (unread=%v failed=%v), Python (unread=%v failed=%v)", g.Hash, g.Unread, g.Failed, p.Unread, p.Failed)
				}
				// An epoch-0 (or earlier) mtime means "no activity": Go models
				// that as the zero time (its app wiring guards LastActivity > 0)
				// while Python keeps 0 — both render an empty secondary row.
				if p.Last > 0 {
					if math.Abs(g.Last-p.Last) >= 1 {
						t.Errorf("%v: last activity Go %v, Python %v", g.Hash, g.Last, p.Last)
					}
					if gotLabel := RelativeTime(mtimeOf(p.Last)); gotLabel != p.Label {
						t.Errorf("%v: Go label %q, Python label %q", g.Hash, gotLabel, p.Label)
					}
				}
				if g.Main != p.Main {
					t.Errorf("%v: main row\nGo:  %q\nPy:  %q", g.Hash, g.Main, p.Main)
				}
				if g.Secondary != p.Secondary {
					t.Errorf("%v: secondary row\nGo:  %q\nPy:  %q", g.Hash, g.Secondary, p.Secondary)
				}
			}
		})
	}
}
