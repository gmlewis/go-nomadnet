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
	"fmt"
	"strconv"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// Field kinds, matching the "type" of a Python INTERFACE_FIELDS entry.
const (
	formEditField     = "edit"
	formDropdownField = "dropdown"
	formCheckField    = "checkbox"
	formMultiField    = "multilist"
	formKVField       = "keyvaluepairs"
	formTableField    = "multitable"
)

// Value transforms, naming the conversions Python's field definitions apply
// (INTERFACE_FIELDS, Interfaces.py:406). They are the only places a field's
// displayed text and its config value differ, so the form applies one on save
// and its inverse on load; every other field's text is its value.
const (
	transformNone        = ""
	transformFrequencyHz = "frequency_hz" // "915" MHz  →  915000000 Hz
	transformCodingRate  = "codingrate"   // "4:5"      →  5
	transformScopeNone   = "discovery_scope"
	// transformAnnounceCap mirrors Python's transform, which yields 2.0 for an
	// empty field rather than nothing: the option is written even when it was
	// never filled in (Interfaces.py:1607).
	transformAnnounceCap = "announce_cap"
)

// InterfaceFormOptions carries the wiring-dependent choices an interface form
// makes.
type InterfaceFormOptions struct {
	// TransportEnabled adds Python's transport-mode common options (outgoing,
	// mode, announce_cap) to the more-options section. Python appends them when
	// app.rns.transport_enabled() (Interfaces.py:1580-1609).
	TransportEnabled bool
}

// InterfaceFormView is the Add/Edit interface form: Python's AddInterfaceView
// (Interfaces.py:1418) and EditInterfaceView (Interfaces.py:1997), which is the
// same form with its fields pre-filled and its title naming the interface.
//
// The form is a urwid Pile: a centred title, a "─" rule, one two-row item per
// field (a 26-column right-aligned label beside the widget, then the field's
// error line), then the "Show more options" button, the section it reveals, a
// rule and the Save/Cancel button row. It is mounted as a DialogLineBox by
// InterfacesDisplay.ShowForm.
type InterfaceFormView struct {
	app       *App
	data      *InterfaceFormData
	ifaceType string
	editing   bool
	origName  string

	title  string
	header string

	rows      []*formRow // the fields shown before "Show more options"
	extraRows []*formRow // the additional + common options that button reveals
	more      bool
	moreBtn   *UrwidButton

	saveBtn   *UrwidButton
	cancelBtn *UrwidButton

	pile    *pileFiller
	dialog  *DialogLineBox
	options InterfaceFormOptions

	// OnSave receives the interface name and its configuration, in display
	// order, once the form validates. It returns the write error, if any.
	OnSave func(name string, props []app.ConfigProp) error
	// OnCancel runs when the form is dismissed without saving.
	OnCancel func()
	// OnNameConflict reports whether a name is already taken by another
	// interface section. The wiring layer, which knows the config's other
	// sections, installs it; nil skips the check (Python on_save checks
	// app.rns.config['interfaces'], Interfaces.py:1887/2132).
	OnNameConflict func(name string) bool
}

// formRow is one field of an interface form: the widget, the text the widget
// holds, and the row it is drawn in.
type formRow struct {
	key        string
	label      string
	kind       string
	transform  string
	validation []string
	options    []string

	input *tview.InputField
	drop  *tview.DropDown
	check *tview.Checkbox
	area  *tview.TextArea

	widgetHeight int
	row          *tview.Flex
	errView      *tview.TextView
}

// formTextAreaRows is the height given to the multi-line widgets (keyvaluepairs
// and multitable). Python renders the same fields as a pile of one-line entries
// with an add button; this port renders them as a small text area instead, one
// entry per line — the value semantics are identical (see formRow.value).
const formTextAreaRows = 4

// NewInterfaceAddFormView builds the Add form for an interface type.
func NewInterfaceAddFormView(app *App, ifType string, opts InterfaceFormOptions) *InterfaceFormView {
	v := newInterfaceFormView(app, ifType, opts)
	v.title = "Add Interface"
	glyphset := GlyphUnicode
	if app != nil {
		glyphset = app.GlyphSet
	}
	v.header = fmt.Sprintf("Add new %v %v", GetInterfaceIcon(glyphset, ifType), ifType)
	v.build()
	return v
}

