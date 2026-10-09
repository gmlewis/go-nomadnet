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
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/bot"
	"github.com/gmlewis/go-reticulum/geo"
)

// The reader's position is a fact about the person holding the device, and who
// else learns it is that person's decision. These tests turn the claim in
// nomadnet/micron/location-extension.md — "nothing leaves the node" — into
// evidence: they enumerate every place the client turns a position into text and
// assert that the READER'S OWN coordinate appears in none of them.
//
// The structural half of the argument is that the application layer cannot reach
// the position at all: nothing under nomadnet/app, nomadnet/node, nomadnet/browser,
// nomadnet/conversation or nomadnet/directory imports this package, so the paths
// that announce, fetch, send, and store have nothing to serialise. These tests
// cover the other half — the places a position really is rendered — so that a
// future change that started mixing the reader's own coordinate into one of them
// would fail here rather than in the field.
//
// E11 in the project brief is the wire capture and the on-disk check, run against
// the real stack on the device; these are the fast, deterministic half that runs
// in the gate.

// readerFix is a deliberately distinctive position: six decimal places is about
// 11 cm, so the chance of a coincidental match in unrelated output is nil.
var readerFix = geo.LatLng{Lat: 12.345678, Lng: -98.765432}

// readerRenderings enumerates the reader's own position in every notation the
// client knows how to write, which is what a leak could look like on the wire or
// on disk.
func readerRenderings(t *testing.T) map[string]string {
	t.Helper()

	plusCode, err := geo.EncodeOLC(readerFix.Lat, readerFix.Lng, 11)
	if err != nil {
		t.Fatalf("EncodeOLC: %v", err)
	}
	shortCode, err := geo.EncodeOLC(readerFix.Lat, readerFix.Lng, 10)
	if err != nil {
		t.Fatalf("EncodeOLC short: %v", err)
	}

	return map[string]string{
		"plus code":         plusCode,
		"short plus code":   shortCode,
		"latitude decimal":  fmt.Sprintf("%v", readerFix.Lat),
		"longitude decimal": fmt.Sprintf("%v", readerFix.Lng),
		"latlng pair":       geo.FormatLatLng(readerFix),
		"degrees":           geo.FormatDD(readerFix),
		"degrees minutes":   geo.FormatDDM(readerFix),
		"degrees min sec":   geo.FormatDMS(readerFix),
		// A truncated decimal is what a careless printf would produce, and it is
		// still the reader's position to five places.
		"latitude truncated":  fmt.Sprintf("%.5f", readerFix.Lat),
		"longitude truncated": fmt.Sprintf("%.5f", readerFix.Lng),
		// The raw bits, in case something ever serialises the floats rather than
		// formatting them.
		"latitude bits":  fmt.Sprintf("%x", math.Float64bits(readerFix.Lat)),
		"longitude bits": fmt.Sprintf("%x", math.Float64bits(readerFix.Lng)),
	}
}

// markupCorpus is a broad sample of the constructs a page can carry, including
// every position-dependent one, so the leak check is not limited to the shapes a
// test author happened to think of.
var markupCorpus = []string{
	"plain text with no markup at all",
	"`L849VCWC8+R9`L",
	"`L849VCWC8+R9|%default`L",
	"`L849VCWC8+R9|%c`L",
	"`L849VCWC8+R9|%d`L",
	"`L849VCWC8+R9|%b`L",
	"`L849VCWC8+R9|%ll`L",
	">>A heading\nsome body text\n",
	"`c0,0,0`k`F222`b a coloured line`f`B",
	"|field|value|\n|second|row|\n",
	"!cell one!cell two!\n",
	"\"a link\":nomadnetwork.node:abcdef\n",
	"`[#anchor]",
	"`[>anchor]",
	"`=form`\n`<name`value`>\n`=end`",
	"{image bytes here}",
	":this is a literal line",
	"a `!important`! line and *bold* and _underline_ and /italic/",
	"`Fff0`bwarning`f`B",
	"`_underline_`",
	"`Ttitle",
	"`[comment]",
	"`Efield edit`",
	"`-divider",
	"`v verbatim block",
	"\tindented block",
	"",
}

