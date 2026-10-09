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

package location

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/bot"
	"github.com/gmlewis/go-reticulum/geo"
)

// testClock is a clock the tests move by hand, so staleness is asserted without
// waiting on real time.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

// Now returns the clock's instant.
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newTestClock builds a clock pinned to a fixed instant.
func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, time.October, 8, 15, 0, 0, 0, time.UTC)}
}

// testFeed stands in for the sensor service on the other side of the app
// boundary. Every connection it hands out receives the same bytes, which is how
// a multiplexed NMEA feed behaves and what lets the position reader and the
// heading reader share one endpoint.
type testFeed struct {
	mu      sync.Mutex
	writers []*io.PipeWriter
	readers []*io.PipeReader
	err     error
}

// Open returns one more connection to the feed.
func (f *testFeed) Open(string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	pr, pw := io.Pipe()
	f.mu.Lock()
	f.writers = append(f.writers, pw)
	f.readers = append(f.readers, pr)
	f.mu.Unlock()
	return pr, nil
}

// Send writes one line to every connection.
func (f *testFeed) Send(line string) {
	f.mu.Lock()
	writers := append([]*io.PipeWriter(nil), f.writers...)
	f.mu.Unlock()
	for _, w := range writers {
		_, _ = io.WriteString(w, line+"\r\n")
	}
}

// Close tears the feed down, which is what makes the readers see end of stream.
func (f *testFeed) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.writers {
		_ = w.Close()
	}
	for _, r := range f.readers {
		_ = r.Close()
	}
}

// fixLine renders a position the same way the Android sensor service does: with
// the package's own emitter, so a test cannot accidentally assert against a
// sentence no parser would accept.
func fixLine(t *testing.T, lat, lng float64, at time.Time) string {
	t.Helper()
	return bot.EmitRMC(bot.GPSFix{
		Valid:      true,
		Lat:        lat,
		Lng:        lng,
		FixQuality: 1,
		TimeUTC:    at,
	}, at)
}

// startSource builds a source over a feed, starts it, and registers the teardown.
func startSource(t *testing.T, opts Options, feed *testFeed) *Source {
	t.Helper()

	source := New(opts)
	if err := source.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if feed != nil {
			feed.Close()
		}
	})
	return source
}

