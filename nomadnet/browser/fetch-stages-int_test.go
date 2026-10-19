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

//go:build integration

package browser

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/testutils"
)

// stageRecorder collects fetch stages under a mutex so a test can assert the
// exact lifecycle sequence.
type stageRecorder struct {
	mu     sync.Mutex
	stages []FetchStage
}

func (r *stageRecorder) report(stage FetchStage) {
	r.mu.Lock()
	r.stages = append(r.stages, stage)
	r.mu.Unlock()
}

func (r *stageRecorder) snapshot() []FetchStage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]FetchStage(nil), r.stages...)
}

func assertStages(t *testing.T, got, want []FetchStage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("stages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stage %v = %v, want %v (full sequence: %v)", i, got[i], want[i], got)
		}
	}
}

// TestIntegrationFetchStagesKnownPath pins the lifecycle stages a fetch reports
// when the destination path is already known: Python goes straight from
// establishing the link to the request (Browser.py:1385-1447), so NO_PATH and
// PATH_REQUESTED never appear.
func TestIntegrationFetchStagesKnownPath(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-node")
	n := node.NewNode("BrowserStagesKnown", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}
	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	var rec stageRecorder
	if _, _, _, err := FetchPageReuseLink(context.Background(), tsClient, nodeHash,
		"/page/index.mu", nil, 15*time.Second, nil, nil, rec.report, nil); err != nil {
		t.Fatalf("FetchPageReuseLink: %v", err)
	}

	assertStages(t, rec.snapshot(), []FetchStage{
		FetchStagePathKnown,
		FetchStageEstablishingLink,
		FetchStageLinkEstablished,
		FetchStageRequesting,
		FetchStageRequestSent,
	})
}

// TestIntegrationFetchStagesUnknownPath drives the path-request branch: with no
// path known the fetch reports NO_PATH, then PATH_REQUESTED (Python
// Browser.py:1385-1389), and once an announce supplies the path it proceeds
// through establishment to the request.
func TestIntegrationFetchStagesUnknownPath(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-node")
	n := node.NewNode("BrowserStagesUnknown", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	nodeHash := n.Destination().Hash
	if tsClient.HasPath(nodeHash) {
		t.Fatal("client already knows the path; the path-request branch cannot be driven")
	}

	stageCh := make(chan FetchStage, 16)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := FetchPageReuseLink(context.Background(), tsClient, nodeHash,
			"/page/index.mu", nil, 15*time.Second, nil, nil,
			func(stage FetchStage) { stageCh <- stage }, nil)
		done <- err
	}()

	// Announce only after the fetch has reported that it requested the path,
	// so the NO_PATH/PATH_REQUESTED pair is deterministic.
	var stages []FetchStage
	timeout := time.After(10 * time.Second)
requested:
	for {
		select {
		case st := <-stageCh:
			stages = append(stages, st)
			if st == FetchStagePathRequested {
				break requested
			}
		case <-timeout:
			t.Fatalf("timed out waiting for PATH_REQUESTED; stages so far: %v", stages)
		}
	}

	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("FetchPageReuseLink: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for the fetch to finish after the announce")
	}
	close(stageCh)
	for st := range stageCh {
		stages = append(stages, st)
	}

	assertStages(t, stages, []FetchStage{
		FetchStageNoPath,
		FetchStagePathRequested,
		FetchStagePathKnown,
		FetchStageEstablishingLink,
		FetchStageLinkEstablished,
		FetchStageRequesting,
		FetchStageRequestSent,
	})
}
