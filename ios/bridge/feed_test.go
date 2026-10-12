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

package bridge

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/bot"
)

// dialFeed connects to a feed, failing the test if it cannot.
func dialFeed(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialing the feed at %v: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// waitForSubscribers blocks until n readers are attached.
//
// A connection is accepted by the kernel as soon as it is dialed, but the feed
// only knows about it once its accept loop has run, so a test that publishes
// immediately after dialing would publish to nobody.
func waitForSubscribers(t *testing.T, f *Feed, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for f.Subscribers() < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := f.Subscribers(); got != n {
		t.Fatalf("Subscribers() = %v, want %v", got, n)
	}
}

// readExactly reads n bytes from r within a bounded time.
func readExactly(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	type result struct {
		p   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, n)
		_, err := io.ReadFull(r, buf)
		ch <- result{buf, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("reading %v bytes: %v", n, got.err)
		}
		return got.p
	case <-time.After(2 * time.Second):
		t.Fatalf("no %v bytes arrived within 2s", n)
		return nil
	}
}

func TestFeedPublishesSentencesToASubscriber(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	conn := dialFeed(t, f.Addr())
	waitForSubscribers(t, f, 1)

	// The feed carries NMEA-0183 sentences, because that is what the client's
	// own sensor reader and the bot both parse: a position published as JSON
	// would reach neither of them.
	at := time.Date(2026, time.October, 8, 20, 39, 40, 123_000_000, time.UTC)
	fix := bot.GPSFix{Valid: true, Lat: 35.123456, Lng: -106.56789, TimeUTC: at}
	want := bot.EmitRMC(fix, at) + bot.EmitGGA(fix)

	if err := f.Publish([]byte(want)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := readExactly(t, conn, len(want)); !bytes.Equal(got, []byte(want)) {
		t.Errorf("the subscriber received %q, want %q", got, want)
	}
}

func TestFeedServesEverySubscriberTheSameBytes(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	// The client and the bot subscribe independently, and neither may be the
	// reason the other misses a position.
	first := dialFeed(t, f.Addr())
	second := dialFeed(t, f.Addr())

	// Both subscriptions have to exist before anything is published, because a
	// feed with no subscribers has nobody to deliver to.
	waitForSubscribers(t, f, 2)

	sentence := bot.EmitHDM(47.5, time.Time{})
	if err := f.Publish([]byte(sentence)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for i, conn := range []net.Conn{first, second} {
		if got := readExactly(t, conn, len(sentence)); !bytes.Equal(got, []byte(sentence)) {
			t.Errorf("subscriber %v received %q, want %q", i, got, sentence)
		}
	}
}

func TestFeedRefusesTheReservedReticulumPorts(t *testing.T) {
	t.Parallel()
	// 37428 carries the shared Reticulum instance and 37429 its control channel.
	// A feed bound to either would take the transport's place and leave the
	// client talking to a sensor stream instead of to Reticulum.
	for _, port := range []int{SharedInstancePort, SharedInstancePort + 1} {
		if _, err := NewFeed(port); err == nil {
			t.Errorf("NewFeed(%v) returned no error, want one", port)
		}
	}
}

func TestFeedBindsLoopbackOnly(t *testing.T) {
	t.Parallel()
	// A position is the most sensitive thing the appliance handles, and it must
	// never be reachable from off the device.
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	host, _, err := net.SplitHostPort(f.Addr())
	if err != nil {
		t.Fatalf("Addr() = %q, which is not host:port: %v", f.Addr(), err)
	}
	if host != "127.0.0.1" {
		t.Errorf("the feed is bound to %v, want 127.0.0.1", host)
	}
}

func TestFeedDefaultPortIsPinned(t *testing.T) {
	t.Parallel()
	// The client's own configuration names this port, so it is a contract
	// between the Swift half, the rendered client config, and this server.
	if DefaultFeedPort != 37430 {
		t.Errorf("DefaultFeedPort = %v, want 37430", DefaultFeedPort)
	}
}

func TestFeedDoesNotStallOnASubscriberThatStoppedReading(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	// A subscriber that has stopped reading is not a reason for the publisher
	// to stop: the position updates come from a sensor callback, and blocking
	// there would stall the device's own fix stream.
	_ = dialFeed(t, f.Addr())
	waitForSubscribers(t, f, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range feedSubscriberBuffer * 4 {
			_ = f.Publish([]byte(bot.EmitHDM(float64(i), time.Time{})))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a subscriber that was not reading")
	}
}

func TestFeedCloseReleasesSubscribersAndIsIdempotent(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	addr := f.Addr()
	conn := dialFeed(t, addr)

	if err := f.Close(); err != nil {
		t.Errorf("the first Close returned %v, want nil", err)
	}
	if err := f.Close(); err != nil {
		t.Errorf("the second Close returned %v, want nil", err)
	}

	// The client's reader is parked on this connection, and a feed that has
	// stopped must end that read rather than leave it hanging.
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("Read returned nil after the feed closed, want an error")
	}

	// And the socket is gone, so a second appliance can bind the same port.
	if _, err := net.Dial("tcp", addr); err == nil {
		t.Error("the feed's socket still accepts connections after Close")
	}
}

func TestFeedRefusesAPortItCannotBind(t *testing.T) {
	t.Parallel()
	first, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	_, port, err := net.SplitHostPort(first.Addr())
	if err != nil {
		t.Fatalf("Addr() = %q: %v", first.Addr(), err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("the feed's port %q is not a number: %v", port, err)
	}
	if _, err := NewFeed(n); err == nil {
		t.Error("a second feed bound the same port, want an error")
	}
}

func TestFeedDropsASampleNobodyCanTakeRatherThanQueueingIt(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	// Publishing with no subscriber is not an error: the feed is there whenever
	// the client attaches, and the appliance starts it before anything listens.
	if err := f.Publish([]byte("$GNRMC,203940.123,A,3507.40736,N,10634.07340,W,0.4,271.3,081026,,,A*00\r\n")); err != nil {
		t.Errorf("Publish with no subscriber returned %v, want nil", err)
	}
}

func TestFeedAddrNamesThePortItBound(t *testing.T) {
	t.Parallel()
	f, err := NewFeed(0)
	if err != nil {
		t.Fatalf("NewFeed: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if !strings.HasPrefix(f.Addr(), "127.0.0.1:") {
		t.Errorf("Addr() = %q, want an address on 127.0.0.1", f.Addr())
	}
	if f.Port() == 0 {
		t.Error("Port() = 0, want the port the feed actually bound")
	}
}
