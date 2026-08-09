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

// Command ping-nomadnet-node checks the reachability of LXMF (nomadnet) nodes
// from the local Reticulum instance.
//
// Each command-line argument is either an LXMF address or the path to a file of
// addresses (one per line):
//
//	go run ./cmd/ping-nomadnet-node 6853937554960093a05764c3974f28e6
//	go run ./cmd/ping-nomadnet-node lxm:6853937554960093a05764c3974f28e6
//	go run ./cmd/ping-nomadnet-node known-nodes.txt
//	go run ./cmd/ping-nomadnet-node node1.txt 6853937554960093a05764c3974f28e6 node2.txt
//
// An LXMF address is the 16-byte (32 hex char) truncated destination hash of a
// node's "lxmf.delivery" destination. It may be given bare (just the hex) or
// with any of the common nomadnet prefixes — lxm:, lxmf:, lxmf@, lxmf://lxmf@,
// lxmf://, node://, nomadnetwork:// — which are stripped before parsing.
//
// A file argument is read line by line. For each line, the start of the line is
// examined: if it begins with an LXMF address the address is taken (the rest of
// the line is kept as a human-readable label); otherwise the whole line is
// skipped. This lets known-nodes.txt mix address lines with comments and
// headers, e.g.:
//
//	# My nodes
//	6853937554960093a05764c3974f28e6  living-room node
//	lxm:bc37348ec27fafad10f3fd2e92ecf5f5  garage
//
// For each address the tool performs a staged reachability check and reports:
//
//  1. Path: whether a transport path to the destination is known or can be
//     requested within --path-timeout. This needs only the hash (it is a
//     transport-level probe) and is the most fundamental "can I reach it" test.
//  2. Identity: whether the peer's identity has been heard via an announce
//     (RecallIdentity). Without it an outbound lxmf.delivery link cannot be
//     built, so a node that has stopped announcing shows up here.
//  3. Link: when both a path and an identity are available, an outbound
//     lxmf.delivery link is opened; establishment reports a real "pong" with
//     round-trip time and hop count.
//
// Flags:
//
//	--rnsconfig DIR    Reticulum config dir (default: ~/.reticulum)
//	--path-timeout D   seconds to wait for a path request (default 15)
//	--link-timeout D   seconds to wait for link establishment (default 15)
//	--identity FILE    ping the node whose identity is in FILE directly
//	-v                 verbose RNS logging
//	-h, --help         show help and exit
package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

const (
	defaultPathTimeout = 15.0
	defaultLinkTimeout = 15.0
	// pollInterval is the granularity at which path/link state is polled.
	pollInterval = 100 * time.Millisecond
	// startupGrace gives AutoInterface discovery a moment before probing.
	startupGrace = 1500 * time.Millisecond
)

// addressPrefixes are stripped from the start of an address token before the
// 32-hex hash is extracted. They cover bare hashes plus the common nomadnet
// address spellings (lxm:, lxmf:, lxmf@, and the clickable scheme wrappers).
var addressPrefixes = []string{
	"lxmf://lxmf@",
	"nomadnetwork://",
	"lxmf://",
	"node://",
	"lxmf:",
	"lxmf@",
	"lxm:",
}

type options struct {
	rnsConfigDir string
	pathTimeout  float64
	linkTimeout  float64
	verbose      bool
	identityFile string // optional: load target identity from file (skip announce wait)
	args         []string
}

type target struct {
	hashHex  string
	label    string
	src      string         // origin (arg or file:line) for diagnostics
	identity *rns.Identity  // when set (from --identity), used directly instead of RecallIdentity
}

type result struct {
	target
	hash        []byte
	pathKnown   bool   // path was already cached before the run
	pathResolved bool  // path is known now (cached or freshly resolved)
	pathFresh   bool   // path had to be requested and resolved
	hops        int    // HopsTo at report time (PathfinderM if unknown)
	identity    bool   // identity was recalled from an announce
	linkUp      bool   // outbound lxmf.delivery link established
	rtt         time.Duration
	status      string // UNREACHABLE / NO ANNOUNCE / LINK FAILED / ONLINE
	detail      string // extra context (errors, times)
}

