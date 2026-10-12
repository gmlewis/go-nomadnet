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
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/bot"
)

// Subsystem is one long-running part of the appliance.
//
// The Android appliance runs each of these as a separate process and supervises
// them from Kotlin. iOS forbids that, so they are goroutines in this process and
// the engine is their supervisor: it brings them up in an order that matters and
// takes them down in the reverse one.
//
// The interface is deliberately this small. Each subsystem already knows how to
// start and stop itself; what the engine adds is only the order and the
// guarantee that nothing is left running.
type Subsystem interface {
	// Name is what the subsystem is called in an error.
	Name() string

	// Start brings the subsystem up. It returns once the subsystem is ready to
	// be depended on, so the next one in the order can be started.
	Start(ctx context.Context) error

	// Close stops it and releases everything it started. It is called at most
	// once, and it must be safe to call on a subsystem whose Start failed.
	Close() error
}

// LocationSample is one GNSS fix from the device.
//
// Every measurement that might not have been taken is paired with a flag saying
// whether it was. A position nobody measured the height of must not be reported
// as standing at sea level, and the NMEA emitters this feeds say so by writing an
// empty field — so the flags travel with the numbers rather than being guessed
// from a zero.
type LocationSample struct {
	// Time is when the fix was taken.
	Time time.Time

	// Lat and Lng are the position in signed decimal degrees.
	Lat float64
	Lng float64

	// AltitudeM is the antenna height above mean sea level in meters, and
	// HasAltitude says whether it was measured.
	AltitudeM   float64
	HasAltitude bool

	// AccuracyM is the receiver's own estimate of the radius it expects the
	// position to lie within, and HasAccuracy says whether it was reported.
	AccuracyM   float64
	HasAccuracy bool

	// SpeedKnots is the speed over ground and CourseDeg the true track, zero
	// when the receiver did not report them.
	SpeedKnots float64
	CourseDeg  float64

	// Satellites is how many satellites the fix used, and FixQuality its
	// fix-quality indicator; both are zero when the receiver did not say.
	Satellites int
	FixQuality int

	// Valid says the receiver reported a live fix.
	Valid bool
}

// HeadingSample is one orientation reading from the device.
//
// The two headings are carried separately because a device with no position
// cannot compute the magnetic variation and therefore cannot honestly report a
// true heading, while a device with a position and a magnetic sensor can report
// both. Each has its own flag for that reason.
type HeadingSample struct {
	// Time is when the reading was taken.
	Time time.Time

	// MagneticDeg is the heading relative to magnetic north.
	MagneticDeg float64
	HasMagnetic bool

	// TrueDeg is the heading relative to true north.
	TrueDeg float64
	HasTrue bool
}

// EngineConfig describes the appliance the engine runs.
type EngineConfig struct {
	// FeedPort is the loopback port the sensor feed binds. Zero means
	// DefaultFeedPort.
	FeedPort int
}

// Engine runs the appliance's subsystems inside one process.
//
// This is the whole reason the appliance exists in the shape it does on iOS.
// Android's appliance is eight programs and a Kotlin supervisor; an iOS process
// may not start a program at all, so the same subsystems run as goroutines here
// and the Swift half talks to them through the C bridge in bridge.go.
type Engine struct {
	cfg  EngineConfig
	subs []Subsystem

	// mu guards the lifecycle. Start and Close hold it for their whole duration,
	// so a subsystem is never started while another is being closed.
	mu      sync.Mutex
	feed    *Feed
	started []Subsystem
	running bool
	closed  bool
}

// NewEngine returns an engine that will run subs, in the order given.
//
// The order is the caller's because it matters: the sensor feed is up before the
// bot reads it, and the transport owns the Reticulum shared instance before the
// hub and the client attach to it.
func NewEngine(cfg EngineConfig, subs ...Subsystem) (*Engine, error) {
	if cfg.FeedPort == 0 {
		cfg.FeedPort = DefaultFeedPort
	}
	if cfg.FeedPort < 0 || cfg.FeedPort > 65535 {
		return nil, fmt.Errorf("bridge: %v is not a port", cfg.FeedPort)
	}
	return &Engine{cfg: cfg, subs: subs}, nil
}

