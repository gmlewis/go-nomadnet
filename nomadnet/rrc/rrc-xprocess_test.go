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

package rrc

import (
	"bufio"
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

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// findRNSPython returns the path of a Python interpreter that can import RNS
// and nomadnet.RRC (the vendored cbor comes with nomadnet). python3.14 has the
// installed packages on this host; python3 (3.13) does not. The test skips if
// no suitable interpreter is available.
func findRNSPython(t *testing.T) string {
	t.Helper()
	candidates := []string{"python3.14", "python3"}
	for _, c := range candidates {
		path, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		check := exec.Command(path, "-c", "import RNS, nomadnet.RRC")
		if err := check.Run(); err == nil {
			return path
		}
	}
	t.Skip("no python interpreter with RNS + nomadnet.RRC available; skipping cross-process RRC test")
	return ""
}

// reserveTCPPortXProc picks a free TCP listen port for the Python RNS
// TCPServerInterface. (Local copy — the go-reticulum helper is in package
// interfaces and unexported.)
func reserveTCPPortXProc(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserveTCPPort: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// writePythonRNSConfigWithTCPServer writes a minimal RNS config whose only
// interface is a TCPServerInterface listening on port. The Go side connects to
// it with a TCPClientInterface. Config format matches RNS's ConfigObj template
// (Reticulum.py:1947+): an [interfaces] section with an indented [[name]]
// subsection; type is the class name "TCPServerInterface" (Reticulum.py:961).
func writePythonRNSConfigWithTCPServer(t *testing.T, configDir string, port int) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
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
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newStartedTSWithTCPClient builds a Go RNS TransportSystem whose only interface
// is a TCPClientInterface connecting to host:port. Inbound bytes feed
// ts.Inbound so announces/links route through the Go RNS stack, mirroring the
// pipe wiring in rrc-int_test.go:newRRCPipes.
func newStartedTSWithTCPClient(t *testing.T, host string, port int) (*rns.TransportSystem, func()) {
	t.Helper()
	dir := testutils.TempDir(t, "nomadnet-rrc-xproc-ts")
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfigRRC(t, cfgDir)
	ts := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(ts, cfgDir); err != nil {
		t.Fatalf("NewReticulum error: %v", err)
	}
	handler := func(data []byte, iface interfaces.Interface) {
		ts.Inbound(data, iface)
	}
	goIface, err := interfaces.NewTCPClientInterface("go_tcp", host, port, false, handler)
	if err != nil {
		t.Fatalf("NewTCPClientInterface: %v", err)
	}
	ts.RegisterInterface(goIface)
	cleanup := func() {
		_ = goIface.Detach()
	}
	return ts, cleanup
}

// lineBuffer collects stdout lines from the Python subprocess under a mutex so
// the test goroutine can poll for expected markers (HASH=, HELLO_*, WELCOME_SENT).
type lineBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (lb *lineBuffer) push(line string) {
	lb.mu.Lock()
	lb.lines = append(lb.lines, line)
	lb.mu.Unlock()
}

// findLine returns the first line with the given prefix and its value (after
// "="), or ok=false if none has appeared yet.
func (lb *lineBuffer) findLine(prefix string) (string, bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for _, l := range lb.lines {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix), true
		}
	}
	return "", false
}

// waitForLine polls for a line with prefix within timeout, returning its value.
func (lb *lineBuffer) waitForLine(t *testing.T, prefix string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v, ok := lb.findLine(prefix); ok {
			return v
		}
		time.Sleep(50 * time.Millisecond)
	}
	lb.mu.Lock()
	got := strings.Join(lb.lines, "\n")
	lb.mu.Unlock()
	t.Fatalf("timed out waiting for %q; python stdout so far:\n%v", prefix, got)
	return ""
}

