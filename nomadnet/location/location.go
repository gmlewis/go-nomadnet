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

// Package location keeps the reader's own position and heading, and answers
// "where am I and which way am I facing" for the rest of the client.
//
// It exists because a `L Micron location construct is resolved against the
// reader, not against the page: only the client that holds the position can turn
// a Plus Code into "8 m away, 025° true, 40° to your left". Python's NomadNet has
// no such construct at all, so this is a gonomadnet extension.
//
// There are two tiers of position, and one rule between them: a live source is
// the only source.
//
//   - A live source is a sensor feed — a device with its own receiver publishing
//     NMEA-0183 sentences over a socket or a device path. A tablet knows where it
//     is and which way it faces, continuously, and the feed is what tells the
//     client so.
//   - A static fix is the [location] fix setting. It is what a sensorless install
//     uses, and it is consulted only when no live source is configured at all.
//     Once a feed is configured, a silent feed means the client has no position:
//     substituting the static fix would leave the reader unable to tell which
//     position they were looking at.
//
// Two properties are the reason this is a type and not a field.
//
// It is read while it is being written. The TUI renders on one goroutine while a
// scanner goroutine keeps the readers up to date, so every access goes through a
// guarded accessor and the whole client passes the race detector.
//
// What it reports is never stale. A reading that arrived longer ago than the
// configured window stops being a position, and the constructs that depend on one
// degrade to the bare Plus Code. A confidently stale position is the same class of
// bug as a confidently wrong one.
//
// Nothing here transmits anything. The position is an input to a distance the
// reader computed for themselves; it is never placed in an announce, a page
// request, a message, or a node's state.
package location

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/bot"
	"github.com/gmlewis/go-reticulum/geo"
)

// SourceKind names which tier answered a request for the reader's position.
type SourceKind string

const (
	// SourceNone means the client has no position at all.
	SourceNone SourceKind = "none"
	// SourceStatic means the static [location] fix answered.
	SourceStatic SourceKind = "static"
	// SourceLive means the sensor feed answered. It is reported even while the
	// feed is silent, so a reader can tell "no feed configured" from "the feed
	// is configured and has gone quiet".
	SourceLive SourceKind = "live"
)

// Options configures a Source.
type Options struct {
	// Feed is the sensor endpoint the position and heading arrive on: a device
	// path, tcp://host:port, or unix://path. An empty Feed means the client has
	// no live source, and the static fix — if any — is what it knows.
	Feed string
	// StaticFix is the parsed [location] fix.
	StaticFix geo.LatLng
	// HasStaticFix reports whether StaticFix is meaningful.
	HasStaticFix bool
	// MaxAge is how long a reading stays current. Zero or less disables ageing,
	// which is only sensible for a source that cannot go quiet.
	MaxAge time.Duration
	// Now is the clock. It defaults to time.Now and is a field so that staleness
	// is testable without waiting on real time.
	Now func() time.Time
	// Open resolves a sensor endpoint into a byte stream. It defaults to
	// bot.OpenSensorSource, the same resolver the Reticulum Buddy's own
	// gps_port and compass_port use, and it is a field so that a test can drive
	// the source without a socket.
	Open func(endpoint string) (io.ReadCloser, error)
}

// Status describes what the client currently knows about its own position. It is
// what lets the person holding the device see their own exposure rather than
// having to take the documentation's word for it.
type Status struct {
	// Feed is the configured sensor endpoint, or empty.
	Feed string
	// Source is the tier that answered.
	Source SourceKind
	// HasFix reports whether a position is currently known.
	HasFix bool
	// FixAge is how long ago the position last arrived. It is zero when there is
	// no fix.
	FixAge time.Duration
	// HasHeading reports whether a heading is currently known.
	HasHeading bool
	// HeadingDeg is the direction the reader faces, in degrees true.
	HeadingDeg float64
	// HeadingAge is how long ago the heading last arrived.
	HeadingAge time.Duration
	// AccuracyM is the receiver's own estimate of the fix's horizontal error in
	// meters. NMEA-0183 carries it in the position-error sentence and nowhere
	// else; HasAccuracy reports whether the receiver measured one at all.
	AccuracyM float64
	// HasAccuracy reports that the receiver measured its own error. A position
	// nobody measured the error of is not accurate to zero meters.
	HasAccuracy bool
	// MaxAge is the staleness window in force.
	MaxAge time.Duration
}

