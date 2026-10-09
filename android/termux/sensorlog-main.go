// sensorlog records the tablet's own position and heading while somebody carries
// it about, so the measurements can be read back afterwards without a cable.
//
// It reads the NMEA feed the gonomadnet appliance publishes on loopback, decodes
// it with the repository's own parsers — the same ones the bot and the client use,
// so there is one interpretation of the sentences rather than two — and writes a
// line per reading to a directory it is given, with a monotonic timestamp so that a
// heading can be lined up with an event even though the NMEA heading sentences
// carry no time of their own.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/bot"
)

const (
	// defaultFeed is the loopback endpoint the appliance publishes its multiplexed
	// NMEA stream on.
	defaultFeed = "tcp://127.0.0.1:37430"
	// sampleInterval is how often the readers are polled for a new reading to record.
	sampleInterval = 200 * time.Millisecond
	// statusInterval is how often a line of status is printed for the operator.
	statusInterval = 2 * time.Second
)

// record is one line of the output: a reading, or a marker the operator typed.
type record struct {
	Kind string `json:"kind"`

	// Seconds is the monotonic clock since the run started, which is what an event is
	// lined up against.
	Seconds float64 `json:"seconds"`
	// Wall is the device's own clock, which is only a convenience.
	Wall string `json:"wall"`

	// Position.
	HasFix      bool    `json:"hasFix"`
	Lat         float64 `json:"lat,omitempty"`
	Lng         float64 `json:"lng,omitempty"`
	AltitudeM   float64 `json:"altM,omitempty"`
	HasAltitude bool    `json:"hasAlt,omitempty"`
	AccuracyM   float64 `json:"accM,omitempty"`
	HasAccuracy bool    `json:"hasAcc,omitempty"`
	Satellites  int     `json:"sats,omitempty"`
	HDOP        float64 `json:"hdop,omitempty"`
	FixQuality  int     `json:"quality,omitempty"`
	CourseDeg   float64 `json:"courseDeg,omitempty"`

	// Heading.
	HasTrue     bool    `json:"hasTrue,omitempty"`
	HasMagnetic bool    `json:"hasMag,omitempty"`
	TrueDeg     float64 `json:"trueDeg,omitempty"`
	MagneticDeg float64 `json:"magDeg,omitempty"`
	Declination float64 `json:"declDeg,omitempty"`

	// Marker.
	Note string `json:"note,omitempty"`
}

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sensorlog <output directory> [feed endpoint]")
		os.Exit(2)
	}
	outDir := os.Args[1]
	feed := defaultFeed
	if len(os.Args) > 2 && strings.TrimSpace(os.Args[2]) != "" {
		feed = os.Args[2]
	}
	if err := run(outDir, feed); err != nil {
		log.Fatalf("sensorlog: %v", err)
	}
}

func run(outDir, feed string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating %v: %w", outDir, err)
	}
	samplesPath := filepath.Join(outDir, "samples.jsonl")
	samples, err := os.Create(samplesPath)
	if err != nil {
		return fmt.Errorf("creating %v: %w", samplesPath, err)
	}
	defer func() { _ = samples.Close() }()
	writer := bufio.NewWriter(samples)

	markersPath := filepath.Join(outDir, "markers.txt")
	markers, err := os.Create(markersPath)
	if err != nil {
		return fmt.Errorf("creating %v: %w", markersPath, err)
	}
	defer func() { _ = markers.Close() }()

	// Two connections to the same multiplexed feed: one reader consumes the position
	// sentences and the other the heading sentences, and neither starves the other.
	gpsStream, err := bot.OpenSensorSource(feed)
	if err != nil {
		return fmt.Errorf("opening %v: %w", feed, err)
	}
	defer func() { _ = gpsStream.Close() }()
	compassStream, err := bot.OpenSensorSource(feed)
	if err != nil {
		return fmt.Errorf("opening %v: %w", feed, err)
	}
	defer func() { _ = compassStream.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gps := bot.NewGPSReader(gpsStream)
	compass := bot.NewCompassReader(compassStream)
	compass.SetLocationSource(func() (bot.GPSFix, bool) {
		fix := gps.LastFix()
		return fix, fix.Valid
	})
	gps.Start(ctx)
	compass.Start(ctx)

	started := time.Now()
	var mu sync.Mutex
	var markersSeen []string
	var notes []string
	finished := make(chan struct{})

	// Anything the operator types is a marker. They are read on their own goroutine so
	// the recording never waits for a keystroke.
	go func() {
		defer close(finished)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			elapsed := time.Since(started).Seconds()
			if text == "" {
				continue
			}
			mu.Lock()
			markersSeen = append(markersSeen, fmt.Sprintf("%8.2f  %s", elapsed, text))
			notes = append(notes, text)
			mu.Unlock()
			_, _ = fmt.Fprintf(markers, "%8.2f  %s\n", elapsed, text)
			_ = markers.Sync()
			if strings.EqualFold(text, "q") {
				return
			}
		}
	}()

	fmt.Printf("recording to %v\n", outDir)
	fmt.Println("type r then Enter after each 90-degree turn of the stand; q then Enter when done")

	var lastFix bot.GPSFix
	var lastHeading bot.CompassHeading
	lastStatus := time.Now()
	var firstFixAt, firstHeadingAt float64
	count := 0

	for {
		select {
		case <-finished:
			return summarise(outDir, started, samplesPath, markersPath, firstFixAt, firstHeadingAt, count)
		case <-time.After(sampleInterval):
		}

		fix := gps.LastFix()
		heading := compass.LastHeading()
		now := time.Since(started).Seconds()

		if fix.Valid && firstFixAt == 0 {
			firstFixAt = now
		}
		if heading.Valid && firstHeadingAt == 0 {
			firstHeadingAt = now
		}

		if changed(lastFix, fix, lastHeading, heading) {
			lastFix, lastHeading = fix, heading
			line, err := json.Marshal(record{
				Kind:        "sample",
				Seconds:     now,
				Wall:        time.Now().Format(time.RFC3339Nano),
				HasFix:      fix.Valid,
				Lat:         fix.Lat,
				Lng:         fix.Lng,
				AltitudeM:   fix.AltitudeM,
				HasAltitude: fix.HasAltitude,
				AccuracyM:   fix.AccuracyM,
				HasAccuracy: fix.HasAccuracy,
				Satellites:  fix.Satellites,
				HDOP:        fix.HDOP,
				FixQuality:  fix.FixQuality,
				CourseDeg:   fix.CourseDeg,
				HasTrue:     heading.HasTrue,
				HasMagnetic: heading.HasMagnetic,
				TrueDeg:     heading.TrueDeg,
				MagneticDeg: heading.MagneticDeg,
				Declination: heading.DeclinationDeg,
			})
			if err != nil {
				return fmt.Errorf("encoding a sample: %w", err)
			}
			if _, err := writer.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("writing a sample: %w", err)
			}
			count++
		}

		if time.Since(lastStatus) >= statusInterval {
			lastStatus = time.Now()
			_ = writer.Flush()
			fmt.Printf("\r%s   [%v samples]", statusLine(fix, heading, now), count)
		}
	}
}