// NewInterfaceEditFormView builds the Edit form for an interface that is already
// in the RNS config, pre-filling every field the type declares plus any common
// option the config carries (Python's _populate_form_fields, Interfaces.py:2020).
func NewInterfaceEditFormView(app *App, ifaceName string, cfg map[string]any, opts InterfaceFormOptions) *InterfaceFormView {
	ifType := IfaceCustom
	if t, ok := cfg["type"]; ok {
		name := fmt.Sprintf("%v", t)
		if _, known := interfaceFieldRegistry[name]; known {
			ifType = name
		}
	}

	v := newInterfaceFormView(app, ifType, opts)
	v.editing = true
	v.origName = ifaceName
	v.title = "Edit Interface: " + ifaceName
	v.header = ifaceName
	v.build()
	v.loadConfig(ifaceName, cfg)
	return v
}

func newInterfaceFormView(app *App, ifType string, opts InterfaceFormOptions) *InterfaceFormView {
	v := &InterfaceFormView{
		app:       app,
		data:      NewInterfaceFormData(ifType),
		ifaceType: ifType,
		options:   opts,
	}
	if opts.TransportEnabled {
		v.data.AddTransportCommonOptions()
	}
	return v
}

// Title returns the form's LineBox title.
func (v *InterfaceFormView) Title() string { return v.title }

// Widget returns the form's body primitive (the pile of rows). It is what the
// display puts inside a DialogLineBox.
func (v *InterfaceFormView) Widget() tview.Primitive { return v.pile }

// MoreOptionsVisible reports whether the more-options section is showing.
func (v *InterfaceFormView) MoreOptionsVisible() bool { return v.more }

// nameInterfaceField is the field every interface form leads with. Python
// creates it separately from INTERFACE_FIELDS and prepends it to the field list
// (Interfaces.py:1430-1438,1656).
var nameInterfaceField = InterfaceField{
	ConfigKey:   "name",
	Type:        formEditField,
	Label:       "Name: ",
	Placeholder: "Enter interface name",
	Validation:  []string{"required"},
}

// build assembles the field rows and the pile that displays them.
func (v *InterfaceFormView) build() {
	v.rows = []*formRow{newFormRow(nameInterfaceField)}
	for _, f := range interfaceFieldList(v.ifaceType) {
		v.rows = append(v.rows, newFormRow(f))
	}
	v.extraRows = nil
	for _, f := range interfaceAdditionalFieldList(v.ifaceType) {
		v.extraRows = append(v.extraRows, newFormRow(f))
	}
	for _, f := range v.data.CommonFieldList() {
		v.extraRows = append(v.extraRows, newFormRow(f))
	}

	v.moreBtn = NewUrwidButton("Show more options").SetSelectedFunc(v.ToggleMoreOptions)
	v.saveBtn = NewUrwidButton("Save").SetSelectedFunc(v.SaveFocused)
	v.cancelBtn = NewUrwidButton("Cancel").SetSelectedFunc(v.Cancel)

	v.rebuild()
}

// rebuild lays the pile out again, adding or dropping the more-options section
// as its visibility changed (urwid's toggle_more_options rewrites the section
// pile's contents).
func (v *InterfaceFormView) rebuild() {
	pile := newPileFiller()

	pile.AddItem(newCenteredText(tcell.ColorDefault, v.header), 1, false)
	pile.AddItem(newDividerRow("─"), 1, false)
	for _, r := range v.rows {
		pile.AddItem(r.row, r.widgetHeight+1, true)
	}
	pile.AddItem(tview.NewBox(), 1, false)
	pile.AddItem(v.moreBtn, 1, true)
	if v.more {
		for _, r := range v.extraRows {
			pile.AddItem(r.row, r.widgetHeight+1, true)
		}
	}
	pile.AddItem(newDividerRow("─"), 1, false)
	pile.AddItem(CreateUrwidButtonRow(v.saveBtn, v.cancelBtn), 1, true)

	v.pile = pile
	v.dialog = NewDialogLineBox(v.title, pile, v.Cancel)
}

// ToggleMoreOptions shows or hides the additional and common options
// (Python's toggle_more_options, Interfaces.py:1727).
func (v *InterfaceFormView) ToggleMoreOptions() {
	if v.more {
		v.HideMoreOptions()
		return
	}
	v.ShowMoreOptions()
}

// ShowMoreOptions reveals the additional and common options.
func (v *InterfaceFormView) ShowMoreOptions() {
	if v.more {
		return
	}
	v.more = true
	v.moreBtn.SetLabel("Hide more options")
	v.rebuild()
	v.refocus()
}

