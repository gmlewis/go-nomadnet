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

// pythonRRCClientScript drives the REAL nomadnet.RRC.RRCHub client against a
// Go RRC server (task 2.2 reverses the 2.1 roles). It:
//   - brings up RNS over a config-supplied TCPServerInterface,
//   - mints a local RNS.Identity and a minimal app/manager so the real RRCManager
//   - RRCHub client logic runs unchanged (only nomadnet's own code sends/receives),
//   - prints READY=1, then polls a hash file published by the Go test for the Go
//     server's rrc.chat destination hash,
//   - registers a message callback that prints RECV_MSG=<text> for any message
//     whose src is NOT the local identity (so local echoes / own sends are filtered),
//   - add_hub(hash, "rrc.chat", "GoHub").connect(), waits for WELCOME, prints
//     WELCOMED=1 + HUB_NAME=,
//   - send_message("general", "Hello from Python!") and prints MSG_SENT=1,
//   - then waits up to 30 s for the Go server to send a MSG (printed as RECV_MSG=).
//
// Using the real nomadnet.RRC client (not a hand-rolled script) is the point of
// task 2.2: it exercises the actual Python _connect_worker, link.identify,
// _send_hello, _on_packet T_MSG/T_WELCOME paths against the Go server.
const pythonRRCClientScript = `
import sys, os, time, threading
import RNS
from nomadnet.vendor import cbor
import nomadnet.RRC as rrc

configdir = sys.argv[1]
hash_file = sys.argv[2]
storagepath = sys.argv[3]

reticulum = RNS.Reticulum(configdir)
identity = RNS.Identity()
own_hash = bytes(identity.hash)

class FakeApp:
    def __init__(self, ident, nick, storage):
        self.identity = ident
        self.peer_settings = {"display_name": nick}
        self.storagepath = storage

app = FakeApp(identity, "PyClient", storagepath)
mgr = rrc.RRCManager(app)

def on_msg(hub, msg):
    try:
        src = msg.src
        if src is not None and bytes(src) == own_hash:
            return
        print("RECV_MSG=" + str(msg.text), flush=True)
    except Exception as e:
        print("RECV_ERR=" + str(e), flush=True)

mgr.set_message_callback(on_msg)

print("READY=1", flush=True)

# Wait for the Go test to publish the server destination hash.
hub_hash = None
deadline = time.monotonic() + 30.0
while time.monotonic() < deadline:
    try:
        with open(hash_file, "r") as f:
            h = f.read().strip()
        if h:
            hub_hash = bytes.fromhex(h)
            break
    except Exception:
        pass
    time.sleep(0.2)

if hub_hash is None:
    print("HASH_TIMEOUT=1", flush=True)
    sys.exit(1)

hub = mgr.add_hub(hub_hash, dest_name="rrc.chat", name="GoHub")
hub.connect()

deadline = time.monotonic() + 30.0
while time.monotonic() < deadline:
    if hub.welcomed:
        break
    time.sleep(0.1)

if not hub.welcomed:
    print("WELCOME_TIMEOUT=1", flush=True)
    sys.exit(1)
print("WELCOMED=1", flush=True)
print("HUB_NAME=" + str(hub.hub_name), flush=True)

try:
    hub.send_message("general", "Hello from Python!")
    print("MSG_SENT=1", flush=True)
except Exception as e:
    print("MSG_SEND_ERR=" + str(e), flush=True)
    sys.exit(1)

# Wait to receive a MSG from the Go server.
deadline = time.monotonic() + 30.0
while time.monotonic() < deadline:
    time.sleep(0.2)
print("CLIENT_DONE=1", flush=True)
`

