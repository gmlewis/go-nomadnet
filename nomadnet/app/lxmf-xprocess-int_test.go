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
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// pythonLXMFSenderScript is a minimal Python LXMF *sender* (the Go app is the
// receiver). It:
//   - brings up RNS over a config-supplied TCPServerInterface,
//   - creates an identity + an LXMF router + a delivery identity (so the Go
//     side sees a real lxmf.delivery source hash),
//   - prints PYTHON_SOURCE=<hex> (the sender's delivery-destination hash) +
//     READY=1,
//   - waits for the Go app to publish its LXMF delivery hash (read from a file),
//     recalls the Go identity once the announce arrives, and sends a single
//     OPPORTUNISTIC LXMF message ("Hello from Python!") to the Go lxmf.delivery
//     destination — a single RNS packet, no link establishment (the Go router's
//     delivery destination has a packet callback, go-reticulum
//     lxmf/router.go:1672),
//   - prints MSG_SENT=1 + MSG_HASH=<hex> (the message hash = the on-disk
//     filename the Go side will write) and lingers so the packet is flushed.
//
// Stamp enforcement is off (Go's RequiredStampCost defaults to nil), so a
// stampless opportunistic message is accepted — matching Python's default
// (enforce_stamps=False).
const pythonLXMFSenderScript = `import os, sys, time, signal
signal.signal(signal.SIGINT, lambda *a: os._exit(0))
signal.signal(signal.SIGTERM, lambda *a: os._exit(0))
import RNS, LXMF

rns_cfg = sys.argv[1]
hash_file = sys.argv[2]
storage = sys.argv[3]

reticulum = RNS.Reticulum(configdir=rns_cfg)
identity = RNS.Identity()
router = LXMF.LXMRouter(identity=identity, storagepath=storage, autopeer=False)
delivery_dest = router.register_delivery_identity(identity, display_name="PySender")
print("PYTHON_SOURCE=" + RNS.hexrep(delivery_dest.hash, delimit=False), flush=True)
print("READY=1", flush=True)

# Wait for the Go side to publish its LXMF delivery destination hash.
deadline = time.time() + 20
go_hash_hex = None
while time.time() < deadline:
    try:
        go_hash_hex = open(hash_file).read().strip()
        if go_hash_hex:
            break
    except Exception:
        pass
    time.sleep(0.1)
if not go_hash_hex:
    print("NO_GO_HASH=1", flush=True); sys.exit(1)
go_hash = bytes.fromhex(go_hash_hex)

# Wait until the Go announce has arrived (recall succeeds + a path exists).
deadline = time.time() + 30
go_identity = None
while time.time() < deadline:
    go_identity = RNS.Identity.recall(go_hash)
    if go_identity is not None and RNS.Transport.has_path(go_hash):
        break
    time.sleep(0.2)
if go_identity is None:
    print("NO_RECALL=1", flush=True); sys.exit(1)

go_dest = RNS.Destination(go_identity, RNS.Destination.OUT, RNS.Destination.SINGLE, "lxmf", "delivery")
msg = LXMF.LXMessage(go_dest, delivery_dest, content="Hello from Python!",
                     desired_method=LXMF.LXMessage.OPPORTUNISTIC)
router.handle_outbound(msg)
# Give the opportunistic packet time to be emitted over the TCP interface.
time.sleep(2)
print("MSG_HASH=" + RNS.hexrep(msg.hash, delimit=False), flush=True)
print("MSG_SENT=1", flush=True)
# Linger so the underlying RNS send completes before the process exits.
time.sleep(3)
`