// HideMoreOptions hides the additional and common options. The values already
// entered are kept and still reach the saved configuration.
func (v *InterfaceFormView) HideMoreOptions() {
	if !v.more {
		return
	}
	v.more = false
	v.moreBtn.SetLabel("Show more options")
	v.rebuild()
	v.refocus()
}

// refocus re-points the app at the rebuilt pile, keeping the more-options
// button focused (Python's toggle leaves focus on the button).
func (v *InterfaceFormView) refocus() {
	if v.app == nil {
		return
	}
	v.app.SetFocus(v.pile)
	for i, it := range v.pile.items {
		if it.widget == tview.Primitive(v.moreBtn) {
			for sel, idx := range v.pile.selectable {
				if idx == i {
					v.pile.SetFocusIndex(sel)
				}
			}
			break
		}
	}
}

// Dialog returns the LineBox the form is shown in.
func (v *InterfaceFormView) Dialog() *DialogLineBox { return v.dialog }

// Cancel dismisses the form without saving.
func (v *InterfaceFormView) Cancel() {
	if v.OnCancel != nil {
		v.OnCancel()
	}
}

// SaveFocused validates the form and, when it is valid, hands the ordered
// configuration to OnSave (Python on_save, Interfaces.py:1882/2126). Validation
// failures are painted on the offending field and nothing is written.
func (v *InterfaceFormView) SaveFocused() {
	if err := v.Save(); err != nil {
		v.showError(err.Error())
	}
}

// Save validates and saves the form, returning the validation or write error.
func (v *InterfaceFormView) Save() error {
	errs := v.validate()
	if len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("%v", strings.Join(msgs, "; "))
	}
	name, props := v.Build()
	if v.OnSave == nil {
		return nil
	}
	return v.OnSave(name, props)
}

// FieldError returns the validation message painted on the named field, or "".
func (v *InterfaceFormView) FieldError(key string) string {
	for _, r := range v.allRows() {
		if r.key == key {
			return r.errView.GetText(true)
		}
	}
	return ""
}

// Value returns a field's text as displayed in its widget (what a person would
// have typed), and whether the form has such a field. For a frequency field the
// text is MHz while the saved value is Hz.
func (v *InterfaceFormView) Value(key string) (string, bool) {
	r := v.row(key)
	if r == nil {
		return "", false
	}
	return r.text(), true
}

// SetValue sets a field's text as if it had been typed.
func (v *InterfaceFormView) SetValue(key, text string) {
	if r := v.row(key); r != nil {
		r.setText(text)
	}
}

// Build returns the interface name and its configuration in display order.
func (v *InterfaceFormView) Build() (string, []app.ConfigProp) {
	name := strings.TrimSpace(v.text("name"))
	if name == "" {
		name = "Untitled interface"
	}
	if v.editing && name == "" {
		name = v.origName
	}

	props := []app.ConfigProp{{Key: "type", Value: v.ifaceType}, {Key: "interface_enabled", Value: true}}
	// A custom interface writes the class name from its own "type" field as the
	// section's type (Python on_save, Interfaces.py:1896-1906).
	if v.ifaceType == IfaceCustom {
		if r := v.row("type"); r != nil {
			if t, ok := r.value(); ok {
				props[0] = app.ConfigProp{Key: "type", Value: t}
			}
		}
	}

	for _, r := range append(append([]*formRow{}, v.rows...), v.extraRows...) {
		switch r.key {
		case "name", "type", "subinterfaces", "custom_parameters":
			continue
		}
		value, ok := r.value()
		if !ok {
			continue
		}
		props = append(props, app.ConfigProp{Key: r.key, Value: value})
	}

	// The two composite fields write their contents at the section's own level
	// (Python on_save: custom_parameters flatten to section keys, subinterfaces
	// to [[[name]]] subsections).
	if r := v.row("custom_parameters"); r != nil {
		if value, ok := r.value(); ok {
			if params, isMap := value.(map[string]any); isMap {
				for _, k := range SortedKeys(params) {
					props = append(props, app.ConfigProp{Key: k, Value: params[k]})
				}
			}
		}
	}
	if r := v.row("subinterfaces"); r != nil {
		if value, ok := r.value(); ok {
			if subs, isMap := value.(map[string]any); isMap {
				for _, k := range SortedKeys(subs) {
					props = append(props, app.ConfigProp{Key: k, Value: subs[k]})
				}
			}
		}
	}

	return name, props
}

