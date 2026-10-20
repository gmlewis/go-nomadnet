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

package tui

import (
	"testing"
	"time"

	"github.com/gmlewis/tview"
)

// TestRNodeCalculatorReadout pins RNodeCalculatorReadout against Python's
// RNodeCalculator.update_calculation (Interfaces.py:296-366). The expected
// strings were computed from Python's interfaces module: the calculator reads
// the form's bandwidth/spreadingfactor/codingrate/txpower (the colon-form
// coding rate "4:5" fails int() and falls back to 5, exactly as Python does)
// plus its own noise-floor and antenna-gain edits (an empty edit is 0).
func TestRNodeCalculatorReadout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fields       map[string]string
		noiseFloor   string
		antennaGain  string
		wantDataRate string
		wantLink     string
		wantSens     string
	}{
		{
			name:         "form defaults (7800/7/4:5, empty edits)",
			fields:       map[string]string{"bandwidth": "7800", "spreadingfactor": "7", "codingrate": "4:5", "txpower": "17"},
			wantDataRate: "Data Rate: 341 bps",
			wantLink:     "Link Budget: 159.6 dB",
			wantSens:     "Sensitivity: -142.6 dBm",
		},
		{
			name:         "missing fields fall back to python defaults (125000/7/5)",
			fields:       map[string]string{},
			wantDataRate: "Data Rate: 5.47 kbps",
			wantLink:     "Link Budget: 147.5 dB",
			wantSens:     "Sensitivity: -130.5 dBm",
		},
		{
			name:         "colon coding rate resolves to 5",
			fields:       map[string]string{"bandwidth": "125000", "spreadingfactor": "7", "codingrate": "4:7", "txpower": "17"},
			wantDataRate: "Data Rate: 5.47 kbps",
			wantLink:     "Link Budget: 147.5 dB",
			wantSens:     "Sensitivity: -130.5 dBm",
		},
		{
			name:         "bare numeric coding rate is honoured",
			fields:       map[string]string{"bandwidth": "125000", "spreadingfactor": "7", "codingrate": "7", "txpower": "17"},
			wantDataRate: "Data Rate: 3.91 kbps",
			wantLink:     "Link Budget: 147.5 dB",
			wantSens:     "Sensitivity: -130.5 dBm",
		},
		{
			name:         "custom noise floor, antenna gain and power",
			fields:       map[string]string{"bandwidth": "125000", "spreadingfactor": "10", "codingrate": "4:5", "txpower": "20"},
			noiseFloor:   "6",
			antennaGain:  "3",
			wantDataRate: "Data Rate: 977 bps",
			wantLink:     "Link Budget: 155.0 dB",
			wantSens:     "Sensitivity: -132.0 dBm",
		},
		{
			name:         "unparseable fields fall back to defaults",
			fields:       map[string]string{"bandwidth": "", "spreadingfactor": "x", "codingrate": "", "txpower": "abc"},
			wantDataRate: "Data Rate: 5.47 kbps",
			wantLink:     "Link Budget: 147.5 dB",
			wantSens:     "Sensitivity: -130.5 dBm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fieldValue := func(key string) (string, bool) {
				v, ok := tt.fields[key]
				return v, ok
			}
			dataRate, link, sens := RNodeCalculatorReadout(fieldValue, tt.noiseFloor, tt.antennaGain)
			if dataRate != tt.wantDataRate {
				t.Errorf("data rate = %q, want %q", dataRate, tt.wantDataRate)
			}
			if link != tt.wantLink {
				t.Errorf("link budget = %q, want %q", link, tt.wantLink)
			}
			if sens != tt.wantSens {
				t.Errorf("sensitivity = %q, want %q", sens, tt.wantSens)
			}
		})
	}
}

// formButtonLabels collects the labels of every UrwidButton in the form's pile.
func formButtonLabels(v *InterfaceFormView) []string {
	var out []string
	for _, it := range v.pile.items {
		if b, ok := it.widget.(*UrwidButton); ok {
			out = append(out, b.Label())
		}
	}
	return out
}