// Start brings up the sensor feed and then every subsystem, in order.
//
// A subsystem that fails to start takes the ones already started down with it:
// half an appliance that reports an error is worse than none, because the parts
// that did start keep the ports and the identity files the next attempt needs.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("bridge: the appliance has been shut down")
	}
	if e.running {
		// Starting an appliance that is already running is not an error: the
		// app's own start button and its launch path can both ask.
		return nil
	}

	feed, err := NewFeed(e.cfg.FeedPort)
	if err != nil {
		return err
	}

	for _, sub := range e.subs {
		if err := sub.Start(ctx); err != nil {
			_ = e.closeSubsLocked()
			_ = feed.Close()
			return fmt.Errorf("bridge: starting the %v: %w", sub.Name(), err)
		}
		e.started = append(e.started, sub)
	}

	e.feed = feed
	e.running = true
	return nil
}

// Close takes every subsystem down in the reverse of the order it started, and
// then the sensor feed.
//
// It is safe to call more than once, and safe to call on an engine that never
// started: an app can be backgrounded, killed and relaunched in any order, and
// the shutdown path is the one that must not be the thing that leaves a port
// bound or a goroutine running.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	err := e.closeSubsLocked()
	if e.feed != nil {
		if ferr := e.feed.Close(); ferr != nil && err == nil {
			err = ferr
		}
		e.feed = nil
	}
	e.running = false
	return err
}

// closeSubsLocked stops the started subsystems, newest first.
func (e *Engine) closeSubsLocked() error {
	var first error
	for _, sub := range slices.Backward(e.started) {
		if err := sub.Close(); err != nil && first == nil {
			first = fmt.Errorf("bridge: stopping the %v: %w", sub.Name(), err)
		}
	}
	e.started = nil
	return first
}

// Feed is the sensor feed subscribers dial, or nil before Start.
func (e *Engine) Feed() *Feed {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.feed
}

// FeedLocation publishes one fix to every subscriber as NMEA-0183 sentences.
//
// NMEA rather than JSON because that is what the readers parse: the client's own
// sensor source and the bot both take NMEA, so converting once, here, is what
// lets the Swift half hand over a fix and forget about it.
func (e *Engine) FeedLocation(s LocationSample) error {
	feed, err := e.runningFeed()
	if err != nil {
		return err
	}

	fix := bot.GPSFix{
		Valid:       s.Valid,
		Lat:         s.Lat,
		Lng:         s.Lng,
		AltitudeM:   s.AltitudeM,
		HasAltitude: s.HasAltitude,
		SpeedKnots:  s.SpeedKnots,
		CourseDeg:   s.CourseDeg,
		Satellites:  s.Satellites,
		FixQuality:  s.FixQuality,
		AccuracyM:   s.AccuracyM,
		HasAccuracy: s.HasAccuracy,
		TimeUTC:     s.Time,
	}

	at := s.Time
	if at.IsZero() {
		at = time.Now()
	}
	// The position, the fix quality and the error estimate are three sentences
	// because NMEA has no one sentence that carries them all, and a reader that
	// gets only the first has a position with no idea how good it is.
	chunk := bot.EmitRMC(fix, at) + bot.EmitGGA(fix) + bot.EmitGST(fix)
	return feed.Publish([]byte(chunk))
}

// FeedHeading publishes one orientation reading as one NMEA-0183 sentence.
//
// One sentence, not two, because that is what the appliance's own reference does
// and what its readers hold: a reader stores a single heading, so publishing a
// magnetic sentence and a true one from the same device would let the second
// replace the first and leave the reader with only whichever arrived last.
//
// The true heading wins when the device knows it, because that is the heading a
// person reads off a map. A withheld reading publishes nothing at all: zero
// degrees is due north, and a reader cannot tell a withheld heading from one.
func (e *Engine) FeedHeading(s HeadingSample) error {
	feed, err := e.runningFeed()
	if err != nil {
		return err
	}

	at := s.Time
	if at.IsZero() {
		at = time.Now()
	}

	var sentence string
	switch {
	case s.HasTrue:
		sentence = bot.EmitHDT(s.TrueDeg, at)
	case s.HasMagnetic:
		sentence = bot.EmitHDM(s.MagneticDeg, at)
	default:
		return nil
	}
	return feed.Publish([]byte(sentence))
}

// runningFeed returns the feed, or the reason there is none.
func (e *Engine) runningFeed() (*Feed, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("bridge: the appliance has been shut down")
	}
	if !e.running || e.feed == nil {
		return nil, errors.New("bridge: the appliance is not running")
	}
	return e.feed, nil
}
