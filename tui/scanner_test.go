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

package tui

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// validAddr is a destination hash of the shape a QR code carries. It is the
// address the My LXMF dialog renders as a QR code, so it is exactly what the
// scanner is expected to hand back.
const validAddr = "2a6105f57145860441a62fe3b2a1352c"

// TestNormalizeScannedAddress pins what a QR payload may look like before it
// is worth putting in an address field. The rule is one maximal run of hex
// digits of exactly 32 characters, which is what makes a bare address, a
// `<addr>`-wrapped one, an `lxmf@<addr>` link and a `0x`-prefixed one all the
// same address — and what makes a 31- or 33-character run no address at all.
func TestNormalizeScannedAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
		wantOK  bool
	}{
		{name: "bare address", payload: validAddr, want: validAddr, wantOK: true},
		{name: "surrounding whitespace", payload: "  " + validAddr + "\n", want: validAddr, wantOK: true},
		{name: "upper case", payload: strings.ToUpper(validAddr), want: validAddr, wantOK: true},
		{name: "angle wrapped", payload: "<" + validAddr + ">", want: validAddr, wantOK: true},
		{name: "spaced angle wrapped", payload: "< " + validAddr + " >", want: validAddr, wantOK: true},
		{name: "0x prefixed", payload: "0x" + validAddr, want: validAddr, wantOK: true},
		{name: "0X prefixed upper case", payload: "0X" + strings.ToUpper(validAddr), want: validAddr, wantOK: true},
		{name: "lxmf link", payload: "lxmf@" + validAddr, want: validAddr, wantOK: true},
		{name: "lxmf link angle wrapped", payload: "lxmf@<" + validAddr + ">", want: validAddr, wantOK: true},
		{name: "nomadnet url", payload: "nomadnet://" + validAddr, want: validAddr, wantOK: true},
		{name: "trailing path", payload: "https://example.test/n/" + validAddr + "/page.mu", want: validAddr, wantOK: true},
		{name: "two addresses takes the first", payload: validAddr + " then " + strings.Repeat("ab", 16), want: validAddr, wantOK: true},

		{name: "empty", payload: "", want: "", wantOK: false},
		{name: "whitespace only", payload: "   ", want: "", wantOK: false},
		{name: "prose", payload: "point the camera at the code", want: "", wantOK: false},
		{name: "one short", payload: validAddr[:31], want: "", wantOK: false},
		{name: "one long", payload: validAddr + "c", want: "", wantOK: false},
		{name: "double length", payload: validAddr + validAddr, want: "", wantOK: false},
		{name: "hex run after a letter", payload: "z" + validAddr, want: validAddr, wantOK: true},
		{name: "hex run before a letter", payload: validAddr + "z", want: validAddr, wantOK: true},
		{name: "non hex character inside", payload: validAddr[:16] + "g" + validAddr[17:], want: "", wantOK: false},
		{name: "only the marker", payload: "0x", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := normalizeScannedAddress(tt.payload)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("normalizeScannedAddress(%q) = (%q, %v), want (%q, %v)", tt.payload, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// scanServer is a stand-in for the appliance's scanner: it accepts one
// connection, reads the request line, and answers with reply. It exists so the
// client half of the protocol is asserted against bytes rather than against a
// device, exactly as the console frame codec is.
type scanServer struct {
	ln    net.Listener
	token string
	reply string

	mu       sync.Mutex
	requests []string
	done     chan struct{}
}

// newScanServer starts a scanner that answers every request with reply.
func newScanServer(t *testing.T, token, reply string) *scanServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening for the scan server: %v", err)
	}
	s := &scanServer{ln: ln, token: token, reply: reply, done: make(chan struct{})}
	t.Cleanup(func() {
		_ = ln.Close()
		<-s.done
	})
	go s.serve()
	return s
}

// serve accepts connections until the listener is closed, answering each.
func (s *scanServer) serve() {
	defer close(s.done)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.handle(conn)
	}
}

// handle answers one connection: a request line carrying the right token is
// answered with the configured reply, and anything else is closed unanswered —
// which is what the appliance does with a request it did not expect.
func (s *scanServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	buf := make([]byte, 0, 256)
	one := make([]byte, 1)
	for {
		n, err := conn.Read(one)
		if err != nil {
			return
		}
		if n == 0 {
			return
		}
		if one[0] == '\n' {
			break
		}
		buf = append(buf, one[0])
		if len(buf) > 1024 {
			return
		}
	}
	line := string(buf)
	s.mu.Lock()
	s.requests = append(s.requests, line)
	s.mu.Unlock()
	if line != scanRequestVerb+" "+s.token {
		return
	}
	_, _ = conn.Write([]byte(s.reply + "\n"))
}

