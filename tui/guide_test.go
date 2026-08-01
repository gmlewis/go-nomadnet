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
	"testing"

	"github.com/rivo/tview"
)

func TestNewGuideDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	if gd == nil {
		t.Fatal("NewGuideDisplay returned nil")
	}
	if gd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestGuideDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	_, ok := gd.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestGuideContent(t *testing.T) {
	t.Parallel()

	// The Introduction topic is embedded verbatim from the Python Guide and
	// rendered through the micron styled-line renderer. Its rendered plain
	// text must mention Nomad Network and Reticulum (the original introContent
	// checked the same substrings).
	content := guideTopicPlainText(guideIntroduction, ThemeDark)
	if len(content) == 0 {
		t.Error("introduction rendered text is empty")
	}
	if !containsStr(content, "Nomad Network") {
		t.Error("introduction missing 'Nomad Network'")
	}
	if !containsStr(content, "Reticulum") {
		t.Error("introduction missing 'Reticulum'")
	}
}

// TestGuideTopicsCount verifies all 12 embedded topics are registered in the
// canonical order from Python's TopicList (Guide.py:167-180), with "First Run"
// at index 8.
func TestGuideTopicsCount(t *testing.T) {
	t.Parallel()

	if len(guideTopics) != 12 {
		t.Fatalf("guideTopics has %v entries, want 12", len(guideTopics))
	}
	if guideTopics[firstRunTopicIndex].label != "First Run" {
		t.Errorf("index %v = %q, want First Run", firstRunTopicIndex, guideTopics[firstRunTopicIndex].label)
	}
	want := []string{
		"Introduction", "Concepts & Terminology", "Channels & RRC", "Interfaces",
		"Hosting a Node", "Configuration Options", "Keyboard Shortcuts", "Markup",
		"First Run", "Network Configuration", "Display Test", "Credits & Licenses",
	}
	for i, w := range want {
		if guideTopics[i].label != w {
			t.Errorf("guideTopics[%v].label = %q, want %q", i, guideTopics[i].label, w)
		}
	}
}

func TestNewInterfacesDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	if id == nil {
		t.Fatal("NewInterfacesDisplay returned nil")
	}
	if id.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestInterfacesDisplayWithInterfaces(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	interfaces := []InterfaceInfo{
		{Name: "TCP", Type: "TCPClientInterface", Status: "connected", Target: "192.168.1.1:4173"},
		{Name: "LoRa", Type: "RNodeInterface", Status: "disconnected", Target: "/dev/ttyUSB0"},
	}

	id := NewInterfacesDisplay(app, interfaces)
	if id == nil {
		t.Fatal("NewInterfacesDisplay returned nil")
	}
	if id.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestFormatBandwidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input float64
		want  string
	}{
		{0, "0 B/s"},
		{512, "512 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"},
		{1048576, "1.0 MB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := formatBandwidth(tt.input)
			if got != tt.want {
				t.Errorf("formatBandwidth(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatInterfaces(t *testing.T) {
	t.Parallel()

	result := formatInterfaces(nil)
	if !containsStr(result, "No interfaces") {
		t.Error("formatInterfaces(nil) missing empty message")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
