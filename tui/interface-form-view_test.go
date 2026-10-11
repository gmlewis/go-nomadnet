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
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-reticulum/testutils"
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

const formTestConfig = `
[reticulum]
  enable_transport = Yes

[logging]
  loglevel = 4

[interfaces]
  [[Existing]]
    type = AutoInterface
    interface_enabled = True

  [[hub]]
    type = TCPClientInterface
    interface_enabled = True
    target_host = 127.0.0.1
    target_port = 4242
    network_name = mynet
    passphrase = secret
    mode = gateway
    i2p_tunneled = True

  [[radio]]
    type = RNodeInterface
    interface_enabled = True
    port = /dev/ttyACM0
    frequency = 868500000
    bandwidth = 62500
    spreadingfactor = 10
    codingrate = 5
    txpower = 17

  [[multi]]
    type = RNodeMultiInterface
    interface_enabled = True
    port = /dev/ttyACM0
    [[[sub1]]]
      bandwidth = 62500
      frequency = 868500000
    [[[sub2]]]
      bandwidth = 125000
      frequency = 915000000

  [[custom]]
    type = MyCustomClass
    interface_enabled = True
    speed = 115200
    some_setting = abc
`

// newFormTestApp returns an App whose RNS config file is the fixture above.
func newFormTestApp(t *testing.T) *app.App {
	t.Helper()
	dir := testutils.TempDirWithConfig(t, "gonomadnet-test-", func(string) string { return formTestConfig })
	return &app.App{RNSConfigDir: dir}
}

// TestInterfaceTypePickerEntries pins the Add-Interface type picker contents:
// Python's add_interface (Interfaces.py:3017-3073) lists the types under four
// icon headings, in a fixed order, and selecting one names the INTERFACE_FIELDS
// key the Add form is then built from.
func TestInterfaceTypePickerEntries(t *testing.T) {
	t.Parallel()

	entries := InterfaceTypePickerEntries(GlyphUnicode)

	var headings []string
	byHeading := map[string][]string{}
	cur := ""
	for _, e := range entries {
		if e.Heading != "" {
			headings = append(headings, e.Heading)
			cur = e.Heading
			continue
		}
		byHeading[cur] = append(byHeading[cur], e.Type)
	}

	wantHeadings := []string{
		GetInterfaceIcon(GlyphUnicode, IfaceAuto) + "  IP Networks",
		GetInterfaceIcon(GlyphUnicode, IfaceRNode) + "  RNodes",
		GetInterfaceIcon(GlyphUnicode, IfaceSerial) + "  Hardware",
		GetInterfaceIcon(GlyphUnicode, IfacePipe) + "  Other",
	}
	if strings.Join(headings, "|") != strings.Join(wantHeadings, "|") {
		t.Errorf("headings = %q, want %q", headings, wantHeadings)
	}

	want := map[string][]string{
		wantHeadings[0]: {IfaceAuto, IfaceTCPClient, IfaceTCPServer, IfaceUDP, IfaceI2P, IfaceBackbone},
		wantHeadings[1]: {IfaceRNode, IfaceRNodeMulti},
		wantHeadings[2]: {IfaceSerial, IfaceKISS, IfaceAX25KISS},
		wantHeadings[3]: {IfacePipe, IfaceCustom},
	}
	for heading, types := range want {
		if got := byHeading[heading]; strings.Join(got, ",") != strings.Join(types, ",") {
			t.Errorf("types under %q = %v, want %v", heading, got, types)
		}
	}

	// Every offered type must name a field group the form can build from, or
	// the picker offers something the Add form cannot render.
	for _, e := range entries {
		if e.Heading != "" {
			continue
		}
		if e.Label == "" {
			t.Errorf("entry for %v has no label", e.Type)
		}
		if e.Type == IfaceTCPClient && e.Label != "TCP Client Interface" {
			t.Errorf("label for %v = %q, want %q", e.Type, e.Label, "TCP Client Interface")
		}
	}
}

