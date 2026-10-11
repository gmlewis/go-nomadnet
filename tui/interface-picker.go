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
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
	"github.com/mattn/go-runewidth"
)

// InterfaceTypeEntry is one row of the Add-Interface type picker: either a
// non-selectable group heading, or a selectable interface type. Exactly one of
// Heading and Type is set.
type InterfaceTypeEntry struct {
	Heading string // group heading text (with its leading icon); "" for an option
	Label   string // option label as displayed; "" for a heading
	Type    string // interface type constant the Add form is built for; "" for a heading
}

// InterfaceTypePickerEntries returns the rows of Python's "Select Interface
// Type" dialog (Interfaces.py:3017-3073): four icon-headed groups listing every
// interface type the Add form can build. The order is Python's, and the heading
// icons come from the same glyph table the interface list uses.
//
// Python gates two of the groups on the host: BackboneInterface is offered only
// when PLATFORM_IS_LINUX, and the RNode/Hardware groups only when pyserial is
// importable. Both hold on the platforms this port ships for (Linux and Android
// are Linux; the Go transport speaks serial natively), so the full list is
// always offered.
func InterfaceTypePickerEntries(glyphset string) []InterfaceTypeEntry {
	var out []InterfaceTypeEntry
	heading := func(ifType, name string) {
		out = append(out, InterfaceTypeEntry{Heading: GetInterfaceIcon(glyphset, ifType) + "  " + name})
	}
	option := func(label, ifType string) {
		out = append(out, InterfaceTypeEntry{Label: label, Type: ifType})
	}

	heading(IfaceAuto, "IP Networks")
	option("Auto Interface", IfaceAuto)
	option("TCP Client Interface", IfaceTCPClient)
	option("TCP Server Interface", IfaceTCPServer)
	option("UDP Interface", IfaceUDP)
	option("I2P Interface", IfaceI2P)
	option("Backbone Interface", IfaceBackbone)

	heading(IfaceRNode, "RNodes")
	option("RNode Interface", IfaceRNode)
	option("RNode Multi Interface", IfaceRNodeMulti)

	heading(IfaceSerial, "Hardware")
	option("Serial Interface", IfaceSerial)
	option("KISS Interface", IfaceKISS)
	option("AX.25 KISS Interface", IfaceAX25KISS)

	heading(IfacePipe, "Other")
	option("Pipe Interface", IfacePipe)
	option("Custom Interface", IfaceCustom)

	return out
}

// InterfaceTypePicker is the "Select Interface Type" body (Python's
// add_interface dialog list, Interfaces.py:3057-3073): a SimpleFocusListWalker
// of heading Texts and selectable InterfaceOptionItems. The display wraps it in
// a DialogLineBox and shows it as an overlay over the interface list; Enter on
// an option names the interface type the Add form is then built for.
//
// urwid's ListBox navigation skips non-selectable widgets, so Up/Down/Home/End
// step between options only and a heading is never focused. A heading draws in
// the default style; an option draws in list_normal, and the focused option in
// list_focus across the full row (urwid's ListBox pads each item to the full
// width, so the highlight spans the row).
type InterfaceTypePicker struct {
	*tview.Box
	app      *App
	entries  []InterfaceTypeEntry
	options  []int // entries indices of the selectable options, in order
	focus    int   // index into options
	OnSelect func(ifType string)
}

// NewInterfaceTypePicker builds the picker for the given glyph set.
func NewInterfaceTypePicker(app *App, glyphset string) *InterfaceTypePicker {
	if app != nil {
		glyphset = app.GlyphSet
	}
	p := &InterfaceTypePicker{Box: tview.NewBox(), app: app, focus: -1}
	p.entries = InterfaceTypePickerEntries(glyphset)
	for i, e := range p.entries {
		if e.Type != "" {
			if p.focus < 0 {
				p.focus = len(p.options) // urwid's walker focuses the first selectable item
			}
			p.options = append(p.options, i)
		}
	}
	return p
}

// Entries returns the picker's rows.
func (p *InterfaceTypePicker) Entries() []InterfaceTypeEntry { return p.entries }

// SelectedType returns the focused interface type, or "" when there is none.
func (p *InterfaceTypePicker) SelectedType() string {
	if p.focus < 0 || p.focus >= len(p.options) {
		return ""
	}
	return p.entries[p.options[p.focus]].Type
}

