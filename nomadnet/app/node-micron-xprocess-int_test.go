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

package app

import (
	"bufio"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// pythonNodeFetchScript is a minimal Python RNS *client* that fetches a Micron
// page from a remote nomadnetwork.node (here the Go app) over a real RNS Link,
// using RNS's generic request/response facility (the same path Python's own
// Browser uses, Browser.py:1436-1442). It:
//   - brings up RNS over a config-supplied TCPServerInterface (the Go app
//     connects to it as a TCPClient, so announces + the link traverse the TCP
//     RNS transport),
//   - prints READY=1,
//   - waits for the Go app to publish its nomadnetwork.node destination hash,
//   - recalls the Go node identity once the announce arrives, establishes an
//     RNS Link to the "nomadnetwork.node" destination, and issues a
//     link.request("/page/index.mu", data=None) — the same call Python's
//     Browser makes (Browser.py:1436),
//   - prints RECV_LEN=<n> + RECV_HEX=<hex> of the raw Micron bytes the node
//     returns, then DONE=1.
//
// The node serves the raw .mu source bytes (Node.serve_page → os.ReadFile, or
// the DefaultIndex Micron string when no index.mu exists); RNS carries them as
// the second element of a msgpack [request_id, response] array (go-reticulum
// rns/link.go:1551-1578, Python RNS/Link.py:897), so receipt.response is the
// exact bytes on disk.
const pythonNodeFetchScript = `import os, sys, time, signal
signal.signal(signal.SIGINT, lambda *a: os._exit(0))
signal.signal(signal.SIGTERM, lambda *a: os._exit(0))
import RNS

configdir = sys.argv[1]
node_hash_file = sys.argv[2]

reticulum = RNS.Reticulum(configdir)
identity = RNS.Identity()
print("READY=1", flush=True)

# Wait for the Go side to publish its nomadnetwork.node destination hash.
deadline = time.time() + 25
node_hash_hex = None
while time.time() < deadline:
    try:
        node_hash_hex = open(node_hash_file).read().strip()
        if node_hash_hex:
            break
    except Exception:
        pass
    time.sleep(0.1)
if not node_hash_hex:
    print("NO_NODE_HASH=1", flush=True); sys.exit(1)
node_hash = bytes.fromhex(node_hash_hex)

# Wait until the Go node announce has arrived (recall succeeds + a path exists).
deadline = time.time() + 40
node_identity = None
while time.time() < deadline:
    node_identity = RNS.Identity.recall(node_hash)
    if node_identity is not None and RNS.Transport.has_path(node_hash):
        break
    if node_identity is None:
        try:
            RNS.Transport.request_path(node_hash)
        except Exception:
            pass
    time.sleep(0.2)
if node_identity is None:
    print("NO_RECALL=1", flush=True); sys.exit(1)

dest = RNS.Destination(node_identity, RNS.Destination.OUT, RNS.Destination.SINGLE, "nomadnetwork", "node")
link = RNS.Link(dest)
deadline = time.time() + 40
while time.time() < deadline:
    if link.status == RNS.Link.ACTIVE:
        break
    time.sleep(0.1)
if link.status != RNS.Link.ACTIVE:
    print("NO_LINK=1", flush=True); sys.exit(1)

result = {}
def got(receipt):
    result["data"] = receipt.response
def failed(receipt):
    result["error"] = True

link.request("/page/index.mu", data=None, response_callback=got, failed_callback=failed)

deadline = time.time() + 40
while time.time() < deadline:
    if "data" in result or "error" in result:
        break
    time.sleep(0.1)

if "error" in result:
    print("REQ_FAILED=1", flush=True); sys.exit(1)
if "data" not in result:
    print("REQ_TIMEOUT=1", flush=True); sys.exit(1)

data = result["data"]
if isinstance(data, str):
    data = data.encode("utf-8")
print("RECV_LEN=" + str(len(data)), flush=True)
print("RECV_HEX=" + data.hex(), flush=True)
print("DONE=1", flush=True)
time.sleep(1)
`

// TestIntegrationNodeServesMicronToPython verifies the Phase 6 cross-process
// parity goal: a Go nomadnet node serves a Micron page, and a Python RNS client
// fetches it over a real TCP RNS transport and receives byte-identical bytes.
//
// The Go node hosts a "nomadnetwork.node" destination (node.go:129) and
// registers an RNS request handler at /page/index.mu returning the raw .mu
// source bytes — here the DefaultIndex Micron string (served when no index.mu
// exists in the pages directory, node.go:246-248). The Python client establishes
// an RNS Link to the node and calls link.request("/page/index.mu"), the same
// call Python's own Browser makes (Browser.py:1436). The received bytes must
// equal node.DefaultIndex, which the unit test TestServeDefaultIndexPythonParity
// pins to Python's Node.DEFAULT_INDEX (testdata/py_default_index.mu) — so this
// test pins both the cross-process wire path AND the byte parity across the
// Go↔Python RNS link.
func TestIntegrationNodeServesMicronToPython(t *testing.T) {
	testutils.SkipShortIntegration(t)
	pyPath := findLXMFPython(t)
	if pyPath == "" {
		t.Skip("no python interpreter with RNS + LXMF available; skipping cross-process node-serving test")
	}

	port := reservePort(t)

	pyDir := testutils.TempDir(t, "nomadnet-node-xproc-py")
	pyCfg := filepath.Join(pyDir, "rnsconfig")
	writePythonRNSConfigTCPServer(t, pyCfg, port)
	hashFile := filepath.Join(pyDir, "nodehash")

	scriptPath := filepath.Join(pyDir, "node_fetch.py")
	if err := os.WriteFile(scriptPath, []byte(pythonNodeFetchScript), 0o644); err != nil {
		t.Fatal(err)
	}

	pyCmd := exec.Command(pyPath, scriptPath, pyCfg, hashFile)
	pyStdout, err := pyCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	pyCmd.Stderr = os.Stderr
	if err := pyCmd.Start(); err != nil {
		t.Fatalf("start python fetcher: %v", err)
	}
	t.Cleanup(func() { _ = pyCmd.Process.Kill(); _, _ = pyCmd.Process.Wait() })

	lb := &xlineBuffer{}
	go func() {
		scanner := bufio.NewScanner(pyStdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			lb.push(scanner.Text())
		}
	}()

	lb.waitFor(t, "READY=", 10*time.Second)

	// Go app transport: a single TransportSystem with a TCP client to the
	// Python TCPServer, so the node announce + the inbound link traverse the
	// TCP RNS transport (mirrors newStartedTSWithTCPClient from the rrc harness).
	goDir := testutils.TempDir(t, "nomadnet-node-xproc-go")
	ts, tsCleanup := newStartedTSApp(t, goDir)
	defer tsCleanup()
	handler := func(data []byte, iface interfaces.Interface) {
		ts.Inbound(data, iface)
	}
	goIface, err := interfaces.NewTCPClientInterface("go_tcp", "127.0.0.1", port, false, handler)
	if err != nil {
		t.Fatalf("NewTCPClientInterface: %v", err)
	}
	ts.RegisterInterface(goIface)
	defer func() { _ = goIface.Detach() }()
	time.Sleep(500 * time.Millisecond)

	appGo := NewAppWithTransport(goDir, WithTransport(ts), WithIdentity(ts.Identity()))
	if err := appGo.InitWithTransport(ts, ts.Identity()); err != nil {
		t.Fatalf("InitWithTransport Go: %v", err)
	}
	defer appGo.Shutdown()

	// Enable node hosting and start the node explicitly (announce-at-start off;
	// the test announces manually for determinism), mirroring
	// TestAppNodeHostingStartsAndAnnounces.
	appGo.EnableNode = true
	appGo.NodeName = "GoNode"
	appGo.NodeAnnounceAtStart = false
	if err := appGo.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}
	if appGo.Node == nil {
		t.Fatal("appGo.Node is nil after startNode")
	}

	// The node's pages directory is empty (fresh storage) → /page/index.mu
	// serves DefaultIndex. Ensure no index.mu lingers from a prior run.
	if appGo.PagesPath != "" {
		_ = os.Remove(filepath.Join(appGo.PagesPath, "index.mu"))
	}

	// Publish the Go nomadnetwork.node destination hash so Python can recall it.
	nodeHash := rns.CalculateHash(appGo.Identity, "nomadnetwork", "node")
	if err := os.WriteFile(hashFile, []byte(hex.EncodeToString(nodeHash)), 0o644); err != nil {
		t.Fatal(err)
	}

	// Announce the node in a loop so Python receives + recalls the identity.
	stopAnn := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopAnn:
				return
			default:
				_ = appGo.Node.Announce()
				time.Sleep(1 * time.Second)
			}
		}
	}()
	defer close(stopAnn)

	// Wait for Python to fetch the page.
	lb.waitFor(t, "DONE=", 60*time.Second)
	recvLen := lb.waitFor(t, "RECV_LEN=", 5*time.Second)
	recvHex := lb.waitFor(t, "RECV_HEX=", 5*time.Second)

	recvBytes, err := hex.DecodeString(strings.TrimSpace(recvHex))
	if err != nil {
		t.Fatalf("decode RECV_HEX: %v", err)
	}
	if string(recvBytes) != node.DefaultIndex {
		t.Errorf("received page bytes (len %s) != node.DefaultIndex (len %d).\nreceived:\n%s\nwant:\n%s",
			strings.TrimSpace(recvLen), len(node.DefaultIndex), recvBytes, node.DefaultIndex)
	}
	// DefaultIndex == Python DEFAULT_INDEX is pinned by
	// nomadnet/node.TestServeDefaultIndexPythonParity (testdata/py_default_index.mu);
	// re-asserting it here keeps the cross-process test self-contained. `go
	// test` runs with the package dir as cwd (nomadnet/app).
	pyDefault, err := os.ReadFile(filepath.Join("..", "node", "testdata", "py_default_index.mu"))
	if err != nil {
		t.Fatalf("read py_default_index.mu: %v", err)
	}
	if string(recvBytes) != string(pyDefault) {
		t.Errorf("received page bytes != Python DEFAULT_INDEX (testdata/py_default_index.mu)")
	}
}