// TestInterfaceAddFormWritesConfigThatReparses is row 1's acceptance test: the
// Add form collects an interface's own settings, and what it writes to the RNS
// config re-parses to the values that were entered.
func TestInterfaceAddFormWritesConfigThatReparses(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	v := NewInterfaceAddFormView(newTestApp(), IfaceRNode, InterfaceFormOptions{TransportEnabled: true})

	if got, want := v.Title(), "Add Interface"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}

	v.SetValue("name", "radio")
	if _, ok := v.Value("port"); !ok {
		t.Error("RNodeInterface form has no port field")
	}
	v.SetValue("port", "/dev/ttyACM0")
	v.SetValue("frequency", "915")
	v.SetValue("txpower", "17")
	v.SetValue("bandwidth", "125000")
	v.SetValue("spreadingfactor", "11")
	v.SetValue("codingrate", "4:5")

	var savedName string
	var savedProps []app.ConfigProp
	v.OnSave = func(name string, props []app.ConfigProp) error {
		savedName, savedProps = name, props
		return a.AddInterfaceConfig(name, props)
	}
	if err := v.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if savedName != "radio" {
		t.Fatalf("OnSave name = %q, want %q", savedName, "radio")
	}

	got, err := a.GetInterfaceConfigMap("radio")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(radio) error: %v", err)
	}
	// The re-parsed values are the config's, not the form's display text: the
	// frequency field is filled in MHz ("915") but the section carries Hz, and
	// the coding rate dropdown reads "4:5" but the section carries 5 — both are
	// Python's field transforms (Interfaces.py:455-486).
	for k, want := range map[string]string{
		"type":            IfaceRNode,
		"port":            "/dev/ttyACM0",
		"frequency":       "915000000",
		"txpower":         "17",
		"bandwidth":       "125000",
		"spreadingfactor": "11",
		"codingrate":      "5",
	} {
		if got[k] != want {
			t.Errorf("re-parsed config[%q] = %q, want %q", k, got[k], want)
		}
	}
	if got["interface_enabled"] != "True" {
		t.Errorf("re-parsed interface_enabled = %q, want %q", got["interface_enabled"], "True")
	}

	// The write must be deterministic: the fields are written in form order, so
	// the section is "type", "interface_enabled", then the fields as displayed.
	// The transport-mode common options follow, because a checkbox and a
	// dropdown always yield a value and Python's announce_cap transform supplies
	// 2.0 for an empty field — so a form saved untouched still carries them.
	var keys []string
	for _, p := range savedProps {
		keys = append(keys, p.Key)
	}
	wantKeys := []string{
		"type", "interface_enabled", "port", "frequency", "txpower", "bandwidth",
		"spreadingfactor", "codingrate", "outgoing", "mode", "announce_cap",
	}
	if strings.Join(keys, ",") != strings.Join(wantKeys, ",") {
		t.Errorf("saved props order = %v, want %v", keys, wantKeys)
	}

	// The added interface must also appear in the file's [interfaces] section
	// alongside the ones that were already there.
	cfg, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) error: %v", err)
	}
	if cfg["target_port"] != "4242" {
		t.Errorf("existing hub interface was disturbed: target_port = %q, want %q", cfg["target_port"], "4242")
	}
}

// TestInterfaceFormMoreOptionsRevealsAdditionalAndCommonFields pins Python's
// "Show more options" toggle (Interfaces.py:1727): the per-type additional
// options and the common options are absent from the form until it is pressed.
func TestInterfaceFormMoreOptionsRevealsAdditionalAndCommonFields(t *testing.T) {
	t.Parallel()

	v := NewInterfaceAddFormView(newTestApp(), IfaceRNode, InterfaceFormOptions{TransportEnabled: true})

	if v.MoreOptionsVisible() {
		t.Error("the more-options section starts hidden")
	}
	hidden := []string{"id_callsign", "id_interval", "network_name", "passphrase", "ifac_size", "bitrate"}
	for _, key := range hidden {
		if slices.Contains(v.VisibleFieldKeys(), key) {
			t.Errorf("field %q is displayed before Show more options", key)
		}
	}

	v.ShowMoreOptions()
	if !v.MoreOptionsVisible() {
		t.Error("ShowMoreOptions did not reveal the section")
	}
	for _, key := range hidden {
		if !slices.Contains(v.VisibleFieldKeys(), key) {
			t.Errorf("field %q is not displayed after Show more options", key)
		}
	}

	v.HideMoreOptions()
	for _, key := range hidden {
		if slices.Contains(v.VisibleFieldKeys(), key) {
			t.Errorf("field %q is still displayed after HideMoreOptions", key)
		}
	}

	// A value entered in the hidden section still reaches the saved config.
	v.ShowMoreOptions()
	v.SetValue("id_callsign", "N0CALL-0")
	v.HideMoreOptions()
	if _, props := v.Build(); !hasProp(props, "id_callsign", "N0CALL-0") {
		t.Errorf("a hidden section's value did not reach the saved configuration: %v", props)
	}
}

