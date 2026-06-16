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
)

func TestInterfaceStoreAdd(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Type: "TCPClientInterface", Status: "connected", Target: "1.2.3.4:4242"})

	iface := store.Get("eth0")
	if iface == nil {
		t.Fatal("Get returned nil")
	}
	if iface.Type != "TCPClientInterface" {
		t.Errorf("Type = %q, want %q", iface.Type, "TCPClientInterface")
	}
}

func TestInterfaceStoreUpdate(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Status: "connected"})
	store.Add(InterfaceInfo{Name: "eth0", Status: "disconnected", Bandwidth: 1000})

	iface := store.Get("eth0")
	if iface.Status != "disconnected" {
		t.Errorf("Status = %q, want %q", iface.Status, "disconnected")
	}
	if iface.Bandwidth != 1000 {
		t.Errorf("Bandwidth = %v, want 1000", iface.Bandwidth)
	}
}

func TestInterfaceStoreDelete(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Status: "connected"})
	store.Add(InterfaceInfo{Name: "eth1", Status: "connected"})

	store.Delete("eth0")
	if store.Get("eth0") != nil {
		t.Error("deleted interface still found")
	}
	if store.Get("eth1") == nil {
		t.Error("non-deleted interface missing")
	}
}

func TestInterfaceStoreList(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth1", Status: "connected"})
	store.Add(InterfaceInfo{Name: "eth0", Status: "connected"})

	all := store.List()
	if len(all) != 2 {
		t.Fatalf("count = %d, want 2", len(all))
	}
	// Should be sorted by name
	if all[0].Name != "eth0" {
		t.Errorf("first = %q, want eth0", all[0].Name)
	}
}

func TestInterfaceStoreConnectedCount(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Status: "connected"})
	store.Add(InterfaceInfo{Name: "eth1", Status: "disconnected"})
	store.Add(InterfaceInfo{Name: "eth2", Status: "connected"})

	if got := store.ConnectedCount(); got != 2 {
		t.Errorf("ConnectedCount() = %d, want 2", got)
	}
}

func TestInterfaceStoreFilterByType(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Type: "TCPClientInterface", Status: "connected"})
	store.Add(InterfaceInfo{Name: "rnode0", Type: "RNodeInterface", Status: "connected"})
	store.Add(InterfaceInfo{Name: "eth1", Type: "TCPClientInterface", Status: "disconnected"})

	tcp := store.FilterByType("TCPClientInterface")
	if len(tcp) != 2 {
		t.Errorf("TCPClientInterface count = %d, want 2", len(tcp))
	}
}

func TestInterfaceStoreSearch(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.Add(InterfaceInfo{Name: "eth0", Status: "connected"})
	store.Add(InterfaceInfo{Name: "rnode0", Status: "connected"})

	eth := store.SearchByName("eth")
	if len(eth) != 1 {
		t.Errorf("Search('eth') returned %d, want 1", len(eth))
	}
}

func TestTrafficRingBuffer(t *testing.T) {
	t.Parallel()

	rb := NewTrafficRingBuffer(5)
	for i := 0; i < 10; i++ {
		rb.Push(float64(i))
	}

	samples := rb.Samples()
	if len(samples) != 5 {
		t.Fatalf("Samples() returned %d, want 5", len(samples))
	}
	// Last 5 values should be 5,6,7,8,9
	expected := []float64{5, 6, 7, 8, 9}
	for i, v := range samples {
		if v != expected[i] {
			t.Errorf("sample[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestTrafficRingBufferEmpty(t *testing.T) {
	t.Parallel()

	rb := NewTrafficRingBuffer(5)
	samples := rb.Samples()
	if len(samples) != 0 {
		t.Errorf("empty buffer Samples() returned %d, want 0", len(samples))
	}
}

func TestInterfaceStoreRecordTraffic(t *testing.T) {
	t.Parallel()

	store := NewInterfaceStore()
	store.RecordTraffic("eth0", 100.0, 50.0)

	samples := store.TrafficSamples("eth0")
	if len(samples) != 1 {
		t.Errorf("Samples() returned %d, want 1", len(samples))
	}
}

func TestFormatInterfaceSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		iface InterfaceInfo
		want  string
	}{
		{
			name:  "connected",
			iface: InterfaceInfo{Name: "eth0", Type: "TCPClient", Status: "connected", Bandwidth: 1000},
			want:  "● eth0 (TCPClient)",
		},
		{
			name:  "disconnected",
			iface: InterfaceInfo{Name: "rnode0", Type: "RNode", Status: "disconnected"},
			want:  "○ rnode0 (RNode)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatInterfaceSummary(tt.iface)
			if !strings.Contains(got, tt.want) {
				t.Errorf("FormatInterfaceSummary() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestFormatInterfaceSummaryTraffic(t *testing.T) {
	t.Parallel()

	iface := InterfaceInfo{
		Name:      "eth0",
		Status:    "connected",
		Traffic:   make([]float64, 10),
		Bandwidth: 1000,
	}

	got := FormatInterfaceSummary(iface)
	if !strings.Contains(got, "[10 samples]") {
		t.Errorf("FormatInterfaceSummary() = %q, want to contain '[10 samples]'", got)
	}
}

func TestFormatInterfaceBandwidth(t *testing.T) {
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
