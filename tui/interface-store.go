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
	"sort"
	"strings"
	"sync"
	"time"
)

// InterfaceStore manages the collection of network interfaces.
// Thread-safe for concurrent access.
type InterfaceStore struct {
	mu            sync.RWMutex
	interfaceList []InterfaceInfo
	trafficHist   map[string]*TrafficRingBuffer
}

// TrafficRingBuffer stores fixed-size traffic history.
type TrafficRingBuffer struct {
	data     []float64
	capacity int
	head     int
	count    int
	mu       sync.Mutex
}

// NewTrafficRingBuffer creates a ring buffer with the given capacity.
func NewTrafficRingBuffer(capacity int) *TrafficRingBuffer {
	return &TrafficRingBuffer{
		data:     make([]float64, capacity),
		capacity: capacity,
	}
}

// Push adds a sample to the ring buffer.
func (rb *TrafficRingBuffer) Push(value float64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = value
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
}

// Samples returns the traffic samples in chronological order.
func (rb *TrafficRingBuffer) Samples() []float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	result := make([]float64, rb.count)
	for i := 0; i < rb.count; i++ {
		result[i] = rb.data[(rb.head-rb.count+i+rb.capacity)%rb.capacity]
	}
	return result
}

// NewInterfaceStore creates an empty interface store.
func NewInterfaceStore() *InterfaceStore {
	return &InterfaceStore{
		interfaceList: make([]InterfaceInfo, 0),
		trafficHist:   make(map[string]*TrafficRingBuffer),
	}
}

// Add adds or updates an interface.
func (is *InterfaceStore) Add(info InterfaceInfo) {
	is.mu.Lock()
	defer is.mu.Unlock()

	for i, existing := range is.interfaceList {
		if existing.Name == info.Name {
			is.interfaceList[i] = info
			return
		}
	}
	is.interfaceList = append(is.interfaceList, info)
}

// Get returns the interface info for the given name, or nil.
func (is *InterfaceStore) Get(name string) *InterfaceInfo {
	is.mu.RLock()
	defer is.mu.RUnlock()

	for i := range is.interfaceList {
		if is.interfaceList[i].Name == name {
			return &is.interfaceList[i]
		}
	}
	return nil
}

// Delete removes an interface by name.
func (is *InterfaceStore) Delete(name string) {
	is.mu.Lock()
	defer is.mu.Unlock()

	for i, info := range is.interfaceList {
		if info.Name == name {
			is.interfaceList = append(is.interfaceList[:i], is.interfaceList[i+1:]...)
			delete(is.trafficHist, name)
			return
		}
	}
}

// List returns all interfaces in sorted order (by name).
func (is *InterfaceStore) List() []InterfaceInfo {
	is.mu.RLock()
	defer is.mu.RUnlock()

	out := make([]InterfaceInfo, len(is.interfaceList))
	copy(out, is.interfaceList)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// ConnectedCount returns the number of connected interfaces.
func (is *InterfaceStore) ConnectedCount() int {
	is.mu.RLock()
	defer is.mu.RUnlock()
	n := 0
	for _, iface := range is.interfaceList {
		if iface.Status == "connected" {
			n++
		}
	}
	return n
}

// FilterByType returns interfaces matching the given type.
func (is *InterfaceStore) FilterByType(ifType string) []InterfaceInfo {
	is.mu.RLock()
	defer is.mu.RUnlock()

	var result []InterfaceInfo
	for _, iface := range is.interfaceList {
		if iface.Type == ifType {
			result = append(result, iface)
		}
	}
	return result
}

// SearchByName returns interfaces whose name contains the query.
func (is *InterfaceStore) SearchByName(query string) []InterfaceInfo {
	all := is.List()
	if query == "" {
		return all
	}
	q := strings.ToLower(query)
	var result []InterfaceInfo
	for _, iface := range all {
		if strings.Contains(strings.ToLower(iface.Name), q) {
			result = append(result, iface)
		}
	}
	return result
}

// RecordTraffic records a new traffic sample for the given interface.
func (is *InterfaceStore) RecordTraffic(name string, rxRate, txRate float64) {
	is.mu.Lock()
	defer is.mu.Unlock()

	rb, ok := is.trafficHist[name]
	if !ok {
		rb = NewTrafficRingBuffer(60)
		is.trafficHist[name] = rb
	}

	now := time.Now()
	_ = now // timestamp available if needed
	_ = rxRate
	_ = txRate

	// Push combined rate for chart display
	rb.Push(rxRate + txRate)
}

// TrafficSamples returns the traffic history for an interface.
func (is *InterfaceStore) TrafficSamples(name string) []float64 {
	is.mu.RLock()
	defer is.mu.RUnlock()

	rb, ok := is.trafficHist[name]
	if !ok {
		return nil
	}
	return rb.Samples()
}

// FormatInterfaceSummary formats a single interface for display.
func FormatInterfaceSummary(iface InterfaceInfo) string {
	statusIcon := "○"
	if iface.Status == "connected" {
		statusIcon = "●"
	}

	trafficInfo := ""
	if len(iface.Traffic) > 0 {
		trafficInfo = fmt.Sprintf(" [%d samples]", len(iface.Traffic))
	}

	return fmt.Sprintf("%s %s (%s) — %s — %s%s",
		statusIcon, iface.Name, iface.Type, iface.Status,
		formatBandwidth(iface.Bandwidth), trafficInfo)
}
