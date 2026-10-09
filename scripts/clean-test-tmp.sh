#!/usr/bin/env bash
# clean-test-tmp.sh — remove leftover test temp dirs/files from /tmp.
#
# The test suites create temp dirs via testutils.TempDir / TempDirMain, which
# register t.Cleanup / TestMain cleanup so a NORMAL run leaves nothing behind.
# The one historical deterministic leak — gorngit's group "<repoRoot>.allowed"
# sibling file written outside its TempDir — was fixed (it now registers an
# explicit t.Cleanup). Remaining /tmp garbage is therefore from KILLED runs
# (Ctrl-C, timeout, OOM, crash): when a test binary is killed, t.Cleanup never
# runs and its temp dirs stay in /tmp. This script reclaims that garbage.
#
# testutils also installs a SIGINT/SIGTERM handler that removes the dirs a run
# created before it exits (see testutils/cleanup.go), so an interrupted run no
# longer strands anything. This script is the backstop for what a handler
# cannot catch: SIGKILL, a `go test -timeout` panic, or os.Exit from an
# unrelated goroutine.
#
# Safety:
#   - Only removes entries whose name starts with one of the known TEST
#     prefixes below. It never touches unrelated /tmp content.
#   - Long-running user nodes (e.g. /tmp/gogit-manual, gonomadnet daemons) do
#     NOT match any of these prefixes and are never touched.
#   - A directory holding a .do-not-sweep file naming a LIVE PID is left alone
#     whatever its name. Tools that build under /tmp drop that marker in their
#     scratch dir, so a sweep cannot delete work in progress; a marker whose PID
#     is gone protects nothing, so a killed run's dir is still reclaimed.
#   - -n  dry run: list what would be removed, remove nothing.
#   - -m MINUTES  only remove entries older than MINUTES mtime (default: 0,
#     i.e. all). Use this to spare dirs from a currently-running test suite.
#   - -c  check: verify every temp-dir prefix the Go sources use is covered by
#     the list below, so it cannot rot silently. Removes nothing.
#   -L  live-safe: remove nothing while any test binary is running. Only
#     needed for an unguarded (-m 0) sweep, where there is no age guard to
#     spare the dirs a live run owns.
#
# Usage:
#   scripts/clean-test-tmp.sh            # remove all test garbage
#   scripts/clean-test-tmp.sh -n         # dry run (list only)
#   scripts/clean-test-tmp.sh -m 30      # remove only entries > 30 min old
#   scripts/clean-test-tmp.sh -m 0 -L    # only when no test binary is running
#   scripts/clean-test-tmp.sh -c         # check the prefix list for drift

set -euo pipefail

dry_run=0
min_minutes=0
check_only=0
live_safe=0
while getopts "nm:cL" opt; do
  case "$opt" in
    n) dry_run=1 ;;
    m) min_minutes="$OPTARG" ;;
    c) check_only=1 ;;
    L) live_safe=1 ;;
    *) echo "usage: $0 [-n] [-m MINUTES] [-c] [-L]" >&2; exit 2 ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(dirname "$script_dir")"