// TestInterfaceFormRNodeCalculatorButton pins the On-Air Calculations wiring in
// the Add form (Python AddInterfaceView, Interfaces.py:1695-1722): an RNode
// form carries a "Show On-Air Calculations" button that reveals the calculator
// panel, and a non-RNode form has no such button (nor a calculator view).
func TestInterfaceFormRNodeCalculatorButton(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	app.killRing = &killRing{}

	// A non-RNode form has no calculator.
	tcp := NewInterfaceAddFormView(app, IfaceTCPClient, InterfaceFormOptions{})
	if tcp.Calculator() != nil {
		t.Error("TCPClientInterface Add form must not build an on-air calculator")
	}
	for _, label := range formButtonLabels(tcp) {
		if label == "Show On-Air Calculations" {
			t.Error("TCPClientInterface Add form must not offer the on-air calculator button")
		}
	}

	// An RNode form has both.
	rnode := NewInterfaceAddFormView(app, IfaceRNode, InterfaceFormOptions{})
	if rnode.Calculator() == nil {
		t.Fatal("RNodeInterface Add form must build an on-air calculator")
	}
	if !hasButtonLabel(rnode, "Show On-Air Calculations") {
		t.Fatalf("RNodeInterface Add form is missing the on-air calculator button; buttons: %v", formButtonLabels(rnode))
	}
	if rnode.CalculatorVisible() {
		t.Error("calculator must start hidden")
	}

	rnode.ToggleCalculator()
	if !rnode.CalculatorVisible() {
		t.Error("ToggleCalculator must reveal the calculator")
	}
	if !hasButtonLabel(rnode, "Hide On-Air Calculations") {
		t.Errorf("button label must flip when shown; buttons: %v", formButtonLabels(rnode))
	}

	// The revealed panel sits in the pile with its full height (10 rows).
	shown := false
	for _, it := range rnode.pile.items {
		if it.widget == rnode.Calculator().Widget() {
			shown = true
			if it.height != rnode.Calculator().Height() {
				t.Errorf("calculator item height = %v, want %v", it.height, rnode.Calculator().Height())
			}
		}
	}
	if !shown {
		t.Error("calculator widget is not in the pile after ToggleCalculator")
	}

	// The readout follows the form's bandwidth field: setting it to 125000
	// changes the data rate from the 7800 default.
	rnode.SetValue("bandwidth", "125000")
	rnode.Calculator().Update()
	dataRate, _, _ := rnode.Calculator().Readout()
	if dataRate != "Data Rate: 5.47 kbps" {
		t.Errorf("readout after bandwidth=125000 = %q, want %q", dataRate, "Data Rate: 5.47 kbps")
	}
}

// hasButtonLabel reports whether the form's pile contains an UrwidButton with
// the given label.
func hasButtonLabel(v *InterfaceFormView, label string) bool {
	for _, it := range v.pile.items {
		if b, ok := it.widget.(*UrwidButton); ok && b.Label() == label {
			return true
		}
	}
	return false
}

var _ = tview.NewBox // keep tview import if assertions above change

