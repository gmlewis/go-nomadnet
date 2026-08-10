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

// Command stress-test-nomadnet stress-tests LXMF (nomadnet) nodes reachable from
// the local Reticulum instance by opening concurrent links and barraging them
// with requests — including malformed paths and payloads — plus optional link
// churn and announce storms. The goal is to surface panics, deadlocks, and
// wedges in go-reticulum or go-nomadnet that ordinary use would not reach.
//
// Command-line parsing is identical to ping-nomadnet-node: each positional
// argument is an LXMF address or a file of addresses, the same address prefixes
// are stripped, the same flags (--rnsconfig, --path-timeout, --link-timeout,
// --identity, --browse, --discover, --request-path, -v, -h) work the same way,
// and --discover listens for an announce before stressing the announced node.
//
//	go run ./cmd/stress-test-nomadnet known-nodes.txt --browse
//	go run ./cmd/stress-test-nomadnet --browse --malformed --concurrency 8 6853937554960093a05764c3974f28e6
//	go run ./cmd/stress-test-nomadnet --discover --browse --churn --duration 60
//
// The tool reports per-target counts (requests sent, responses, failures, link
// establishes/drops) and flags a target UNRESPONSIVE when it stops answering
// after previously responding — the signal that it has crashed or wedged. The
// tool cannot read the target's logs, so that signal is how a found bug shows
// up here.
//
// Flags:
//
//	--rnsconfig DIR       Reticulum config dir (default: ~/.reticulum)
//	--path-timeout D       seconds to wait for a path request (default 15)
//	--link-timeout D       seconds to wait for link establishment (default 15)
//	--identity FILE        stress the node whose identity is in FILE
//	--browse               target nomadnetwork.node and request pages (the
//	                      surface where nomadnet page/micron parsing bugs live)
//	--lxmf                 target the node's lxmf.delivery destination instead
//	                      of nomadnetwork.node
//	--discover            listen for an announce, then stress that node
//	--request-path PATH    default page path for normal requests (default /page/index.mu)
//	--duration SECS        run length in seconds (default 30; 0 = request-bounded)
//	--concurrency N        concurrent links per target (default 4)
//	--requests N           requests per link in sustained mode (default 50)
//	--request-timeout D    seconds to wait per request response (default 10)
//	--rate N               max requests/sec total, 0 = unbounded (default 0)
//	--malformed            include malformed/edge-case request paths, payloads,
//	                      and broadcast corrupted announce packets (network-wide)
//	--announce-storm       repeatedly announce a synthetic destination to flood
//	                      the network's announce handling (network-wide)
//	--churn                rapid link establish/teardown instead of sustained links
//	-v                     verbose RNS logging
//	-h, --help             show help and exit
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

const (
	defaultPathTimeout    = 15.0
	defaultLinkTimeout    = 15.0
	defaultRequestTimeout = 10.0
	defaultDuration       = 30.0
	defaultConcurrency    = 4
	defaultRequests       = 50
	// pollInterval is the granularity at which path state is polled.
	pollInterval = 100 * time.Millisecond
	// startupGrace gives AutoInterface discovery a moment before probing.
	startupGrace = 1500 * time.Millisecond
	// unresponsiveStreak is the number of consecutive request failures (after at
	// least one success) that flags a target UNRESPONSIVE — i.e. it answered
	// before and now does not, which is the crash/wedge signal.
	unresponsiveStreak = 5
	// announceStormInterval is the gap between storm announces (~50/sec).
	announceStormInterval = 20 * time.Millisecond
	// malformedAnnounceInterval paces corrupted-announce broadcasts.
	malformedAnnounceInterval = 50 * time.Millisecond
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
	browse       bool   // browse mode: target nomadnetwork.node and request pages
	lxmf         bool   // target the node's lxmf.delivery destination instead of nomadnetwork.node
	discover     bool   // discover mode: listen for an announce, then stress it
	requestPath  string // default page/file path for normal requests
	args         []string

	// stress-specific
	duration        float64 // seconds; 0 = request-bounded (sustained mode only)
	concurrency     int
	requestsPerLink int
	requestTimeout  float64
	rate            int  // max requests/sec total; 0 = unbounded
	malformed       bool // include malformed paths/payloads + corrupted announces
	announceStorm   bool // flood announces network-wide
	churn           bool // rapid link establish/teardown instead of sustained
}

// destAppAspects returns the RNS app name and aspects for the active mode.
//
// The default (and --browse) target is nomadnetwork.node — the destination
// whose hash the user supplies and the surface where page-serving stress is
// meaningful. --lxmf switches the target to the same node's lxmf.delivery
// destination (a different hash from the same identity); --browse always
// targets nomadnetwork.node since only it serves pages.
func destAppAspects(opts *options) (string, []string) {
	if opts != nil && opts.browse {
		return "nomadnetwork", []string{"node"}
	}
	if opts != nil && opts.lxmf {
		return "lxmf", []string{"delivery"}
	}
	return "nomadnetwork", []string{"node"}
}

