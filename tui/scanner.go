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
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// The camera scan protocol.
//
// A dialog that takes a typed hex address can offer to read one from a QR code
// instead, but the client has no camera: on Android it is a separate process
// with no access to one, and on iOS an application may not start a program
// either. So the client asks the appliance to scan, over a loopback connection
// the appliance listens on, and the appliance answers with what the camera
// read. The appliance names its listener and the token that guards it in the
// environment (scanAddrEnv, scanTokenEnv), exactly as it names the console's
// wheel behaviour — see MainActivity.consoleHostSpec on Android and the bridge
// on iOS.
//
// One connection carries one request: the client dials, sends the request line,
// reads one reply line, and closes. A request the appliance did not expect — a
// wrong token, a line that is not the request — is answered by closing the
// connection without opening the camera.
const (
	// ScanAddrEnv names the host:port the appliance's scanner listens on.
	ScanAddrEnv = "GONOMADNET_SCAN_ADDR"

	// ScanTokenEnv names the token that request must carry. It is what stops
	// another application on the tablet from opening the camera through the
	// appliance's listener.
	ScanTokenEnv = "GONOMADNET_SCAN_TOKEN"

	// scanRequestVerb begins the one request line: "SCAN <token>".
	scanRequestVerb = "SCAN"

	// scanReplyVerb begins a successful reply: "ADDR <payload>", where the
	// payload is the QR code's text exactly as the camera read it. It is the
	// client that decides whether that text is an address.
	scanReplyVerb = "ADDR"

	// scanCancelReply is the reply to a reader who backed out of the camera.
	scanCancelReply = "CANCEL"

	// defaultScanTimeout bounds a scan the caller gave no deadline for. It is
	// long, because a reader aims a camera at their own pace, and it exists only
	// so that an appliance that has gone away cannot strand the goroutine that
	// is waiting on it.
	defaultScanTimeout = 2 * time.Minute

	// maxScanReply is the longest reply line this client will read. A QR code
	// holds at most a few kilobytes, and a line longer than this is not one.
	maxScanReply = 8 * 1024
)

var (
	// ErrScanCancelled reports that the reader dismissed the scanner without
	// reading anything. It is told apart from every other failure because it is
	// the one that costs the reader nothing: their typing is left alone.
	ErrScanCancelled = errors.New("the scan was cancelled")

	// ErrNoScanReply reports that the scanner did not answer, or answered with
	// something this protocol does not define.
	ErrNoScanReply = errors.New("the scanner sent no reply")
)

// Scanner reads a QR code with the appliance's camera.
//
// It is the whole of what a dialog needs to know about the camera: the transport
// it travels over and the device at the other end of it are the appliance's
// business. A dialog reaches it through App.Scanner, which is nil when the
// client is running somewhere with no camera to offer — a desktop terminal, or
// any of the test and parity harnesses — and a nil Scanner is what keeps the
// "< Scan >" button out of the dialog.
type Scanner interface {
	// Scan opens the appliance's camera and returns the text of the first QR
	// code the reader presents, exactly as the camera read it, or
	// ErrScanCancelled if the reader backed out.
	Scan(ctx context.Context) (string, error)
}

// tcpScanner is the Scanner that asks the appliance over its loopback listener.
type tcpScanner struct {
	addr    string
	token   string
	timeout time.Duration
}

// newTCPScanner returns a scanner that sends its requests to addr, carrying
// token, and gives up after timeout if the caller's context has no sooner
// deadline of its own.
func newTCPScanner(addr, token string, timeout time.Duration) *tcpScanner {
	return &tcpScanner{addr: addr, token: token, timeout: timeout}
}

