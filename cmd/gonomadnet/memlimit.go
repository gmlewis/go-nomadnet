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

package main

// Memory tuning for long-running nodes on small devices.
//
// A gonomadnet process on a memory-constrained host (e.g. a Jetson Nano with
// 2 GiB RAM) can grow its RSS until it brings the device to its knees. Live-heap
// profiling shows the Go heap itself stays modest (~single-digit MB) while
// allocation churn (the terminal draw path, per-announce known-destination
// re-saves) is high; with default GC settings (GOGC=100, no soft limit) the
// runtime's RSS watermark floats well above the live heap and Go is slow to
// return freed spans to the OS. Setting a soft memory limit caps that watermark
// regardless of churn, without touching the (separate, cumulative) leak sources
// in go-reticulum.
//
// To avoid being unnecessarily restrictive on powerful machines, the soft limit
// is ONLY auto-applied when the host reports less than 4 GiB of RAM. On a
// desktop/laptop with plenty of memory, applyMemoryTuning is a no-op and Go
// runs with its defaults. Both knobs are overridable via environment variables:
//
//	GONOMADNET_MEMLIMIT  soft heap limit in MiB (e.g. 512). 0 disables the
//	                     limit (sets it to math.MaxInt64).
//	GONOMADNET_GOGC      GC percent (default 100). Lower (e.g. 50) collects
//	                     sooner at the cost of more GC CPU; raise (e.g. 200) to
//	                     reduce GC CPU when a soft limit is already set.
//
// Forcing prompt return of freed memory to the OS additionally requires the
// runtime GODEBUG flag, which is read at process start and so cannot be set
// reliably from Go code — launch with:
//
//	GODEBUG=madvdontneed=1 gonomadnet -textui ...

import (
	"log"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// smallDeviceThreshold is the total-RAM cutoff below which a soft heap limit is
// auto-applied. 4 GiB: a 2 GiB Jetson qualifies; a typical desktop does not.
const smallDeviceThreshold int64 = 4 << 30

// applyMemoryTuning configures the Go runtime's soft memory limit and (optionally)
// GC percent. It is safe to call once at startup; on large hosts it does nothing.
func applyMemoryTuning() {
	// 1) Soft memory limit.
	if v, ok := os.LookupEnv("GONOMADNET_MEMLIMIT"); ok {
		mb, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			if mb > 0 {
				debug.SetMemoryLimit(int64(mb) << 20)
				log.Printf("memlimit: GONOMADNET_MEMLIMIT=%vMB -> soft heap limit %vMB", mb, mb)
			} else {
				debug.SetMemoryLimit(math.MaxInt64)
				log.Printf("memlimit: GONOMADNET_MEMLIMIT=0 -> soft heap limit disabled")
			}
		}
	} else if total := totalMemoryBytes(); total > 0 && total < smallDeviceThreshold {
		// 25% of total RAM, floored at 256 MiB so very small boards still get a
		// workable ceiling. SetMemoryLimit is soft: if live heap ever exceeds
		// it, the runtime lets the heap grow rather than OOM-killing the process.
		limit := max(total/4, 256<<20)
		debug.SetMemoryLimit(limit)
		log.Printf("memlimit: small device (%vMB total) -> auto soft heap limit %vMB (override with GONOMADNET_MEMLIMIT=MB; 0 disables)", total>>20, limit>>20)
	}

	// 2) GC percent. Left at the Go default (100) unless overridden: a soft
	// limit already bounds the heap, and lowering GOGC raises GC frequency and
	// CPU, which worsens draw sluggishness on a CPU-bound terminal. Set
	// GONOMADNET_GOGC=50 to trade CPU for a lower RSS watermark.
	if v, ok := os.LookupEnv("GONOMADNET_GOGC"); ok {
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			debug.SetGCPercent(p)
			log.Printf("memlimit: GONOMADNET_GOGC=%v -> GC percent %v", p, p)
		}
	}
}

// totalMemoryBytes returns the host's total physical RAM in bytes, or 0 if it
// cannot be determined (e.g. non-Linux). Only /proc/meminfo is consulted, so on
// macOS/dev machines this returns 0 and applyMemoryTuning leaves Go defaults in
// place — exactly the desired "don't restrict powerful machines" behavior.
func totalMemoryBytes() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return kb * 1024
			}
		}
		break
	}
	return 0
}