// TestRNodeCalculatorDebounce pins Python's 0.3 s recompute debounce
// (Interfaces.py:291-302): a change schedules one delayed recalculation, and a
// burst of changes within the window collapses into a single recompute of the
// final values.
func TestRNodeCalculatorDebounce(t *testing.T) {
	t.Parallel()

	clk := &fakeClock{}
	reads := 0
	fieldValue := func(key string) (string, bool) {
		reads++
		switch key {
		case "bandwidth":
			return "7800", true
		case "spreadingfactor":
			return "7", true
		case "codingrate":
			return "4:5", true
		case "txpower":
			return "17", true
		}
		return "", false
	}
	c := newRNodeCalculatorView(nil, fieldValue, clk)
	_, initialLink, _ := c.Readout()

	reads = 0
	c.Update()
	perUpdate := reads
	if perUpdate == 0 {
		t.Fatal("Update read no fields")
	}

	// A single change: nothing before 0.3 s, exactly one recompute at it.
	reads = 0
	c.SetNoiseFloor("-120")
	clk.advance(299 * time.Millisecond)
	if reads != 0 {
		t.Errorf("recomputed after 299 ms (%v field reads), want the 0.3 s alarm", reads)
	}
	if _, link, _ := c.Readout(); link != initialLink {
		t.Errorf("readout changed before the alarm: %q", link)
	}
	clk.advance(time.Millisecond)
	if reads != perUpdate {
		t.Errorf("recomputed %v times at 300 ms, want one (%v field reads)", reads/perUpdate, perUpdate)
	}
	if _, link, _ := c.Readout(); link == initialLink {
		t.Error("readout did not change after the alarm")
	}

	// A burst of three changes within the window collapses to one recompute.
	reads = 0
	c.SetNoiseFloor("-100")
	clk.advance(100 * time.Millisecond)
	c.SetNoiseFloor("-110")
	clk.advance(100 * time.Millisecond)
	c.SetNoiseFloor("-120")
	clk.advance(299 * time.Millisecond)
	if reads != 0 {
		t.Fatalf("recomputed during the burst (%v field reads), want a trailing-edge alarm", reads)
	}
	clk.advance(time.Millisecond)
	if reads != perUpdate {
		t.Errorf("burst recomputed %v field reads, want one update (%v)", reads, perUpdate)
	}
}

// TestInterfaceFormCalculatorFieldWiring pins connect_all_field_signals
// (Interfaces.py:284-294): only bandwidth, spreading factor, coding rate and
// transmit power (plus the calculator's own two edits) schedule a recompute; a
// change to any other form field does not.
func TestInterfaceFormCalculatorFieldWiring(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	app.killRing = &killRing{}

	form := NewInterfaceAddFormView(app, IfaceRNode, InterfaceFormOptions{})
	calc := form.Calculator()
	if calc == nil {
		t.Fatal("RNode form has no calculator")
	}
	clk := &fakeClock{}
	// The recompute runs on the timer goroutine; with no event loop under test,
	// update directly instead of marshalling.
	calc.app = nil
	calc.debounce = newDebouncedCallWithClock(rnodeCalculatorDebounce, calc.debouncedUpdate, clk)
	// Count recomputes: a spurious one would recompute the same readout, so
	// comparing the strings alone cannot see it.
	reads := 0
	orig := calc.FieldValue
	calc.FieldValue = func(key string) (string, bool) {
		reads++
		return orig(key)
	}

	_, before, _ := calc.Readout()

	// A non-calculator field must not schedule anything.
	reads = 0
	form.SetValue("name", "radio")
	clk.advance(time.Second)
	if reads != 0 {
		t.Errorf("changing the name recomputed the calculator (%v field reads, want none)", reads)
	}
	if _, after, _ := calc.Readout(); after != before {
		t.Errorf("changing the name recomputed the calculator (%q -> %q)", before, after)
	}

	// A calculator field schedules one recompute at 0.3 s.
	reads = 0
	form.SetValue("bandwidth", "125000")
	clk.advance(299 * time.Millisecond)
	if reads != 0 {
		t.Errorf("bandwidth recomputed before 0.3 s (%v field reads)", reads)
	}
	if _, after, _ := calc.Readout(); after != before {
		t.Errorf("bandwidth recomputed before 0.3 s (%q -> %q)", before, after)
	}
	clk.advance(time.Millisecond)
	if reads == 0 {
		t.Error("bandwidth change did not recompute the calculator at 0.3 s")
	}
	dataRate, after, _ := calc.Readout()
	if after == before {
		t.Error("bandwidth change did not change the readout")
	}
	if dataRate == "" {
		t.Error("data rate is empty after recompute")
	}
}