// hasProp reports whether props carries the given key with the given value.
func hasProp(props []app.ConfigProp, key string, value any) bool {
	for _, p := range props {
		if p.Key == key && p.Value == value {
			return true
		}
	}
	return false
}

// TestInterfacesDisplayShowTypePickerSelectsType pins the Add flow's first step
// as the page mounts it: Ctrl-A shows the picker as its own page, and choosing
// an option hands the interface type to the caller (Python's
// InterfaceOptionItem keypress → switch_to_add_interface, Interfaces.py:1259).
func TestInterfacesDisplayShowTypePickerSelectsType(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	var chosen string
	id.ShowTypePicker(func(ifType string) { chosen = ifType })

	if !id.pages.HasPage("picker") {
		t.Fatal("ShowTypePicker did not mount the picker page")
	}
	if id.pages.GetPageCount() > 1 && id.pickerOverlay == nil {
		t.Error("ShowTypePicker did not record the overlay")
	}
	if got := app.GetFocus(); got == nil {
		t.Error("ShowTypePicker left the app without focus")
	}

	picker, ok := id.pickerOverlay.Dialog().Content().(*InterfaceTypePicker)
	if !ok {
		t.Fatalf("the picker dialog's content is %T, want *InterfaceTypePicker", id.pickerOverlay.Dialog().Content())
	}
	picker.SetSelected(IfaceRNode)
	picker.SelectFocused()
	if chosen != IfaceRNode {
		t.Errorf("chosen type = %q, want %q", chosen, IfaceRNode)
	}

	id.CloseTypePicker()
	if id.pages.HasPage("picker") {
		t.Error("CloseTypePicker did not remove the picker page")
	}
}

// TestInterfacesDisplayShowFormMountsAndCloses pins the Add flow's second step:
// the form is mounted as its own page with focus on it, and Cancel restores the
// interface list (Python's on_back → switch_to_list, Interfaces.py:1966-1967).
func TestInterfacesDisplayShowFormMountsAndCloses(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	cancelled := false
	form := NewInterfaceAddFormView(app, IfacePipe, InterfaceFormOptions{})
	form.OnCancel = func() { cancelled = true }
	id.ShowForm(form)

	if !id.pages.HasPage("form") {
		t.Fatal("ShowForm did not mount the form page")
	}
	if id.FormView() != form {
		t.Error("ShowForm did not record the form view")
	}
	if got := app.GetFocus(); got == nil {
		t.Error("ShowForm left the app without focus")
	}

	form.Cancel()
	if !cancelled {
		t.Error("Cancel did not run the form's own cancel handler")
	}
	if id.pages.HasPage("form") {
		t.Error("the form page survived a cancel")
	}
	if id.FormView() != nil {
		t.Error("FormView still returns the closed form")
	}
}

// TestInterfaceAddFormValidatesRequiredFields pins that a required field left
// empty stops the save and writes nothing (Python on_save → validate_all,
// Interfaces.py:1862-1894).
func TestInterfaceAddFormValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	v := NewInterfaceAddFormView(newTestApp(), IfaceTCPClient, InterfaceFormOptions{TransportEnabled: true})

	called := false
	v.OnSave = func(name string, props []app.ConfigProp) error {
		called = true
		return a.AddInterfaceConfig(name, props)
	}

	v.SetValue("name", "")
	v.SetValue("target_host", "127.0.0.1")
	if err := v.Save(); err == nil {
		t.Error("Save with an empty name should fail")
	}
	if called {
		t.Error("OnSave ran for an invalid form")
	}
	if msg := v.FieldError("name"); msg == "" {
		t.Error("no validation error reported for the empty name")
	}

	v.SetValue("name", "client")
	v.SetValue("target_host", "")
	if err := v.Save(); err == nil {
		t.Error("Save with an empty required target_host should fail")
	}

	v.SetValue("target_host", "127.0.0.1")
	v.SetValue("target_port", "4242")
	if err := v.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if !called {
		t.Error("OnSave did not run for a valid form")
	}
}