// Scan asks the appliance to read one QR code and returns what it read.
//
// The request is one line and the reply is one line, so a reply that is not one
// of the two defined lines — including no reply at all, which is how the
// appliance refuses a request it did not expect — is ErrNoScanReply rather than
// an address to be guessed at.
func (t *tcpScanner) Scan(ctx context.Context) (string, error) {
	deadline := time.Now().Add(t.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", t.addr)
	if err != nil {
		return "", fmt.Errorf("asking the scanner at %v to read a code: %w", t.addr, err)
	}
	defer func() { _ = conn.Close() }()

	// The reader is aiming a camera, so the wait is theirs and not the client's:
	// a context the caller gives up on has to end it, and closing the connection
	// is what unblocks the read that is already in progress.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("setting the scanner's deadline: %w", err)
	}
	if _, err := conn.Write([]byte(scanRequestVerb + " " + t.token + "\n")); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %v", ErrNoScanReply, err)
	}

	line, err := bufio.NewReaderSize(conn, maxScanReply).ReadString('\n')
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %v", ErrNoScanReply, err)
	}

	switch reply := strings.TrimRight(line, "\r\n"); {
	case reply == scanCancelReply:
		return "", ErrScanCancelled
	case strings.HasPrefix(reply, scanReplyVerb+" "):
		return strings.TrimPrefix(reply, scanReplyVerb+" "), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrNoScanReply, reply)
	}
}

// ScannerFromEnv returns the scanner the appliance named in this process's
// environment, or nil when it named none.
//
// An endpoint with no token is no scanner rather than an unguarded one: the
// token is the only thing between the appliance's listener and any other
// application on the device, so a launch that forgot it must not be a launch
// that quietly drops the guard.
func ScannerFromEnv() Scanner {
	addr := strings.TrimSpace(os.Getenv(ScanAddrEnv))
	token := strings.TrimSpace(os.Getenv(ScanTokenEnv))
	if addr == "" || token == "" {
		return nil
	}
	return newTCPScanner(addr, token, defaultScanTimeout)
}

// normalizeScannedAddress returns the Reticulum address a scanned payload
// carries, and whether it carries one at all.
//
// A QR code that means to hand over an address does not always hold a bare one:
// it may be wrapped in the angle brackets this client prints addresses in, or
// prefixed with 0x, or be a link of the form "lxmf@<addr>" or a URL with the
// address somewhere in its path. All of those carry the same thing — one run of
// exactly 32 hex digits — so that is what is looked for, and everything else in
// the payload is ignored. Requiring the run to be exactly 32 is what refuses a
// 31- or 33-character one, a 64-character public key, and a longer word that
// merely contains 32 hex digits.
func normalizeScannedAddress(payload string) (string, bool) {
	lowered := strings.ToLower(payload)
	for i := 0; i < len(lowered); {
		if !isHexDigit(lowered[i]) {
			i++
			continue
		}
		j := i
		for j < len(lowered) && isHexDigit(lowered[j]) {
			j++
		}
		if j-i == 32 {
			return lowered[i:j], true
		}
		i = j
	}
	return "", false
}

// isHexDigit reports whether b is one of the sixteen lower-case hex digits.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}

// NewScanButton builds the "< Scan >" button a dialog offers beside a field
// that takes a hex address, or returns nil when there is no camera to scan
// with.
//
// A nil return is the parity guarantee: on a desktop terminal, and in every
// test and parity harness, the appliance offers no scanner, so the dialog is
// exactly the one the Python original draws. Callers must therefore add the
// button only when it is not nil.
//
// setText installs a scanned address in the dialog's field. It is a function
// rather than an interface because the two field types a dialog may use —
// ReadlineEdit and tview.InputField — disagree about SetText's return type, and
// the address is only ever put in, never read back: a scan that is cancelled or
// that reads something which is not an address leaves whatever the reader had
// already typed exactly where it was.
func NewScanButton(app *App, setText func(string)) *UrwidButton {
	if app == nil || setText == nil || app.Scanner() == nil {
		return nil
	}

	button := NewUrwidButton("Scan")
	var scanning atomic.Bool
	button.SetSelectedFunc(func() {
		// A reader who holds the button down is still aiming one camera.
		if !scanning.CompareAndSwap(false, true) {
			return
		}
		scanner := app.Scanner()
		go func() {
			payload, err := scanner.Scan(context.Background())
			// The result is applied on the event loop, because it changes the
			// dialog the reader is looking at.
			app.QueueUpdateDraw(func() {
				scanning.Store(false)
				if err != nil {
					if !errors.Is(err, ErrScanCancelled) {
						log.Printf("tui: the appliance could not scan a code: %v", err)
					}
					return
				}
				address, ok := normalizeScannedAddress(payload)
				if !ok {
					log.Printf("tui: the scanned code carries no address")
					return
				}
				setText(address)
			})
		}()
	})
	return button
}