// validate runs each row's validation, painting every message on its row.
func (v *InterfaceFormView) validate() []ValidationError {
	var errs []ValidationError
	for _, r := range v.allRows() {
		msg := r.validate()
		if msg != "" {
			errs = append(errs, ValidationError{FieldKey: r.key, Message: r.label + " " + msg})
		}
		r.errView.SetText(msg)
	}

	// A name that is already taken is rejected by the page, which knows the
	// other sections (Python on_save checks app.rns.config['interfaces'];
	// Interfaces.py:1887). The Add form has no other names to compare against
	// here, so the wiring layer adds that check with the same message.
	if v.OnNameConflict != nil {
		if name := strings.TrimSpace(v.text("name")); name != "" && v.OnNameConflict(name) {
			msg := fmt.Sprintf("Interface name '%v' already exists", name)
			errs = append(errs, ValidationError{FieldKey: "name", Message: msg})
			if r := v.row("name"); r != nil {
				r.errView.SetText(msg)
			}
		}
	}

	return errs
}

// showError paints a save failure on the form's title area.
func (v *InterfaceFormView) showError(msg string) {
	if r := v.row("name"); r != nil && r.errView.GetText(true) == "" {
		r.errView.SetText(msg)
	}
}

// allRows returns every field row the form holds, whether or not the
// more-options section is currently showing.
func (v *InterfaceFormView) allRows() []*formRow {
	return append(append([]*formRow{}, v.rows...), v.extraRows...)
}

// VisibleFieldKeys returns the keys of the field rows currently displayed, in
// order: the form's own fields, then the more-options section's when it is
// showing.
func (v *InterfaceFormView) VisibleFieldKeys() []string {
	rows := v.rows
	if v.more {
		rows = v.allRows()
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.key)
	}
	return keys
}

func (v *InterfaceFormView) row(key string) *formRow {
	for _, r := range v.allRows() {
		if r.key == key {
			return r
		}
	}
	return nil
}

// text returns a field's displayed text, or "".
func (v *InterfaceFormView) text(key string) string {
	if r := v.row(key); r != nil {
		return r.text()
	}
	return ""
}

// loadConfig fills the form from an existing interface configuration
// (Python's _populate_form_fields, Interfaces.py:2020).
func (v *InterfaceFormView) loadConfig(name string, cfg map[string]any) {
	v.SetValue("name", name)
	if v.ifaceType == IfaceCustom {
		if t, ok := cfg["type"]; ok {
			v.SetValue("type", fmt.Sprintf("%v", t))
		}
	}

	for _, r := range v.allRows() {
		switch r.key {
		case "name", "type":
			continue
		}
		raw, ok := cfg[r.key]
		if !ok {
			continue
		}
		r.loadValue(fmt.Sprintf("%v", raw))
	}

	// A custom interface keeps every non-standard key as a custom parameter
	// (Python _populate_form_fields, Interfaces.py:2028-2037).
	if v.ifaceType == IfaceCustom {
		params := map[string]any{}
		for k, val := range cfg {
			if customInterfaceStandardKeys[k] {
				continue
			}
			params[k] = val
		}
		if len(params) > 0 && v.row("custom_parameters") != nil {
			v.row("custom_parameters").loadRaw(params)
		}
	}
}

// customInterfaceStandardKeys are the section keys that are not custom
// parameters (Python's standard_keys, Interfaces.py:2029).
var customInterfaceStandardKeys = map[string]bool{
	"type": true, "interface_enabled": true, "enabled": true, "description": true,
	"network_name": true, "bitrate": true, "passphrase": true, "ifac_size": true,
	"mode": true, "outgoing": true, "announce_cap": true,
}