# Known TEST temp-dir/file prefixes from both repos' suites. This file is
# duplicated at ../go-nomadnet/scripts/clean-test-tmp.sh — keep the two copies
# IDENTICAL, because the repos share one /tmp and either suite's run should
# reclaim the other's residue too. `-c` verifies that this list covers the
# prefixes of whichever repo the copy lives in.
#
# NOTE: "gorrcbot-test-" is spelled out rather than shortened to "gorrcbot-",
# because an operator running the bot logs it to /tmp/gorrcbot-<epoch>.log and a
# bare prefix would delete a live service's log.
#
# NOTE: bare "gogit-" is intentionally NOT listed — the user's long-running
# manual node lives at /tmp/gogit-manual and must never be touched. Only the
# specific gogit-remote-rns test-suite prefixes are listed (none of them match
# gogit-manual). The same care applies to any bare prefix that could match a
# user process: list the specific test prefix, not the whole family. That is
# also why the loopback-/repeat-/probe- pairs below are listed by their full
# names rather than shortened to their common stem.
#
# NOTE: bare "gonomadnet-" and bare "gorns-" were both listed here once, and
# both deleted a live release build: they matched /tmp/gonomadnet-release-* and
# /tmp/gorns-release-*, the scratch dirs the release publishers build into. A
# concurrent test run's sweep removed those artifacts mid-publish, and because
# `go build -o` silently recreates a missing output directory, nothing failed
# until gh reported a missing asset. The families are now spelled out, and the
# dirs a tool owns are listed in not_swept_prefixes below, which -c enforces.
prefixes=(
  gorngit- rngit- gornx- gornsh- gornstatus- gorncp- gornodeconf-
  gornid- gornir- gornpath- gornpkg- gornprobe- gornsd-
  gornsh_py_wrapper_ missing-gornpath-binary
  gorrcd- rrcd- rrc- gorngcs- gorrcbot-test- gobot-test- grb-test-
  test-update-offline-data-
  gogit-clone- gogit-remote-rns- gogit-seed- gogit-reclone-
  golxmd-test- golxmd-filter
  pluginstore- prettysize-parity- prettyspeed-parity- testutils-signal-cleanup-
  rns-test-
  rns-discovery- rns-kd- rns-payload-sizes rns-local-client-hops
  rns-shared-serving rns-both-clients rns-ratchet-test-
  rns-auto-parity- rns-pipe-parity- rns-tcp-parity- rns-tcp-kiss-parity-
  rns-udp-parity- rns-serial-parity- rns-hashlist- rns-cache-clean-
  rns-clean-ratchet- rns-retain-clean- rns-void-queues- rns-persist-reentrant-
  rns-pathtable-midpersist- rns-preset-log- rns-pubtofile- rns-location-cmd-
  rns-entropy-test-
  lxmf-int- lxmf-peer- lxmf-tcp- py-interop-
  nomadnet-rrc- gonet-
  gonomadnet-test- gonomadnet-tc- gonomadnet-bench-config
  gonomadnet-tmux-test-suite- nomadnet-tmux-test-suite-
  gonomadnet-test-conversations- gonomadnet-input-box nomadnet-input-box
  nomadnet-app-test nomadnet-config-test nomadnet-node-
  nomadnet-conversation-test nomadnet-directory-test nomadnet-dir-persist
  nomadnet-int- nomadnet-lxmf-xproc- nomadnet-peersettings-test
  nomadnet-storage-test nomadnet-cbor- nomadnet-wasmpages-
  nomadnet-sent-test nomadnet-app-page-log nomadnet-browser-wasm
  nomadnet-tui-demo nomadnet-tui-form-wasm instance-lock-test-
  nomadnet-view-mu-
  loopback-C loopback-S repeat-C repeat-S probe-cfg- probe-rns-
  browser-cache-test- browser-download browser-fetch- browser-partial
  pipe-repeat-ts rns-local-parity- go-reticulum-large-py-to-go-
  go-reticulum- go-ret-local-
  probe_lxmf_store kiss-escape- rnstatus-parity-
  logger- logging- cbor-ximpl- expand-home- registry-interop-
  toml-interop- rooms-load- router-part-registry trust- pretty-date-parity-
  publish-android-
  pyfloat-parity- size-str-parity- speed-str-parity-
)

# Temp names `-c` recognizes but the sweep deliberately never touches. Two
# kinds live here:
#
#   - Created somewhere other than /tmp, so the sweep could not reach them even
#     if it tried: a server-side staging dir inside a repository's group
#     directory, and a Python subprocess's storage dir nested inside the test's
#     own temp dir (which removes it with its parent).
#
#   - Name FRAGMENTS appended to a caller's prefix, e.g.
#     TempDir(t, prefix+"initiator-"). The effective name always begins with
#     that caller's own prefix (gorngit-, gogit-remote-rns-), which IS swept
#     above; listing the fragment here just keeps -c from calling it new.
#
#   - Belonging to a long-running server rather than a test, so the sweep must
#     leave it alone even though -c sees the call site.
#
#   - A tool's own /tmp scratch or output path. These are a live run's working
#     state, not test residue: the release publishers build into
#     publish-release-<repo>-*, tooling/heap-dump.sh captures profiles into
#     gonomadnet-pprof-<ts>/, scripts/run-bench.sh tees results into
#     gonomadnet-bench-latest-<ts>.txt, tooling/sweep.sh builds
#     gonomadnet-sweep-bin, and a bench test writes gonomadnet-bench-config.
#     Listing them here is what keeps -c honest: a swept prefix that matched any
#     of them would make this script delete live state, and -c fails on exactly
#     that contradiction.
#
# "gobot-" is the gobot CLI's own scratch history directory. It is created with
# os.MkdirTemp("", ...), so it lives under $TMPDIR rather than /tmp, and a bare
# prefix is exactly the kind that could otherwise match a user's own /tmp file.
not_swept_prefixes=(
  .gorngit-clone- ratchet-encrypt- gobot-
  initiator- listener- src-
  serve-page-rns-
  publish-release- go-cache gogit-manual gorrcbot-
  gonomadnet-pprof- gonomadnet-bench-latest- gonomadnet-sweep-bin
)