type target struct {
	hashHex  string
	label    string
	src      string        // origin (arg or file:line) for diagnostics
	identity *rns.Identity // when set (from --identity or --discover), used directly
}

// resolved is a target that has cleared the identity + path stages and is ready
// to be stressed with links.
type resolved struct {
	target
	app        string
	aspects    []string
	targetHash []byte
	hops       int
}

// reachable holds the resolution outcome for a target that could not be
// resolved (unreachable / no announce / invalid).
type reachable struct {
	target
	ok     bool
	status string
	detail string
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

	fmt.Printf("stress-test-nomadnet — go-reticulum %v\n", rns.VERSION)
	if dir := ret.ConfigDir(); dir != "" {
		fmt.Printf("RNS config: %s\n", dir)
	}
	printInterfaces(ret)

	app, aspects := destAppAspects(opts)
	mode := app + "." + strings.Join(aspects, ".")
	fmt.Printf("Mode: stress %s — concurrency %d, requests/link %d, duration %.0fs, malformed=%v, churn=%v, announce-storm=%v\n",
		mode, opts.concurrency, opts.requestsPerLink, opts.duration, opts.malformed, opts.churn, opts.announceStorm)

	// Resolve the target list. --discover listens for an announce and yields one
	// pre-loaded target (same as ping-nomadnet-node); otherwise collectTargets
	// parses addresses/files identically to ping.
	var rawTargets []target
	if opts.discover {
		t, ok := discoverTarget(ts, opts)
		if !ok {
			fmt.Printf("\nNO ANNOUNCE: no %s announce received within %.0fs\n", mode, opts.pathTimeout)
			return 1
		}
		rawTargets = []target{t}
	} else {
		ts2, err := collectTargets(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if len(ts2) == 0 {
			fmt.Fprintln(os.Stderr, "no LXMF addresses to stress")
			fmt.Fprint(os.Stderr, usageText)
			return 2
		}
		rawTargets = ts2
	}

	fmt.Printf("Stressing %d node(s) — path timeout %.0fs, link timeout %.0fs\n",
		len(rawTargets), opts.pathTimeout, opts.linkTimeout)

	// Give AutoInterface discovery a moment to bring interfaces up.
	time.Sleep(startupGrace)

	// Resolve identity + path for each target (ping stages 1-2). Targets that
	// cannot be resolved are reported as unreachable and skipped.
	var resolvedTargets []resolved
	var unreachable []reachable
	for _, t := range rawTargets {
		r, rb := resolveTarget(ts, t, opts)
		if rb.ok {
			resolvedTargets = append(resolvedTargets, r)
		} else {
			unreachable = append(unreachable, rb)
		}
	}

	if len(resolvedTargets) == 0 {
		fmt.Println("\nNo reachable targets to stress:")
		for _, u := range unreachable {
			fmt.Printf("  %s  %s  (%s)\n", u.hashHex, u.status, u.detail)
		}
		return 1
	}

	// Optional network-wide storms run for the whole duration. They broadcast
	// to every peer (not just the named targets), so they are opt-in.
	duration := opts.duration
	if opts.churn && duration <= 0 {
		duration = defaultDuration
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(duration*float64(time.Second)))
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	var stormWG sync.WaitGroup
	if opts.announceStorm {
		stormWG.Go(func() {
			runAnnounceStorm(ctx, ts, logger)
		})
	}
	if opts.malformed {
		stormWG.Go(func() {
			runMalformedAnnounceStorm(ctx, ts, logger)
		})
	}

	// Stress each reachable target concurrently. Each target gets its own
	// stats; workers within a target share them.
	stats := make([]*targetStats, len(resolvedTargets))
	var wg sync.WaitGroup
	for i, res := range resolvedTargets {
		stats[i] = newTargetStats()
		wg.Add(1)
		go func(res resolved, st *targetStats) {
			defer wg.Done()
			stressTarget(ctx, ts, res, opts, st)
		}(res, stats[i])
	}
	wg.Wait()
	cancel()
	stormWG.Wait()

	printReport(resolvedTargets, unreachable, stats, opts)
	return reportExitCode(unreachable, stats)
}

// resolveTarget performs the identity + path stages (mirroring ping-nomadnet-node
// stages 1-2). On success it returns a resolved target ready for linking; on
// failure it returns a reachable describing why (NO ANNOUNCE / UNREACHABLE /
// INVALID).
func resolveTarget(ts *rns.TransportSystem, t target, opts *options) (resolved, reachable) {
	app, aspects := destAppAspects(opts)
	rb := reachable{target: t, status: "ONLINE"}

	sourceHash, err := hex.DecodeString(t.hashHex)
	if err != nil || len(sourceHash) != rns.TruncatedHashLength/8 {
		rb.ok = false
		rb.status = "INVALID"
		rb.detail = fmt.Sprintf("not a %d-char hex hash: %q", rns.TruncatedHashLength/4, t.hashHex)
		return resolved{target: t}, rb
	}

	identity := t.identity
	if identity == nil {
		// A shared-instance client only knows identities persisted before it
		// started or forwarded after it connected. A path request for the source
		// hash prompts the network to re-announce (carrying the identity), so we
		// issue one and poll RecallIdentity for the full timeout rather than
		// failing on the first miss. (The local node's own identity is never
		// learned this way — use --identity FILE for it.)
		_ = ts.RequestPath(sourceHash)
		deadline := time.Now().Add(time.Duration(opts.pathTimeout * float64(time.Second)))
		for time.Now().Before(deadline) {
			if id := rns.RecallIdentity(ts, sourceHash); id != nil {
				identity = id
				break
			}
			time.Sleep(pollInterval)
		}
	}
	if identity == nil {
		rb.ok = false
		rb.status = "NO ANNOUNCE"
		rb.detail = "identity never recalled (no announce received from this node within timeout); pass --identity FILE"
		return resolved{target: t}, rb
	}

	targetHash := rns.CalculateHash(identity, app, aspects...)

	if !ts.HasPath(targetHash) {
		if err := ts.RequestPath(targetHash); err != nil {
			rb.detail = fmt.Sprintf("path request failed: %v", err)
		}
		deadline := time.Now().Add(time.Duration(opts.pathTimeout * float64(time.Second)))
		for time.Now().Before(deadline) && !ts.HasPath(targetHash) {
			time.Sleep(pollInterval)
		}
	}
	if !ts.HasPath(targetHash) {
		rb.ok = false
		rb.status = "UNREACHABLE"
		rb.detail = "no transport path within timeout (node not announcing / interfaces down)"
		return resolved{target: t}, rb
	}

	rb.ok = true
	// Store the recalled identity back onto the target so establishLink can
	// build the outbound SINGLE destination. Without this, targets loaded from
	// a hash file (no preloaded identity) reach establishLink with a nil
	// identity and NewDestination fails immediately ("can't create outbound
	// SINGLE destination without an identity") — every link fails before a
	// single request is sent. ping-nomadnet-node keeps the recalled identity
	// in local scope through NewDestination; this tool splits resolution and
	// link setup across functions, so the identity must be carried in res.
	t.identity = identity
	return resolved{
		target:     t,
		app:        app,
		aspects:    aspects,
		targetHash: targetHash,
		hops:       ts.HopsTo(targetHash),
	}, rb
}

// discoverTarget listens for an announce matching the active aspect and returns
// the first announced node as a pre-loaded target (identity captured from the
// announce, so no RecallIdentity is needed). Mirrors ping-nomadnet-node's
// --discover path.
func discoverTarget(ts *rns.TransportSystem, opts *options) (target, bool) {
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
		return target{
			hashHex:  hex.EncodeToString(ann.destHash),
			label:    announceLabel(ann.appData),
			src:      "discover:" + aspectFilter,
			identity: ann.identity,
		}, true
	case <-time.After(deadline):
		return target{}, false
	}
}