// TestInterfaceAddFormRejectsAnExistingName pins the duplicate-name check: the
// page supplies the predicate (it is the layer that knows the config's other
// sections) and the form refuses to save and marks the name field (Python
// on_save against app.rns.config['interfaces'], Interfaces.py:1887-1891).
func TestInterfaceAddFormRejectsAnExistingName(t *testing.T) {
	t.Parallel()

	v := NewInterfaceAddFormView(newTestApp(), IfaceTCPServer, InterfaceFormOptions{})
	v.OnNameConflict = func(name string) bool { return name == "hub" }

	saved := false
	v.OnSave = func(string, []app.ConfigProp) error {
		saved = true
		return nil
	}

	v.SetValue("name", "fresh")
	v.SetValue("listen_ip", "0.0.0.0")
	if err := v.Save(); err != nil {
		t.Fatalf("Save() with a free name error: %v", err)
	}
	if !saved {
		t.Error("a free name was refused")
	}

	saved = false
	v.SetValue("name", "hub")
	if err := v.Save(); err == nil {
		t.Error("Save() with a taken name should fail")
	}
	if saved {
		t.Error("OnSave ran for a taken name")
	}
	if v.FieldError("name") == "" {
		t.Error("the taken name was not marked on the form")
	}
}

// TestInterfaceEditFormPreservesExistingSettings pins Python's
// EditInterfaceView: every setting already in the config is pre-loaded into the
// form and survives a save that changes nothing else — including the common
// options (network_name, passphrase, mode) that only the more-options section
// shows, so a plain save cannot silently erase them.
func TestInterfaceEditFormPreservesExistingSettings(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	existing, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) error: %v", err)
	}

	v := NewInterfaceEditFormView(newTestApp(), "hub", existing, InterfaceFormOptions{TransportEnabled: true})

	if got, want := v.Title(), "Edit Interface: hub"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got, _ := v.Value("name"); got != "hub" {
		t.Errorf("name = %q, want %q", got, "hub")
	}
	if got, _ := v.Value("target_host"); got != "127.0.0.1" {
		t.Errorf("target_host = %q, want %q", got, "127.0.0.1")
	}
	// A setting that lives in the more-options section is loaded even though the
	// section starts hidden, so a plain save cannot silently erase it.
	if got, _ := v.Value("network_name"); got != "mynet" {
		t.Errorf("network_name = %q, want %q", got, "mynet")
	}

	// Rename, and change one setting.
	v.SetValue("name", "hub2")
	v.SetValue("target_port", "4343")

	var savedName string
	var savedProps []app.ConfigProp
	v.OnSave = func(name string, props []app.ConfigProp) error {
		savedName, savedProps = name, props
		return a.EditInterfaceConfig("hub", name, props)
	}
	if err := v.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if savedName != "hub2" {
		t.Fatalf("OnSave name = %q, want %q", savedName, "hub2")
	}
	// The saved section keeps its shape: the type leads, followed by the enabled
	// flag and then the settings in the order the form displays them.
	if len(savedProps) < 2 || savedProps[0].Key != "type" || savedProps[1].Key != "interface_enabled" {
		t.Errorf("saved props do not lead with type/interface_enabled: %+v", savedProps)
	}

	got, err := a.GetInterfaceConfigMap("hub2")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub2) error: %v", err)
	}
	want := map[string]string{
		"type":         IfaceTCPClient,
		"target_host":  "127.0.0.1",
		"target_port":  "4343",
		"network_name": "mynet",
		"passphrase":   "secret",
		"mode":         "gateway",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("re-parsed config[%q] = %q, want %q", k, got[k], w)
		}
	}

	old, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) error: %v", err)
	}
	if len(old) != 0 {
		t.Errorf("the old section survived the rename: %v", old)
	}
}

