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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// TestApplyConfigUIModeMapping verifies the user_interface string maps to the
// correct App.UIMode constant, matching Python applyConfig's uimode selection.
func TestApplyConfigUIModeMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ui   string
		want int
	}{
		{"none", UINone},
		{"menu", UIMenu},
		{"text", UIText},
		{"graphical", UIGraphical},
		{"web", UIWeb},
	}
	for _, c := range cases {
		t.Run(c.ui, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Client: config.ClientConfig{UserInterface: c.ui}}
			a := &App{}
			a.applyConfig(cfg)
			if a.UIMode != c.want {
				t.Errorf("UIMode(%q) = %v, want %v", c.ui, a.UIMode, c.want)
			}
		})
	}
}
