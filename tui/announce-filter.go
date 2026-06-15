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

import "strings"

// AnnounceFilter provides filtering and searching over announce entries.
// Supports filtering by type, trust level, and text search across
// name/hash fields. Matches Python's NetworkDisplay filter logic.
type AnnounceFilter struct {
	entries      []AnnounceEntry
	typeFilter   string
	trustFilter  string
	searchFilter string
}

// NewAnnounceFilter creates a filter over the given announce entries.
func NewAnnounceFilter(entries []AnnounceEntry) *AnnounceFilter {
	af := &AnnounceFilter{}
	if entries != nil {
		af.entries = make([]AnnounceEntry, len(entries))
		copy(af.entries, entries)
	}
	return af
}

// SetTypeFilter filters entries by announce type ("node", "peer", "pn").
// Empty string clears the filter.
func (af *AnnounceFilter) SetTypeFilter(typ string) {
	af.typeFilter = strings.ToLower(typ)
}

// SetTrustFilter filters entries by trust level.
// Empty string clears the filter.
func (af *AnnounceFilter) SetTrustFilter(level string) {
	af.trustFilter = strings.ToLower(level)
}

// SetSearch sets a text search filter matched against display name
// and source hash. Empty string clears the filter.
func (af *AnnounceFilter) SetSearch(text string) {
	af.searchFilter = strings.ToLower(text)
}

// ClearFilters removes all active filters.
func (af *AnnounceFilter) ClearFilters() {
	af.typeFilter = ""
	af.trustFilter = ""
	af.searchFilter = ""
}

// UpdateEntries replaces the underlying entry list.
func (af *AnnounceFilter) UpdateEntries(entries []AnnounceEntry) {
	af.entries = make([]AnnounceEntry, len(entries))
	copy(af.entries, entries)
}

// Count returns the number of entries in the filter (before filtering).
func (af *AnnounceFilter) Count() int {
	return len(af.entries)
}

// Filtered returns entries matching all active filters.
func (af *AnnounceFilter) Filtered() []AnnounceEntry {
	if len(af.entries) == 0 {
		return nil
	}

	var result []AnnounceEntry
	for _, e := range af.entries {
		if af.typeFilter != "" && !strings.EqualFold(e.Type, af.typeFilter) {
			continue
		}
		if af.trustFilter != "" && !strings.EqualFold(e.TrustLevel, af.trustFilter) {
			continue
		}
		if af.searchFilter != "" {
			nameMatch := strings.Contains(strings.ToLower(e.DisplayName), af.searchFilter)
			hashMatch := strings.Contains(strings.ToLower(e.SourceHash), af.searchFilter)
			if !nameMatch && !hashMatch {
				continue
			}
		}
		result = append(result, e)
	}
	return result
}
