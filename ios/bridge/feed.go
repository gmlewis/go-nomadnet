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
	"errors"
	"fmt"
	"net"
	"sync"
)

const (
	// DefaultFeedPort is the loopback port the appliance's sensor feed listens
	// on. It is a contract, not a preference: the client's own [location]
	// sensor_feed names it, the bot's configuration names it, and the two have
	// to reach the same server.
	DefaultFeedPort = 37430

	// SharedInstancePort carries the Reticulum shared instance the whole
	// appliance talks through, and SharedInstancePort+1 its control channel.
	// Neither may be taken by the sensor feed, or the client would find a sensor
	// stream where it expected its transport.
	SharedInstancePort = 37428

	// feedSubscriberBuffer is how many published chunks a subscriber may fall
	// behind before its samples are dropped. It matches the converter queue the
	// Android appliance bounded at 64: the feed is a live stream, and an old
	// position delivered late is worse than one that never arrived.
	feedSubscriberBuffer = 64
)

// Feed is the appliance's sensor stream: the Swift half publishes position and
// heading sentences into it, and the client and the bot subscribe to it.
//
// On Android this is a Kotlin TCP server fed by a separate gonsensor process.
// iOS has no processes, so the server is here, in the same Go runtime as
// everything else that reads it.
//
// It carries NMEA-0183 sentences rather than JSON because that is what its
// readers parse: the client's own sensor source and the bot both take NMEA, so
// the conversion happens once, at the point the sample arrives, and every
// subscriber sees the same bytes.
type Feed struct {
	ln net.Listener

	// mu guards the subscriber set and the closed flag. It is held only for the
	// length of a map operation, never across a write: publishing a position
	// happens on the thread that received it from the device.
	mu     sync.Mutex
	subs   map[*feedSubscriber]struct{}
	closed bool
	wg     sync.WaitGroup
}

// feedSubscriber is one attached reader and the writer goroutine that serves it.
type feedSubscriber struct {
	conn net.Conn
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

// NewFeed binds a sensor feed on the loopback interface.
//
// A port of zero asks the kernel for a free one, which is what a test wants; an
// appliance asks for DefaultFeedPort. Nothing else may be used: the Reticulum
// ports are reserved, and the binding is loopback-only because a position must
// never be reachable from off the device.
func NewFeed(port int) (*Feed, error) {
	if port == SharedInstancePort || port == SharedInstancePort+1 {
		return nil, fmt.Errorf(
			"bridge: port %v carries the Reticulum shared instance, so the sensor feed cannot bind it",
			port,
		)
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("bridge: %v is not a port", port)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("bridge: binding the sensor feed on 127.0.0.1:%v: %w", port, err)
	}

	f := &Feed{ln: ln, subs: make(map[*feedSubscriber]struct{})}
	f.wg.Add(1)
	go f.acceptLoop()
	return f, nil
}

// Addr is the address subscribers dial.
func (f *Feed) Addr() string { return f.ln.Addr().String() }

// Port is the port the feed bound.
func (f *Feed) Port() int {
	if addr, ok := f.ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// Subscribers is how many readers are attached.
func (f *Feed) Subscribers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

// acceptLoop takes the client's and the bot's connections as they arrive.
func (f *Feed) acceptLoop() {
	defer f.wg.Done()
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			// A closed listener is how the feed stops, and no subscriber is left
			// waiting on a socket that is gone.
			return
		}
		sub := &feedSubscriber{
			conn: conn,
			ch:   make(chan []byte, feedSubscriberBuffer),
			done: make(chan struct{}),
		}
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			_ = conn.Close()
			return
		}
		f.subs[sub] = struct{}{}
		f.mu.Unlock()

		f.wg.Add(1)
		go f.serve(sub)
	}
}

// serve writes published chunks to one subscriber until it goes away.
func (f *Feed) serve(sub *feedSubscriber) {
	defer f.wg.Done()
	defer f.drop(sub)
	for {
		select {
		case chunk := <-sub.ch:
			if _, err := sub.conn.Write(chunk); err != nil {
				return
			}
		case <-sub.done:
			return
		}
	}
}

// drop removes a subscriber and releases its connection.
func (f *Feed) drop(sub *feedSubscriber) {
	f.mu.Lock()
	delete(f.subs, sub)
	f.mu.Unlock()
	sub.once.Do(func() { close(sub.done) })
	_ = sub.conn.Close()
}

// Publish sends one chunk of NMEA sentences to every subscriber.
//
// A subscriber that has fallen behind by feedSubscriberBuffer chunks loses the
// new one rather than slowing the publisher down. That is deliberate: Publish
// runs on the thread that just took a fix from the device, and a stalled reader
// there would stall the device's own position updates.
func (f *Feed) Publish(chunk []byte) error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return errors.New("bridge: the sensor feed is closed")
	}
	subs := make([]*feedSubscriber, 0, len(f.subs))
	for sub := range f.subs {
		subs = append(subs, sub)
	}
	f.mu.Unlock()

	// Each subscriber gets its own copy: one subscriber's writer goroutine must
	// not be able to observe another's buffer, and the caller's slice is not
	// ours to keep.
	for _, sub := range subs {
		buf := make([]byte, len(chunk))
		copy(buf, chunk)
		select {
		case sub.ch <- buf:
		default:
		}
	}
	return nil
}

// Close stops the feed and releases every subscriber.
//
// It is safe to call more than once, because the engine's own shutdown path and
// the app's are not ordered with respect to each other.
func (f *Feed) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	subs := make([]*feedSubscriber, 0, len(f.subs))
	for sub := range f.subs {
		subs = append(subs, sub)
	}
	f.mu.Unlock()

	err := f.ln.Close()
	for _, sub := range subs {
		sub.once.Do(func() { close(sub.done) })
		_ = sub.conn.Close()
	}
	f.wg.Wait()
	return err
}