// TestIntegrationXProcessMSGRoundTrip verifies a full RRC MSG round-trip across a
// real Go↔Python RNS link bridged by a TCP RNS transport (task 2.2), with roles
// reversed from 2.1: the Go side is the RRC *server* and the Python side drives
// the real nomadnet.RRC.RRCHub *client*.
//
//   - Python subprocess: TCPServerInterface + real RRCManager/RRCHub client.
//   - Go side: TCPClientInterface + a real RRCHub acting as server (announces
//     rrc.chat, handles HELLO→WELCOME, records inbound MSG, sends an outbound MSG).
//
// Direction 1 (Python client → Go server): the Python client send_message's
// "Hello from Python!"; the Go server's HandleData records it and the test
// asserts serverHub.GetMessages("general") contains the text.
//
// Direction 2 (Go server → Python client): the Go server SendMessage's
// "Hello from Go!"; the Python client's message callback prints RECV_MSG= and
// the test asserts the text matches.
func TestIntegrationXProcessMSGRoundTrip(t *testing.T) {
	testutils.SkipShortIntegration(t)
	pyPath := findRNSPython(t)

	pyPort := reserveTCPPortXProc(t)
	pyCfgDir := filepath.Join(testutils.TempDir(t, "nomadnet-rrc-xproc-py"), "config")
	writePythonRNSConfigWithTCPServer(t, pyCfgDir, pyPort)

	pyStorage := testutils.TempDir(t, "nomadnet-rrc-xproc-py-storage")
	hashDir := testutils.TempDir(t, "nomadnet-rrc-xproc-hash")
	hashFile := filepath.Join(hashDir, "hubhash")

	scriptPath := filepath.Join(filepath.Dir(pyCfgDir), "rrc_client.py")
	if err := os.WriteFile(scriptPath, []byte(pythonRRCClientScript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(pyPath, scriptPath, pyCfgDir, hashFile, pyStorage)
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

	// Wait for the Python TCP server to be up before connecting the Go client.
	lb.waitForLine(t, "READY=", 10*time.Second)

	ts, tsCleanup := newStartedTSWithTCPClient(t, "127.0.0.1", pyPort)
	defer tsCleanup()
	// Give the TCP client a moment to connect before announcing.
	time.Sleep(500 * time.Millisecond)

	serverDest, err := rns.NewDestination(ts, ts.Identity(), rns.DestinationIn, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("server dest error: %v", err)
	}

	serverMgr := NewManager(tempDirRRC(t), func() []byte { return ts.Identity().Hash })
	serverMgr.SetNickname("GoHub")
	serverHub := serverMgr.AddHub(serverDest.Hash, "rrc.chat", "GoHub")

	serverLinkCh := make(chan *rns.Link, 1)
	serverDest.SetLinkEstablishedCallback(func(l *rns.Link) {
		serverHub.SetLink(l)
		l.SetPacketCallback(func(data []byte, _ *rns.Packet) { serverHub.HandleData(data) })
		select {
		case serverLinkCh <- l:
		default:
		}
	})

	// Publish the Go server destination hash so the Python client can dial it.
	if err := os.WriteFile(hashFile, []byte(hex.EncodeToString(serverDest.Hash)), 0o644); err != nil {
		t.Fatal(err)
	}

	// Announce the Go RRC destination in a loop so the Python client can recall
	// the Go identity and establish an RNS Link to it.
	stopAnn := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopAnn:
				return
			default:
				_ = serverDest.Announce(nil)
				time.Sleep(2 * time.Second)
			}
		}
	}()
	defer close(stopAnn)

	// Wait for the Python client to complete the HELLO/WELCOME handshake.
	lb.waitForLine(t, "WELCOMED=", 40*time.Second)

	// Wait for the link to be established on the Go side too.
	select {
	case <-serverLinkCh:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for server-side link establishment")
	}

	// Wait for the Python client to send its MSG.
	lb.waitForLine(t, "MSG_SENT=", 10*time.Second)

	// Direction 1: the Go server should have recorded the Python-sent MSG.
	deadline := time.Now().Add(20 * time.Second)
	var foundPy bool
	for time.Now().Before(deadline) {
		for _, m := range serverHub.GetMessages("general") {
			if m.Text == "Hello from Python!" {
				foundPy = true
				break
			}
		}
		if foundPy {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !foundPy {
		t.Fatal("Go server did not record the Python-sent MSG \"Hello from Python!\"")
	}

	// Direction 2: the Go server sends a MSG to the Python client.
	serverHub.SendMessage("general", "Hello from Go!")

	recv := lb.waitForLine(t, "RECV_MSG=", 20*time.Second)
	if strings.TrimSpace(recv) != "Hello from Go!" {
		t.Errorf("Python received RECV_MSG = %q, want %q", recv, "Hello from Go!")
	}
}

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