// TestInterfaceEditFormLoadsTransformedValues pins the load direction of the
// field transforms, which is the half a save alone cannot show: a section's
// frequency is in Hz but the form displays MHz, and a coding rate is an integer
// but the form's dropdown shows "4:5" (Python _populate_form_fields,
// Interfaces.py:2039-2062).
func TestInterfaceEditFormLoadsTransformedValues(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	existing, err := a.GetInterfaceConfigMap("radio")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(radio) error: %v", err)
	}

	v := NewInterfaceEditFormView(newTestApp(), "radio", existing, InterfaceFormOptions{})

	for key, want := range map[string]string{
		"name":            "radio",
		"port":            "/dev/ttyACM0",
		"frequency":       "868.5",
		"txpower":         "17",
		"bandwidth":       "62500",
		"spreadingfactor": "10",
		"codingrate":      "4:5",
	} {
		if got, _ := v.Value(key); got != want {
			t.Errorf("Value(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestInterfaceEditFormUnchangedSavePreservesTheSection pins the whole point of
// the Edit form: opening an interface and pressing Save without changing
// anything writes back exactly what was there — no field dropped, none invented,
// none renamed. It uses a non-transport form, which is what the appliance's
// attached client is (its config renders enable_transport = no), so no
// transport-mode option is added to the section.
func TestInterfaceEditFormUnchangedSavePreservesTheSection(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	before, err := a.GetInterfaceConfigMap("radio")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(radio) error: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("the radio fixture section is empty")
	}

	v := NewInterfaceEditFormView(newTestApp(), "radio", before, InterfaceFormOptions{})
	v.OnSave = func(name string, props []app.ConfigProp) error {
		return a.EditInterfaceConfig("radio", name, props)
	}
	if err := v.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	after, err := a.GetInterfaceConfigMap("radio")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(radio) after save error: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("an unchanged save altered the section:\nbefore = %v\nafter  = %v", before, after)
	}
}

// TestInterfaceEditFormLoadsCheckboxOptions pins the checkbox half of the
// form's populate step (Python's _set_field_value, Interfaces.py:2064-2070): a
// per-type additional option that is a checkbox reads back as its state, not
// as text.
func TestInterfaceEditFormLoadsCheckboxOptions(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	existing, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) error: %v", err)
	}

	v := NewInterfaceEditFormView(newTestApp(), "hub", existing, InterfaceFormOptions{})

	if got, _ := v.Value("i2p_tunneled"); got != "true" {
		t.Errorf("Value(i2p_tunneled) = %q, want %q", got, "true")
	}
	if got, _ := v.Value("kiss_framing"); got != "false" {
		t.Errorf("Value(kiss_framing) = %q, want %q", got, "false")
	}
}

// TestInterfaceEditFormSaveWritesCheckboxState pins a Python behaviour worth
// knowing before it surprises someone: a checkbox always yields a value, so a
// save writes its state even when it is false — an unchanged save of a
// TCPClientInterface adds `kiss_framing = False` to a section that did not have
// it (FormCheckbox.get_value returns the state and Python's `value != ""` test
// is true for False, Interfaces.py:1926-1930).
func TestInterfaceEditFormSaveWritesCheckboxState(t *testing.T) {
	t.Parallel()

	a := newFormTestApp(t)
	before, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) error: %v", err)
	}
	if _, had := before["kiss_framing"]; had {
		t.Fatal("the fixture already carries kiss_framing; this test needs it absent")
	}

	v := NewInterfaceEditFormView(newTestApp(), "hub", before, InterfaceFormOptions{})
	v.OnSave = func(name string, props []app.ConfigProp) error {
		return a.EditInterfaceConfig("hub", name, props)
	}
	if err := v.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	after, err := a.GetInterfaceConfigMap("hub")
	if err != nil {
		t.Fatalf("GetInterfaceConfigMap(hub) after save error: %v", err)
	}
	if after["i2p_tunneled"] != "True" {
		t.Errorf("i2p_tunneled = %q, want %q", after["i2p_tunneled"], "True")
	}
	if after["kiss_framing"] != "False" {
		t.Errorf("kiss_framing = %q, want %q", after["kiss_framing"], "False")
	}
}

// drawnText renders a primitive at the given grid size and returns everything it
// drew, so a test can assert on screen content rather than on widget state.
func drawnText(t *testing.T, root tview.Primitive, w, h int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	root.SetRect(0, 0, w, h)
	root.Draw(screen)
	// Draw writes the back buffer; Show is what renders it into the front buffer
	// that GetContents reads.
	screen.Show()
	return screenText(screen)
}

// TestInterfaceFormRendersFitsTheGrid pins that the form actually draws, at the
// host console's 80x24 and at the tablet's real grid. The appliance's console
// host starts the pty at 80x24 and the client resizes it once the terminal view
// has been measured (measured as at least 151 columns), so a form that only
// renders at one of those widths would ship broken to the other.
//
// The form is mounted the way the page mounts it — as an 85% x 85% overlay — so
// this also pins the overlay geometry: the dialog's rect must be 85% of the grid
// with no inset (Python's AddInterfaceView overlay, Interfaces.py:1464-1472).
func TestInterfaceFormRendersFitsTheGrid(t *testing.T) {
	t.Parallel()

	grids := []struct {
		w, h  int
		wantW int
		wantH int
	}{
		{w: 80, h: 24, wantW: 68, wantH: 20},
		{w: 151, h: 46, wantW: 128, wantH: 39},
	}

	for _, grid := range grids {
		t.Run(fmt.Sprintf("%vx%v", grid.w, grid.h), func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			id := NewInterfacesDisplay(app, nil)
			form := NewInterfaceAddFormView(app, IfaceRNode, InterfaceFormOptions{TransportEnabled: true})
			id.ShowForm(form)

			text := drawnText(t, id.pages, grid.w, grid.h)

			if x, y, w, h := form.Dialog().GetRect(); w != grid.wantW || h != grid.wantH {
				t.Errorf("the form overlay rect is %vx%v at (%v,%v), want %vx%v", w, h, x, y, grid.wantW, grid.wantH)
			}
			for _, want := range []string{"Add Interface", "Name: ", "Frequency (MHz): ", "Show more options"} {
				if !strings.Contains(text, want) {
					t.Errorf("the drawn form does not contain %q", want)
				}
			}
		})
	}
}

