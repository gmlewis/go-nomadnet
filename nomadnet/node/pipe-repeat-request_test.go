//go:build integration

package node

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestPipeRepeatRequestOnSameLink models the stress-test-nomadnet topology
// precisely: a node on tsA, a client tsB joined by a REAL interface pair
// (pipes), the client opening Nconcurrent links to the node and barraging each
// with sequential page requests. The stress tool (go-reticulum client) sees
// each link answer ~1-4 requests then die, regardless of whether the server is
// gonomadnet or Python nomadnet — so the defect is in the go-reticulum CLIENT
// link. This test reproduces it over a real interface (vs the in-process local
// interface, which did not reproduce).
func TestPipeRepeatRequestOnSameLink(t *testing.T) {
	testutils.SkipShortIntegration(t)

	logger := rns.NewLogger()
	logger.SetLogLevel(rns.LogError)
	if os.Getenv("RNS_TEST_VERBOSE") != "" {
		logger.SetLogLevel(rns.LogDebug)
	}

	tsA, cleanupA := newStartedTSLogger(t, logger)
	defer cleanupA()
	tsB, cleanupB := newStartedTSLogger(t, logger)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	pageSize := 200
	if s := os.Getenv("RNS_TEST_PAGE_BYTES"); s != "" {
		fmt.Sscanf(s, "%d", &pageSize)
	}
	concurrency := 1
	if s := os.Getenv("RNS_TEST_CONCURRENCY"); s != "" {
		fmt.Sscanf(s, "%d", &concurrency)
	}
	requestsPerLink := 8
	if s := os.Getenv("RNS_TEST_REQUESTS"); s != "" {
		fmt.Sscanf(s, "%d", &requestsPerLink)
	}
	t.Logf("pageBytes=%d concurrency=%d requests/link=%d", pageSize, concurrency, requestsPerLink)

	dir := tempDirInt(t)
	writeFile(t, dir+"/index.mu", ">> Pipe\n\n"+strings.Repeat("x", pageSize)+"\nEND\n")
	n := NewNode("PipeNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if tsB.HasPath(n.Destination().Hash) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !tsB.HasPath(n.Destination().Hash) {
		t.Fatal("client never learned path to node")
	}

	reqTimeout := 30 * time.Second
	linkDeadline := 15 * time.Second

	var wg sync.WaitGroup
	type linkResult struct {
		linkID   int
		ok, fail int
		lastRTT  time.Duration
		closedAt int
		results  []string
	}
	results := make([]linkResult, concurrency)

	for li := 0; li < concurrency; li++ {
		wg.Add(1)
		go func(li int) {
			defer wg.Done()
			lr := linkResult{linkID: li, closedAt: -1}

			outDest, err := rns.NewDestination(tsB, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
			if err != nil {
				lr.results = append(lr.results, fmt.Sprintf("NewDestination: %v", err))
				results[li] = lr
				return
			}
			link, err := rns.NewLink(tsB, outDest)
			if err != nil {
				lr.results = append(lr.results, fmt.Sprintf("NewLink: %v", err))
				results[li] = lr
				return
			}
			established := make(chan struct{}, 1)
			closed := make(chan struct{}, 1)
			link.SetLinkEstablishedCallback(func(*rns.Link) {
				select {
				case established <- struct{}{}:
				default:
				}
			})
			link.SetLinkClosedCallback(func(*rns.Link) {
				select {
				case closed <- struct{}{}:
				default:
				}
			})
			if err := link.Establish(); err != nil {
				lr.results = append(lr.results, fmt.Sprintf("Establish: %v", err))
				results[li] = lr
				return
			}
			select {
			case <-established:
			case <-closed:
				lr.results = append(lr.results, "closed before established")
				results[li] = lr
				return
			case <-time.After(linkDeadline):
				lr.results = append(lr.results, "establish timeout")
				go func() { defer func() { _ = recover() }(); link.Teardown() }()
				results[li] = lr
				return
			}

			for i := 1; i <= requestsPerLink; i++ {
				got := make(chan struct {
					bytes int
					fail  string
				}, 2)
				start := time.Now()
				_, err := link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
					if data, ok := rr.Response.([]byte); ok {
						select {
						case got <- struct {
							bytes int
							fail  string
						}{bytes: len(data)}:
						default:
						}
					} else {
						select {
						case got <- struct {
							bytes int
							fail  string
						}{fail: fmt.Sprintf("status=%v", rr.Status)}:
						default:
						}
					}
				}, func(rr *rns.RequestReceipt) {
					select {
					case got <- struct {
						bytes int
						fail  string
					}{fail: fmt.Sprintf("failed status=%v", rr.Status)}:
					default:
					}
				}, nil, reqTimeout, 0)
				if err != nil {
					lr.results = append(lr.results, fmt.Sprintf("req%d: send err %v", i, err))
					lr.fail++
					break
				}
				select {
				case r := <-got:
					if r.fail != "" {
						lr.results = append(lr.results, fmt.Sprintf("req%d: %s (%v)", i, r.fail, time.Since(start)))
						lr.fail++
						// A failure likely means the link is dead; stop.
						goto done
					}
					lr.ok++
					lr.lastRTT = time.Since(start)
					lr.results = append(lr.results, fmt.Sprintf("req%d: ok %dB %v", i, r.bytes, time.Since(start)))
				case <-closed:
					lr.closedAt = i
					lr.results = append(lr.results, fmt.Sprintf("req%d: LINK CLOSED (%v)", i, time.Since(start)))
					goto done
				case <-time.After(reqTimeout + 2*time.Second):
					lr.results = append(lr.results, fmt.Sprintf("req%d: TIMEOUT (%v)", i, time.Since(start)))
					lr.fail++
					goto done
				}
			}
		done:
			results[li] = lr
			go func() { defer func() { _ = recover() }(); link.Teardown() }()
		}(li)
	}
	wg.Wait()

	totalOK, totalFail := 0, 0
	for _, lr := range results {
		totalOK += lr.ok
		totalFail += lr.fail
		t.Logf("link %d: ok=%d fail=%d closedAt=%v lastRTT=%v", lr.linkID, lr.ok, lr.fail, lr.closedAt, lr.lastRTT)
		for _, r := range lr.results {
			t.Logf("  link %d: %s", lr.linkID, r)
		}
	}
	t.Logf("TOTAL ok=%d fail=%d", totalOK, totalFail)

	// Every request on every link should succeed (server never closes; client
	// doesn't teardown between requests). Any failure reproduces the bug.
	if totalFail != 0 {
		t.Fatalf("reproduced link-dies-after-N: %d ok, %d fail (see logs)", totalOK, totalFail)
	}
}

// newStartedTSLogger is newStartedTS but with a caller-supplied logger so the
// verbose flag propagates to both transport systems.
func newStartedTSLogger(t *testing.T, logger *rns.Logger) (*rns.TransportSystem, func()) {
	t.Helper()
	dir := testutils.TempDir(t, "pipe-repeat-ts")
	cfgDir := dir + "/config"
	writeRNSConfigRaw(t, cfgDir, "No", "4")
	ts := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(ts, cfgDir); err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}
	return ts, func() { ts.Stop() }
}