// seen returns the request lines the server was given, in order.
func (s *scanServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// addr returns the host:port the client should be pointed at.
func (s *scanServer) addr() string { return s.ln.Addr().String() }

// TestTCPScannerRoundTrip pins the request the client sends and the reply it
// accepts: a scan the reader completed comes back as the QR payload.
func TestTCPScannerRoundTrip(t *testing.T) {
	t.Parallel()

	srv := newScanServer(t, "0f1e2d3c", scanReplyVerb+" lxmf@"+validAddr)
	sc := newTCPScanner(srv.addr(), "0f1e2d3c", time.Second)

	got, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got != "lxmf@"+validAddr {
		t.Errorf("Scan = %q, want %q", got, "lxmf@"+validAddr)
	}
	if seen := srv.seen(); len(seen) != 1 || seen[0] != "SCAN 0f1e2d3c" {
		t.Errorf("the request line was %q, want [%q]", seen, "SCAN 0f1e2d3c")
	}
}

// TestTCPScannerCancelled pins that a reader who backs out of the camera is
// told apart from one whose scan failed: the dialog leaves their typing alone
// only for the first.
func TestTCPScannerCancelled(t *testing.T) {
	t.Parallel()

	srv := newScanServer(t, "token", scanCancelReply)
	sc := newTCPScanner(srv.addr(), "token", time.Second)

	if _, err := sc.Scan(context.Background()); !errors.Is(err, ErrScanCancelled) {
		t.Errorf("Scan after a cancel = %v, want ErrScanCancelled", err)
	}
}

// TestTCPScannerWrongToken pins that the appliance's refusal to answer an
// unexpected request is an error rather than an empty address.
func TestTCPScannerWrongToken(t *testing.T) {
	t.Parallel()

	srv := newScanServer(t, "the-right-token", scanReplyVerb+" "+validAddr)
	sc := newTCPScanner(srv.addr(), "the-wrong-token", time.Second)

	if _, err := sc.Scan(context.Background()); !errors.Is(err, ErrNoScanReply) {
		t.Errorf("Scan with the wrong token = %v, want ErrNoScanReply", err)
	}
}

// TestTCPScannerMalformedReply pins that a reply the protocol does not define
// is refused rather than half-read.
func TestTCPScannerMalformedReply(t *testing.T) {
	t.Parallel()

	srv := newScanServer(t, "token", "SURELY NOT "+validAddr)
	sc := newTCPScanner(srv.addr(), "token", time.Second)

	if _, err := sc.Scan(context.Background()); !errors.Is(err, ErrNoScanReply) {
		t.Errorf("Scan of a malformed reply = %v, want ErrNoScanReply", err)
	}
}

// TestTCPScannerNoServer pins that a scanner the client cannot reach fails
// rather than waiting for the reader's whole session.
func TestTCPScannerNoServer(t *testing.T) {
	t.Parallel()

	// A listener that is closed before it is used: the port is then one
	// nothing is listening on, so the dial is refused at once.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("closing the listener: %v", err)
	}

	sc := newTCPScanner(addr, "token", time.Second)
	if _, err := sc.Scan(context.Background()); err == nil {
		t.Error("Scan against a closed port succeeded, want an error")
	}
}

// TestTCPScannerHonoursContext pins that a scan the client gave up on does not
// leave a goroutine waiting on the appliance.
func TestTCPScannerHonoursContext(t *testing.T) {
	t.Parallel()

	// A listener that accepts and then says nothing: the reader is still
	// aiming the camera, and only the caller's own deadline ends the wait.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer func() { _ = ln.Close() }()
	held := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		held <- conn
	}()

	sc := newTCPScanner(ln.Addr().String(), "token", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel once the request has been read, so the wait is on the reply
		// rather than on the connection.
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	if _, err := sc.Scan(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Scan with a cancelled context = %v, want context.Canceled", err)
	}
	if conn := <-held; conn != nil {
		_ = conn.Close()
	}
}