// TestIntegrationLXMFReceiveFromPython verifies the cross-process
// parity goal: a Python sender sends an LXMF message over a real TCP RNS
// transport to the Go app; the Go app ingests it into a conversation whose
// on-disk layout matches Python's — conversations/<source_hash_hex>/
// <message_hash_hex> holding the msgpack {state, lxmf_bytes,
// transport_encrypted, transport_encryption, method} container
// (nomadnet/conversation/cache.go Ingest + lxmf.Message.WriteToDirectory).
//
// The Go lxmf.delivery destination has a packet callback (go-reticulum
// lxmf/router.go:1672), so the Python sender uses an OPPORTUNISTIC single-packet
// LXMF message (no link establishment). The on-disk message-hash filename must
// equal Python's msg.hash — pinning LXMF message-hash parity across the wire.
func TestIntegrationLXMFReceiveFromPython(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)
	pyPath := findLXMFPython(t)
	if pyPath == "" {
		t.Skip("no python interpreter with RNS + LXMF available; skipping cross-process LXMF test")
	}

	// Reserve a TCP port for the Python TCPServerInterface (the Go app connects
	// to it as a TCPClient).
	port := reservePort(t)

	// Python-side dirs: RNS config (TCPServer) + LXMF storage.
	pyDir := testutils.TempDir(t, "nomadnet-lxmf-xproc-py")
	pyCfg := filepath.Join(pyDir, "rnsconfig")
	writePythonRNSConfigTCPServer(t, pyCfg, port)
	pyStorage := filepath.Join(pyDir, "lxmfstorage")
	if err := os.MkdirAll(pyStorage, 0o755); err != nil {
		t.Fatal(err)
	}
	hashFile := filepath.Join(pyDir, "gohash")

	scriptPath := filepath.Join(pyDir, "lxmf_sender.py")
	if err := os.WriteFile(scriptPath, []byte(pythonLXMFSenderScript), 0o644); err != nil {
		t.Fatal(err)
	}

	pyCmd := exec.Command(pyPath, scriptPath, pyCfg, hashFile, pyStorage)
	pyStdout, err := pyCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	pyCmd.Stderr = os.Stderr
	if err := pyCmd.Start(); err != nil {
		t.Fatalf("start python sender: %v", err)
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

	// Wait for the Python sender to be up and report its delivery-source hash.
	lb.waitFor(t, "READY=", 10*time.Second)
	pySource := lb.waitFor(t, "PYTHON_SOURCE=", 10*time.Second)
	// RNS destination hashes are truncated (16 bytes → 32 hex); the sender's
	// lxmf.delivery destination hash is the conversation source key.
	if len(pySource) != 32 {
		t.Fatalf("PYTHON_SOURCE = %q (len %v), want 32 hex chars", pySource, len(pySource))
	}
	pySourceBytes, err := hex.DecodeString(pySource)
	if err != nil {
		t.Fatalf("decode PYTHON_SOURCE %q: %v", pySource, err)
	}

	// Go app transport (single TransportSystem; the TCP client is registered
	// onto it so announces + the inbound delivery packet route through the Go
	// RNS stack, mirroring newStartedTSWithTCPClient from the rrc harness).
	goDir := testutils.TempDir(t, "nomadnet-lxmf-xproc-go")
	// Private config with enable_node = no so InitWithTransport does not
	// auto-start a node (this test exercises LXMF delivery, not hosting; see
	// writeTestNomadNetConfig).
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
	// Wait for the TCP client to connect before announcing (poll the interface
	// status; the fixed sleep it replaces always paid 500ms).
	if !testutils.PollUntil(5*time.Second, func() bool { return goIface.Status() }) {
		t.Fatalf("Go TCP client interface never connected")
	}

	appGo := NewAppWithTransport(goDir, WithTransport(ts), WithIdentity(ts.Identity()))
	if err := appGo.InitWithTransport(ts, ts.Identity()); err != nil {
		t.Fatalf("InitWithTransport Go: %v", err)
	}
	defer appGo.Shutdown()

	// Capture the delivered message on a channel for a fast signal.
	receivedCh := make(chan *lxmf.Message, 1)
	appGo.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	// Publish the Go LXMF delivery hash so Python can recall it + send.
	if err := os.WriteFile(hashFile, []byte(hex.EncodeToString(appGo.LXMFDest.Hash)), 0o644); err != nil {
		t.Fatal(err)
	}
	stopAnn := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopAnn:
				return
			default:
				_ = appGo.Router.Announce(appGo.LXMFDest.Hash)
				time.Sleep(1 * time.Second)
			}
		}
	}()
	defer close(stopAnn)

	// Wait for Python to send the message.
	lb.waitFor(t, "MSG_SENT=", 40*time.Second)
	msgHashHex := lb.waitFor(t, "MSG_HASH=", 5*time.Second)

	// Wait for the Go app to ingest the delivered message (callback fires when
	// the router delivers the opportunistic packet).
	var got *lxmf.Message
	select {
	case got = <-receivedCh:
	case <-time.After(20 * time.Second):
		t.Fatal("Go app did not deliver the Python-sent LXMF message via DeliveryCallback")
	}
	if got.ContentString() != "Hello from Python!" {
		t.Errorf("delivered content = %q, want %q", got.ContentString(), "Hello from Python!")
	}

	// Verify the on-disk layout matches Python's: the message lives at
	// <conversationpath>/<source_hash_hex>/<message_hash_hex>.
	convDir := filepath.Join(appGo.ConversationPath, pySource)
	deadline := time.Now().Add(5 * time.Second)
	var onDisk string
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(convDir)
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				// LXMF message hash = FullHash (32 bytes → 64 hex), matching
				// Python's RNS.Identity.full_hash (LXMessage.py:368) — the
				// on-disk filename (LXMessage.py:675) is this 64-hex hash.
				if len(name) == 64 && isHex(name) {
					onDisk = name
					break
				}
			}
		}
		if onDisk != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if onDisk == "" {
		t.Fatalf("no message file found under %v", convDir)
	}
	if onDisk != msgHashHex {
		t.Errorf("on-disk filename = %q, want Python MSG_HASH %q (LXMF message-hash parity)", onDisk, msgHashHex)
	}

	// Verify the ingested message loads back through the production path with
	// the right content + source hash.
	deadline = time.Now().Add(5 * time.Second)
	var msgs []conversation.MessageDisplayData
	for time.Now().Before(deadline) {
		msgs = appGo.ConversationMessages(pySource)
		if len(msgs) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(msgs) != 1 {
		t.Fatalf("ConversationMessages(%v) = %v entries, want 1", pySource, len(msgs))
	}
	if msgs[0].Content != "Hello from Python!" {
		t.Errorf("loaded content = %q, want %q", msgs[0].Content, "Hello from Python!")
	}
	if !bytes.Equal(msgs[0].SourceHash, pySourceBytes) {
		t.Errorf("loaded source hash = %x, want Python delivery hash %x", msgs[0].SourceHash, pySourceBytes)
	}
}