# temp_prefixes_in_sources prints every temp-dir prefix the Go sources in this
# repo pass to a temp-dir constructor.
#
# The prefix is the first string argument of a TempDir/TempDirBench/
# TempDirMain/TempDirWithConfig call — first rather than last, because the call
# is often nested inside another one:
#
#   filepath.Join(testutils.TempDir(t, "golxmd-filter"), "plugin.wasm")
#
# os.MkdirTemp takes the base directory first, so there the prefix is the
# SECOND string argument (os.MkdirTemp("/tmp", "kiss-escape-")).
#
# os.CreateTemp is deliberately not scanned: its pattern's leading "*" is
# replaced by random characters, so its name has no prefix a sweeper could
# match (and it uses $TMPDIR, not /tmp).
#
# Two blind spots, both quiet misses rather than false alarms: a call whose
# arguments span several lines, and a prefix passed as a variable rather than a
# literal. Every call site in this repo is a single-line literal today.
temp_prefixes_in_sources() {
  local go_files
  go_files=$(grep -rl --include='*.go' -E 'TempDir[A-Za-z]*\(|os\.MkdirTemp\(' "$repo_root" || true)
  [ -n "$go_files" ] || return 0

  # shellcheck disable=SC2086
  awk '
    # Comment-only lines are skipped: several mention os.MkdirTemp("") to
    # explain why they do NOT use it.
    /^[[:space:]]*\/\// { next }
    {
      if (!match($0, /(TempDir[A-Za-z]*|os\.MkdirTemp)\(/)) { next }
      mkdirTemp = (substr($0, RSTART, RLENGTH) ~ /^os\.MkdirTemp/)
      # Keep only this call own argument list: cutting at the first ")" drops
      # whatever follows the call, so the quotes of an enclosing call are not
      # mistaken for the prefix —
      #   filepath.Join(t.TempDir(), "rns-storage")
      #   filepath.Join(testutils.TempDir(t, "golxmd-filter"), "plugin.wasm")
      # The prefix is the first argument, so nothing nested can precede it.
      # Go own t.TempDir() has an empty list and yields nothing here.
      rest = substr($0, RSTART + RLENGTH)
      cut = index(rest, ")")
      if (cut > 0) { rest = substr(rest, 1, cut - 1) }
      if (split(rest, parts, "\"") < 2) { next }
      # Splitting on a one-character separator alternates outside/inside, so
      # the even-indexed fields are the quoted strings in argument order.
      print mkdirTemp ? parts[4] : parts[2]
    }
  ' $go_files | grep -v '^$' | sort -u
}

if [ "$check_only" -eq 1 ]; then
  uncovered=0
  while IFS= read -r found; do
    [ -n "$found" ] || continue
    covered=0
    for p in "${prefixes[@]}" "${not_swept_prefixes[@]}"; do
      case "$found" in
        "$p"*) covered=1; break ;;
      esac
    done
    if [ "$covered" -eq 0 ]; then
      echo "NOT COVERED: temp prefix '$found' is used by this repo's sources but" >&2
      echo "             matches no entry in the prefixes list above." >&2
      uncovered=$((uncovered + 1))
    fi
  done < <(temp_prefixes_in_sources)

  # A not_swept_prefixes entry that a swept prefix also matches is a
  # contradiction the sweep resolves the wrong way: the name is documented as
  # "the sweep deliberately never touches this", and yet a live sweep would
  # delete it. Two bare family prefixes once did exactly that to the release
  # publishers' build dirs, so the check now fails on it instead of trusting the
  # two lists to stay apart by hand.
  contradictions=0
  for n in "${not_swept_prefixes[@]}"; do
    for p in "${prefixes[@]}"; do
      case "$n" in
        "$p"*)
          echo "CONTRADICTION: not_swept_prefixes entry '$n' is matched by swept" >&2
          echo "              prefix '$p', so a sweep would delete it. Narrow '$p'" >&2
          echo "              or drop the entry." >&2
          contradictions=$((contradictions + 1))
          ;;
      esac
    done
  done

  if [ "$uncovered" -gt 0 ] || [ "$contradictions" -gt 0 ]; then
    echo "check-test-tmp: $uncovered temp prefix(es) missing from the list," \
      "$contradictions not-swept entry/entries a swept prefix would delete" >&2
    exit 1
  fi
  echo "check-test-tmp: every temp prefix is covered and no swept prefix reaches a not-swept name"
  exit 0
