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
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/bot"
)

// fakeSubsystem is a subsystem that records the order it was started and stopped
// in, so the engine's ordering is asserted rather than assumed.
type fakeSubsystem struct {
	name string

	// startErr is what Start reports, for the case of a subsystem that refuses
	// to come up.
	startErr error

	mu     sync.Mutex
	order  *[]string
	starts int
	closes int
}

func (f *fakeSubsystem) Name() string { return f.name }

func (f *fakeSubsystem) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if f.order != nil {
		*f.order = append(*f.order, "start "+f.name)
	}
	return f.startErr
}

func (f *fakeSubsystem) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	if f.order != nil {
		*f.order = append(*f.order, "close "+f.name)
	}
	return nil
}

// freePort returns a port nothing is listening on, so an engine under test does
// not contend for the appliance's pinned one.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("releasing the reserved port: %v", err)
	}
	return port
}

// newTestEngine returns an engine on a free feed port, with a fresh order log its
// subsystems record into.
func newTestEngine(t *testing.T, subs ...Subsystem) (*Engine, *[]string) {
	t.Helper()
	var order []string
	for i, sub := range subs {
		if f, ok := sub.(*fakeSubsystem); ok {
			f.order = &order
			subs[i] = f
		}
	}
	e, err := NewEngine(EngineConfig{FeedPort: freePort(t)}, subs...)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return e, &order
}