// newFormRow builds one field row.
func newFormRow(f InterfaceField) *formRow {
	r := &formRow{
		key:        f.ConfigKey,
		label:      f.Label,
		kind:       f.Type,
		transform:  f.Transform,
		validation: f.Validation,
		options:    f.Options,
	}
	if r.label == "" {
		r.label = humanizeConfigKey(f.ConfigKey) + ": "
	}

	var widget tview.Primitive
	switch f.Type {
	case formDropdownField:
		r.drop = tview.NewDropDown().SetOptions(f.Options, nil)
		if idx := indexOfOption(f.Options, f.Default); idx >= 0 {
			r.drop.SetCurrentOption(idx)
		}
		widget = r.drop
	case formCheckField:
		r.check = tview.NewCheckbox()
		widget = r.check
	case formMultiField:
		r.input = tview.NewInputField().SetPlaceholder(f.Placeholder)
		widget = r.input
	case formKVField, formTableField:
		r.area = tview.NewTextArea()
		widget = r.area
		r.widgetHeight = formTextAreaRows
	default:
		r.input = tview.NewInputField().SetPlaceholder(f.Placeholder)
		widget = r.input
	}
	if r.widgetHeight == 0 {
		r.widgetHeight = 1
	}

	label := tview.NewTextView().SetTextAlign(tview.AlignRight)
	label.SetText(r.label)
	cols := tview.NewFlex().
		AddItem(label, 26, 0, false).
		AddItem(widget, 0, 1, true)
	r.errView = tview.NewTextView()
	errRow := tview.NewFlex().
		AddItem(tview.NewBox(), 24, 0, false).
		AddItem(r.errView, 0, 1, false)
	r.row = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cols, r.widgetHeight, 0, false).
		AddItem(errRow, 1, 0, false)
	return r
}

// text returns the row's displayed text.
func (r *formRow) text() string {
	switch r.kind {
	case formCheckField:
		if r.check.IsChecked() {
			return "true"
		}
		return "false"
	case formDropdownField:
		_, t := r.drop.GetCurrentOption()
		return t
	case formKVField, formTableField:
		return r.area.GetText()
	default:
		return r.input.GetText()
	}
}

// setText sets the row's displayed text.
func (r *formRow) setText(text string) {
	switch r.kind {
	case formCheckField:
		r.check.SetChecked(parseFormBool(text))
	case formDropdownField:
		r.selectOption(text)
	case formKVField, formTableField:
		r.area.SetText(text, false)
	default:
		r.input.SetText(text)
	}
}

// selectOption selects the dropdown option matching a config value: by exact
// text first, then by comparing each option's transformed value, which is how
// Python's _set_field_value matches (Interfaces.py:2071-2085).
func (r *formRow) selectOption(raw string) {
	if idx := indexOfOption(r.options, raw); idx >= 0 {
		r.drop.SetCurrentOption(idx)
		return
	}
	for i, opt := range r.options {
		if fmt.Sprintf("%v", applyFormTransform(r.transform, opt)) == raw {
			r.drop.SetCurrentOption(i)
			return
		}
	}
}

// loadValue fills the row from a config value, inverting the field's transform.
func (r *formRow) loadValue(raw string) {
	switch r.transform {
	case transformFrequencyHz:
		if hz, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
			r.setText(formatMHz(hz / 1000000))
			return
		}
	case transformCodingRate:
		if _, after, ok := strings.Cut(raw, ":"); ok {
			r.setText(after)
			return
		}
	}
	if r.kind == formMultiField {
		r.setText(strings.Join(splitList(raw), ", "))
		return
	}
	r.setText(raw)
}

// loadRaw fills a composite row (keyvaluepairs, multitable) from a nested
// configuration map.
func (r *formRow) loadRaw(value map[string]any) {
	if r.kind == formTableField {
		var lines []string
		for _, name := range SortedKeys(value) {
			sub, ok := value[name].(map[string]any)
			if !ok {
				continue
			}
			var parts []string
			for _, k := range SortedKeys(sub) {
				parts = append(parts, fmt.Sprintf("%v=%v", k, sub[k]))
			}
			lines = append(lines, fmt.Sprintf("%v: %v", name, strings.Join(parts, ", ")))
		}
		r.area.SetText(strings.Join(lines, "\n"), false)
		return
	}
	var lines []string
	for _, k := range SortedKeys(value) {
		lines = append(lines, fmt.Sprintf("%v = %v", k, value[k]))
	}
	r.area.SetText(strings.Join(lines, "\n"), false)
}

// value returns the row's configuration value, and whether it is non-empty.
// Python's on_save writes a field only when get_value() is neither None nor ""
// (Interfaces.py:1908-1936).
func (r *formRow) value() (any, bool) {
	switch r.kind {
	case formCheckField:
		// A checkbox always yields a value, including false (Python's
		// FormCheckbox.get_value returns the state, and False != "" holds).
		return r.check.IsChecked(), true
	case formDropdownField:
		_, t := r.drop.GetCurrentOption()
		if r.transform == transformScopeNone && t == "None" {
			return "", false
		}
		return applyFormTransform(r.transform, t), t != ""
	case formMultiField:
		items := splitList(r.input.GetText())
		if len(items) == 0 {
			return "", false
		}
		return strings.Join(items, ","), true
	case formKVField:
		pairs := parseKeyValueLines(r.area.GetText())
		if len(pairs) == 0 {
			return nil, false
		}
		return pairs, true
	case formTableField:
		subs := parseSubinterfaceLines(r.area.GetText())
		if len(subs) == 0 {
			return nil, false
		}
		return subs, true
	default:
		text := strings.TrimSpace(r.input.GetText())
		if text == "" {
			if r.transform == transformAnnounceCap {
				return "2.0", true
			}
			return "", false
		}
		return applyFormTransform(r.transform, text), true
	}
}

