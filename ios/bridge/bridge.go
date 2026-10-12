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

//go:build ios

package bridge

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/gmlewis/go-nomadnet/nomadnet/version"
)

// This is the appliance's C boundary, and the only file in the package that imports "C".
//
// It is built only for GOOS=ios. A file that imports "C" makes cgo generate Go
// sources, and an analyzer that walks the package is then shown code nobody here wrote
// and nobody here can keep clean. Confining the boundary to one file keeps the rest of the
// appliance ordinary Go, and this file is verified by the build that uses it:
// scripts/build-ios-app.sh compiles it for ios/arm64 and fails if any entry point is
// missing from the archive.
//
// Every entry point is a thin wrapper over an ordinary Go function that a test calls, so
// the appliance's behavior is tested without a device, a simulator or a C compiler.

// The C boundary's own error channel.
//
// It lives here because only the exported functions below report through it: a status code
// says a call failed, and the reason it failed is a sentence the Swift half shows or logs.
// A failure recorded by something that is not the boundary would be a failure nobody is
// going to ask about.
var (
	lastErrMu sync.Mutex
	lastErr   string
)

// setLastError records why the last exported call failed.
func setLastError(format string, args ...any) {
	applianceMu.Lock()
	lastErr = fmt.Sprintf(format, args...)
	applianceMu.Unlock()
}

// LastError is why the last exported call failed, or the empty string.
func LastError() string {
	applianceMu.Lock()
	defer applianceMu.Unlock()
	return lastErr
}

// ClearLastError forgets the last failure.
func ClearLastError() {
	applianceMu.Lock()
	lastErr = ""
	applianceMu.Unlock()
}

// Appliance is the process's host, for the host binary that has to register the
// client runner.
//
// It is the host that is the appliance now, which is not the one a stop shut down:
// registering the runner once at init has to reach the host that starts later.
func Appliance() *Host { return currentAppliance() }

// The C boundary's own helpers.
//
// They exist only for the exported functions below: a measurement the device did not take
// arrives across the boundary as a NaN, and a timestamp as seconds since the epoch, because
// that is what a C caller has to say them in.

// timeFromUnix turns the seconds-since-the-epoch a device reports into a time. A stamp of
// zero, or one that is not a number, is the zero time, which is how the NMEA emitters are
// told that nobody said when the reading was taken.
func timeFromUnix(seconds float64) time.Time {
	if math.IsNaN(seconds) || seconds <= 0 {
		return time.Time{}
	}
	whole, frac := math.Modf(seconds)
	return time.Unix(int64(whole), int64(frac*float64(time.Second))).UTC()
}

// isNaN reports a measurement the device did not take.
func isNaN(v float64) bool { return math.IsNaN(v) }

// unsafePointer reinterprets a C pointer for C.free. It is here rather than inline so that
// the one unsafe conversion in the package is in one place.
func unsafePointer(p *C.char) unsafe.Pointer { return unsafe.Pointer(p) }

// The entry points the Swift half calls.
//
// Each returns a status code (see appliance.go) and, on failure, leaves a reason
// that GonomadnetLastError hands back. Every one is a wrapper over an ordinary Go
// function, because the alternative — reaching this logic only through a
// simulator and a C compiler — is how a bridge ends up with its whole behavior in
// the one place nobody can test.

//export GonomadnetStart
func GonomadnetStart(configDir, rnsConfigDir, clientRNSDir *C.char, feedPort C.int) C.int {
	ClearLastError()

	dirs := ApplianceDirs{
		ClientConfig: C.GoString(configDir),
		ClientRNS:    C.GoString(clientRNSDir),
		TransportRNS: C.GoString(rnsConfigDir),
	}
	err := StartAppliance(context.Background(), dirs.ClientConfig, dirs.TransportRNS, int(feedPort), dirs.Subsystems()...)
	if err != nil {
		setLastError("starting the appliance: %v", err)
	}
	return C.int(statusFor(err))
}

//export GonomadnetStop
func GonomadnetStop() C.int {
	ClearLastError()
	if err := StopAppliance(); err != nil {
		setLastError("stopping the appliance: %v", err)
		return C.int(statusFor(err))
	}
	return C.int(StatusOK)
}

//export GonomadnetStartConsole
func GonomadnetStartConsole(socketPath *C.char, cols, rows C.uint16_t) C.int {
	ClearLastError()
	if cols == 0 {
		cols = DefaultConsoleCols
	}
	if rows == 0 {
		rows = DefaultConsoleRows
	}
	if err := StartConsoleSession(context.Background(), C.GoString(socketPath), uint16(cols), uint16(rows)); err != nil {
		setLastError("starting the console: %v", err)
		return C.int(statusFor(err))
	}
	return C.int(StatusOK)
}

// GonomadnetFeedLocation publishes one GNSS fix.
//
// A measurement the device did not take is passed as NaN, which is what an
// unmeasured field means in a C API that has no room for a flag beside every
// double: NMEA writes an empty field for it, so a height nobody measured is never
// reported as standing at sea level.
//
//export GonomadnetFeedLocation
func GonomadnetFeedLocation(lat, lon, alt, hAcc, speed, course C.double, ts C.double) C.int {
	ClearLastError()

	sample := LocationSample{
		Time:      timeFromUnix(float64(ts)),
		Lat:       float64(lat),
		Lng:       float64(lon),
		Valid:     !isNaN(float64(lat)) && !isNaN(float64(lon)),
		AltitudeM: float64(alt),
		AccuracyM: float64(hAcc),
		SpeedKnots: func() float64 {
			if isNaN(float64(speed)) {
				return 0
			}
			return float64(speed)
		}(),
		CourseDeg: func() float64 {
			if isNaN(float64(course)) {
				return 0
			}
			return float64(course)
		}(),
	}
	sample.HasAltitude = !isNaN(float64(alt))
	sample.HasAccuracy = !isNaN(float64(hAcc))

	if err := FeedLocation(sample); err != nil {
		setLastError("publishing a position: %v", err)
		return C.int(statusFor(err))
	}
	return C.int(StatusOK)
}

// GonomadnetFeedHeading publishes one orientation reading.
//
// The attitude angles and the axis flag are accepted because the C ABI is fixed
// by bridge.h, and a heading sentence in NMEA-0183 has no field to put them in, so
// they are dropped here rather than invented into one. A heading the device could
// not measure is passed as NaN and publishes nothing at all: zero degrees is due
// north, and the reader cannot tell the two apart.
//
//export GonomadnetFeedHeading
func GonomadnetFeedHeading(magnetic, trueHeading, yaw, pitch, roll C.double, isBackAxis C.int) C.int {
	ClearLastError()

	_, _, _, _, _ = yaw, pitch, roll, isBackAxis, magnetic

	sample := HeadingSample{
		Time:        time.Now(),
		MagneticDeg: float64(magnetic),
		HasMagnetic: !isNaN(float64(magnetic)),
		TrueDeg:     float64(trueHeading),
		HasTrue:     !isNaN(float64(trueHeading)),
	}
	if err := FeedHeading(sample); err != nil {
		setLastError("publishing a heading: %v", err)
		return C.int(statusFor(err))
	}
	return C.int(StatusOK)
}

//export GonomadnetLastError
func GonomadnetLastError() *C.char {
	return C.CString(LastError())
}

//export GonomadnetFreeError
func GonomadnetFreeError(p *C.char) {
	C.free(unsafePointer(p))
}

//export GonomadnetVersion
func GonomadnetVersion() *C.char {
	return C.CString(version.VERSION)
}