func TestEngineStartsSubsystemsInOrderAndStopsThemInReverse(t *testing.T) {
	t.Parallel()
	// The order is the appliance: the transport owns the shared instance before
	// the hub attaches to it, and the feed is up before the bot reads it. Taking
	// them down in the reverse order is what keeps a hub from being left talking
	// to a transport that has gone.
	a := &fakeSubsystem{name: "transport"}
	b := &fakeSubsystem{name: "hub"}
	c := &fakeSubsystem{name: "bot"}
	e, order := newTestEngine(t, a, b, c)
	t.Cleanup(func() { _ = e.Close() })

	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := e.Feed(); got == nil {
		t.Fatal("the sensor feed was not started")
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	want := []string{"start transport", "start hub", "start bot", "close bot", "close hub", "close transport"}
	if strings.Join(*order, ", ") != strings.Join(want, ", ") {
		t.Errorf("the lifecycle was %v, want %v", *order, want)
	}
}

func TestEngineStartIsIdempotent(t *testing.T) {
	t.Parallel()
	// The app's launch path and its start button can both ask, and the second ask
	// must not try to bind a port that is already bound.
	sub := &fakeSubsystem{name: "transport"}
	e, _ := newTestEngine(t, sub)
	t.Cleanup(func() { _ = e.Close() })

	for i := range 2 {
		if err := e.Start(context.Background()); err != nil {
			t.Fatalf("Start %v: %v", i+1, err)
		}
	}
	if sub.starts != 1 {
		t.Errorf("the subsystem was started %v times, want 1", sub.starts)
	}
}

func TestEngineCloseIsIdempotentAndSafeBeforeStart(t *testing.T) {
	t.Parallel()
	sub := &fakeSubsystem{name: "transport"}
	e, _ := newTestEngine(t, sub)

	// A never-started engine is exactly the state an app that failed to launch is
	// in when it is being torn down.
	if err := e.Close(); err != nil {
		t.Errorf("Close before Start returned %v, want nil", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("the second Close returned %v, want nil", err)
	}
	if sub.closes != 0 {
		t.Errorf("a subsystem that never started was closed %v times, want 0", sub.closes)
	}
}

func TestEngineTakesDownWhatItStartedWhenASubsystemFails(t *testing.T) {
	t.Parallel()
	// Half an appliance is worse than none: the parts that did come up hold the
	// ports and the identity files the next attempt needs.
	first := &fakeSubsystem{name: "transport"}
	broken := &fakeSubsystem{name: "hub", startErr: errors.New("no route to the shared instance")}
	never := &fakeSubsystem{name: "bot"}
	e, order := newTestEngine(t, first, broken, never)
	t.Cleanup(func() { _ = e.Close() })

	err := e.Start(context.Background())
	if err == nil {
		t.Fatal("Start returned no error, want the hub's failure")
	}
	if !strings.Contains(err.Error(), "hub") {
		t.Errorf("Start error = %v, want it to name the hub", err)
	}

	want := []string{"start transport", "start hub", "close transport"}
	if strings.Join(*order, ", ") != strings.Join(want, ", ") {
		t.Errorf("the failed start did %v, want %v", *order, want)
	}
	if never.starts != 0 {
		t.Errorf("the bot was started after the hub failed %v times, want 0", never.starts)
	}
	if e.Feed() != nil {
		t.Error("the sensor feed was left running after a failed start")
	}
}

func TestEngineRefusesASampleOnceItHasStopped(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A device callback can arrive after the app has gone; it must be reported
	// rather than written to a socket that is closed.
	if err := e.FeedLocation(LocationSample{Lat: 35, Lng: -106, Valid: true}); err == nil {
		t.Error("FeedLocation after Close returned nil, want an error")
	}
	if err := e.FeedHeading(HeadingSample{MagneticDeg: 47.5, HasMagnetic: true}); err == nil {
		t.Error("FeedHeading after Close returned nil, want an error")
	}
}

func TestEngineRefusesASampleBeforeItHasStarted(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	t.Cleanup(func() { _ = e.Close() })

	if err := e.FeedLocation(LocationSample{Lat: 35, Lng: -106, Valid: true}); err == nil {
		t.Error("FeedLocation before Start returned nil, want an error")
	}
}

func TestEngineFeedsAFixTheClientsOwnReaderParses(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })

	conn := dialFeed(t, e.Feed().Addr())
	waitForSubscribers(t, e.Feed(), 1)

	at := time.Date(2026, time.October, 8, 20, 39, 40, 123_000_000, time.UTC)
	sample := LocationSample{
		Time:        at,
		Lat:         35.123456,
		Lng:         -106.56789,
		AltitudeM:   1620.5,
		HasAltitude: true,
		AccuracyM:   3.8,
		HasAccuracy: true,
		SpeedKnots:  0.4,
		CourseDeg:   271.3,
		Satellites:  8,
		FixQuality:  1,
		Valid:       true,
	}
	if err := e.FeedLocation(sample); err != nil {
		t.Fatalf("FeedLocation: %v", err)
	}

	// The proof that matters is not that bytes came out but that the CLIENT's own
	// reader makes the same fix of them: the appliance's whole job is to hand the
	// client a position it can use.
	want := bot.EmitRMC(bot.GPSFix{
		Valid: true, Lat: sample.Lat, Lng: sample.Lng,
		AltitudeM: sample.AltitudeM, HasAltitude: true,
		SpeedKnots: sample.SpeedKnots, CourseDeg: sample.CourseDeg,
		Satellites: sample.Satellites, FixQuality: sample.FixQuality,
		AccuracyM: sample.AccuracyM, HasAccuracy: true, TimeUTC: at,
	}, at) + bot.EmitGGA(bot.GPSFix{
		Valid: true, Lat: sample.Lat, Lng: sample.Lng,
		AltitudeM: sample.AltitudeM, HasAltitude: true,
		Satellites: sample.Satellites, FixQuality: sample.FixQuality, TimeUTC: at,
	}) + bot.EmitGST(bot.GPSFix{
		Valid: true, Lat: sample.Lat, Lng: sample.Lng,
		AccuracyM: sample.AccuracyM, HasAccuracy: true, TimeUTC: at,
	})

	got := readExactly(t, conn, len(want))
	if string(got) != want {
		t.Fatalf("the feed published %q, want %q", got, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reader := bot.NewGPSReader(strings.NewReader(string(got)))
	go func() { _ = reader.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	var fix bot.GPSFix
	for time.Now().Before(deadline) {
		if fix = reader.LastFix(); fix.Valid {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !fix.Valid {
		t.Fatal("the client's own reader found no valid fix in what the engine published")
	}
	// A sentence carries four decimal minutes, which bounds the round trip at
	// about 18 cm, so the fix is compared within a metre rather than for equality.
	if math.Abs(fix.Lat-sample.Lat) > 1e-5 || math.Abs(fix.Lng-sample.Lng) > 1e-5 {
		t.Errorf("the reader made the fix %v,%v, want %v,%v", fix.Lat, fix.Lng, sample.Lat, sample.Lng)
	}
	if !fix.HasAltitude || fix.AltitudeM != sample.AltitudeM {
		t.Errorf("the reader made the altitude %v (measured: %v), want %v", fix.AltitudeM, fix.HasAltitude, sample.AltitudeM)
	}
}

func TestEngineFeedsAHeadingTheClientsOwnReaderParses(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })

	conn := dialFeed(t, e.Feed().Addr())
	waitForSubscribers(t, e.Feed(), 1)

	sample := HeadingSample{Time: time.Now(), MagneticDeg: 47.5, HasMagnetic: true, TrueDeg: 51.2, HasTrue: true}
	if err := e.FeedHeading(sample); err != nil {
		t.Fatalf("FeedHeading: %v", err)
	}

	// The true heading is the one published when the device knows both: a reader
	// holds one heading, and the one a person reads off a map is the true one.
	want := bot.EmitHDT(sample.TrueDeg, sample.Time)
	got := readExactly(t, conn, len(want))
	if string(got) != want {
		t.Fatalf("the feed published %q, want %q", got, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reader := bot.NewCompassReader(strings.NewReader(string(got)))
	go func() { _ = reader.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	var heading bot.CompassHeading
	for time.Now().Before(deadline) {
		if heading = reader.LastHeading(); heading.Valid {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !heading.Valid {
		t.Fatal("the client's own reader found no heading in what the engine published")
	}
	if !heading.HasTrue || math.Abs(heading.TrueDeg-sample.TrueDeg) > 1e-3 {
		t.Errorf("the reader made the true heading %v (known: %v), want %v",
			heading.TrueDeg, heading.HasTrue, sample.TrueDeg)
	}
}

func TestEngineFeedsAMagneticHeadingWhenItIsAllThereIs(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })

	conn := dialFeed(t, e.Feed().Addr())
	waitForSubscribers(t, e.Feed(), 1)

	sample := HeadingSample{Time: time.Now(), MagneticDeg: 47.5, HasMagnetic: true}
	if err := e.FeedHeading(sample); err != nil {
		t.Fatalf("FeedHeading: %v", err)
	}

	// A device with no position cannot compute the magnetic variation and so
	// cannot honestly report a true heading; a magnetic heading is what it has.
	want := bot.EmitHDM(sample.MagneticDeg, sample.Time)
	got := readExactly(t, conn, len(want))
	if string(got) != want {
		t.Fatalf("the feed published %q, want %q", got, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reader := bot.NewCompassReader(strings.NewReader(string(got)))
	go func() { _ = reader.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	var heading bot.CompassHeading
	for time.Now().Before(deadline) {
		if heading = reader.LastHeading(); heading.Valid {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !heading.HasMagnetic || math.Abs(heading.MagneticDeg-sample.MagneticDeg) > 1e-3 {
		t.Errorf("the reader made the magnetic heading %v (known: %v), want %v",
			heading.MagneticDeg, heading.HasMagnetic, sample.MagneticDeg)
	}
}

func TestEnginePublishesNothingForAWithheldHeading(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })

	conn := dialFeed(t, e.Feed().Addr())
	waitForSubscribers(t, e.Feed(), 1)

	// A heading the device could not measure is silence, not a zero: zero degrees
	// is due north, and a reader cannot tell the two apart.
	if err := e.FeedHeading(HeadingSample{Time: time.Now()}); err != nil {
		t.Fatalf("FeedHeading: %v", err)
	}
	if err := e.FeedHeading(HeadingSample{Time: time.Now(), MagneticDeg: 12, HasMagnetic: true}); err != nil {
		t.Fatalf("FeedHeading: %v", err)
	}

	want := bot.EmitHDM(12, time.Time{})
	// The withheld reading must not have produced a sentence, so the first bytes
	// the reader sees are the second reading's, byte for byte.
	got := readExactly(t, conn, len(want))
	if string(got) != want {
		t.Errorf("the feed published %q, want the measured reading only (%q)", got, want)
	}
}

func TestEngineDefaultFeedPortIsThePinnedOne(t *testing.T) {
	t.Parallel()
	e, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if e.cfg.FeedPort != DefaultFeedPort {
		t.Errorf("the engine's default feed port is %v, want %v", e.cfg.FeedPort, DefaultFeedPort)
	}
}

func TestEngineRefusesAPortThatIsNotOne(t *testing.T) {
	t.Parallel()
	if _, err := NewEngine(EngineConfig{FeedPort: 70000}); err == nil {
		t.Error("NewEngine with a port above 65535 returned no error, want one")
	}
}