// TestTCPScannerEndpointAsTheApplianceNamesIt pins that the client dials the endpoint it
// was given, verbatim — including an IPv6 literal, which an appliance on a dual-stack
// device binds its loopback listener to and which Go refuses unless it is bracketed.
// `net.Dial("tcp", "::1:46635")` is "too many colons in address", so an appliance that
// passes its bound address unbracketed hands the client a scan button that can never work.
// It is the appliance's job to bracket; this asserts the client is not the thing that
// should have guessed.
func TestTCPScannerEndpointAsTheApplianceNamesIt(t *testing.T) {
	t.Parallel()

	// A listener on the IPv6 loopback, as the Android appliance's is.
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback on this machine: %v", err)
	}
	srv := &scanServer{ln: ln, token: "token", reply: scanReplyVerb + " " + validAddr, done: make(chan struct{})}
	t.Cleanup(func() {
		_ = ln.Close()
		<-srv.done
	})
	go srv.serve()

	bracketed := ln.Addr().String() // "[::1]:port", which is the form Go dials
	sc := newTCPScanner(bracketed, "token", time.Second)
	got, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan against the bracketed IPv6 endpoint %q: %v", bracketed, err)
	}
	if got != validAddr {
		t.Errorf("Scan = %q, want %q", got, validAddr)
	}

	// And the form the appliance must not send: the same address with its brackets gone,
	// which is what a plain host:port join produces.
	unbracketed := strings.TrimPrefix(strings.Replace(ln.Addr().String(), ":", "", 1), "[")
	unbracketed = strings.Replace(unbracketed, "]", "", 1)
	if _, err := newTCPScanner(unbracketed, "token", time.Second).Scan(context.Background()); err == nil {
		t.Errorf("Scan against the unbracketed IPv6 endpoint %q succeeded, which would mean this "+
			"client accepts an endpoint the appliance should never send", unbracketed)
	}
}

// TestScannerFromEnvUnset pins the capability gate: with nothing in the
// environment the client has no scanner, which is what keeps the dialogs
// byte-identical to the Python original on a desktop terminal.
func TestScannerFromEnvUnset(t *testing.T) {
	t.Setenv(ScanAddrEnv, "")
	t.Setenv(ScanTokenEnv, "")

	if sc := ScannerFromEnv(); sc != nil {
		t.Errorf("ScannerFromEnv with no endpoint = %v, want nil", sc)
	}
}

// TestScannerFromEnvSet pins that an appliance that names its scanner gets one,
// and that a scan reaches the named endpoint with the named token.
func TestScannerFromEnvSet(t *testing.T) {
	srv := newScanServer(t, "abc123", scanReplyVerb+" "+validAddr)
	t.Setenv(ScanAddrEnv, srv.addr())
	t.Setenv(ScanTokenEnv, "abc123")

	sc := ScannerFromEnv()
	if sc == nil {
		t.Fatal("ScannerFromEnv with an endpoint = nil, want a scanner")
	}
	got, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if normalize, ok := normalizeScannedAddress(got); !ok || normalize != validAddr {
		t.Errorf("Scan = %q, which is not the address the appliance sent", got)
	}
}

// TestScannerFromEnvWithoutToken pins that an endpoint with no token is no
// scanner: the token is what stops another application on the tablet from
// opening the camera through the appliance's listener.
func TestScannerFromEnvWithoutToken(t *testing.T) {
	t.Setenv(ScanAddrEnv, "127.0.0.1:1234")
	t.Setenv(ScanTokenEnv, "")

	if sc := ScannerFromEnv(); sc != nil {
		t.Errorf("ScannerFromEnv without a token = %v, want nil", sc)
	}
}

// fakeScanner is a Scanner that answers without a camera. entered receives one
// token per call, so a test can tell a scan that is in flight from one that
// never started.
type fakeScanner struct {
	payload string
	err     error
	release chan struct{}
	entered chan struct{}

	mu    sync.Mutex
	calls int
}

func newFakeScanner(payload string) *fakeScanner {
	return &fakeScanner{payload: payload, entered: make(chan struct{}, 8)}
}

// Scan records the call and answers with the configured payload.
func (f *fakeScanner) Scan(ctx context.Context) (string, error) {
	f.mu.Lock()
	f.calls++
	entered, release := f.entered, f.release
	f.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.payload, f.err
}

// callCount returns how many scans have been started.
func (f *fakeScanner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// pressScan activates a scan button the way a reader does: Enter on the button.
func pressScan(btn *UrwidButton) {
	btn.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
}

// drainUpdate returns the next update the app queued, so a test can run it on
// its own goroutine and assert what the reader would have seen.
func drainUpdate(t *testing.T, app *App) func() {
	t.Helper()
	select {
	case f := <-app.updates:
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("the scan result was never applied to the dialog")
		return nil
	}
}

// TestNewScanButtonWithoutScanner pins that the button is absent when there is
// no camera to scan with — the parity guarantee: on a desktop terminal the
// dialog must render exactly as the Python original does.
func TestNewScanButtonWithoutScanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	field := NewReadlineEdit(app.killRing, "Addr : ", "")

	if btn := NewScanButton(app, field.SetText); btn != nil {
		t.Errorf("NewScanButton with no scanner = %v, want nil", btn)
	}
}

// TestNewScanButtonNilAppAndSetter pins that the absence of either half of the
// button is absence of the button, not a crash.
func TestNewScanButtonNilAppAndSetter(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner(validAddr))
	if btn := NewScanButton(app, nil); btn != nil {
		t.Errorf("NewScanButton with no setter = %v, want nil", btn)
	}
	if btn := NewScanButton(nil, func(string) {}); btn != nil {
		t.Errorf("NewScanButton with no app = %v, want nil", btn)
	}
}

