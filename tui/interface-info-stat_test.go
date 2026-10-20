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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
)

// TestInterfaceInfoFromStatCarriesParams pins the seam the Interfaces page
// builds its rows from. The detail view's parameter blocks are built from
// InterfaceInfo.Params, so an InterfaceStat whose parameters are dropped on the
// way in makes every interface read "No additional parameters".
func TestInterfaceInfoFromStatCarriesParams(t *testing.T) {
	t.Parallel()

	stat := app.InterfaceStat{
		Name:      "RNode LoRa",
		Type:      "RNodeInterface",
		Enabled:   true,
		Connected: false,
		TX:        1024,
		RX:        2048,
		Bitrate:   125000,
		Params: map[string]string{
			"port":      "/dev/ttyACM0",
			"frequency": "915000000",
		},
	}

	info := InterfaceInfoFromStat(stat)

	if info.Name != stat.Name || info.Type != stat.Type || info.Enabled != stat.Enabled ||
		info.Connected != stat.Connected || info.TX != stat.TX || info.RX != stat.RX ||
		info.Bitrate != stat.Bitrate {
		t.Errorf("InterfaceInfoFromStat(%+v) = %+v, want every status field carried through", stat, info)
	}
	if len(info.Params) != len(stat.Params) {
		t.Fatalf("InterfaceInfoFromStat carried %v parameters, want %v", len(info.Params), len(stat.Params))
	}
	for k, v := range stat.Params {
		if info.Params[k] != v {
			t.Errorf("parameter %q = %q, want %q", k, info.Params[k], v)
		}
	}

	// And the view built from it lists them.
	view := NewInterfaceShow(NewApp(ThemeDark, GlyphUnicode, ColorModeTrue), info)
	rendered := view.buildBodyText(94)
	for _, want := range []string{"Port: /dev/ttyACM0", "Frequency: 915.000 MHz"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the detail view does not render %q:\n%v", want, rendered)
		}
	}
	if strings.Contains(rendered, "No additional parameters") {
		t.Errorf("the detail view says there are no parameters for an interface that has two")
	}
}