// Describe renders the status as one line a person can read: what the client
// knows, where it came from, how old it is, and how much to trust it. The privacy
// model is only real if the reader can check their own exposure, and this line is
// where they check it.
func (s Status) Describe() string {
	switch s.Source {
	case SourceStatic:
		return "a static fix from the [location] setting; no live sensor feed is configured"
	case SourceLive:
		if !s.HasFix {
			return fmt.Sprintf("no fix from the sensor feed %v", s.Feed)
		}
		described := "a live fix, " + describeAge(s.FixAge)
		if s.HasAccuracy {
			described += fmt.Sprintf(", +-%v m", roundTo(s.AccuracyM, 1))
		}
		if s.HasHeading {
			described += fmt.Sprintf(", facing %v true (%v)", roundTo(s.HeadingDeg, 0), describeAge(s.HeadingAge))
		}
		return described
	default:
		return "unknown; no [location] fix and no sensor feed"
	}
}

// describeAge renders an age at the resolution a person reads.
func describeAge(age time.Duration) string {
	switch {
	case age < time.Second:
		return "just now"
	case age < 2*time.Second:
		return "1 second old"
	case age < time.Minute:
		return fmt.Sprintf("%v seconds old", int(age.Round(time.Second)/time.Second))
	default:
		minutes := int(age.Round(time.Minute) / time.Minute)
		if minutes <= 1 {
			return "1 minute old"
		}
		return fmt.Sprintf("%v minutes old", minutes)
	}
}

// roundTo renders a value at the given number of decimal places, with integers
// written without a trailing point.
func roundTo(value float64, digits int) string {
	return strconv.FormatFloat(value, 'f', digits, 64)
}

// Source is the client's own position and heading, safe to read from any
// goroutine while a sensor feed keeps it up to date.
type Source struct {
	// opts is fixed at construction, so it needs no lock.
	opts Options

	// mu guards the readers, which Start installs and Close releases.
	mu      sync.RWMutex
	gps     *bot.GPSReader
	compass *bot.CompassReader

	// fixArrival and headingArrival are the Unix nanoseconds at which bytes last
	// arrived on each half of the feed. They are the ageing clock, and they are
	// deliberately the *arrival* time rather than the receiver's own timestamp:
	// a receiver whose clock is wrong would otherwise age a good fix out
	// instantly, or hold a dead one forever.
	fixArrival     atomic.Int64
	headingArrival atomic.Int64

	// closeOnce guards the readers' teardown, so Close is idempotent.
	closeOnce sync.Once
}

// New builds a source from the options, filling in the defaults a caller may
// leave out.
func New(opts Options) *Source {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Open == nil {
		opts.Open = bot.OpenSensorSource
	}
	return &Source{opts: opts}
}

// Start opens the sensor feed and begins keeping the position and heading
// current. It returns at once: the readers scan on their own goroutines.
//
// A feed that cannot be resolved — a malformed endpoint, or a device path that
// does not open — is reported, because a misconfiguration the client silently
// ignores is a client that never has a position and never says why. A socket that
// cannot be *reached* is not an error: the sensor service on the other side may
// simply not have started, and the stream reconnects itself when it appears.
func (s *Source) Start(ctx context.Context) error {
	if s.opts.Feed == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	gpsStream, err := s.opts.Open(s.opts.Feed)
	if err != nil {
		return fmt.Errorf("location: sensor feed %v: %w", s.opts.Feed, err)
	}
	compassStream, err := s.opts.Open(s.opts.Feed)
	if err != nil {
		_ = gpsStream.Close()
		return fmt.Errorf("location: sensor feed %v: %w", s.opts.Feed, err)
	}

	gps := bot.NewGPSReader(&stampingReader{src: gpsStream, stamp: &s.fixArrival, now: s.now})
	compass := bot.NewCompassReader(&stampingReader{src: compassStream, stamp: &s.headingArrival, now: s.now})
	// The compass corrects a magnetic heading to true north with the World
	// Magnetic Model at the client's own position. That is the same conversion
	// the Reticulum Buddy performs, and it is why the two agree about which way
	// the device faces.
	compass.SetLocationSource(func() (bot.GPSFix, bool) {
		fix := gps.LastFix()
		return fix, fix.Valid
	})

	s.mu.Lock()
	s.gps, s.compass = gps, compass
	s.mu.Unlock()

	gps.Start(ctx)
	compass.Start(ctx)
	return nil
}

// Close stops both readers and releases the feed. It is idempotent, and it waits
// for the scan goroutines, so the source owns nothing after it returns.
func (s *Source) Close() error {
	var first error
	s.closeOnce.Do(func() {
		gps, compass := s.readers()
		if gps != nil {
			first = gps.Close()
		}
		if compass != nil {
			if err := compass.Close(); err != nil && first == nil {
				first = err
			}
		}
	})
	return first
}