// announceLabel renders an announce's app_data as a human-readable label.
func announceLabel(appData []byte) string {
	if len(appData) == 0 {
		return "(no app_data)"
	}
	if s := strings.TrimSpace(string(appData)); s != "" {
		return fmt.Sprintf("announced as %q", s)
	}
	return fmt.Sprintf("app_data %d bytes", len(appData))
}

// stressTarget runs concurrency workers against one resolved target.
func stressTarget(ctx context.Context, ts *rns.TransportSystem, res resolved, opts *options, st *targetStats) {
	corpus := buildCorpus(opts)
	lim := newLimiter(opts.rate, ctx.Done())
	linkDeadline := time.Duration(opts.linkTimeout * float64(time.Second))
	reqTimeout := time.Duration(opts.requestTimeout * float64(time.Second))

	var workers sync.WaitGroup
	for w := 0; w < opts.concurrency; w++ {
		workers.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("stress worker recovered: %v", r)
				}
			}()
			if opts.churn {
				runChurnWorker(ctx, ts, res, opts, st, corpus, lim, linkDeadline, reqTimeout)
			} else {
				runSustainedWorker(ctx, ts, res, opts, st, corpus, lim, linkDeadline, reqTimeout)
			}
		})
	}
	workers.Wait()
}

// runSustainedWorker establishes one link, then sends requestsPerLink requests
// over it, re-establishing if the link drops, until the request budget or run
// duration is exhausted.
func runSustainedWorker(ctx context.Context, ts *rns.TransportSystem, res resolved, opts *options, st *targetStats, corpus []requestCase, lim *limiter, linkDeadline, reqTimeout time.Duration) {
	sent := 0
	for sent < opts.requestsPerLink {
		select {
		case <-ctx.Done():
			return
		default:
		}

		link, ok := establishLink(ts, res, linkDeadline, st)
		if !ok {
			// Could not establish; brief backoff, then retry unless the run is
			// over. A persistent failure to establish is itself a strong signal
			// that the target is wedged.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		var dropped atomic.Int32
		link.SetLinkClosedCallback(func(*rns.Link) {
			if dropped.CompareAndSwap(0, 1) {
				st.recordLinkDrop()
			}
		})

		for sent < opts.requestsPerLink {
			select {
			case <-ctx.Done():
				teardownLink(link)
				return
			default:
			}
			if !lim.wait(ctx.Done()) {
				teardownLink(link)
				return
			}
			rc := corpus[sent%len(corpus)]
			sent++
			status, rtt := sendOne(link, rc.path, rc.data, reqTimeout)
			st.recordRequest(status, rtt)
			if dropped.Load() == 1 || status == statusSendError {
				// Link died under us; re-establish and continue sending.
				break
			}
		}
		teardownLink(link)
	}
}

// runChurnWorker repeatedly establishes a link, sends a single request, and
// tears it down — maximizing link setup/teardown churn to exercise the link
// table, proof exchange, and link pruning paths. Runs until the duration ends.
func runChurnWorker(ctx context.Context, ts *rns.TransportSystem, res resolved, opts *options, st *targetStats, corpus []requestCase, lim *limiter, linkDeadline, reqTimeout time.Duration) {
	iter := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !lim.wait(ctx.Done()) {
			return
		}
		link, ok := establishLink(ts, res, linkDeadline, st)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		rc := corpus[iter%len(corpus)]
		iter++
		status, rtt := sendOne(link, rc.path, rc.data, reqTimeout)
		st.recordRequest(status, rtt)
		teardownLink(link)
	}
}