// TestScanButtonFillsTheField pins the whole point of the feature: a QR code
// read by the appliance becomes the address in the field, normalized, with the
// caret left after it.
func TestScanButtonFillsTheField(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner("lxmf@<" + strings.ToUpper(validAddr) + ">"))
	// The app's own update queue is drained by the test instead of by the
	// event loop, so the scan's result is applied where the test can see it.
	app.updates = make(chan func(), 8)

	field := NewReadlineEdit(app.killRing, "Addr : ", "")
	btn := NewScanButton(app, field.SetText)
	if btn == nil {
		t.Fatal("NewScanButton with a scanner = nil, want a button")
	}
	if got := btn.Label(); got != "Scan" {
		t.Errorf("the button is labelled %q, want %q", got, "Scan")
	}

	pressScan(btn)
	drainUpdate(t, app)()

	if got := field.GetText(); got != validAddr {
		t.Errorf("the field holds %q, want %q", got, validAddr)
	}
	if got := field.CursorPos(); got != len(validAddr) {
		t.Errorf("the caret is at %v, want it after the address at %v", got, len(validAddr))
	}
}

// TestScanButtonLeavesTheFieldOnCancel pins that a cancelled scan costs the
// reader nothing: what they had already typed is still there.
func TestScanButtonLeavesTheFieldOnCancel(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	sc := newFakeScanner("")
	sc.err = ErrScanCancelled
	app.SetScanner(sc)
	app.updates = make(chan func(), 8)

	field := NewReadlineEdit(app.killRing, "Addr : ", "")
	field.SetText("2a61")
	btn := NewScanButton(app, field.SetText)

	pressScan(btn)
	drainUpdate(t, app)()

	if got := field.GetText(); got != "2a61" {
		t.Errorf("the field holds %q after a cancelled scan, want the typed %q", got, "2a61")
	}
}

// TestScanButtonIgnoresAPayloadThatIsNotAnAddress pins that a QR code carrying
// something else — a URL, a vCard, a wifi credential — is refused rather than
// pasted into an address field.
func TestScanButtonIgnoresAPayloadThatIsNotAnAddress(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner("WIFI:S=guest;T=WPA;P=hunter2;;"))
	app.updates = make(chan func(), 8)

	field := NewReadlineEdit(app.killRing, "Addr : ", "")
	field.SetText("2a61")
	btn := NewScanButton(app, field.SetText)

	pressScan(btn)
	drainUpdate(t, app)()

	if got := field.GetText(); got != "2a61" {
		t.Errorf("the field holds %q after a QR code that is not an address, want %q", got, "2a61")
	}
}

// TestScanButtonIgnoresASecondPressWhileScanning pins that holding down the
// button does not open a stack of cameras: the reader is aiming one.
func TestScanButtonIgnoresASecondPressWhileScanning(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	sc := newFakeScanner(validAddr)
	sc.release = make(chan struct{})
	app.SetScanner(sc)
	app.updates = make(chan func(), 8)

	field := NewReadlineEdit(app.killRing, "Addr : ", "")
	btn := NewScanButton(app, field.SetText)

	pressScan(btn)
	<-sc.entered // the first scan is in flight
	pressScan(btn)
	pressScan(btn)

	// A second scan would have announced itself on sc.entered by now; this
	// wait is what gives a goroutine that should never exist the chance to.
	select {
	case <-sc.entered:
		t.Error("a second scan started while the first was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(sc.release)
	drainUpdate(t, app)()
	if got := sc.callCount(); got != 1 {
		t.Errorf("the appliance was asked to scan %v times, want 1", got)
	}
}

// TestScanButtonOnAPlainInputField pins that the button serves the plain
// tview.InputField the URL dialog uses as well as the readline field the other
// dialogs use.
func TestScanButtonOnAPlainInputField(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.SetScanner(newFakeScanner(validAddr))
	app.updates = make(chan func(), 8)

	field := tview.NewInputField()
	btn := NewScanButton(app, func(text string) { field.SetText(text) })
	if btn == nil {
		t.Fatal("NewScanButton on a *tview.InputField = nil, want a button")
	}

	pressScan(btn)
	drainUpdate(t, app)()

	if got := field.GetText(); got != validAddr {
		t.Errorf("the field holds %q, want %q", got, validAddr)
	}
}

// TestAppScannerDefault pins that an app built without an appliance has no
// scanner, which is the state every existing test and every desktop terminal
// runs in.
func TestAppScannerDefault(t *testing.T) {
	t.Parallel()

	if sc := newTestApp().Scanner(); sc != nil {
		t.Errorf("a fresh app's scanner = %v, want nil", sc)
	}
}