fi

# test_binary_alive reports whether a Go test binary is running.
#
# A live test process owns the temp dirs it created. An -m 0 sweep carries no
# age guard, so it would delete those dirs out from under it and fail a run for
# reasons that look nothing like a cleanup problem — a concurrent `go test` in
# another terminal, or another agent's run in this repo. -L therefore skips the
# sweep entirely while one is alive. Whatever such a run leaves behind is
# reclaimed by the next run's age-guarded sweep, once it is old enough.
test_binary_alive() {
  if ! command -v pgrep >/dev/null 2>&1; then
    # Without pgrep there is no way to tell, and sweeping blind is the riskier
    # choice, so report a live run and let the age guard do the work instead.
    return 0
  fi
  if pgrep -f '[.]test' >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

if [ "$live_safe" -eq 1 ] && test_binary_alive; then
  echo "sweep skipped: a test binary is running (-L)"
  exit 0
fi

# mtime_in_seconds prints the modification time of a path as epoch seconds.
# macOS stat and GNU stat disagree on the flag, and neither is guaranteed, so
# fall back to date(1).
mtime_in_seconds() {
  stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || date -r "$1" +%s 2>/dev/null || echo 0
}

# in_use_marker is the file a tool writing a /tmp scratch dir drops inside it.
# The file names the tool's PID, and in_use decides on that PID being alive, so
# a marker left behind by a killed run protects nothing.
in_use_marker=".do-not-sweep"

# in_use reports whether a live process claims a temp entry, which is the one
# signal that outranks the name: a name is only ever a guess at what a directory
# belongs to, and the guess is what deleted a running release build. A tool that
# wants a guarantee it will not be swept mid-run leaves the marker.
in_use() { # $1 = path
  local marker="$1/${in_use_marker}" pid
  [ -f "$marker" ] || return 1
  pid=$(tr -dc '0-9' <"$marker")
  [ -n "$pid" ] || return 1
  kill -0 "$pid" 2>/dev/null
}

removed=0
for p in "${prefixes[@]}"; do
  for f in /tmp/${p}*; do
    [ -e "$f" ] || [ -L "$f" ] || continue
    if in_use "$f"; then
      [ "$dry_run" -eq 1 ] && echo "DRY-RUN keep (in use) $f"
      continue
    fi
    if [ "$min_minutes" -gt 0 ]; then
      # Skip entries modified within the last min_minutes minutes.
      mtime=$(mtime_in_seconds "$f")
      age_min=$(( ($(date +%s) - mtime) / 60 ))
      [ "$age_min" -lt "$min_minutes" ] && continue
    fi
    if [ "$dry_run" -eq 1 ]; then
      echo "DRY-RUN rm -rf $f"
    else
      rm -rf "$f"
    fi
    removed=$((removed + 1))
  done
done

# A couple of exact-name files that some suites drop in /tmp.
for f in /tmp/reticulum-phase-files.txt; do
  [ -e "$f" ] || continue
  if [ "$dry_run" -eq 1 ]; then echo "DRY-RUN rm -rf $f"; else rm -rf "$f"; fi
  removed=$((removed + 1))
done

if [ "$dry_run" -eq 1 ]; then
  echo "dry run: $removed entries would be removed"
else
  echo "removed $removed test entries from /tmp"
fi
