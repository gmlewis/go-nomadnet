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
	"time"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// rnodeCalculatorHeight is the panel's fixed rendered row count: the leading
// "-" divider, the noise-floor and antenna-gain edit rows, a blank divider, the
// "On-Air Calculations:" caption, the three readout rows, a blank divider and
// the trailing note — ten rows, matching Python's RNodeCalculator layout
// (Interfaces.py:244-273).
const rnodeCalculatorHeight = 10

// rnodeCalculatorDebounce is Python's recompute delay: _queue_update cancels
// and re-arms a 0.3 s alarm on every field change (Interfaces.py:291-302), so
// typing a value costs one recompute, not one per keystroke.
const rnodeCalculatorDebounce = 300 * time.Millisecond

// RNodeCalculatorView is the "On-Air Calculations" panel the RNode Add/Edit
// form reveals behind the "Show On-Air Calculations" button, mirroring Python's
// RNodeCalculator (Interfaces.py:230-366). It shows two inputs — noise floor
// (dB) and antenna gain (dBi) — and three computed readouts (Data Rate, Link
// Budget, Sensitivity) derived from the parent form's bandwidth, spreading
// factor, coding rate and transmit power via RNodeCalculatorReadout.
type RNodeCalculatorView struct {
	app         *App
	widget      *tview.Flex
	noiseFloor  *ReadlineEdit
	antennaGain *ReadlineEdit
	dataRate    *tview.TextView
	linkBudget  *tview.TextView
	sensitivity *tview.TextView

	// FieldValue reads a parent form field's current text by config key
	// (bandwidth / spreadingfactor / codingrate / txpower). Nil leaves each
	// value at Python's per-field default.
	FieldValue func(key string) (string, bool)

	// debounce coalesces the field-change burst into Python's single delayed
	// recompute (Interfaces.py:291-302).
	debounce *debouncedCall
}

// NewRNodeCalculatorView builds the on-air calculator panel. fieldValue reads
// the parent form's parameter fields; nil falls back to Python's defaults.
func NewRNodeCalculatorView(app *App, fieldValue func(key string) (string, bool)) *RNodeCalculatorView {
	return newRNodeCalculatorView(app, fieldValue, nil)
}

// newRNodeCalculatorView is the test seam: a non-nil clock makes the 0.3 s
// debounce deterministic instead of wall-clock.
func newRNodeCalculatorView(app *App, fieldValue func(key string) (string, bool), clk clock) *RNodeCalculatorView {
	c := &RNodeCalculatorView{app: app, FieldValue: fieldValue}
	if clk != nil {
		c.debounce = newDebouncedCallWithClock(rnodeCalculatorDebounce, c.debouncedUpdate, clk)
	} else {
		c.debounce = NewDebouncedCall(rnodeCalculatorDebounce, c.debouncedUpdate)
	}

	kr := &killRing{}
	if app != nil && app.killRing != nil {
		kr = app.killRing
	}
	c.noiseFloor = NewReadlineEdit(kr, "", "")
	c.noiseFloor.SetFieldBackgroundColor(tcell.ColorDefault)
	c.antennaGain = NewReadlineEdit(kr, "", "")
	c.antennaGain.SetFieldBackgroundColor(tcell.ColorDefault)

	// Python connects the 'change' signal of both edits (and the parent's RNode
	// fields) to a debounced recompute (Interfaces.py:284-302); every change
	// re-arms the 0.3 s timer, so a burst of keystrokes recomputes once.
	c.noiseFloor.SetChangedFunc(func(string) { c.QueueUpdate() })
	c.antennaGain.SetChangedFunc(func(string) { c.QueueUpdate() })

	c.dataRate = tview.NewTextView().SetDynamicColors(false)
	c.linkBudget = tview.NewTextView().SetDynamicColors(false)
	c.sensitivity = tview.NewTextView().SetDynamicColors(false)

	// Python's label column is 28 columns wide and right-aligned
	// (Interfaces.py:252-261).
	labelRow := func(text string, field tview.Primitive) tview.Primitive {
		label := tview.NewTextView().SetTextAlign(tview.AlignRight).SetText(text)
		return tview.NewFlex().
			AddItem(label, 28, 0, false).
			AddItem(field, 0, 1, true)
	}

	// "On-Air Calculations:" uses Python's connected_status style: a dark-green
	// foreground on the terminal default background (the palette's
	// connected_status entry is foreground-only, Interfaces.py:265).
	tc := GetThemeColors(ThemeDark)
	if app != nil {
		tc = GetThemeColors(app.Theme)
	}
	caption := tview.NewTextView().SetDynamicColors(true).
		SetText(buildColorTag(tc["connected_status"], tcell.ColorDefault) + "On-Air Calculations:" + "[-:-]")

	c.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(newDividerRow("-"), 1, 0, false).
		AddItem(labelRow("Enter Noise Floor (dB): ", c.noiseFloor), 1, 0, false).
		AddItem(labelRow("Enter Antenna Gain (dBi): ", c.antennaGain), 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(caption, 1, 0, false).
		AddItem(c.dataRate, 1, 0, false).
		AddItem(c.linkBudget, 1, 0, false).
		AddItem(c.sensitivity, 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(tview.NewTextView().SetDynamicColors(false).
			SetText("These calculations will update as you change RNode parameters"), 1, 0, false)

	c.Update()
	return c
}

// Widget returns the panel's primitive.
func (c *RNodeCalculatorView) Widget() tview.Primitive { return c.widget }

// Height returns the panel's fixed row count.
func (c *RNodeCalculatorView) Height() int { return rnodeCalculatorHeight }

// QueueUpdate schedules Python's delayed recompute (Interfaces.py:291-302):
// each change cancels the pending alarm and re-arms it 0.3 s out, so a typing
// burst produces one recalculation.
func (c *RNodeCalculatorView) QueueUpdate() {
	if c == nil || c.debounce == nil {
		return
	}
	c.debounce.Trigger()
}

// debouncedUpdate applies a due recompute. The timer runs off the event loop,
// so view mutations are marshalled onto it when an app is available; a bare
// calculator (tests, construction) updates directly.
func (c *RNodeCalculatorView) debouncedUpdate() {
	if c.app != nil {
		c.app.QueueUpdateDraw(c.Update)
		return
	}
	c.Update()
}

// Update recomputes the readout from the current field and edit values,
// mirroring Python's RNodeCalculator.update_calculation (Interfaces.py:296-366).
func (c *RNodeCalculatorView) Update() {
	if c == nil {
		return
	}
	dataRate, linkBudget, sensitivity := RNodeCalculatorReadout(c.FieldValue, c.noiseFloor.GetText(), c.antennaGain.GetText())
	c.dataRate.SetText(dataRate)
	c.linkBudget.SetText(linkBudget)
	c.sensitivity.SetText(sensitivity)
}

// Readout returns the three current readout strings (for tests).
func (c *RNodeCalculatorView) Readout() (dataRate, linkBudget, sensitivity string) {
	return c.dataRate.GetText(false), c.linkBudget.GetText(false), c.sensitivity.GetText(false)
}

// SetNoiseFloor seeds the noise-floor edit (Python's ReadlineEdit default "0").
func (c *RNodeCalculatorView) SetNoiseFloor(text string) { c.noiseFloor.SetText(text) }

// SetAntennaGain seeds the antenna-gain edit.
func (c *RNodeCalculatorView) SetAntennaGain(text string) { c.antennaGain.SetText(text) }