func main() {
	log.SetFlags(0)
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, err := parseFlags(args)
	if err != nil {
		if err == errHelp {
			fmt.Print(usageText)
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}

	targets, err := collectTargets(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no LXMF addresses to ping")
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}

	logger := rns.NewLogger()
	level := rns.LogError
	if opts.verbose {
		level = rns.LogInfo
	}
	logger.SetLogLevel(level)

	ts := rns.NewTransportSystem(logger)
	ret, err := rns.NewReticulumWithLogger(ts, opts.rnsConfigDir, logger)
	if err != nil {
		log.Fatalf("Could not initialize Reticulum: %v", err)
	}
	defer func() {
		if err := ret.Close(); err != nil {
			logger.Warning("Could not close Reticulum properly: %v", err)
		}
	}()

	fmt.Printf("ping-nomadnet-node — go-reticulum %v\n", rns.VERSION)
	if dir := ret.ConfigDir(); dir != "" {
		fmt.Printf("RNS config: %s\n", dir)
	}
	fmt.Printf("Pinging %d node(s) — path timeout %.0fs, link timeout %.0fs\n",
		len(targets), opts.pathTimeout, opts.linkTimeout)

	printInterfaces(ret)

	// Give AutoInterface discovery a moment to bring interfaces up before we
	// start issuing path requests.
	time.Sleep(startupGrace)

	results := pingAll(ts, targets, opts)

	printReport(results)
	return reportExitCode(results)
}

// pingAll pings every target concurrently and returns results in input order.
func pingAll(ts *rns.TransportSystem, targets []target, opts *options) []result {
	results := make([]result, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t target) {
			defer wg.Done()
			results[i] = pingOne(ts, t, opts)
		}(i, t)
	}
	wg.Wait()
	return results
}

// pingOne performs the staged reachability check for a single target.
func pingOne(ts *rns.TransportSystem, t target, opts *options) result {
	r := result{target: t, hops: -1}

	hash, err := hex.DecodeString(t.hashHex)
	if err != nil || len(hash) != rns.TruncatedHashLength/8 {
		r.status = "INVALID"
		r.detail = fmt.Sprintf("not a %d-char hex hash: %q", rns.TruncatedHashLength/4, t.hashHex)
		return r
	}
	r.hash = hash

	// Stage 1: path. A path is needed for any further link work, and a path
	// request is itself a transport-level reachability probe.
	r.pathKnown = ts.HasPath(hash)
	if r.pathKnown {
		r.pathResolved = true
	} else {
		if err := ts.RequestPath(hash); err != nil {
			r.detail = fmt.Sprintf("path request failed: %v", err)
		}
		deadline := time.Now().Add(time.Duration(opts.pathTimeout * float64(time.Second)))
		for time.Now().Before(deadline) && !ts.HasPath(hash) {
			time.Sleep(pollInterval)
		}
		r.pathResolved = ts.HasPath(hash)
		r.pathFresh = r.pathResolved
	}
	if r.pathResolved {
		r.hops = ts.HopsTo(hash)
	}

	// Stage 2: identity. The peer's public key must have been heard via an
	// announce before an outbound SINGLE destination can be built. When the
	// identity was supplied via --identity, use it directly (no announce wait).
	identity := t.identity
	if identity == nil {
		identity = rns.RecallIdentity(ts, hash)
	}
	r.identity = identity != nil

	// Stage 3: link. Only attempt when both a path and an identity exist; a
	// SINGLE destination cannot be constructed without the recalled identity.
	switch {
	case !r.pathResolved:
		r.status = "UNREACHABLE"
		if r.detail == "" {
			r.detail = "no transport path within timeout (node not announcing / interfaces down)"
		}
		return r
	case !r.identity:
		r.status = "NO ANNOUNCE"
		if r.detail == "" {
			r.detail = "path known but identity never heard (no announce received from this node)"
		}
		return r
	}

	dest, err := rns.NewDestination(ts, identity, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("could not build lxmf.delivery destination: %v", err)
		return r
	}
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("could not create link: %v", err)
		return r
	}

	established := make(chan struct{})
	var closeOnce sync.Once
	closed := make(chan struct{})
	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		close(established)
		// Tear the link down once the pong is recorded (mirrors nomadnet's
		// PingPeer). Run on a goroutine so the RNS worker is not blocked.
		go func() {
			defer func() { _ = recover() }()
			l.Teardown()
		}()
	})
	link.SetLinkClosedCallback(func(l *rns.Link) {
		// If the link activated, the established callback already recorded the
		// pong and the subsequent teardown-close is a no-op.
		select {
		case <-established:
			return
		default:
		}
		closeOnce.Do(func() { close(closed) })
	})

	start := time.Now()
	if err := link.Establish(); err != nil {
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("link handshake did not start: %v", err)
		return r
	}

	linkDeadline := time.Duration(opts.linkTimeout * float64(time.Second))
	select {
	case <-established:
		r.linkUp = true
		r.rtt = time.Since(start)
		r.hops = ts.HopsTo(hash)
		r.status = "ONLINE"
		r.detail = fmt.Sprintf("path %s", pathSource(r))
		return r
	case <-closed:
		r.status = "LINK FAILED"
		r.detail = "link closed without establishing (peer unreachable or refused)"
		return r
	case <-time.After(linkDeadline):
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("link establishment timed out after %.0fs", opts.linkTimeout)
		return r
	}
}

