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
	"strings"
)

// FormFieldEntry represents a single form field's state in an interface
// add/edit form. Matches Python's AddInterfaceView field tracking.
type FormFieldEntry struct {
	ConfigKey  string
	Label      string
	Type       string
	Value      string
	Validation []string
	Options    []string
	SubFields  map[string]SubField
}

// ValidationError describes a validation failure on a form field.
type ValidationError struct {
	FieldKey string
	Message  string
}

// InterfaceFormData manages the form state for adding or editing an
// interface. It tracks field values, additional options, common
// options, and provides validation and config-building logic.
// Matches Python's AddInterfaceView at Interfaces.py:1418.
type InterfaceFormData struct {
	IfaceType        string
	Fields           map[string]*FormFieldEntry
	AdditionalFields map[string]*FormFieldEntry
	CommonFields     map[string]*FormFieldEntry
}

// commonInterfaceOptions defines the common fields shown under "more
// options" for every interface type. Matches Python's
// COMMON_INTERFACE_OPTIONS at Interfaces.py:367.
var commonInterfaceOptions = []InterfaceField{
	{ConfigKey: "network_name", Type: "edit", Label: "Virtual Network Name: ", Placeholder: "Optional virtual network name"},
	{ConfigKey: "passphrase", Type: "edit", Label: "IFAC Passphrase: ", Placeholder: "IFAC authentication passphrase"},
	{ConfigKey: "ifac_size", Type: "edit", Label: "IFAC Size: ", Placeholder: "8 - 512", Validation: []string{"number"}},
	{ConfigKey: "bitrate", Type: "edit", Label: "Inferred Bitrate: ", Placeholder: "Automatically determined", Validation: []string{"number"}},
}

// transportCommonOptions are the transport-mode options Python appends to the
// common options when the node runs as a transport
// (Interfaces.py:1580-1609). Python appends them to the module-global
// COMMON_INTERFACE_OPTIONS list, so once any form has been opened in transport
// mode they stay for the rest of the process; here they are added to the form
// data on construction instead.
var transportCommonOptions = []InterfaceField{
	{ConfigKey: "outgoing", Type: "checkbox", Label: "Allow outgoing traffic", Default: "true"},
	{ConfigKey: "mode", Type: "dropdown", Label: "Interface Mode: ", Options: []string{"full", "gateway", "access_point", "roaming", "boundary"}, Default: "full"},
	{ConfigKey: "announce_cap", Type: "edit", Label: "Announce Cap: ", Placeholder: "Default: 2.0", Validation: []string{"float"}, Transform: transformAnnounceCap},
}

// AddTransportCommonOptions adds the transport-mode common options (Python's
// transport_enabled() branch of _initialize_common_fields).
func (fd *InterfaceFormData) AddTransportCommonOptions() {
	for _, opt := range transportCommonOptions {
		if _, ok := fd.CommonFields[opt.ConfigKey]; ok {
			continue
		}
		fd.CommonFields[opt.ConfigKey] = &FormFieldEntry{
			ConfigKey:  opt.ConfigKey,
			Label:      opt.Label,
			Type:       opt.Type,
			Value:      opt.Default,
			Validation: opt.Validation,
			Options:    opt.Options,
		}
	}
}

// CommonFieldList returns the common options in the order the form displays
// them: the shared options first, then the transport-mode ones. Only the
// options this instance holds are returned, so a client that is not a transport
// gets no mode/outgoing/announce_cap rows.
func (fd *InterfaceFormData) CommonFieldList() []InterfaceField {
	var out []InterfaceField
	for _, f := range append(append([]InterfaceField{}, commonInterfaceOptions...), transportCommonOptions...) {
		if _, ok := fd.CommonFields[f.ConfigKey]; ok {
			out = append(out, f)
		}
	}
	return out
}

// NewInterfaceFormData creates form data for the given interface type,
// initializing all fields from the INTERFACE_FIELDS registry, common
// options, and the required "name" field.
func NewInterfaceFormData(ifType string) *InterfaceFormData {
	fd := &InterfaceFormData{
		IfaceType:        ifType,
		Fields:           make(map[string]*FormFieldEntry),
		AdditionalFields: make(map[string]*FormFieldEntry),
		CommonFields:     make(map[string]*FormFieldEntry),
	}

	fd.Fields["name"] = &FormFieldEntry{
		ConfigKey:  "name",
		Label:      "Name: ",
		Type:       "edit",
		Validation: []string{"required"},
	}

	groups := InterfaceFieldsFor(ifType)
	for _, group := range groups {
		for _, field := range group.Fields {
			label := field.Label
			if label == "" {
				label = humanizeConfigKey(field.ConfigKey) + ": "
			}
			fd.Fields[field.ConfigKey] = &FormFieldEntry{
				ConfigKey:  field.ConfigKey,
				Label:      label,
				Type:       field.Type,
				Value:      field.Default,
				Validation: field.Validation,
				Options:    field.Options,
				SubFields:  field.SubFields,
			}
		}
		for _, opt := range group.AdditionalOptions {
			label := opt.Label
			if label == "" {
				label = humanizeConfigKey(opt.ConfigKey) + ": "
			}
			fd.AdditionalFields[opt.ConfigKey] = &FormFieldEntry{
				ConfigKey:  opt.ConfigKey,
				Label:      label,
				Type:       opt.Type,
				Value:      opt.Default,
				Validation: opt.Validation,
				Options:    opt.Options,
			}
		}
	}

	for _, opt := range commonInterfaceOptions {
		fd.CommonFields[opt.ConfigKey] = &FormFieldEntry{
			ConfigKey:  opt.ConfigKey,
			Label:      opt.Label,
			Type:       opt.Type,
			Value:      opt.Default,
			Validation: opt.Validation,
			Options:    opt.Options,
		}
	}

	return fd
}

// humanizeConfigKey converts a snake_case config key to a capitalized
// human-readable label. For example, "listen_ip" becomes "Listen Ip".
func humanizeConfigKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			// Capitalize the first rune (not the first byte) so a
			// multibyte leading character is not split into an
			// incomplete UTF-8 sequence.
			r := []rune(p)
			parts[i] = strings.ToUpper(string(r[0])) + string(r[1:])
		}
	}
	return strings.Join(parts, " ")
}
