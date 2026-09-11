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

	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// pythonNodeFormFetchScript is the Python RNS source of truth for a form
// submission against a .wasm executable page: a Python client establishes a
// Link to the Go node's nomadnetwork.node destination and calls
// link.request("/page/dynamic-page.wasm", data={...}) with a dict, exactly as
// Python's Browser does when a Micron form is submitted (Browser.py:1436
// passes request_data straight through). It prints RECV_HEX=<hex> of the raw
// Micron bytes the node renders, then DONE=1.
const pythonNodeFormFetchScript = `import os, sys, time, signal
signal.signal(signal.SIGINT, lambda *a: os._exit(0))
signal.signal(signal.SIGTERM, lambda *a: os._exit(0))
import RNS

configdir = sys.argv[1]
node_hash_file = sys.argv[2]

reticulum = RNS.Reticulum(configdir)
identity = RNS.Identity()
print("READY=1", flush=True)

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

# A Micron form submission: field_<name> entries collected from the form
# widgets plus any var_<k>=<v> entries carried by the submit link.
form_data = {"field_name": "glenn", "field_message": "hello world", "var_topic": "wasm", "ignored": 123}
link.request("/page/dynamic-page.wasm", data=form_data, response_callback=got, failed_callback=failed)

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

// TestIntegrationNodeFormFieldsPythonToGo pins the cross-process form-field
// path end to end: a Python RNS client submits a dict of Micron form fields to
// a .wasm executable page hosted by the Go node over a real TCP RNS link, and
// the rendered page carries the field_*/var_* entries as its request_data map
// while unrelated entries are dropped (Python Node.py:168-172).
func TestIntegrationNodeFormFieldsPythonToGo(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)
	if !wasmpages.Enabled() {
		t.Skip("wasm pages are not linked in this build (-tags wago); skipping cross-process form-field test")
	}
	pyPath := findLXMFPython(t)
	if pyPath == "" {
		t.Skip("no python interpreter with RNS available; skipping cross-process form-field test")
	}

	port := reservePort(t)

	pyDir := testutils.TempDir(t, "nomadnet-node-form-py")
	pyCfg := filepath.Join(pyDir, "rnsconfig")
	writePythonRNSConfigTCPServer(t, pyCfg, port)
	hashFile := filepath.Join(pyDir, "nodehash")

	scriptPath := filepath.Join(pyDir, "node_form_fetch.py")
	if err := os.WriteFile(scriptPath, []byte(pythonNodeFormFetchScript), 0o644); err != nil {
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

	goDir := testutils.TempDir(t, "nomadnet-node-form-go")
	writeTestNomadNetConfig(t, goDir)
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
	if !testutils.PollUntil(5*time.Second, func() bool { return goIface.Status() }) {
		t.Fatalf("Go TCP client interface never connected")
	}

	appGo := NewAppWithTransport(goDir, WithTransport(ts), WithIdentity(ts.Identity()))
	if err := appGo.InitWithTransport(ts, ts.Identity()); err != nil {
		t.Fatalf("InitWithTransport Go: %v", err)
	}
	defer appGo.Shutdown()

	// The node scans its pages directory at start, so the .wasm page must be
	// on disk before startNode registers request handlers for it.
	if err := os.MkdirAll(appGo.PagesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	wasmSrc, err := os.ReadFile(sharedWasmPageFixturePath(t))
	if err != nil {
		t.Fatalf("read shared wasm fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appGo.PagesPath, "dynamic-page.wasm"), wasmSrc, 0o644); err != nil {
		t.Fatal(err)
	}

	appGo.EnableNode = true
	appGo.NodeName = "GoNode"
	appGo.NodeAnnounceAtStart = false
	if err := appGo.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}
	if appGo.Node == nil {
		t.Fatal("appGo.Node is nil after startNode")
	}

	nodeHash := rns.CalculateHash(appGo.Identity, "nomadnetwork", "node")
	if err := os.WriteFile(hashFile, []byte(hex.EncodeToString(nodeHash)), 0o644); err != nil {
		t.Fatal(err)
	}

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

	lb.waitFor(t, "DONE=", 60*time.Second)
	recvHex := lb.waitFor(t, "RECV_HEX=", 5*time.Second)
	recvBytes, err := hex.DecodeString(strings.TrimSpace(recvHex))
	if err != nil {
		t.Fatalf("decode RECV_HEX: %v", err)
	}
	rendered := string(recvBytes)
	if !strings.HasPrefix(rendered, ">WASM PAGE\n----\n") {
		t.Fatalf("rendered page = %q, want the wasm fixture prefix", rendered)
	}
	wantRequestData := `"request_data":{"field_message":"hello world","field_name":"glenn","var_topic":"wasm"}`
	if !strings.Contains(rendered, wantRequestData) {
		t.Errorf("rendered page does not carry the submitted form fields.\nrendered:\n%v\nwant substring:\n%v", rendered, wantRequestData)
	}
	if strings.Contains(rendered, "ignored") {
		t.Errorf("rendered page carries the non-field entry: %v", rendered)
	}
}

// sharedWasmPageFixturePath returns the path of the dynamic-page.wasm fixture
// shipped in the repository's wasm-pages asset directory, so the cross-process
// tests render the same module a user installs.
func sharedWasmPageFixturePath(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("..", "..", "assets", "wasm-pages", "dynamic-page.wasm")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("resolve %v: %v", rel, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("wasm page fixture missing: %v", err)
	}
	return abs
}
