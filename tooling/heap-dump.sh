#!/usr/bin/env bash
# heap-dump.sh — capture gonomadnet runtime memory/goroutine profiles to text
# files for offline analysis (no web browser required).
#
# Run this on a host that can reach the running gonomadnet pprof endpoint
# (i.e. gonomadnet was started with -pprof-addr), ideally while the node
# actually feels bloated/slow — a profile captured at startup won't show
# cumulative growth. For best results capture TWICE and diff: once shortly
# after start, once again after hours of use/browsing.
#
#   gonomadnet -textui -pprof-addr 127.0.0.1:6060
#   ./tooling/heap-dump.sh                            # default addr, auto-find PID
#   ./tooling/heap-dump.sh 127.0.0.1:6060 12345       # explicit addr + PID
#
# Output lands in /tmp/gonomadnet-pprof-<timestamp>/ . Requires curl and
# `go tool pprof` on the capturing host (Go need not be on the node itself if
# the pprof port is reachable over the network/SSH tunnel).
set -euo pipefail

ADDR="${1:-127.0.0.1:6060}"
PID="${2:-}"
TS="$(date +%Y%m%d-%H%M%S)"
OUT="/tmp/gonomadnet-pprof-${TS}"
PP="http://${ADDR}/debug/pprof"

mkdir -p "$OUT"
echo "Capturing to $OUT  (pprof at $PP)"

# Process-level RSS/VmSize/threads. This is the key column for separating
# "live heap is small but RSS is huge" (GC headroom + lazy scavenge) from a
# true live-heap leak — the heap profiles below show the live heap; this shows
# what top/free actually see.
if [[ -z "$PID" ]]; then
  PID="$(pgrep -x gonomadnet | head -1 || true)"
fi
if [[ -n "$PID" && -r "/proc/$PID/status" ]]; then
  grep -E '^(Name|VmRSS|VmSize|VmData|Threads):' "/proc/$PID/status" > "$OUT/process-memory.txt"
  echo "  PID $PID RSS: $(awk '/VmRSS/{print $2" "$3}' "$OUT/process-memory.txt")"
elif [[ -n "$PID" ]]; then
  ps -o pid,rss,vsz,comm -p "$PID" > "$OUT/process-memory.txt" 2>/dev/null || true
fi

# Heap: in-use BYTES (what is retained right now).
go tool pprof -top -nodecount=60   "$PP/heap" > "$OUT/heap-inuse-top.txt"
go tool pprof -tree -nodecount=100 "$PP/heap" > "$OUT/heap-inuse-tree.txt"
# Heap: in-use OBJECT counts — catches many-small-object leaks (leaked
# goroutines, map entries, *Resource / *PacketReceipt structs) that can look
# small in bytes.
go tool pprof -top -inuse_objects -nodecount=60 "$PP/heap" > "$OUT/heap-inuse-objects.txt"

# Heap: allocation churn over a 30s window (what is being allocated fast).
#   high churn + low in-use = GC-able (draw path).  high churn + high in-use = a leak.
echo "  sampling alloc churn for 30s (blocks until done)..."
go tool pprof -top -alloc_space   -nodecount=60 "$PP/heap?seconds=30" > "$OUT/heap-alloc-30s-top.txt"
go tool pprof -top -alloc_objects -nodecount=60 "$PP/heap?seconds=30" > "$OUT/heap-alloc-30s-objs.txt"

# Goroutines: count + where each is blocked. The line to look for when
# diagnosing QueueUpdateDraw goroutine accumulation is a stack through
# tview.(*Application).QueueUpdateDraw -> runtime.chansend.
curl -s "$PP/goroutine?debug=1" > "$OUT/goroutine-summary.txt"
curl -s "$PP/goroutine?debug=2" > "$OUT/goroutine-traces.txt"

# Raw profiles for later interactive browsing:  go tool pprof <file>.pb.gz
curl -s -o "$OUT/heap.pb.gz"      "$PP/heap"
curl -s -o "$OUT/goroutine.pb.gz" "$PP/goroutine"

echo
echo "Done. Quick peek:"
echo "  goroutine total : $(grep -m1 '^goroutine profile:' "$OUT/goroutine-summary.txt")"
[[ -f "$OUT/process-memory.txt" ]] && echo "  process memory  : $OUT/process-memory.txt"
echo "  heap in-use top : $OUT/heap-inuse-top.txt"
echo "  heap in-use objs: $OUT/heap-inuse-objects.txt"
echo "  goroutine summary: $OUT/goroutine-summary.txt"
echo
echo "Compare two runs:  diff -ru <(ls <early-dir>) <(ls <late-dir>)  and diff the matching .txt files."