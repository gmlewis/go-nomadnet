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

// ValidateAll validates all fields and returns any errors found.
// existingNames is an optional set of names that should be considered
// duplicates. Matches Python's validate_all() at Interfaces.py:1846.
func (fd *InterfaceFormData) ValidateAll(existingNames ...map[string]bool) []ValidationError {
	var errs []ValidationError

	validateFields := func(fields map[string]*FormFieldEntry) {
		for key, field := range fields {
			for _, v := range field.Validation {
				switch v {
				case "required":
					if strings.TrimSpace(field.Value) == "" {
						errs = append(errs, ValidationError{
							FieldKey: key,
							Message:  fmt.Sprintf("%v is required", field.Label),
						})
					}
				case "number":
					trimmed := strings.TrimSpace(field.Value)
					if trimmed != "" {
						var n float64
						if _, err := fmt.Sscanf(trimmed, "%f", &n); err != nil {
							errs = append(errs, ValidationError{
								FieldKey: key,
								Message:  fmt.Sprintf("%v must be a number", field.Label),
							})
						}
					}
				case "float":
					trimmed := strings.TrimSpace(field.Value)
					if trimmed != "" {
						var f float64
						if _, err := fmt.Sscanf(trimmed, "%f", &f); err != nil {
							errs = append(errs, ValidationError{
								FieldKey: key,
								Message:  fmt.Sprintf("%v must be a number", field.Label),
							})
						}
					}
				}
			}
		}
	}

	validateFields(fd.Fields)
	validateFields(fd.AdditionalFields)
	validateFields(fd.CommonFields)

	if len(existingNames) > 0 {
		nameVal := strings.TrimSpace(fd.Fields["name"].Value)
		if nameVal != "" && existingNames[0][nameVal] {
			errs = append(errs, ValidationError{
				FieldKey: "name",
				Message:  fmt.Sprintf("Interface name '%v' already exists", nameVal),
			})
		}
	}

	return errs
}

// BuildConfig constructs the interface configuration dictionary from
// current field values. Empty values are excluded. Matches Python's
// on_save() at Interfaces.py:1864.
func (fd *InterfaceFormData) BuildConfig() map[string]any {
	config := map[string]any{
		"type":              fd.IfaceType,
		"interface_enabled": true,
	}

	if fd.IfaceType == IfaceCustom {
		if typeField, ok := fd.Fields["type"]; ok && typeField.Value != "" {
			config["type"] = typeField.Value
		}
	}

	writeFields := func(fields map[string]*FormFieldEntry) {
		for key, field := range fields {
			if key == "name" || key == "custom_parameters" || key == "type" {
				continue
			}
			value := strings.TrimSpace(field.Value)
			if value != "" {
				config[field.ConfigKey] = value
			}
		}
	}

	writeFields(fd.Fields)
	writeFields(fd.AdditionalFields)
	writeFields(fd.CommonFields)

	return config
}

// humanizeConfigKey converts a snake_case config key to a capitalized
// human-readable label. For example, "listen_ip" becomes "Listen Ip".
func humanizeConfigKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// PopulateFromConfig populates form fields from an existing interface
// configuration dictionary. This is used by EditInterfaceView to
// pre-fill the form. Matches Python's EditInterfaceView.__init__()
// and _populate_form_fields() at Interfaces.py:1997.
func (fd *InterfaceFormData) PopulateFromConfig(ifaceName string, config map[string]any) {
	fd.Fields["name"].Value = ifaceName

	if fd.IfaceType == IfaceCustom {
		if typeField, ok := fd.Fields["type"]; ok {
			if t, ok := config["type"]; ok {
				typeField.Value = fmt.Sprintf("%v", t)
			}
		}
	}

	populateMap := func(fields map[string]*FormFieldEntry) {
		for key, field := range fields {
			if key == "name" || key == "type" || key == "custom_parameters" {
				continue
			}
			if rawVal, ok := config[key]; ok {
				field.Value = formatFieldValue(key, rawVal, field.Type)
			}
		}
	}

	populateMap(fd.Fields)
	populateMap(fd.AdditionalFields)
	populateMap(fd.CommonFields)
}

// formatFieldValue converts a config value to a string suitable for a
// form field. Special-cases RNode frequency (Hz → MHz) and booleans.
func formatFieldValue(key string, rawVal any, fieldType string) string {
	switch fieldType {
	case "checkbox":
		b, ok := rawVal.(bool)
		if ok {
			if b {
				return "true"
			}
			return "false"
		}
		return fmt.Sprintf("%v", rawVal)
	default:
		if key == "frequency" {
			if freq, ok := rawVal.(float64); ok {
				mhz := freq / 1000000.0
				return fmt.Sprintf("%v", mhz)
			}
			if freq, ok := rawVal.(int64); ok {
				mhz := float64(freq) / 1000000.0
				return fmt.Sprintf("%v", mhz)
			}
			if freq, ok := rawVal.(int); ok {
				mhz := float64(freq) / 1000000.0
				return fmt.Sprintf("%v", mhz)
			}
		}
		return fmt.Sprintf("%v", rawVal)
	}
}
