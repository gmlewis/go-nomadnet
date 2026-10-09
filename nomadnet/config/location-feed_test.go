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

package config

import (
	"testing"
	"time"
)

// TestApplyLocationSensorFeed pins the live half of the [location] section: a
// sensor feed is how a tablet or a phone hands this client its own live position,
// and it is deliberately absent by default so that a sensorless install keeps
// using its static fix. Live and static are the two tiers of one rule — a live
// source is the only source — so the section must never enable a feed on its own.
func TestApplyLocationSensorFeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        map[string]map[string]string
		wantFeed   string
		wantMaxAge time.Duration
		wantStatic string
		wantKnown  bool
	}{
		{
			name: "absent section leaves the feed off",
			raw:  map[string]map[string]string{},
		},
		{
			name: "a fix alone never enables a feed",
			raw: map[string]map[string]string{
				"location": {"fix": "37.4220, -122.0841"},
			},
			wantStatic: "37.4220, -122.0841",
			wantKnown:  true,
		},
		{
			name: "a feed alone leaves the client position-less until it hears one",
			raw: map[string]map[string]string{
				"location": {"sensor_feed": "tcp://127.0.0.1:37430"},
			},
			wantFeed: "tcp://127.0.0.1:37430",
		},
		{
			name: "a feed beside a fix keeps both, the feed winning at read time",
			raw: map[string]map[string]string{
				"location": {
					"fix":         "45.0,-93.0",
					"sensor_feed": "unix://@gonomadnet/sensor",
				},
			},
			wantFeed:   "unix://@gonomadnet/sensor",
			wantStatic: "45.0,-93.0",
			wantKnown:  true,
		},
		{
			name: "an explicit max age is honoured",
			raw: map[string]map[string]string{
				"location": {"sensor_feed": "tcp://127.0.0.1:1", "max_age": "45"},
			},
			wantFeed:   "tcp://127.0.0.1:1",
			wantMaxAge: 45 * time.Second,
		},
		{
			name: "a nonsensical max age falls back to the default",
			raw: map[string]map[string]string{
				"location": {"sensor_feed": "tcp://127.0.0.1:1", "max_age": "soon"},
			},
			wantFeed:   "tcp://127.0.0.1:1",
			wantMaxAge: DefaultLocationMaxAge,
		},
		{
			name: "a negative max age falls back to the default",
			raw: map[string]map[string]string{
				"location": {"sensor_feed": "tcp://127.0.0.1:1", "max_age": "-5"},
			},
			wantFeed:   "tcp://127.0.0.1:1",
			wantMaxAge: DefaultLocationMaxAge,
		},
		{
			name: "a zero max age means no ageing is asked for",
			raw: map[string]map[string]string{
				"location": {"sensor_feed": "tcp://127.0.0.1:1", "max_age": "0"},
			},
			wantFeed:   "tcp://127.0.0.1:1",
			wantMaxAge: DefaultLocationMaxAge,
		},
		{
			name: "blank values are absent values",
			raw: map[string]map[string]string{
				"location": {"fix": "   ", "sensor_feed": "  "},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := DefaultConfig()
			c.Raw = tc.raw
			c.applyLocation()

			if got := c.Location.SensorFeed; got != tc.wantFeed {
				t.Fatalf("SensorFeed = %q, want %q", got, tc.wantFeed)
			}
			// A case that does not name a window is asserting the default one.
			wantMaxAge := tc.wantMaxAge
			if wantMaxAge == 0 {
				wantMaxAge = DefaultLocationMaxAge
			}
			if got := c.Location.MaxAge; got != wantMaxAge {
				t.Fatalf("MaxAge = %v, want %v", got, wantMaxAge)
			}
			if got := c.Location.Fix; got != tc.wantStatic {
				t.Fatalf("Fix = %q, want %q", got, tc.wantStatic)
			}
			if c.Location.Known != tc.wantKnown {
				t.Fatalf("Known = %v, want %v", c.Location.Known, tc.wantKnown)
			}
		})
	}
}

// TestDefaultLocationMaxAgeIsBounded asserts the default staleness window is
// short enough that a dead feed degrades to no position promptly, because a
// confidently stale position is the same class of bug as a confidently wrong one.
func TestDefaultLocationMaxAgeIsBounded(t *testing.T) {
	t.Parallel()

	if DefaultLocationMaxAge <= 0 {
		t.Fatalf("DefaultLocationMaxAge = %v, want a positive window", DefaultLocationMaxAge)
	}
	if DefaultLocationMaxAge > time.Minute {
		t.Fatalf("DefaultLocationMaxAge = %v, want no longer than a minute", DefaultLocationMaxAge)
	}
}