// --- small helpers (local copies of the rrc-xprocess_test harness, which
// lives in package rrc and is not importable from package app). ---

func findLXMFPython(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"python3.14", "python3"} {
		path, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		check := exec.Command(path, "-c", "import RNS, LXMF")
		if err := check.Run(); err == nil {
			return path
		}
	}
	return ""
}

func reservePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func writePythonRNSConfigTCPServer(t *testing.T, dir string, port int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`[reticulum]
share_instance = No

[logging]
loglevel = 4

[interfaces]

  [[TCP Server]]
    type = TCPServerInterface
    enabled = Yes
    listen_ip = 127.0.0.1
    listen_port = %v
`, port)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// xlineBuffer collects the Python subprocess's stdout lines for polling.
type xlineBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (lb *xlineBuffer) push(line string) {
	lb.mu.Lock()
	lb.lines = append(lb.lines, line)
	lb.mu.Unlock()
}

func (lb *xlineBuffer) waitFor(t *testing.T, prefix string, timeout time.Duration) string {
	t.Helper()
	var v string
	if !testutils.PollUntil(timeout, func() bool {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		for _, l := range lb.lines {
			if strings.HasPrefix(l, prefix) {
				v = strings.TrimSpace(strings.TrimPrefix(l, prefix))
				return true
			}
		}
		return false
	}) {
		lb.mu.Lock()
		all := strings.Join(lb.lines, "\n")
		lb.mu.Unlock()
		t.Fatalf("timeout waiting for %q; python stdout:\n%v", prefix, all)
	}
	return v
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