// validate returns the row's validation message, or "" (Python's FormField
// validate, FormWidgets.py:118-140).
func (r *formRow) validate() string {
	text := strings.TrimSpace(r.text())
	for _, rule := range r.validation {
		switch rule {
		case "required":
			if text == "" {
				return "This field is required"
			}
		case "number":
			if text != "" && !isFormNumber(text) {
				return "This field must be a number"
			}
		case "float":
			if text != "" {
				if _, err := strconv.ParseFloat(text, 64); err != nil {
					return "This field must be decimal number"
				}
			}
		}
	}
	return ""
}

// applyFormTransform converts a field's displayed text to its config value.
// The int-valued transforms of Python's definitions (int(x.strip())) are absent
// deliberately: they change the value's Go type, not the text the config file
// carries.
func applyFormTransform(transform, text string) any {
	switch transform {
	case transformFrequencyHz:
		if mhz, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
			return int64(mhz * 1000000)
		}
		return text
	case transformCodingRate:
		if _, after, ok := strings.Cut(text, ":"); ok {
			return after
		}
		return text
	default:
		return text
	}
}

// formatMHz renders a frequency in MHz the way Python's Edit form does:
// six decimal places with trailing zeros and a trailing point stripped
// (Interfaces.py:2050-2052), so 915000000 Hz reads "915".
func formatMHz(mhz float64) string {
	s := strconv.FormatFloat(mhz, 'f', 6, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// parseKeyValueLines reads a keyvaluepairs field: one "key = value" entry per
// line. Python's FormKeyValuePairs collects the same pairs from a row of edits
// and returns them as a dict.
func parseKeyValueLines(text string) map[string]any {
	out := map[string]any{}
	for line := range strings.SplitSeq(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseSubinterfaceLines reads a multitable field: one
// "name: key=value, key=value" entry per line, naming the subsection and its
// settings. Python's FormMultiTable collects the same nested dict from a grid
// of edits.
func parseSubinterfaceLines(text string) map[string]any {
	out := map[string]any{}
	for line := range strings.SplitSeq(text, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		sub := map[string]any{}
		for part := range strings.SplitSeq(rest, ",") {
			k, v, ok := strings.Cut(part, "=")
			k = strings.TrimSpace(k)
			if !ok || k == "" {
				continue
			}
			sub[k] = strings.TrimSpace(v)
		}
		out[name] = sub
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitList splits a comma-separated field value the way Python's FormMultiList
// stores one entry per comma (FormWidgets.py:266-273).
func splitList(text string) []string {
	var out []string
	for part := range strings.SplitSeq(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseFormBool reads a checkbox field's textual state the way Python's
// _set_field_value does (Interfaces.py:2068).
func parseFormBool(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "false", "off", "no", "0", "":
		return false
	}
	return true
}

// isFormNumber reports whether text is a number the way Python's FormEdit
// "number" validation does: digits with optional sign and decimal point.
func isFormNumber(text string) bool {
	cleaned := strings.NewReplacer("-", "", ".", "").Replace(text)
	if cleaned == "" {
		return false
	}
	for _, r := range cleaned {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// indexOfOption returns the index of text in options, or -1.
func indexOfOption(options []string, text string) int {
	for i, o := range options {
		if o == text {
			return i
		}
	}
	return -1
}

// interfaceFieldList returns the ordered field definitions for a type.
func interfaceFieldList(ifType string) []InterfaceField {
	var out []InterfaceField
	for _, group := range InterfaceFieldsFor(ifType) {
		out = append(out, group.Fields...)
	}
	return out
}

// interfaceAdditionalFieldList returns the ordered additional options for a type.
func interfaceAdditionalFieldList(ifType string) []InterfaceField {
	var out []InterfaceField
	for _, group := range InterfaceFieldsFor(ifType) {
		out = append(out, group.AdditionalOptions...)
	}
	return out
}