// SetSelected moves the highlight to the option with the given interface type.
func (p *InterfaceTypePicker) SetSelected(ifType string) {
	for i, idx := range p.options {
		if p.entries[idx].Type == ifType {
			p.focus = i
			return
		}
	}
}

// SelectFocused activates the focused option.
func (p *InterfaceTypePicker) SelectFocused() {
	if ifType := p.SelectedType(); ifType != "" && p.OnSelect != nil {
		p.OnSelect(ifType)
	}
}

// moveFocus steps the highlight by delta, clamped at both ends (urwid's ListBox
// does not wrap on Up/Down).
func (p *InterfaceTypePicker) moveFocus(delta int) {
	if len(p.options) == 0 {
		return
	}
	if p.focus < 0 {
		p.focus = 0
	}
	p.focus = min(max(p.focus+delta, 0), len(p.options)-1)
}

// Focus implements tview.Primitive.
func (p *InterfaceTypePicker) Focus(delegate func(tview.Primitive)) { p.Box.Focus(delegate) }

// Draw renders the picker's rows within its rect.
func (p *InterfaceTypePicker) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	x, y, w, h := p.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	focusStyle, optionStyle := tcell.StyleDefault, tcell.StyleDefault
	if p.app != nil {
		focusStyle = p.app.Styles.Style("list_focus")
		optionStyle = p.app.Styles.Style("list_normal")
	}
	focused := -1
	if p.focus >= 0 && p.focus < len(p.options) {
		focused = p.options[p.focus]
	}

	for i, e := range p.entries {
		if i >= h {
			break
		}
		switch {
		case e.Heading != "":
			printPickerText(screen, e.Heading, x, y+i, w, tcell.ColorDefault)
		case i == focused:
			// Fill the row so the highlight spans the dialog, as urwid's
			// ListBox does when it pads an item to the full width.
			for col := range w {
				screen.SetContent(x+col, y+i, ' ', nil, focusStyle)
			}
			printPickerStyledText(screen, e.Label, x, y+i, w, focusStyle)
		default:
			printPickerStyledText(screen, e.Label, x, y+i, w, optionStyle)
		}
	}
}

// printPickerText draws plain text, and printPickerStyledText draws it with a
// foreground/background style (tview.Print takes only a colour, so a styled row
// needs the screen directly).
func printPickerText(screen tcell.Screen, text string, x, y, maxWidth int, color tcell.Color) {
	printPickerStyledText(screen, text, x, y, maxWidth, tcell.StyleDefault.Foreground(color))
}

func printPickerStyledText(screen tcell.Screen, text string, x, y, maxWidth int, style tcell.Style) {
	col := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if col+rw > maxWidth {
			break
		}
		screen.SetContent(x+col, y, r, nil, style)
		col += rw
	}
}

// InputHandler handles list navigation and activation. Esc is left to the
// enclosing DialogLineBox, which owns the dismiss handler.
func (p *InterfaceTypePicker) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return p.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			p.moveFocus(-1)
		case tcell.KeyDown:
			p.moveFocus(1)
		case tcell.KeyPgUp:
			p.moveFocus(-min(len(p.options), 10))
		case tcell.KeyPgDn:
			p.moveFocus(min(len(p.options), 10))
		case tcell.KeyHome:
			p.moveFocus(-len(p.options))
		case tcell.KeyEnd:
			p.moveFocus(len(p.options))
		case tcell.KeyEnter:
			p.SelectFocused()
		}
	})
}

// MouseHandler implements click-to-select: a click on an option row focuses it
// and activates it, and the wheel steps the highlight.
func (p *InterfaceTypePicker) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return p.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		_, by, _, _ := p.GetRect()
		if !p.InRect(x, y) {
			return false, nil
		}
		row := y - by
		switch action {
		case tview.MouseLeftClick:
			if row < 0 || row >= len(p.entries) || p.entries[row].Type == "" {
				return false, nil
			}
			for i, idx := range p.options {
				if idx == row {
					p.focus = i
					break
				}
			}
			setFocus(p)
			p.SelectFocused()
			return true, p
		case tview.MouseScrollUp:
			p.moveFocus(-1)
			return true, nil
		case tview.MouseScrollDown:
			p.moveFocus(1)
			return true, nil
		}
		return false, nil
	})
}