// waitForViewer polls the viewer until it satisfies want, which keeps the tests
// fast in the ordinary case and still bounded when the source is broken.
func waitForViewer(t *testing.T, source *Source, want func(micron.Viewer) bool) micron.Viewer {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got := source.Viewer(); want(got) {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the source never satisfied the condition; viewer = %+v", source.Viewer())
	return micron.Viewer{}
}

// TestSourceWithNeitherFeedNorFixKnowsNothing pins the bottom tier: a client
// with no live source and no static fix reports no position, and every
// position-dependent construct degrades to the bare Plus Code.
func TestSourceWithNeitherFeedNorFixKnowsNothing(t *testing.T) {
	t.Parallel()

	source := startSource(t, Options{Now: newTestClock().Now}, nil)
	if got := source.Viewer(); got.Known {
		t.Fatalf("Viewer() = %+v, want no position", got)
	}
	if got := source.Status(); got.HasFix {
		t.Fatalf("Status() = %+v, want no fix", got)
	}
}

// TestSourceWithoutAFeedUsesTheStaticFix pins the middle tier: an install with no
// live source keeps the position its configuration gave it.
func TestSourceWithoutAFeedUsesTheStaticFix(t *testing.T) {
	t.Parallel()

	source := startSource(t, Options{
		StaticFix:    geo.LatLng{Lat: 45.0, Lng: -93.0},
		HasStaticFix: true,
		Now:          newTestClock().Now,
	}, nil)

	got := source.Viewer()
	if !got.Known || got.Pos.Lat != 45.0 || got.Pos.Lng != -93.0 {
		t.Fatalf("Viewer() = %+v, want the static fix", got)
	}
	if got.HasHeading {
		t.Fatalf("Viewer() = %+v, want no heading with no compass source", got)
	}
	if status := source.Status(); status.Source != SourceStatic {
		t.Fatalf("Status().Source = %q, want %q", status.Source, SourceStatic)
	}
}

// TestLiveFeedBeatsTheStaticFix pins the precedence rule the whole design rests
// on: a configured live source is the only source, so the static fix never leaks
// into a reading taken while the feed is up.
func TestLiveFeedBeatsTheStaticFix(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := startSource(t, Options{
		Feed:         "tcp://127.0.0.1:37429",
		StaticFix:    geo.LatLng{Lat: 45.0, Lng: -93.0},
		HasStaticFix: true,
		MaxAge:       30 * time.Second,
		Now:          clock.Now,
		Open:         feed.Open,
	}, feed)

	feed.Send(fixLine(t, 35.123456, -106.56789, clock.Now()))

	got := waitForViewer(t, source, func(v micron.Viewer) bool { return v.Known })
	if isNear(got.Pos.Lat, 45.0) {
		t.Fatalf("Viewer() = %+v, want the live fix rather than the static one", got)
	}
	if !isNear(got.Pos.Lat, 35.123456) || !isNear(got.Pos.Lng, -106.56789) {
		t.Fatalf("Viewer() = %+v, want the position the feed published", got)
	}
	if status := source.Status(); status.Source != SourceLive {
		t.Fatalf("Status().Source = %q, want %q", status.Source, SourceLive)
	}
}

// TestUnreachableFeedNeverFallsBackToTheStaticFix pins the other half of the same
// rule: a feed that is configured but silent means the client has no position. It
// must not quietly substitute a different one, because the reader would have no
// way to tell which position they were looking at.
func TestUnreachableFeedNeverFallsBackToTheStaticFix(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	source := startSource(t, Options{
		Feed:         "tcp://127.0.0.1:37429",
		StaticFix:    geo.LatLng{Lat: 45.0, Lng: -93.0},
		HasStaticFix: true,
		MaxAge:       30 * time.Second,
		Now:          newTestClock().Now,
		Open:         feed.Open,
	}, feed)

	if got := source.Viewer(); got.Known {
		t.Fatalf("Viewer() = %+v, want no position while the feed is silent", got)
	}
	if status := source.Status(); status.Source != SourceLive {
		t.Fatalf("Status().Source = %q, want %q so the reader can see the feed is configured", status.Source, SourceLive)
	}
}

// TestStaleFixDegradesToNoPosition asserts a feed that goes quiet stops being a
// position, because a confidently stale position is the same class of bug as a
// confidently wrong one.
func TestStaleFixDegradesToNoPosition(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := startSource(t, Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    clock.Now,
		Open:   feed.Open,
	}, feed)

	feed.Send(fixLine(t, 35.123456, -106.56789, clock.Now()))
	waitForViewer(t, source, func(v micron.Viewer) bool { return v.Known })

	clock.Advance(29 * time.Second)
	if got := source.Viewer(); !got.Known {
		t.Fatalf("Viewer() = %+v, want the fix still current just inside the window", got)
	}

	clock.Advance(2 * time.Second)
	if got := source.Viewer(); got.Known {
		t.Fatalf("Viewer() = %+v, want the fix aged out past the window", got)
	}

	// A fresh reading brings it back, so the ageing is a window and not a latch.
	feed.Send(fixLine(t, 36.0, -107.0, clock.Now()))
	waitForViewer(t, source, func(v micron.Viewer) bool { return v.Known })
}

// TestStaleHeadingDegradesToNoHeading asserts the heading ages out on its own, so
// the location card never computes an instruction from a direction the tablet was
// facing minutes ago.
func TestStaleHeadingDegradesToNoHeading(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := startSource(t, Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    clock.Now,
		Open:   feed.Open,
	}, feed)

	feed.Send(fixLine(t, 35.123456, -106.56789, clock.Now()))
	// A true-heading sentence is the one shape whose value survives to the
	// viewer unchanged. A magnetic one is corrected to true north with the World
	// Magnetic Model against the live position above, which is the compass
	// reader's own tested behaviour and not this package's business.
	feed.Send(bot.EmitHDT(47.5, clock.Now()))
	got := waitForViewer(t, source, func(v micron.Viewer) bool { return v.HasHeading })
	if !isNear(got.HeadingDeg, 47.5) {
		t.Fatalf("HeadingDeg = %v, want 47.5", got.HeadingDeg)
	}

	clock.Advance(31 * time.Second)
	got = waitForViewer(t, source, func(v micron.Viewer) bool { return !v.HasHeading })
	if got.HasHeading {
		t.Fatalf("Viewer() = %+v, want the stale heading withheld", got)
	}
	// The position must age out with it: both readings are equally old.
	if got.Known {
		t.Fatalf("Viewer() = %+v, want the equally stale position withheld", got)
	}
}