// pythonRRCServerScript is a minimal RRC *server* (the Python nomadnet package
// ships only an RRC client; the reference hub is the external rrcd). It:
//   - brings up RNS over a config-supplied TCPServerInterface,
//   - creates a SINGLE IN "rrc.chat" destination and announces it in a loop,
//   - prints HASH=<hex> so the Go client knows which destination to dial,
//   - on an inbound HELLO packet prints its golden fields (name/ver/caps/src/nick)
//     and replies with a WELCOME carrying hub name "PyHub", ver "0.1", empty caps,
//     and the standard limits {0:32, 1:64, 2:350, 3:32, 4:240} (RRC.py:73-82).
//
// It uses nomadnet's vendored cbor and the RRC protocol constants directly, so
// the wire bytes match the Python client contract byte-for-byte.
const pythonRRCServerScript = `
import sys, os, time, threading
import RNS
from nomadnet.vendor import cbor
import nomadnet.RRC as rrc

configdir = sys.argv[1]
reticulum = RNS.Reticulum(configdir)

identity = RNS.Identity()
dest = RNS.Destination(identity, RNS.Destination.IN, RNS.Destination.SINGLE, "rrc", "chat")

# The packet callback only receives (data, packet); the link itself is captured
# here when the link-established callback fires, so on_packet can reply on it.
link_ref = [None]

def _s(x):
    if isinstance(x, (bytes, bytearray)):
        return x.decode("utf-8")
    return str(x)

def on_link(link):
    link_ref[0] = link
    link.set_packet_callback(on_packet)

def on_packet(data, packet):
    try:
        env = cbor.decode(data)
    except Exception:
        return
    if not isinstance(env, dict):
        return
    if env.get(rrc.K_T) == rrc.T_HELLO:
        body = env.get(rrc.K_BODY, {}) or {}
        caps = body.get(rrc.B_HELLO_CAPS, {}) or {}
        src = env.get(rrc.K_SRC, b"")
        nick = env.get(rrc.K_NICK, "")
        print("HELLO_NAME=" + _s(body.get(rrc.B_HELLO_NAME)), flush=True)
        print("HELLO_VER=" + _s(body.get(rrc.B_HELLO_VER)), flush=True)
        print("HELLO_CAPS=" + ",".join(sorted(str(k) for k in caps.keys())), flush=True)
        src_hex = src.hex() if isinstance(src, (bytes, bytearray)) else ""
        print("HELLO_SRC=" + src_hex, flush=True)
        print("HELLO_NICK=" + _s(nick), flush=True)
        wbody = {
            rrc.B_WELCOME_HUB: b"PyHub",
            rrc.B_WELCOME_VER: b"0.1",
            rrc.B_WELCOME_CAPS: {},
            rrc.B_WELCOME_LIMITS: {
                rrc.L_MAX_NICK_BYTES: 32,
                rrc.L_MAX_ROOM_NAME_BYTES: 64,
                rrc.L_MAX_MSG_BODY_BYTES: 350,
                rrc.L_MAX_ROOMS_PER_SESSION: 32,
                rrc.L_RATE_LIMIT_MSGS_PER_MINUTE: 240,
            },
        }
        wenv = {
            rrc.K_V: rrc.RRC_VERSION,
            rrc.K_T: rrc.T_WELCOME,
            rrc.K_ID: os.urandom(8),
            rrc.K_TS: int(time.time() * 1000),
            rrc.K_BODY: wbody,
        }
        RNS.Packet(link_ref[0], cbor.encode(wenv)).send()
        print("WELCOME_SENT=1", flush=True)

dest.set_link_established_callback(on_link)

def announce_loop():
    while True:
        try:
            dest.announce()
        except Exception:
            pass
        time.sleep(2)

print("HASH=" + dest.hash.hex(), flush=True)
print("READY=1", flush=True)
threading.Thread(target=announce_loop, daemon=True).start()

while True:
    time.sleep(1)
`