// establishLink builds the outbound SINGLE destination, creates a link, and
// waits for establishment (or a close/timeout). Records the establish/establish
// failure into st.
func establishLink(ts *rns.TransportSystem, res resolved, linkDeadline time.Duration, st *targetStats) (*rns.Link, bool) {
	dest, err := rns.NewDestination(ts, res.identity, rns.DestinationOut, rns.DestinationSingle, res.app, res.aspects...)
	if err != nil {
		st.recordEstablishFail()
		return nil, false
	}
	link, err := rns.NewLink(ts, dest)
	if err != nil {
		st.recordEstablishFail()
		return nil, false
	}

	established := make(chan struct{})
	var closeOnce sync.Once
	closed := make(chan struct{})
	link.SetLinkEstablishedCallback(func(*rns.Link) { close(established) })
	link.SetLinkClosedCallback(func(*rns.Link) {
		select {
		case <-established:
			return
		default:
			closeOnce.Do(func() { close(closed) })
		}
	})

	if err := link.Establish(); err != nil {
		st.recordEstablishFail()
		return nil, false
	}

	select {
	case <-established:
		st.recordEstablish()
		return link, true
	case <-closed:
		st.recordEstablishFail()
		return nil, false
	case <-time.After(linkDeadline):
		go func() { defer func() { _ = recover() }(); link.Teardown() }()
		st.recordEstablishFail()
		return nil, false
	}
}

// teardownLink tears a link down in the background (best effort).
func teardownLink(link *rns.Link) {
	go func() {
		defer func() { _ = recover() }()
		link.Teardown()
	}()
}

const (
	statusOK        = "ok"
	statusFailed    = "failed"
	statusSendError = "send-error"
	statusTimeout   = "timeout"
)

// sendOne issues a single request over an active link and waits for the
// response, a failure, or a backstop timeout. Returns a status and RTT.
func sendOne(link *rns.Link, path string, data any, timeout time.Duration) (string, time.Duration) {
	type res struct {
		ok  bool
		rtt time.Duration
	}
	ch := make(chan res, 2)
	start := time.Now()
	_, err := link.Request(path, data, func(rr *rns.RequestReceipt) {
		if rr.Status == rns.RequestReady {
			ch <- res{ok: true, rtt: time.Since(start)}
		} else {
			ch <- res{ok: false}
		}
	}, func(rr *rns.RequestReceipt) {
		ch <- res{ok: false}
	}, nil, timeout)
	if err != nil {
		return statusSendError, 0
	}
	select {
	case r := <-ch:
		if r.ok {
			return statusOK, r.rtt
		}
		return statusFailed, 0
	case <-time.After(timeout + 2*time.Second):
		return statusTimeout, 0
	}
}

// requestCase is one (path, data) pair to send over a link.
type requestCase struct {
	path string
	data any
	tag  string
}

