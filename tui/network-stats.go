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
	"strconv"
	"time"
)

// AnnounceTimeLabel builds the "Announced : <when>" status line for the local
// peer, matching Python's AnnounceTime.update_time (Network.py:1036). A nil
// lastAnnounce yields "Never"; otherwise the timestamp is rendered with
// prettyDateAt against the supplied now (Python's pretty_date uses datetime.now).
func AnnounceTimeLabel(lastAnnounce *int64, now time.Time) string {
	s := "Never"
	if lastAnnounce != nil {
		s = prettyDateAt(time.Unix(*lastAnnounce, 0), now)
	}
	return "Announced : " + s
}

// NodeAnnounceTimeLabel builds the "Last Announce  : <when>" status line for
// the local node, matching Python's NodeAnnounceTime.update_time
// (Network.py:1068).
func NodeAnnounceTimeLabel(nodeLastAnnounce *int64, now time.Time) string {
	s := "Never"
	if nodeLastAnnounce != nil {
		s = prettyDateAt(time.Unix(*nodeLastAnnounce, 0), now)
	}
	return "Last Announce  : " + s
}

// NodeActiveConnectionsLabel builds the "Connected Now  : <n>" status line,
// matching Python's NodeActiveConnections.update_stat (Network.py:1099). With
// no node the value is "None".
func NodeActiveConnectionsLabel(linkCount int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(linkCount)
	}
	return "Connected Now  : " + s
}

// NodeStorageStatsLabel builds the "LXMF Storage   : <usage>" status line,
// matching Python's NodeStorageStats.update_stat (Network.py:1130). When the
// node is absent or propagation is disabled the value is "None". With a known
// limit it reports "pct%, used of limit"; with a nil limit it reports just the
// used size. pct uses Python's round((used/limit)*100, 1) which matches Go's
// %.1f banker's rounding.
func NodeStorageStatsLabel(used, limit *int64, hasNode, propagationEnabled bool) string {
	s := "None"
	if hasNode && propagationEnabled {
		pctStr := ""
		limitStr := ""
		if limit != nil && used != nil {
			pct := (float64(*used) / float64(*limit)) * 100
			pctStr = fmt.Sprintf("%.1f%%, ", pct)
			limitStr = " of " + Prettysize(float64(*limit))
		}
		usedStr := ""
		if used != nil {
			usedStr = Prettysize(float64(*used))
		}
		s = pctStr + usedStr + limitStr
	}
	return "LXMF Storage   : " + s
}

// NodeTotalConnectionsLabel builds the "Total Connects : <n>" status line,
// matching Python's NodeTotalConnections.update_stat (Network.py:1173).
func NodeTotalConnectionsLabel(connects int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(connects)
	}
	return "Total Connects : " + s
}

// NodeTotalPagesLabel builds the "Served Pages   : <n>" status line, matching
// Python's NodeTotalPages.update_stat (Network.py:1205).
func NodeTotalPagesLabel(pages int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(pages)
	}
	return "Served Pages   : " + s
}

// NodeTotalFilesLabel builds the "Served Files   : <n>" status line, matching
// Python's NodeTotalFiles.update_stat (Network.py:1237).
func NodeTotalFilesLabel(files int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(files)
	}
	return "Served Files   : " + s
}
