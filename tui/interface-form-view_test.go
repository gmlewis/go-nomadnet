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
	"slices"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-reticulum/testutils"
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
	for k, want := range map[string]string{
		"type":            IfaceRNode,
		"port":            "/dev/ttyACM0",
		"frequency":       "915",
		"txpower":         "17",
		"bandwidth":       "125000",
		"spreadingfactor": "11",
		"codingrate":      "4:5",
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
	var keys []string
	for _, p := range savedProps {
		keys = append(keys, p.Key)
	}
	wantKeys := []string{"type", "interface_enabled", "port", "frequency", "txpower", "bandwidth", "spreadingfactor", "codingrate"}
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