// buildCorpus returns the request corpus. Without --malformed it is a small set
// of plausible nomadnet page paths with nil data; with --malformed it adds long
// paths, path traversal, NUL/control bytes, wide unicode, huge segments, and
// odd request payload types — the inputs most likely to find parsing panics.
func buildCorpus(opts *options) []requestCase {
	var out []requestCase
	add := func(path string, data any, tag string) {
		out = append(out, requestCase{path: path, data: data, tag: tag})
	}

	// Normal cases: plausible page fetches against nomadnetwork.node.
	add(opts.requestPath, nil, "normal")
	add("/page/index.mu", nil, "index")
	add("/", nil, "root")
	add("/page/conversations.mu", nil, "conversations")

	if !opts.malformed {
		return out
	}

	// Malformed paths.
	add("", nil, "empty-path")
	add("/", nil, "slash")
	add(strings.Repeat("/", 500), nil, "slashes")
	add("/page/"+strings.Repeat("../", 80)+"etc/passwd", nil, "traversal")
	add("/page/%2e%2e%2f%2e%2e%2fetc%2fpasswd", nil, "encoded-traversal")
	add("/page/\x00index.mu", nil, "nul-mid")
	add("/page/index.mu\x00extra", nil, "nul-tail")
	add("/page/"+string([]byte{0x01, 0x02, 0x03, 0x04, 0x05})+".mu", nil, "control")
	add("/page/"+strings.Repeat("🏳️🌈", 100)+".mu", nil, "wide-unicode")
	add("/page/"+strings.Repeat("é", 500)+".mu", nil, "combining")
	add("/page/"+strings.Repeat("A", 10000)+".mu", nil, "long-segment")
	add("/page/"+strings.Repeat("/d", 500)+"eep.mu", nil, "deep")
	add("/page/index.mu?query=1&other=2", nil, "query")
	add("/page/index.mu#fragment", nil, "fragment")
	add("/page/.hidden", nil, "dotfile")
	add("/page/.nomadnode/config", nil, "config-path")
	add("/page/CONSOLE.mu", nil, "upper")
	add("/page/index.MU", nil, "ext-case")
	add("/page/"+strings.Repeat("x", 1<<16)+".mu", nil, "huge")
	add("/announce", nil, "announce-path")
	add("/path", nil, "path-path")
	add("/status", nil, "status-path")

	// Malformed payloads (msgpacked as the request data field).
	add(opts.requestPath, []byte{}, "empty-bytes")
	add(opts.requestPath, []byte(strings.Repeat("\x00", 1024)), "nul-bytes")
	add(opts.requestPath, []byte(strings.Repeat("\xff", 1024)), "ff-bytes")
	add(opts.requestPath, strings.Repeat("x", 1<<16), "huge-string")
	add(opts.requestPath, 12345, "int")
	add(opts.requestPath, []any{nil, nil, nil}, "nil-slice")
	add(opts.requestPath, map[string]any{"field": strings.Repeat("x", 100000)}, "big-map")
	add(opts.requestPath, []any{1, "a", nil, true, []byte{0, 1, 2}}, "mixed")

	return out
}

// targetStats is the per-target counter set, updated concurrently by all of the
// target's workers.
type targetStats struct {
	mu sync.Mutex

	requestsSent   int
	responsesOK    int
	failures       int
	timeouts       int
	sendErrors     int
	establishes    int
	establishFails int
	linkDrops      int

	consecutiveFails int
	hadSuccess       bool
	unresponsive     bool

	rttMin time.Duration
	rttMax time.Duration
	rttSum time.Duration
	rttN   int
}

func newTargetStats() *targetStats { return &targetStats{rttMin: -1} }

func (s *targetStats) recordRequest(status string, rtt time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestsSent++
	switch status {
	case statusOK:
		s.responsesOK++
		s.consecutiveFails = 0
		s.hadSuccess = true
		if rtt > 0 {
			if s.rttMin < 0 || rtt < s.rttMin {
				s.rttMin = rtt
			}
			if rtt > s.rttMax {
				s.rttMax = rtt
			}
			s.rttSum += rtt
			s.rttN++
		}
	case statusTimeout:
		s.timeouts++
		s.failures++
		s.recordFail()
	case statusSendError:
		s.sendErrors++
		s.failures++
		s.recordFail()
	default:
		s.failures++
		s.recordFail()
	}
}

// recordFail updates the unresponsive streak. Caller must hold s.mu.
func (s *targetStats) recordFail() {
	s.consecutiveFails++
	if s.hadSuccess && s.consecutiveFails >= unresponsiveStreak {
		s.unresponsive = true
	}
}

func (s *targetStats) recordEstablish() {
	s.mu.Lock()
	s.establishes++
	s.mu.Unlock()
}

func (s *targetStats) recordEstablishFail() {
	s.mu.Lock()
	s.establishFails++
	s.mu.Unlock()
}

func (s *targetStats) recordLinkDrop() {
	s.mu.Lock()
	s.linkDrops++
	s.mu.Unlock()
}