// pathSource describes how the path was obtained for the detail line.
func pathSource(r result) string {
	switch {
	case r.pathFresh:
		return "freshly resolved"
	case r.pathKnown:
		return "cached (stale?)"
	default:
		return "unknown"
	}
}

func printInterfaces(ret *rns.Reticulum) {
	snap, err := ret.InterfaceStats()
	if err != nil || snap == nil {
		fmt.Println("Interfaces: (unavailable)")
		return
	}
	if len(snap.Interfaces) == 0 {
		fmt.Println("Interfaces: none — check ~/.reticulum/config (no interfaces means no peers can be reached)")
		return
	}
	fmt.Println("Interfaces:")
	for _, ifc := range snap.Interfaces {
		state := "DOWN"
		if ifc.Status {
			state = "UP"
		}
		fmt.Printf("  %-16s %-14s %s  %d bps\n", ifc.Name, ifc.Type, state, ifc.Bitrate)
	}
}

func printReport(results []result) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 78))
	fmt.Println("RESULTS")
	fmt.Println(strings.Repeat("=", 78))
	for _, r := range results {
		label := r.label
		if label == "" {
			label = "(no label)"
		}
		fmt.Printf("\n%s  %s\n", r.hashHex, label)
		fmt.Printf("  status : %s\n", r.status)
		if r.hops >= 0 && r.hops < rns.PathfinderM {
			fmt.Printf("  hops   : %d\n", r.hops)
		} else if r.hops >= 0 {
			fmt.Printf("  hops   : ?\n")
		}
		if r.linkUp {
			fmt.Printf("  rtt    : %d ms\n", r.rtt.Milliseconds())
		}
		if r.detail != "" {
			fmt.Printf("  note   : %s\n", r.detail)
		}
	}

	// Compact summary line per node.
	fmt.Println()
	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("%-34s  %-12s  %s\n", "ADDRESS", "STATUS", "LABEL")
	for _, r := range results {
		fmt.Printf("%-34s  %-12s  %s\n", r.hashHex, r.status, r.label)
	}
}

// reportExitCode is 0 only if every target is ONLINE; otherwise 1 so the tool is
// usable as a connectivity check in scripts.
func reportExitCode(results []result) int {
	for _, r := range results {
		if r.status != "ONLINE" {
			return 1
		}
	}
	return 0
}

// collectTargets turns the raw arguments into a flat list of ping targets,
// classifying each arg as an LXMF address or a filename. When opts.identityFile
// is set, a target is added from that identity file (its lxmf.delivery hash is
// computed directly, so no announce needs to be heard).
func collectTargets(opts *options) ([]target, error) {
	var out []target
	if opts.identityFile != "" {
		id, err := rns.FromFile(opts.identityFile, nil)
		if err != nil {
			return nil, fmt.Errorf("--identity %s: %w", opts.identityFile, err)
		}
		hash := rns.CalculateHash(id, "lxmf", "delivery")
		out = append(out, target{
			hashHex:  hex.EncodeToString(hash),
			label:   fmt.Sprintf("(identity %s)", opts.identityFile),
			src:     opts.identityFile,
			identity: id,
		})
	}
	for _, arg := range opts.args {
		if hexStr, ok := parseArgAsAddress(arg); ok {
			out = append(out, target{hashHex: hexStr, src: arg})
			continue
		}
		// Treat as a filename.
		fileTargets, err := targetsFromFile(arg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", arg, err)
		}
		out = append(out, fileTargets...)
	}
	return out, nil
}

// targetsFromFile parses a file of addresses, one per line. A line contributes
// a target only if it begins with an LXMF address; the remainder of the line is
// kept as the label.
func targetsFromFile(path string) ([]target, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []target
	for lineNo, raw := range strings.Split(string(data), "\n") {
		hashHex, label, ok := extractAddressFromLine(raw)
		if !ok {
			continue
		}
		out = append(out, target{
			hashHex: hashHex,
			label:   label,
			src:     fmt.Sprintf("%s:%d", path, lineNo+1),
		})
	}
	return out, nil
}