// changed reports whether either reader has something new, so an unchanged feed does
// not fill the log with copies of the same reading.
func changed(prevFix bot.GPSFix, fix bot.GPSFix, prevHeading bot.CompassHeading, heading bot.CompassHeading) bool {
	return !sameFix(prevFix, fix) || !sameHeading(prevHeading, heading)
}

// sameFix compares the fields a recording cares about.
func sameFix(a, b bot.GPSFix) bool {
	return a.Valid == b.Valid && a.Lat == b.Lat && a.Lng == b.Lng &&
		a.AltitudeM == b.AltitudeM && a.AccuracyM == b.AccuracyM &&
		a.Satellites == b.Satellites && a.FixQuality == b.FixQuality
}

// sameHeading compares the fields a recording cares about.
func sameHeading(a, b bot.CompassHeading) bool {
	return a.Valid == b.Valid && a.TrueDeg == b.TrueDeg && a.MagneticDeg == b.MagneticDeg &&
		a.HasTrue == b.HasTrue && a.HasMagnetic == b.HasMagnetic
}

// statusLine is the one-line readout the operator watches.
func statusLine(fix bot.GPSFix, heading bot.CompassHeading, now float64) string {
	position := "no position yet"
	if fix.Valid {
		position = fmt.Sprintf("%.6f,%.6f", fix.Lat, fix.Lng)
		if fix.HasAccuracy {
			position += fmt.Sprintf(" +-%.1fm", fix.AccuracyM)
		}
		if fix.Satellites > 0 {
			position += fmt.Sprintf(" sats%d", fix.Satellites)
		}
	}
	direction := "no heading"
	switch {
	case heading.HasTrue:
		direction = fmt.Sprintf("hdg %.1f true", heading.TrueDeg)
	case heading.HasMagnetic:
		direction = fmt.Sprintf("hdg %.1f mag", heading.MagneticDeg)
	}
	return fmt.Sprintf("%-42s  %-16s %6.1fs", position, direction, now)
}

// summarise writes the one file somebody reads first.
func summarise(outDir string, started time.Time, samplesPath, markersPath string, firstFixAt, firstHeadingAt float64, count int) error {
	summaryPath := filepath.Join(outDir, "summary.txt")
	var b strings.Builder
	fmt.Fprintf(&b, "run started:  %v\n", started.Format(time.RFC3339))
	fmt.Fprintf(&b, "duration:     %.1f s\n", time.Since(started).Seconds())
	fmt.Fprintf(&b, "samples:      %d\n", count)
	if firstFixAt > 0 {
		fmt.Fprintf(&b, "first fix:    %.1f s after the start\n", firstFixAt)
	} else {
		fmt.Fprintf(&b, "first fix:    never\n")
	}
	if firstHeadingAt > 0 {
		fmt.Fprintf(&b, "first heading: %.1f s after the start\n", firstHeadingAt)
	} else {
		fmt.Fprintf(&b, "first heading: never; the reference axis was probably ill-conditioned\n")
	}
	fmt.Fprintf(&b, "samples:      %v\n", samplesPath)
	fmt.Fprintf(&b, "markers:      %v\n", markersPath)
	if err := os.WriteFile(summaryPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing %v: %w", summaryPath, err)
	}
	fmt.Printf("\n%v", b.String())
	return nil
}