// targetStatsSnapshot is a mutex-free copy of the counters for reporting.
type targetStatsSnapshot struct {
	requestsSent     int
	responsesOK      int
	failures         int
	timeouts         int
	sendErrors       int
	establishes      int
	establishFails   int
	linkDrops        int
	consecutiveFails int
	hadSuccess       bool
	unresponsive     bool
	rttMin           time.Duration
	rttMax           time.Duration
	rttSum           time.Duration
	rttN             int
}

// snapshot copies the counters for reporting.
func (s *targetStats) snapshot() targetStatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return targetStatsSnapshot{
		requestsSent:     s.requestsSent,
		responsesOK:      s.responsesOK,
		failures:         s.failures,
		timeouts:         s.timeouts,
		sendErrors:       s.sendErrors,
		establishes:      s.establishes,
		establishFails:   s.establishFails,
		linkDrops:        s.linkDrops,
		consecutiveFails: s.consecutiveFails,
		hadSuccess:       s.hadSuccess,
		unresponsive:     s.unresponsive,
		rttMin:           s.rttMin,
		rttMax:           s.rttMax,
		rttSum:           s.rttSum,
		rttN:             s.rttN,
	}
}

// limiter is a simple token-bucket rate limiter shared across all workers. A
// nil limiter (rate <= 0) is unbounded.
type limiter struct {
	tokens chan struct{}
	stop   chan struct{}
}

func newLimiter(rate int, done <-chan struct{}) *limiter {
	if rate <= 0 {
		return nil
	}
	if rate > 1000 {
		rate = 1000
	}
	l := &limiter{tokens: make(chan struct{}, rate), stop: make(chan struct{})}
	for i := 0; i < rate; i++ {
		l.tokens <- struct{}{}
	}
	interval := time.Second / time.Duration(rate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-l.stop:
				return
			case <-t.C:
				select {
				case l.tokens <- struct{}{}:
				default:
				}
			}
		}
	}()
	return l
}

func (l *limiter) wait(done <-chan struct{}) bool {
	if l == nil {
		return true
	}
	select {
	case <-l.tokens:
		return true
	case <-done:
		return false
	}
}

// runAnnounceStorm announces a synthetic IN SINGLE destination repeatedly to
// flood every peer's inbound announce handling (rate table, ValidateAnnounce,
// handleAnnounce). It broadcasts network-wide, so it is opt-in.
func runAnnounceStorm(ctx context.Context, ts *rns.TransportSystem, logger *rns.Logger) {
	id, err := rns.NewIdentity(true, logger)
	if err != nil {
		log.Printf("announce storm: could not create identity: %v", err)
		return
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "nomadnet", "stress")
	if err != nil {
		log.Printf("announce storm: could not create destination: %v", err)
		return
	}
	appData := []byte("stress-test-nomadnet-storm")
	count := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("announce storm: sent %d announces", count)
			return
		default:
		}
		if err := dest.Announce(appData); err != nil {
			log.Printf("announce storm: Announce error: %v", err)
		}
		count++
		select {
		case <-ctx.Done():
			log.Printf("announce storm: sent %d announces", count)
			return
		case <-time.After(announceStormInterval):
		}
	}
}

// runMalformedAnnounceStorm builds a valid signed announce packet, then
// broadcasts corrupted variants (truncated payloads and bit-flipped bytes) to
// exercise every peer's inbound announce parsing defensively. On a fixed
// target these are rejected gracefully; on a buggy one they may panic or wedge.
// Best-effort wire-level fuzzing. Broadcasts network-wide, so it is opt-in
// (gated by --malformed).
func runMalformedAnnounceStorm(ctx context.Context, ts *rns.TransportSystem, logger *rns.Logger) {
	id, err := rns.NewIdentity(true, logger)
	if err != nil {
		log.Printf("malformed announce: could not create identity: %v", err)
		return
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "nomadnet", "stress2")
	if err != nil {
		log.Printf("malformed announce: could not create destination: %v", err)
		return
	}
	p, err := dest.BuildAnnouncePacket([]byte("malformed"))
	if err != nil {
		log.Printf("malformed announce: could not build packet: %v", err)
		return
	}
	variants := corruptAnnounceVariants(p)
	count := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("malformed announce: sent %d corrupted packets", count)
			return
		default:
		}
		for _, raw := range variants {
			cp := *p
			cp.Raw = raw
			cp.Packed = true
			cp.Data = nil
			// A down interface or transient error is not fatal to the storm;
			// intentionally ignore the result and keep going.
			_ = ts.Outbound(&cp)
			count++
		}
		select {
		case <-ctx.Done():
			log.Printf("malformed announce: sent %d corrupted packets", count)
			return
		case <-time.After(malformedAnnounceInterval):
		}
	}
}