// Viewer returns the reader's position and heading as the renderers want them.
// It is safe to call from any goroutine, and it reports no position when the
// client has none, which is what makes the position-dependent constructs degrade
// rather than guess.
func (s *Source) Viewer() micron.Viewer {
	if s.opts.Feed == "" {
		if s.opts.HasStaticFix {
			return micron.Viewer{Pos: s.opts.StaticFix, Known: true}
		}
		return micron.Viewer{}
	}

	gps, compass := s.readers()
	if gps == nil || s.stale(&s.fixArrival) {
		return micron.Viewer{}
	}
	fix := gps.LastFix()
	if !fix.Valid {
		return micron.Viewer{}
	}
	viewer := micron.Viewer{Pos: fix.Position(), Known: true}

	if compass == nil || s.stale(&s.headingArrival) {
		return viewer
	}
	if deg, ok := headingDegrees(compass.LastHeading()); ok {
		viewer.HeadingDeg, viewer.HasHeading = deg, true
	}
	return viewer
}

// Status reports what the client knows and how old it is.
func (s *Source) Status() Status {
	status := Status{
		Feed:   s.opts.Feed,
		MaxAge: s.opts.MaxAge,
		Source: SourceNone,
	}
	if s.opts.Feed == "" {
		if s.opts.HasStaticFix {
			status.Source = SourceStatic
			status.HasFix = true
		}
		return status
	}

	status.Source = SourceLive
	gps, compass := s.readers()
	if gps != nil && !s.stale(&s.fixArrival) {
		if fix := gps.LastFix(); fix.Valid {
			status.HasFix = true
			status.FixAge = s.age(&s.fixArrival)
		}
	}
	if gps != nil && !s.stale(&s.fixArrival) {
		if fix := gps.LastFix(); fix.Valid && fix.HasAccuracy {
			status.AccuracyM = fix.AccuracyM
			status.HasAccuracy = true
		}
	}
	if compass != nil && !s.stale(&s.headingArrival) {
		if deg, ok := headingDegrees(compass.LastHeading()); ok {
			status.HasHeading = true
			status.HeadingDeg = deg
			status.HeadingAge = s.age(&s.headingArrival)
		}
	}
	return status
}

// readers returns the two sensor readers, or nils before Start has run.
func (s *Source) readers() (*bot.GPSReader, *bot.CompassReader) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gps, s.compass
}

// now reports the current instant.
func (s *Source) now() time.Time { return s.opts.Now() }

// stale reports whether the reading a stamp belongs to has aged out. A stamp of
// zero means nothing has ever arrived on that half of the feed, which is stale by
// definition.
func (s *Source) stale(stamp *atomic.Int64) bool {
	if s.opts.MaxAge <= 0 {
		return false
	}
	at := stamp.Load()
	if at == 0 {
		return true
	}
	return s.now().Sub(time.Unix(0, at)) > s.opts.MaxAge
}

// age reports how long ago a stamp was taken.
func (s *Source) age(stamp *atomic.Int64) time.Duration {
	at := stamp.Load()
	if at == 0 {
		return 0
	}
	return s.now().Sub(time.Unix(0, at))
}

// headingDegrees picks the heading a relative bearing can be measured from,
// preferring the true one and falling back to the magnetic one — which the reader
// has already turned true whenever it knows where the client is.
func headingDegrees(heading bot.CompassHeading) (float64, bool) {
	switch {
	case heading.HasTrue:
		return heading.TrueDeg, true
	case heading.HasMagnetic:
		return heading.MagneticDeg, true
	default:
		return 0, false
	}
}

// stampingReader records when bytes last arrived on a sensor stream. It is the
// whole ageing mechanism: no timer runs, nothing polls, and a feed that goes quiet
// simply stops moving its stamp. Without it a reader could only report what the
// last sentence said, with no way to tell a live fix from one an hour old.
type stampingReader struct {
	// src is the sensor stream.
	src io.Reader
	// stamp records the arrival instant.
	stamp *atomic.Int64
	// now is the clock.
	now func() time.Time
}

// Read forwards to the stream and stamps any bytes that came back.
func (r *stampingReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.stamp.Store(r.now().UnixNano())
	}
	return n, err
}

// Close releases the stream.
func (r *stampingReader) Close() error {
	if closer, ok := r.src.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
