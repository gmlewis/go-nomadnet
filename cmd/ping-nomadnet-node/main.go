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
	browse       bool   // browse mode: target nomadnetwork.node and fetch a page
	discover     bool   // discover mode: listen for an announce, then link/browse it
	requestPath  string // page/file path to request in browse mode
	args         []string
}

// destAppAspects returns the RNS app name and aspects for the active mode:
// ping mode targets "lxmf.delivery"; browse mode targets "nomadnetwork.node".
func destAppAspects(opts *options) (string, []string) {
	if opts != nil && opts.browse {
		return "nomadnetwork", []string{"node"}
	}
	return "lxmf", []string{"delivery"}
}

type target struct {
	hashHex  string
	label    string
	src      string        // origin (arg or file:line) for diagnostics
	identity *rns.Identity // when set (from --identity), used directly instead of RecallIdentity
}

type result struct {
	target
	hash         []byte
	pathKnown    bool // path was already cached before the run
	pathResolved bool // path is known now (cached or freshly resolved)
	pathFresh    bool // path had to be requested and resolved
	hops         int  // HopsTo at report time (PathfinderM if unknown)
	identity     bool // identity was recalled from an announce
	linkUp       bool // outbound lxmf.delivery link established
	rtt          time.Duration
	status       string // UNREACHABLE / NO ANNOUNCE / LINK FAILED / ONLINE
	detail       string // extra context (errors, times)
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
	printInterfaces(ret)

	// Discover mode: listen for an announce matching the active aspect, then
	// link to (and in --browse mode, request a page from) the announced node.
	// This is the path a nomadnet/MeshChat browser takes when it has no prior
	// identity for a node, and is the most faithful reproduction of the
	// 0.4.0->0.7.0 "can't browse" symptom. The handler is registered before any
	// grace so the very first announce (sent ~StartAnnounceDelay after the node
	// starts) is not missed.
	if opts.discover {
		app, aspects := destAppAspects(opts)
		aspectFilter := app + "." + strings.Join(aspects, ".")
		action := "pinging lxmf.delivery"
		if opts.browse {
			action = fmt.Sprintf("browsing %s", opts.requestPath)
		}
		fmt.Printf("Mode: discover — listening for %s announces, then %s (path timeout %.0fs)\n",
			aspectFilter, action, opts.pathTimeout)
		r := discover(ts, opts)
		printReport([]result{r})
		return reportExitCode([]result{r})
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

	fmt.Printf("Pinging %d node(s) — path timeout %.0fs, link timeout %.0fs\n",
		len(targets), opts.pathTimeout, opts.linkTimeout)

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
//
// In ping mode (default) it targets the node's "lxmf.delivery" destination and
// reports link establishment (a "pong"). In browse mode (--browse) it targets
// the node's "nomadnetwork.node" destination and issues a page request, exactly
// mirroring what the nomadnet/Python browser does to "browse" a node.
func pingOne(ts *rns.TransportSystem, t target, opts *options) result {
	r := result{target: t, hops: -1}
	app, aspects := destAppAspects(opts)

	// Source hash: the address given on the command line. When --identity was
	// supplied this is already the (mode-correct) target hash; otherwise it is a
	// known destination hash of the node (e.g. its lxmf.delivery hash) used to
	// recall the peer's identity.
	sourceHash, err := hex.DecodeString(t.hashHex)
	if err != nil || len(sourceHash) != rns.TruncatedHashLength/8 {
		r.status = "INVALID"
		r.detail = fmt.Sprintf("not a %d-char hex hash: %q", rns.TruncatedHashLength/4, t.hashHex)
		return r
	}

	// Stage 1: identity. The peer's public key must be known to build an
	// outbound SINGLE destination. With --identity it is loaded directly;
	// otherwise it is recalled from a received announce (any of the node's
	// announced destinations will yield the same identity).
	identity := t.identity
	if identity == nil {
		identity = rns.RecallIdentity(ts, sourceHash)
	}
	r.identity = identity != nil
	if !r.identity {
		r.status = "NO ANNOUNCE"
		r.detail = "identity never recalled (no announce received from this node); pass --identity FILE"
		return r
	}

	// The destination we actually link to is derived from the identity + the
	// mode's app/aspects (lxmf.delivery for ping, nomadnetwork.node for browse).
	targetHash := rns.CalculateHash(identity, app, aspects...)
	r.hash = targetHash
	if hex.EncodeToString(targetHash) != t.hashHex {
		r.detail = fmt.Sprintf("browsing nomadnetwork.node %s (from %s)", hex.EncodeToString(targetHash), t.hashHex)
	}

	// Stage 2: path. A path is needed to establish a link, and a path request
	// is itself a transport-level reachability probe.
	r.pathKnown = ts.HasPath(targetHash)
	if r.pathKnown {
		r.pathResolved = true
	} else {
		if err := ts.RequestPath(targetHash); err != nil {
			r.detail = fmt.Sprintf("path request failed: %v", err)
		}
		deadline := time.Now().Add(time.Duration(opts.pathTimeout * float64(time.Second)))
		for time.Now().Before(deadline) && !ts.HasPath(targetHash) {
			time.Sleep(pollInterval)
		}
		r.pathResolved = ts.HasPath(targetHash)
		r.pathFresh = r.pathResolved
	}
	if r.pathResolved {
		r.hops = ts.HopsTo(targetHash)
	}
	if !r.pathResolved {
		r.status = "UNREACHABLE"
		if !strings.Contains(r.detail, "browsing") {
			r.detail = "no transport path within timeout (node not announcing / interfaces down)"
		} else {
			r.detail += " — no transport path within timeout"
		}
		return r
	}

	// Stage 3: link. Build the outbound SINGLE destination and establish a link.
	dest, err := rns.NewDestination(ts, identity, rns.DestinationOut, rns.DestinationSingle, app, aspects...)
	if err != nil {
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("could not build %s.%s destination: %v", app, strings.Join(aspects, "."), err)
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
	})
	link.SetLinkClosedCallback(func(l *rns.Link) {
		// If the link activated, the established callback already recorded the
		// outcome; the subsequent close (after our teardown) is a no-op.
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
	teardown := func() {
		go func() {
			defer func() { _ = recover() }()
			link.Teardown()
		}()
	}

	select {
	case <-established:
		r.linkUp = true
		r.rtt = time.Since(start)
		r.hops = ts.HopsTo(targetHash)
		if !opts.browse {
			r.status = "ONLINE"
			if r.detail == "" {
				r.detail = fmt.Sprintf("path %s", pathSource(r))
			}
			teardown()
			return r
		}
		// Browse mode: issue a page request over the active link and report the
		// response (mirrors nomadnet/browser fetchBytes → link.Request).
		data, rerr := browseRequest(link, opts.requestPath, linkDeadline)
		teardown()
		if rerr != nil {
			r.status = "BROWSE FAILED"
			r.detail = fmt.Sprintf("request %s: %v", opts.requestPath, rerr)
			return r
		}
		r.status = "BROWSE OK"
		r.detail = fmt.Sprintf("got %d bytes from %s (%s)", len(data), opts.requestPath, pathSource(r))
		return r
	case <-closed:
		r.status = "LINK FAILED"
		r.detail = "link closed without establishing (peer unreachable or refused)"
		return r
	case <-time.After(linkDeadline):
		r.status = "LINK FAILED"
		r.detail = fmt.Sprintf("link establishment timed out after %.0fs", opts.linkTimeout)
		teardown()
		return r
	}
}

// browseRequest issues a single request for path over an active link and waits
// for the response, mirroring nomadnet/browser fetchBytes step 4
// (link.Request with response/failed callbacks).
func browseRequest(link *rns.Link, path string, timeout time.Duration) ([]byte, error) {
	type bres struct {
		data []byte
		err  error
	}
	res := make(chan bres, 2)
	_, err := link.Request(path, nil, func(rr *rns.RequestReceipt) {
		if rr.Status != rns.RequestReady {
			res <- bres{err: errBrowseFailed}
			return
		}
		res <- bres{data: rr.GetResponse()}
	}, func(rr *rns.RequestReceipt) {
		res <- bres{err: errBrowseFailed}
	}, nil, timeout)
	if err != nil {
		return nil, err
	}
	select {
	case b := <-res:
		return b.data, b.err
	case <-time.After(timeout):
		return nil, errBrowseTimeout
	}
}

var (
	errBrowseFailed  = fmt.Errorf("request failed")
	errBrowseTimeout = fmt.Errorf("request timed out")
)

// discover listens for an announce matching the active mode's aspect
// (nomadnetwork.node in --browse mode, lxmf.delivery otherwise), captures the
// first one, then links to the announced destination and — in --browse mode —
// requests the page. This mirrors what a nomadnet/MeshChat browser does to find
// and open a node it has no prior identity for: it hears the announce, recalls
// the identity, and opens a link. Unlike --identity (which loads the target key
// from disk and bypasses discovery entirely), --discover exercises the real
// announce/discovery path that MeshChat depends on, making it the faithful
// reproduction of the 0.4.0->0.7.0 "cannot browse any node" symptom.
//
// On an announce the destination hash and the announced identity are both
// delivered to the handler, so no RecallIdentity is needed: the captured
// identity is handed straight to pingOne as a pre-loaded target, reusing the
// exact same path/link/browse stages as every other probe.
func discover(ts *rns.TransportSystem, opts *options) result {
	app, aspects := destAppAspects(opts)
	aspectFilter := app + "." + strings.Join(aspects, ".")

	type announce struct {
		destHash []byte
		identity *rns.Identity
		appData  []byte
	}
	annCh := make(chan announce, 1)
	var once sync.Once
	ts.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter: aspectFilter,
		ReceivedAnnounceWithContext: func(destHash []byte, id *rns.Identity, appData []byte, isPathResponse bool) {
			// Ignore path responses: they re-advertise a destination's path but
			// are not a fresh announce from the node itself.
			if isPathResponse {
				return
			}
			once.Do(func() {
				select {
				case annCh <- announce{destHash: destHash, identity: id, appData: appData}:
				default:
				}
			})
		},
	})

	deadline := time.Duration(opts.pathTimeout * float64(time.Second))
	select {
	case ann := <-annCh:
		// The announced identity is known directly; hand it to pingOne as a
		// pre-loaded target so it skips RecallIdentity and proceeds to the
		// path/link/browse stages against the announced destination hash.
		t := target{
			hashHex:  hex.EncodeToString(ann.destHash),
			label:    announceLabel(ann.appData),
			src:      "discover:" + aspectFilter,
			identity: ann.identity,
		}
		r := pingOne(ts, t, opts)
		if r.detail == "" {
			r.detail = t.label
		} else {
			r.detail = t.label + " — " + r.detail
		}
		return r
	case <-time.After(deadline):
		return result{
			target: target{hashHex: "(none)", label: "(discover)", src: "discover:" + aspectFilter},
			hops:   -1,
			status: "NO ANNOUNCE",
			detail: fmt.Sprintf("no %s announce received within %.0fs (node not announcing or not reachable)", aspectFilter, opts.pathTimeout),
		}
	}
}

// announceLabel renders an announce's app_data as a human-readable label. For a
// nomadnetwork.node announce the app_data is the node name; for lxmf.delivery it
// is an LXMF app_data map. Both are shown best-effort as text, falling back to a
// byte-count when the data is non-printable or empty.
func announceLabel(appData []byte) string {
	if len(appData) == 0 {
		return "(no app_data)"
	}
	if s := strings.TrimSpace(string(appData)); s != "" {
		return fmt.Sprintf("announced as %q", s)
	}
	return fmt.Sprintf("app_data %d bytes", len(appData))
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
		if r.status != "ONLINE" && r.status != "BROWSE OK" {
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
		app, aspects := destAppAspects(opts)
		hash := rns.CalculateHash(id, app, aspects...)
		out = append(out, target{
			hashHex:  hex.EncodeToString(hash),
			label:    fmt.Sprintf("(identity %s)", opts.identityFile),
			src:      opts.identityFile,
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
	opts := &options{pathTimeout: defaultPathTimeout, linkTimeout: defaultLinkTimeout, requestPath: "/page/index.mu"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			return nil, errHelp
		case "-v", "--verbose":
			opts.verbose = true
		case "--browse":
			opts.browse = true
		case "--discover":
			opts.discover = true
		case "--request-path":
			v, next, err := flagValue(args, i, "--request-path")
			if err != nil {
				return nil, err
			}
			opts.requestPath = v
			i = next
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
  --browse              browse mode: target the node's nomadnetwork.node
                      destination and request a page (what a browser does),
                      instead of pinging lxmf.delivery
  --discover            discover mode: listen for an announce matching the
                      active aspect (nomadnetwork.node with --browse,
                      lxmf.delivery otherwise), then link to the announced
                      node. This exercises the real announce/discovery path
                      a browser relies on (unlike --identity, which bypasses
                      it). No positional addresses are required.
  --request-path PATH   page/file path to request in --browse mode
                      (default /page/index.mu)

exit codes:
  0   every node is ONLINE
  1   at least one node is not ONLINE
  2   bad usage / no addresses
`