// parseArgAsAddress reports whether arg is itself a standalone LXMF address
// (an address token with no trailing label). A bare 32-hex hash or any
// prefixed form qualifies.
func parseArgAsAddress(arg string) (string, bool) {
	hexStr, rest, ok := extractLeadingAddress(arg)
	if !ok {
		return "", false
	}
	// A standalone address arg has nothing but the address in it.
	return hexStr, strings.TrimSpace(rest) == ""
}

// extractAddressFromLine extracts a leading LXMF address from a line and
// returns the hex plus the trailing label text.
func extractAddressFromLine(line string) (string, string, bool) {
	return extractLeadingAddress(line)
}

// extractLeadingAddress strips known address prefixes from s, then pulls the
// leading 32-hex run. It returns the hex, the remainder of s, and ok.
func extractLeadingAddress(s string) (hexStr, rest string, ok bool) {
	t := strings.TrimSpace(s)
	for _, p := range addressPrefixes {
		if len(t) > len(p) && strings.EqualFold(t[:len(p)], p) {
			t = t[len(p):]
			break
		}
	}
	// Pull the leading hex run (the hash). Anything non-hex ends it.
	end := 0
	for end < len(t) {
		c := t[end]
		if !isHexByte(c) {
			break
		}
		end++
	}
	run := t[:end]
	if len(run) != rns.TruncatedHashLength/4 {
		return "", "", false
	}
	label := strings.TrimLeft(t[end:], " \t,:;|")
	return strings.ToLower(run), label, true
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

var errHelp = fmt.Errorf("help requested")

func parseFlags(args []string) (*options, error) {
	opts := &options{pathTimeout: defaultPathTimeout, linkTimeout: defaultLinkTimeout}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			return nil, errHelp
		case "-v", "--verbose":
			opts.verbose = true
		case "--rnsconfig":
			v, next, err := flagValue(args, i, "--rnsconfig")
			if err != nil {
				return nil, err
			}
			opts.rnsConfigDir = v
			i = next
		case "--path-timeout":
			v, next, err := flagValue(args, i, "--path-timeout")
			if err != nil {
				return nil, err
			}
			f, err := parseFloat(v)
			if err != nil {
				return nil, fmt.Errorf("--path-timeout: %w", err)
			}
			opts.pathTimeout = f
			i = next
		case "--link-timeout":
			v, next, err := flagValue(args, i, "--link-timeout")
			if err != nil {
				return nil, err
			}
			f, err := parseFloat(v)
			if err != nil {
				return nil, fmt.Errorf("--link-timeout: %w", err)
			}
			opts.linkTimeout = f
			i = next
		case "--identity":
			v, next, err := flagValue(args, i, "--identity")
			if err != nil {
				return nil, err
			}
			opts.identityFile = v
			i = next
		default:
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return nil, fmt.Errorf("flag provided but not defined: %s", arg)
			}
			opts.args = append(opts.args, arg)
		}
	}
	return opts, nil
}

func flagValue(args []string, i int, name string) (string, int, error) {
	if i+1 >= len(args) {
		return "", 0, fmt.Errorf("flag needs an argument: %s", name)
	}
	return args[i+1], i + 1, nil
}

func parseFloat(s string) (float64, error) {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0, err
	}
	return f, nil
}

const usageText = `
usage: ping-nomadnet-node [-h] [-v] [--rnsconfig DIR]
                          [--path-timeout SECONDS] [--link-timeout SECONDS]
                          <addr|file> [<addr|file> ...]

Check the reachability of LXMF (nomadnet) nodes from the local Reticulum
instance. Each argument is an LXMF address (a 32-hex destination hash, bare or
prefixed with lxm:/lxmf:/lxmf@/lxmf://) or a path to a file of addresses.

positional arguments:
  addr                an LXMF address (32-hex hash of an lxmf.delivery node)
  file                a file with one address per line; lines not starting
                      with an LXMF address are skipped; trailing text on a
                      line is used as the node label

options:
  -h, --help            show this help message and exit
  -v, --verbose         verbose RNS logging
  --rnsconfig DIR       Reticulum config dir (default: ~/.reticulum)
  --path-timeout SECS   seconds to wait for a path request (default 15)
  --link-timeout SECS   seconds to wait for link establishment (default 15)
  --identity FILE       ping the node whose identity is in FILE (its
                      lxmf.delivery hash is computed directly; no announce
                      needs to be heard first)

exit codes:
  0   every node is ONLINE
  1   at least one node is not ONLINE
  2   bad usage / no addresses
`