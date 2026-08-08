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

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// TestApplyConfigPrintingFrom verifies the print_from parsing in applyConfig
// matches Python NomadNetworkApp.applyConfig (NomadNetworkApp.py:1170-1218):
//   - "everywhere" → print_all_messages
//   - "trusted"    → print_trusted_messages
//   - a 32-hex destination hash → allowed_message_print_destinations entry
//   - a comma list → allowed list, with "trusted" entries enabling trusted
//   - unset        → allowed stays nil
//
// print_from and message_template are only processed when print_messages is
// true, matching the Python `if self.print_messages:` guard.
func TestApplyConfigPrintingFrom(t *testing.T) {
	t.Parallel()

	const hexHash = "0123456789abcdef0123456789abcdef" // 32 hex chars

	cases := []struct {
		name           string
		printFrom      string
		wantAll        bool
		wantTrusted    bool
		wantAllowedNil bool
		wantAllowed    []string
	}{
		{name: "everywhere", printFrom: "everywhere", wantAll: true, wantAllowedNil: true},
		{name: "trusted", printFrom: "trusted", wantTrusted: true, wantAllowedNil: true},
		{name: "hex_hash", printFrom: hexHash, wantAllowed: []string{hexHash}},
		{name: "unset", printFrom: "", wantAllowedNil: true},
		{name: "comma_list_with_trusted", printFrom: hexHash + ", trusted", wantTrusted: true, wantAllowed: []string{hexHash, "trusted"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Printing: config.PrintingConfig{
				PrintMessages: true,
				PrintFrom:     c.printFrom,
			}}
			a := &App{}
			a.applyConfig(cfg)
			if a.PrintAllMessages != c.wantAll {
				t.Errorf("PrintAllMessages = %v, want %v", a.PrintAllMessages, c.wantAll)
			}
			if a.PrintTrustedMessages != c.wantTrusted {
				t.Errorf("PrintTrustedMessages = %v, want %v", a.PrintTrustedMessages, c.wantTrusted)
			}
			if c.wantAllowedNil {
				if a.AllowedMessagePrintDestinations != nil {
					t.Errorf("AllowedMessagePrintDestinations = %v, want nil", a.AllowedMessagePrintDestinations)
				}
			} else {
				if !equalStrSlices(a.AllowedMessagePrintDestinations, c.wantAllowed) {
					t.Errorf("AllowedMessagePrintDestinations = %v, want %v", a.AllowedMessagePrintDestinations, c.wantAllowed)
				}
			}
		})
	}
}

// TestApplyConfigPrintingTemplate verifies the message_template handling:
// unset → default template; existing file → file contents.
func TestApplyConfigPrintingTemplate(t *testing.T) {
	t.Parallel()

	t.Run("default_when_unset", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Printing: config.PrintingConfig{PrintMessages: true}}
		a := &App{}
		a.applyConfig(cfg)
		if a.PrintingTemplateMsg != defaultPrintingTemplate {
			t.Error("PrintingTemplateMsg should be default when template unset")
		}
	})

	t.Run("reads_existing_file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "tpl.txt")
		want := "CUSTOM TEMPLATE BODY\n"
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("write template: %v", err)
		}
		cfg := &config.Config{Printing: config.PrintingConfig{
			PrintMessages:   true,
			MessageTemplate: path,
		}}
		a := &App{}
		a.applyConfig(cfg)
		if a.PrintingTemplateMsg != want {
			t.Errorf("PrintingTemplateMsg = %q, want %q", a.PrintingTemplateMsg, want)
		}
	})

	t.Run("not_processed_when_print_messages_false", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Printing: config.PrintingConfig{
			PrintMessages: false,
			PrintFrom:     "everywhere",
		}}
		a := &App{}
		a.applyConfig(cfg)
		if a.PrintAllMessages {
			t.Error("PrintAllMessages should be false when print_messages is false")
		}
	})
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