// TestInterfaceFormScrollsToItsButtons pins that the Save/Cancel row is
// reachable, not just present in the widget tree: the pile is taller than the
// 85% overlay once the more-options section is open, and urwid's Filler (which
// pileFiller replicates) trims the top to keep the focused item in view rather
// than dropping the rows below. So with focus on the button row the buttons are
// what is drawn.
func TestInterfaceFormScrollsToItsButtons(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)
	form := NewInterfaceAddFormView(app, IfaceRNode, InterfaceFormOptions{TransportEnabled: true})
	form.ShowMoreOptions()
	id.ShowForm(form)

	if text := drawnText(t, id.pages, 151, 46); strings.Contains(text, "Cancel") {
		t.Error("the button row is drawn while focus is still on the first field; this test needs it off-screen")
	}

	// Move focus onto the last selectable row — the Save/Cancel row, where
	// Tab/Down from the field above lands.
	form.pile.SetFocusIndex(len(form.pile.selectable) - 1)
	if text := drawnText(t, id.pages, 151, 46); !strings.Contains(text, "Save") || !strings.Contains(text, "Cancel") {
		t.Error("the Save/Cancel row is not drawn after moving focus onto it")
	}
}

// TestInterfaceTypePickerRenders pins that the picker draws its headings and
// options through the page's overlay, so Ctrl-A shows a usable list.
func TestInterfaceTypePickerRenders(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)
	id.ShowTypePicker(func(string) {})

	text := drawnText(t, id.pages, 151, 46)
	for _, want := range []string{"Select Interface Type", "IP Networks", "Auto Interface", "Custom Interface", "Other"} {
		if !strings.Contains(text, want) {
			t.Errorf("the drawn picker does not contain %q", want)
		}
	}
}

// TestInterfaceEditFormRoundTripsCompositeSections pins the two interface types
// whose settings are not a flat list of keys: RNodeMultiInterface, whose
// sub-interfaces are [[[subsection]]]s, and CustomInterface, whose parameters are
// whatever keys the section carries. Both are loaded into the form's composite
// rows and written back unchanged, so neither an edit nor a look at the page can
// delete them.
func TestInterfaceEditFormRoundTripsCompositeSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		iface   string
		row     string
		wantSub []string
	}{
		{
			name:    "rnode multi sub-interfaces",
			iface:   "multi",
			row:     "subinterfaces",
			wantSub: []string{"sub1: bandwidth=62500, frequency=868500000", "sub2: bandwidth=125000, frequency=915000000"},
		},
		{
			name:    "custom parameters",
			iface:   "custom",
			row:     "custom_parameters",
			wantSub: []string{"some_setting = abc", "speed = 115200"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := newFormTestApp(t)
			before, err := a.GetInterfaceConfigMap(tt.iface)
			if err != nil {
				t.Fatalf("GetInterfaceConfigMap(%v) error: %v", tt.iface, err)
			}

			v := NewInterfaceEditFormView(newTestApp(), tt.iface, before, InterfaceFormOptions{})
			got, _ := v.Value(tt.row)
			for _, want := range tt.wantSub {
				if !strings.Contains(got, want) {
					t.Errorf("Value(%q) = %q, want it to contain %q", tt.row, got, want)
				}
			}

			v.OnSave = func(name string, props []app.ConfigProp) error {
				return a.EditInterfaceConfig(tt.iface, name, props)
			}
			if err := v.Save(); err != nil {
				t.Fatalf("Save() error: %v", err)
			}

			after, err := a.GetInterfaceConfigMap(tt.iface)
			if err != nil {
				t.Fatalf("GetInterfaceConfigMap(%v) after save error: %v", tt.iface, err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Errorf("an unchanged save altered the section:\nbefore = %v\nafter  = %v", before, after)
			}
		})
	}
}