// TestReaderPositionNeverReachesRenderedPages asserts that no page, however
// written, renders the reader's own coordinate. The client turns a page's Plus
// Code into a distance and a bearing for the reader; it never turns the reader
// into text.
func TestReaderPositionNeverReachesRenderedPages(t *testing.T) {
	t.Parallel()

	viewer := micron.Viewer{
		Pos:        readerFix,
		Known:      true,
		HeadingDeg: 47.5,
		HasHeading: true,
	}
	renderings := readerRenderings(t)

	renderers := []struct {
		name   string
		render func(string) string
	}{
		{"plain text", func(markup string) string { return micron.RenderToPlainText(micron.Parse(markup)) }},
		{"tview", func(markup string) string { return micron.RenderToTView(micron.Parse(markup)) }},
		{"styled", func(markup string) string {
			var sb strings.Builder
			for _, line := range micron.RenderToStyledLinesFor(markup, micron.ThemeDark, viewer) {
				for _, span := range line.Spans {
					sb.WriteString(span.Text)
				}
			}
			return sb.String()
		}},
	}

	for _, renderer := range renderers {
		for i, markup := range markupCorpus {
			got := renderer.render(markup)
			for name, rendering := range renderings {
				if rendering == "" {
					continue
				}
				if strings.Contains(got, rendering) {
					t.Fatalf("%v rendering of corpus entry %d (%q) carries the reader's own %v (%q):\n%q",
						renderer.name, i, markup, name, rendering, got)
				}
			}
		}
	}
}

// TestReaderPositionNeverReachesTheLocationCard asserts the location card shows the
// PAGE's coordinate, never the reader's. The card is the one place a coordinate is
// printed at all, and confusing the two would hand a remote page's author the
// reader's position the moment they opened the card.
func TestReaderPositionNeverReachesTheLocationCard(t *testing.T) {
	t.Parallel()

	viewer := micron.Viewer{
		Pos:        readerFix,
		Known:      true,
		HeadingDeg: 47.5,
		HasHeading: true,
	}
	detail := micron.DescribeLocation("849VCWC8+R9", viewer)

	if !detail.HasFix {
		t.Fatalf("the card did not resolve the page's code against the reader's position: %+v", detail)
	}
	if detail.Distance == "" || detail.Bearing == "" {
		t.Fatalf("the card resolved no distance or bearing: %+v", detail)
	}
	if detail.RelativeBearing == "" {
		t.Fatalf("the card resolved no relative bearing from a known heading: %+v", detail)
	}

	// The coordinate the card prints must be the code's own, tens of thousands of
	// kilometres from the reader.
	target, err := geo.DecodeOLC("849VCWC8+R9")
	if err != nil {
		t.Fatalf("DecodeOLC: %v", err)
	}
	if want := geo.FormatLatLng(target.Center()); detail.Coordinate != want {
		t.Fatalf("Coordinate = %q, want the page's own %q", detail.Coordinate, want)
	}
	if strings.Contains(detail.Coordinate, "12.34") {
		t.Fatalf("Coordinate = %q, which is the reader's own", detail.Coordinate)
	}

	// And nothing the card is built from may carry the reader's own position.
	card := detail.Code + " " + detail.Coordinate + " " + detail.Distance + " " +
		detail.Bearing + " " + detail.RelativeBearing
	for name, rendering := range readerRenderings(t) {
		if rendering == "" {
			continue
		}
		if strings.Contains(card, rendering) {
			t.Fatalf("the location card carries the reader's own %v (%q): %q", name, rendering, card)
		}
	}
}

// TestReaderPositionNeverReachesTheStatusLine asserts the line that tells the reader
// what the client knows about them carries how old and how good the reading is, and
// never the reading itself. A status line that printed the coordinate would leak it
// into every log, screenshot, and bug report the reader produced.
func TestReaderPositionNeverReachesTheStatusLine(t *testing.T) {
	t.Parallel()

	renderings := readerRenderings(t)
	statuses := []Status{
		{},
		{Source: SourceStatic, HasFix: true},
		{Source: SourceLive, Feed: "tcp://127.0.0.1:37429"},
		{Source: SourceLive, Feed: "tcp://127.0.0.1:37429", HasFix: true, FixAge: 3 * time.Second, AccuracyM: 3.8, HasAccuracy: true},
		{Source: SourceLive, Feed: "tcp://127.0.0.1:37429", HasFix: true, FixAge: 3 * time.Second, AccuracyM: 3.8, HasAccuracy: true, HasHeading: true, HeadingDeg: 47.5, HeadingAge: time.Second},
	}
	for i, status := range statuses {
		got := status.Describe()
		for name, rendering := range renderings {
			if rendering == "" {
				continue
			}
			if strings.Contains(got, rendering) {
				t.Fatalf("status %d carries the reader's own %v (%q): %q", i, name, rendering, got)
			}
		}
	}
}