// TestIntegrationXProcessHelloWelcome verifies the RRC HELLO/WELCOME handshake
// across a real Go↔Python RNS link bridged by a TCP RNS transport (task 2.1):
//
//   - A Python subprocess runs a minimal RRC server (rrc.chat, announced over a
//     TCPServerInterface).
//   - The Go RRC hub (client) connects over a TCPClientInterface, learns the
//     server identity from the announce, establishes the RNS link, and sends
//     HELLO.
//   - The Python server receives HELLO and replies WELCOME.
//   - The Go hub becomes Welcomed and parses the WELCOME body.
//
// Golden values are captured from the Python RRC contract (RRC.py:69-82,84-85):
// HELLO body name="nomadnet", ver="0.1", caps {0,1}; WELCOME body hub="PyHub",
// ver="0.1", caps={}, limits {0:32, 1:64, 2:350, 3:32, 4:240}.
func TestIntegrationXProcessHelloWelcome(t *testing.T) {
	testutils.SkipShortIntegration(t)
	pyPath := findRNSPython(t)

	pyPort := reserveTCPPortXProc(t)
	pyCfgDir := filepath.Join(testutils.TempDir(t, "nomadnet-rrc-xproc-py"), "config")
	writePythonRNSConfigWithTCPServer(t, pyCfgDir, pyPort)

	scriptPath := filepath.Join(filepath.Dir(pyCfgDir), "rrc_server.py")
	if err := os.WriteFile(scriptPath, []byte(pythonRRCServerScript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(pyPath, scriptPath, pyCfgDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	lb := &lineBuffer{}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			lb.push(scanner.Text())
		}
	}()

	// Wait for the Python server to publish its destination hash.
	hashHex := lb.waitForLine(t, "HASH=", 10*time.Second)
	serverHash, err := hex.DecodeString(strings.TrimSpace(hashHex))
	if err != nil || len(serverHash) == 0 {
		t.Fatalf("invalid HASH from python: %q (err %v)", hashHex, err)
	}

	// Bring up the Go RNS stack + TCP client and dial the Python server.
	ts, tsCleanup := newStartedTSWithTCPClient(t, "127.0.0.1", pyPort)
	defer tsCleanup()

	// Give the TCP client a moment to connect before relying on announces.
	time.Sleep(500 * time.Millisecond)

	mgr := NewManager(tempDirRRC(t), func() []byte { return ts.Identity().Hash })
	mgr.SetNickname("TestClient")
	hub := mgr.AddHub(serverHash, "rrc.chat", "PyHub")
	hub.SetTransport(ts)

	established := make(chan struct{}, 1)
	hub.SetOnLinkEstablished(func() {
		select {
		case established <- struct{}{}:
		default:
		}
	})

	hub.ConnectAsync()

	// Wait for link establishment, then WELCOME.
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for Go→Python link establishment")
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		welcomed := hub.Welcomed
		hub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	hub.lock.Lock()
	welcomed := hub.Welcomed
	hubName := hub.HubName
	hubVer := hub.HubVersion
	maxNick := hub.MaxNickBytes
	maxRoom := hub.MaxRoomNameBytes
	maxMsg := hub.MaxMsgBodyBytes
	maxRooms := hub.MaxRoomsPerSession
	rate := hub.RateLimitMsgsPerMin
	hub.lock.Unlock()

	if !welcomed {
		t.Fatal("Go hub did not receive WELCOME from Python server")
	}
	if hubName != "PyHub" {
		t.Errorf("WELCOME HubName = %q, want %q", hubName, "PyHub")
	}
	if hubVer != "0.1" {
		t.Errorf("WELCOME HubVersion = %q, want %q", hubVer, "0.1")
	}
	if maxNick != 32 {
		t.Errorf("WELCOME MaxNickBytes = %v, want 32", maxNick)
	}
	if maxRoom != 64 {
		t.Errorf("WELCOME MaxRoomNameBytes = %v, want 64", maxRoom)
	}
	if maxMsg != 350 {
		t.Errorf("WELCOME MaxMsgBodyBytes = %v, want 350", maxMsg)
	}
	if maxRooms != 32 {
		t.Errorf("WELCOME MaxRoomsPerSession = %v, want 32", maxRooms)
	}
	if rate != 240 {
		t.Errorf("WELCOME RateLimitMsgsPerMin = %v, want 240", rate)
	}

	// Assert the Python server saw the golden HELLO fields the Go hub sent.
	nameVal := lb.waitForLine(t, "HELLO_NAME=", 10*time.Second)
	if strings.TrimSpace(nameVal) != "nomadnet" {
		t.Errorf("Python saw HELLO_NAME = %q, want %q", nameVal, "nomadnet")
	}
	verVal := lb.waitForLine(t, "HELLO_VER=", 5*time.Second)
	if strings.TrimSpace(verVal) != "0.1" {
		t.Errorf("Python saw HELLO_VER = %q, want %q", verVal, "0.1")
	}
	capsVal := lb.waitForLine(t, "HELLO_CAPS=", 5*time.Second)
	if strings.TrimSpace(capsVal) != "0,1" {
		t.Errorf("Python saw HELLO_CAPS = %q, want %q", capsVal, "0,1")
	}
	nickVal := lb.waitForLine(t, "HELLO_NICK=", 5*time.Second)
	if strings.TrimSpace(nickVal) != "TestClient" {
		t.Errorf("Python saw HELLO_NICK = %q, want %q", nickVal, "TestClient")
	}
	srcVal := lb.waitForLine(t, "HELLO_SRC=", 5*time.Second)
	if dec, err := hex.DecodeString(strings.TrimSpace(srcVal)); err != nil || len(dec) == 0 {
		t.Errorf("Python saw HELLO_SRC = %q, want non-empty hex identity hash", srcVal)
	}
	if _, ok := lb.findLine("WELCOME_SENT="); !ok {
		t.Error("Python server did not report WELCOME_SENT")
	}
}