// corruptAnnounceVariants returns corrupted copies of a valid announce packet's
// Raw bytes: truncations that keep the header but shorten the data (to hit the
// min-length guard in ValidateAnnounce), bit flips in the data region (bad
// signatures / hashes), and one header-flag flip (to exercise Unpack). These
// are best-effort and intentionally varied.
func corruptAnnounceVariants(p *rns.Packet) [][]byte {
	valid := p.Raw
	if len(valid) == 0 {
		return nil
	}
	headerLen := len(valid) - len(p.Data)
	headerLen = max(headerLen, 0)
	var out [][]byte
	// Truncations: keep the header, drop the tail of the data.
	for _, keep := range []int{0, 8, 16, 32, 64, 128} {
		end := headerLen + keep
		end = min(end, len(valid))
		end = max(end, headerLen)
		out = append(out, append([]byte(nil), valid[:end]...))
	}
	// Bit flips in the data region (signatures/hashes will not verify).
	for _, off := range []int{0, 16, 32, 64, len(p.Data) / 2} {
		pos := headerLen + off
		if pos <= 0 || pos >= len(valid) {
			continue
		}
		b := append([]byte(nil), valid...)
		b[pos] ^= 0xFF
		out = append(out, b)
	}
	// One header-flag flip to exercise the Unpack path.
	hb := append([]byte(nil), valid...)
	hb[0] ^= 0x01
	out = append(out, hb)
	return out
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

func printReport(resolvedTargets []resolved, unreachable []reachable, stats []*targetStats, opts *options) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 78))
	fmt.Println("STRESS RESULTS")
	fmt.Println(strings.Repeat("=", 78))

	for _, u := range unreachable {
		fmt.Printf("\n%s  %s\n", u.hashHex, nonEmpty(u.label, "(no label)"))
		fmt.Printf("  status : %s\n", u.status)
		if u.detail != "" {
			fmt.Printf("  note   : %s\n", u.detail)
		}
	}

	for i, res := range resolvedTargets {
		s := stats[i].snapshot()
		label := nonEmpty(res.label, "(no label)")
		status := targetStatus(s)
		fmt.Printf("\n%s  %s\n", res.hashHex, label)
		fmt.Printf("  status   : %s\n", status)
		if res.hops >= 0 && res.hops < rns.PathfinderM {
			fmt.Printf("  hops     : %d\n", res.hops)
		}
		fmt.Printf("  requests : %d sent, %d ok, %d failed (%d timeouts, %d send-errors)\n",
			s.requestsSent, s.responsesOK, s.failures, s.timeouts, s.sendErrors)
		fmt.Printf("  links    : %d established, %d failed, %d dropped\n",
			s.establishes, s.establishFails, s.linkDrops)
		if s.rttN > 0 {
			avg := s.rttSum / time.Duration(s.rttN)
			fmt.Printf("  rtt      : min %d ms, avg %d ms, max %d ms (n=%d)\n",
				s.rttMin.Milliseconds(), avg.Milliseconds(), s.rttMax.Milliseconds(), s.rttN)
		}
		if s.unresponsive {
			fmt.Printf("  *** TARGET UNRESPONSIVE — answered before, now silent (likely crashed or wedged)\n")
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("%-34s  %-16s  %s\n", "ADDRESS", "STATUS", "SENT/OK/FAIL")
	for i, res := range resolvedTargets {
		s := stats[i].snapshot()
		fmt.Printf("%-34s  %-16s  %d/%d/%d\n", res.hashHex, targetStatus(s), s.requestsSent, s.responsesOK, s.failures)
	}
}

func targetStatus(s targetStatsSnapshot) string {
	switch {
	case s.unresponsive:
		return "UNRESPONSIVE"
	case s.establishes == 0 && s.establishFails > 0:
		return "ALL LINKS FAILED"
	case s.responsesOK == 0 && s.requestsSent > 0:
		return "NO RESPONSES"
	case s.responsesOK > 0:
		return "STRESSED"
	default:
		return "IDLE"
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// reportExitCode is 0 only if every target was reached and stayed responsive;
// 1 if any target was unreachable, unresponsive, or never responded.
func reportExitCode(unreachable []reachable, stats []*targetStats) int {
	bad := false
	if len(unreachable) > 0 {
		bad = true
	}
	for _, st := range stats {
		s := st.snapshot()
		if s.unresponsive || s.responsesOK == 0 || (s.establishes == 0 && s.establishFails > 0) {
			bad = true
		}
	}
	if bad {
		return 1
	}
	return 0
}

// collectTargets turns the raw arguments into a flat list of targets,
// classifying each arg as an LXMF address or a filename. When opts.identityFile
// is set, a target is added from that identity file (its mode-correct hash is
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
	opts := &options{
		pathTimeout:     defaultPathTimeout,
		linkTimeout:     defaultLinkTimeout,
		requestPath:     "/page/index.mu",
		duration:        defaultDuration,
		concurrency:     defaultConcurrency,
		requestsPerLink: defaultRequests,
		requestTimeout:  defaultRequestTimeout,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			return nil, errHelp
		case "-v", "--verbose":
			opts.verbose = true
		case "--browse":
			opts.browse = true
		case "--lxmf":
			opts.lxmf = true
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
		case "--duration":
			v, next, err := flagValue(args, i, "--duration")
			if err != nil {
				return nil, err
			}
			f, err := parseFloat(v)
			if err != nil {
				return nil, fmt.Errorf("--duration: %w", err)
			}
			opts.duration = f
			i = next
		case "--concurrency":
			v, next, err := flagValue(args, i, "--concurrency")
			if err != nil {
				return nil, err
			}
			n, err := parseInt(v)
			if err != nil {
				return nil, fmt.Errorf("--concurrency: %w", err)
			}
			if n < 1 {
				return nil, fmt.Errorf("--concurrency must be >= 1")
			}
			opts.concurrency = n
			i = next
		case "--requests":
			v, next, err := flagValue(args, i, "--requests")
			if err != nil {
				return nil, err
			}
			n, err := parseInt(v)
			if err != nil {
				return nil, fmt.Errorf("--requests: %w", err)
			}
			if n < 1 {
				return nil, fmt.Errorf("--requests must be >= 1")
			}
			opts.requestsPerLink = n
			i = next
		case "--request-timeout":
			v, next, err := flagValue(args, i, "--request-timeout")
			if err != nil {
				return nil, err
			}
			f, err := parseFloat(v)
			if err != nil {
				return nil, fmt.Errorf("--request-timeout: %w", err)
			}
			opts.requestTimeout = f
			i = next
		case "--rate":
			v, next, err := flagValue(args, i, "--rate")
			if err != nil {
				return nil, err
			}
			n, err := parseInt(v)
			if err != nil {
				return nil, fmt.Errorf("--rate: %w", err)
			}
			if n < 0 {
				return nil, fmt.Errorf("--rate must be >= 0")
			}
			opts.rate = n
			i = next
		case "--malformed":
			opts.malformed = true
		case "--announce-storm":
			opts.announceStorm = true
		case "--churn":
			opts.churn = true
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

func parseInt(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

const usageText = `
usage: stress-test-nomadnet [-h] [-v] [--rnsconfig DIR]
                           [--path-timeout SECONDS] [--link-timeout SECONDS]
                           [--identity FILE] [--lxmf | --browse] [--discover]
                           [--duration SECONDS] [--concurrency N] [--requests N]
                           [--request-timeout SECONDS] [--rate N]
                           [--malformed] [--announce-storm] [--churn]
                           <addr|file> [<addr|file> ...]

Stress-test nomadnet nodes reachable from the local Reticulum instance by
opening concurrent links and barraging them with requests — including malformed
paths and payloads — plus optional link churn and announce storms, to surface
panics, deadlocks, and wedges in go-reticulum or go-nomadnet.

Command-line parsing is identical to ping-nomadnet-node: each argument is a
32-hex destination hash (bare or prefixed with lxm:/lxmf:/lxmf@/lxmf://) or a
path to a file of addresses (one per line; lines not starting with an address
are skipped; trailing text is the label). By default the hash is the node's
nomadnetwork.node destination; pass --lxmf to target lxmf.delivery instead.

positional arguments:
  addr                a 32-hex nomadnetwork.node destination hash
  file                a file with one address per line

options:
  -h, --help            show this help message and exit
  -v, --verbose         verbose RNS logging
  --rnsconfig DIR       Reticulum config dir (default: ~/.reticulum)
  --path-timeout SECS   seconds to wait for a path request (default 15)
  --link-timeout SECS   seconds to wait for link establishment (default 15)
  --identity FILE       stress the node whose identity is in FILE
  --lxmf                target the node's lxmf.delivery destination instead of
                        nomadnetwork.node (a different hash from the same
                        identity)
  --browse              target nomadnetwork.node and request pages (the surface
                        where nomadnet page/micron parsing bugs live)
  --discover            listen for an announce matching the active aspect, then
                        stress that node (no positional addresses required)
  --request-path PATH   default page path for normal requests (default /page/index.mu)

stress options:
  --duration SECS       run length in seconds (default 30; 0 = request-bounded,
                        sustained mode only)
  --concurrency N       concurrent links per target (default 4)
  --requests N          requests per link in sustained mode (default 50)
  --request-timeout D   seconds to wait per request response (default 10)
  --rate N              max requests/sec total, 0 = unbounded (default 0)
  --malformed           include malformed/edge-case request paths, payloads, and
                        broadcast corrupted announce packets (network-wide)
  --announce-storm       repeatedly announce a synthetic destination to flood
                        the network's announce handling (network-wide)
  --churn                rapid link establish/teardown instead of sustained links

A target is flagged UNRESPONSIVE when it answered at least one request and then
failed 5 requests in a row — the signal that it has crashed or wedged (the tool
cannot read the target's logs directly).

exit codes:
  0   every target was reached and stayed responsive
  1   at least one target was unreachable, unresponsive, or never responded
  2   bad usage / no addresses
`