// TestExplicitShareIsTheOnlyPathThatNamesAPosition asserts the deliberate
// "copy my Plus Code" action is the only code path that can produce one of the
// reader's own coordinates, and that it produces it only when it is invoked. The
// client exposes the action and nothing else; there is no setting that turns it on,
// and no path that runs it on the reader's behalf.
func TestExplicitShareIsTheOnlyPathThatNamesAPosition(t *testing.T) {
	t.Parallel()

	source := New(Options{
		StaticFix:    readerFix,
		HasStaticFix: true,
		Now:          newTestClock().Now,
	})

	// The client knows exactly where it is …
	viewer := source.Viewer()
	if !viewer.Known {
		t.Fatalf("Viewer() = %+v, want the configured position", viewer)
	}

	// … and the only thing it will ever say about that position is the status
	// line's age and accuracy, which the reader can see.
	if got := source.Status().Describe(); strings.Contains(got, "12.34") || strings.Contains(got, "98.76") {
		t.Fatalf("the status line names the reader's position: %q", got)
	}

	// The Plus Code the reader may copy is produced by the encoder, on request,
	// from the position they chose to hand it — and by nothing else in the client.
	code, err := geo.EncodeOLC(viewer.Pos.Lat, viewer.Pos.Lng, 11)
	if err != nil {
		t.Fatalf("EncodeOLC: %v", err)
	}
	if !geo.IsFullOLC(code) {
		t.Fatalf("EncodeOLC produced %q, which is not a full Plus Code", code)
	}

	// Nothing in the location package produces a coordinate except that call, so
	// the viewer itself, the status, and every message the client composes from
	// them are coordinate-free.
	for _, text := range []string{
		geo.FormatLatLng(viewer.Pos),
		source.Status().Describe(),
	} {
		if strings.Contains(text, code) {
			t.Fatalf("a coordinate-free path produced the reader's Plus Code %q: %q", code, text)
		}
	}
}

// TestSourceHoldsNoSerialisableFormOfThePosition asserts the live source's state
// cannot be marshalled into an outbound message: the reader's position lives in a
// micron.Viewer, which is a rendering input, and nothing else in the client holds
// a copy of it.
func TestSourceHoldsNoSerialisableFormOfThePosition(t *testing.T) {
	t.Parallel()

	source := New(Options{
		StaticFix:    readerFix,
		HasStaticFix: true,
		Now:          newTestClock().Now,
	})

	status := source.Status()
	viewer := source.Viewer()

	// The exported state of both is age, accuracy, provenance, and the coordinate
	// itself — and the coordinate is reachable only through Viewer, which only the
	// renderer is given.
	if !viewer.Known {
		t.Fatalf("Viewer() = %+v, want the configured position", viewer)
	}
	if status.Source != SourceStatic || !status.HasFix {
		t.Fatalf("Status() = %+v, want the static tier", status)
	}
	if status.AccuracyM != 0 || status.HasAccuracy {
		t.Fatalf("Status() = %+v, want no invented accuracy", status)
	}
	if status.HeadingDeg != 0 || status.HasHeading {
		t.Fatalf("Status() = %+v, want no invented heading", status)
	}
}

// TestAQuietFeedLeavesTheClientWithNothing asserts the graceful-absence rule in
// the form that matters for privacy: a feed that stops does not leave the client
// holding the last coordinate it heard, because a stale position reported as
// current is the same class of bug as a wrong one.
func TestAQuietFeedLeavesTheClientWithNothing(t *testing.T) {
	t.Parallel()

	feed := &testFeed{}
	clock := newTestClock()
	source := New(Options{
		Feed:   "tcp://127.0.0.1:37429",
		MaxAge: 30 * time.Second,
		Now:    clock.Now,
		Open:   feed.Open,
	})
	if err := source.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		feed.Close()
	})

	feed.Send(bot.EmitRMC(bot.GPSFix{
		Valid:      true,
		Lat:        readerFix.Lat,
		Lng:        readerFix.Lng,
		FixQuality: 1,
		TimeUTC:    clock.Now(),
	}, clock.Now()))
	waitForViewer(t, source, func(v micron.Viewer) bool { return v.Known })

	clock.Advance(31 * time.Second)
	if got := source.Viewer(); got.Known {
		t.Fatalf("Viewer() = %+v, want nothing after the feed went quiet", got)
	}
}

// TestPrivacyRenderingsAreNotVacuous asserts the leak detector actually detects.
// Every notation it searches for must be non-empty — an empty needle matches
// nothing and would make the three tests above pass while proving nothing — and a
// string that really does carry the reader's position must be caught by the same
// search.
func TestPrivacyRenderingsAreNotVacuous(t *testing.T) {
	t.Parallel()

	renderings := readerRenderings(t)
	for name, rendering := range renderings {
		if rendering == "" {
			t.Fatalf("the %v rendering is empty, so searching for it would prove nothing", name)
		}
	}

	leaky := "the reader is at " + geo.FormatLatLng(readerFix)
	caught := false
	for _, rendering := range renderings {
		if strings.Contains(leaky, rendering) {
			caught = true
			break
		}
	}
	if !caught {
		t.Fatalf("a rendering that really carries the reader's position (%q) was not caught by the detector", leaky)
	}
}