// TestViewerIsRaceFree hammers the accessor from many goroutines while the feed
// keeps publishing, which is the race a bare field would have and the reason the
// accessor exists.
func TestViewerIsRaceFree(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := startSource(t, Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    clock.Now,
		Open:   feed.Open,
	}, feed)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Go(func() {
		for i := 1; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			lat := 35.0 + float64(i%1000)/1000
			feed.Send(fixLine(t, lat, -106.5, clock.Now()))
			feed.Send(bot.EmitHDM(float64(i%360), clock.Now()))
		}
	})

	for range 8 {
		wg.Go(func() {
			for range 400 {
				viewer := source.Viewer()
				if viewer.Known {
					_ = geo.FormatLatLng(viewer.Pos)
				}
				_ = source.Status()
			}
		})
	}

	wg.Go(func() {
		for range 200 {
			clock.Advance(10 * time.Millisecond)
		}
	})

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestMalformedFeedIsReported asserts a configured feed that no transport can
// dial is refused with an error naming it, rather than leaving the client
// silently positionless with no explanation.
func TestMalformedFeedIsReported(t *testing.T) {
	t.Parallel()

	source := New(Options{Feed: "udp://127.0.0.1:37429", Now: newTestClock().Now})
	err := source.Start(t.Context())
	if err == nil {
		t.Fatalf("Start accepted the unsupported scheme udp")
	}
	if !strings.Contains(err.Error(), "udp://127.0.0.1:37429") {
		t.Fatalf("Start error = %q, want it to name the endpoint", err)
	}
	if !strings.Contains(err.Error(), "location") {
		t.Fatalf("Start error = %q, want it to name the subsystem", err)
	}
}

// TestStatusReportsAge asserts the client can say how old what it knows is, which
// is what makes the privacy model checkable by the person holding the device
// rather than merely asserted in documentation.
func TestStatusReportsAge(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := startSource(t, Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    clock.Now,
		Open:   feed.Open,
	}, feed)

	feed.Send(fixLine(t, 35.123456, -106.56789, clock.Now()))
	waitForViewer(t, source, func(v micron.Viewer) bool { return v.Known })

	clock.Advance(5 * time.Second)
	status := source.Status()
	if !status.HasFix {
		t.Fatalf("Status() = %+v, want a fix", status)
	}
	if status.FixAge < 4*time.Second || status.FixAge > 6*time.Second {
		t.Fatalf("FixAge = %v, want about five seconds", status.FixAge)
	}
	if status.MaxAge != 30*time.Second {
		t.Fatalf("MaxAge = %v, want 30s", status.MaxAge)
	}
	if status.Feed != "tcp://127.0.0.1:37429" {
		t.Fatalf("Feed = %q, want the configured endpoint", status.Feed)
	}
}

// TestCloseIsIdempotent asserts closing twice is safe, because the TUI's teardown
// path and a deferred close can both run.
func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	source := startSource(t, Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    newTestClock().Now,
		Open:   feed.Open,
	}, feed)

	if err := source.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// isNear reports whether a value is within a meter-scale tolerance of want.
func isNear(got, want float64) bool {
	diff := got - want
	return diff < 1e-4 && diff > -1e-4
}

// TestStatusDescribe asserts the one line that tells a reader what the client
// knows about them. It has to distinguish "no feed configured" from "the feed is
// configured and has gone quiet", because those need different actions from the
// person holding the device.
func TestStatusDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{
			name:   "nothing at all",
			status: Status{},
			want:   "unknown; no [location] fix and no sensor feed",
		},
		{
			name:   "a static fix",
			status: Status{Source: SourceStatic, HasFix: true},
			want:   "a static fix from the [location] setting; no live sensor feed is configured",
		},
		{
			name: "a configured feed that has never delivered",
			status: Status{
				Source: SourceLive,
				Feed:   "tcp://127.0.0.1:37429",
			},
			want: "no fix from the sensor feed tcp://127.0.0.1:37429",
		},
		{
			name: "a live fix with an accuracy",
			status: Status{
				Source:      SourceLive,
				Feed:        "tcp://127.0.0.1:37429",
				HasFix:      true,
				FixAge:      3 * time.Second,
				AccuracyM:   3.8,
				HasAccuracy: true,
			},
			want: "a live fix, 3 seconds old, +-3.8 m",
		},
		{
			name: "a live fix and a heading",
			status: Status{
				Source:     SourceLive,
				Feed:       "tcp://127.0.0.1:37429",
				HasFix:     true,
				FixAge:     500 * time.Millisecond,
				HasHeading: true,
				HeadingDeg: 47.5,
				HeadingAge: 900 * time.Millisecond,
			},
			// A heading is rendered to the nearest whole degree.
			want: "a live fix, just now, facing 48 true (just now)",
		},
		{
			name: "a minute old",
			status: Status{
				Source: SourceLive,
				Feed:   "tcp://127.0.0.1:37429",
				HasFix: true,
				FixAge: 90 * time.Second,
			},
			want: "a live fix, 2 minutes old",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.status.Describe(); got != tc.want {
				t.Fatalf("Describe() = %q, want %q", got, tc.want)
			}
		})
	}
}
