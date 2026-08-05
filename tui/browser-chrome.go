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

	"github.com/gdamore/tcell/v2"
)

// browserStatus represents the browser request lifecycle, mirroring the
// Browser.* status constants (Browser.py:80-101). It drives the footer status
// text via browserStatusText.
type browserStatus int

const (
	browserDisconnected browserStatus = iota
	browserNoPath
	browserPathRequested
	browserEstablishingLink
	browserLinkTimeout
	browserLinkEstablished
	browserRequesting
	browserRequestSent
	browserRequestFailed
	browserRequestTimeout
	browserReceivingResponse
	browserDone
)

// sizeStr formats a byte count using base-1000 units with no space between the
// number and the unit, matching Python Browser.size_str (Browser.py:1817-1834).
// Plain bytes print with 0 decimals; K/M/G/... values print with 2 decimals.
// With suffix "b" the value is converted to bits (×8) first, for the transfer
// speed figure. Values that exceed the Z prefix roll over to "Y".
//
// Distinct from Prettysize (which inserts a space and is used elsewhere for
// RNS.prettysize): the browser chrome uses size_str's space-less form
// ("815B", "8.48Kb/s").
func sizeStr(num float64, suffix string) string {
	units := []string{"", "K", "M", "G", "T", "P", "E", "Z"}
	const lastUnit = "Y"
	if suffix == "b" {
		num *= 8
	}
	for _, unit := range units {
		if absFloat(num) < 1000.0 {
			if unit == "" {
				return fmt.Sprintf("%.0f%s%s", num, unit, suffix)
			}
			return fmt.Sprintf("%.2f%s%s", num, unit, suffix)
		}
		num /= 1000.0
	}
	return fmt.Sprintf("%.2f%s%s", num, lastUnit, suffix)
}

// browserStatusText renders the footer status line for a given lifecycle state,
// matching Python Browser.status_text (Browser.py:1756-1803). For browserDone
// with a transfer size it returns "Done" plus the stats string
// ("Done  ▤ <size>   ↓<size> in <t>s   ◷ <speed>b/s"); for a cached load it
// returns " (cached)"; the in-flight states return their human-readable label.
// savedFileName non-empty switches the "Done" prefix to "Saved <name>".
func browserStatusText(g map[string]string, status browserStatus,
	responseSize, transferSize int64, responseTime float64,
	hasTransfer bool, loadedFromCache bool, savedFileName string) string {
	statsString := ""
	if status == browserDone && hasTransfer {
		timeStr := "None"
		if responseTime > 0 {
			timeStr = fmt.Sprintf("%.2f", responseTime)
		}
		respSize := responseSize
		// if savedFileName != "" {
		//   resp_size = saved_file_size; caller passes responseSize already
		//   adjusted when a file was saved (Python uses saved_file_size).
		// }
		if respSize > 0 {
			statsString = "  " + glyph(g, "page") + sizeStr(float64(respSize), "B")
		}
		if transferSize > 0 {
			statsString += "   " + glyph(g, "arrow_d") + sizeStr(float64(transferSize), "B") + " in " + timeStr
		}
		if transferSize > 0 && responseTime > 0 {
			statsString += "s   " + glyph(g, "speed") + sizeStr(float64(transferSize)/responseTime, "b") + "/s"
		}
	} else if loadedFromCache {
		statsString = " (cached)"
	}

	switch status {
	case browserNoPath:
		return "No path to destination known"
	case browserPathRequested:
		return "Path requested, waiting for path..."
	case browserEstablishingLink:
		return "Establishing link..."
	case browserLinkTimeout:
		return "Link establishment timed out"
	case browserLinkEstablished:
		return "Link established"
	case browserRequesting:
		return "Sending request..."
	case browserRequestSent:
		return "Request sent, awaiting response..."
	case browserRequestFailed:
		return "Request failed"
	case browserRequestTimeout:
		return "Request timed out"
	case browserReceivingResponse:
		return "Receiving response..."
	case browserDone:
		if savedFileName == "" {
			return "Done" + statsString
		}
		return "Saved " + savedFileName + statsString
	case browserDisconnected:
		return "Disconnected"
	default:
		return "Browser Status Unknown"
	}
}

// glyph returns g[key] (falling back to the plain/unicode default) so chrome
// helpers tolerate a nil glyph set (unit tests use newTestApp which may have no
// Glyphs).
func glyph(g map[string]string, key string) string {
	if g == nil {
		return ""
	}
	return g[key]
}

// browserControlsColor is the header/footer chrome text color (Python
// "browser_controls" style: #bbb dark / #444 light), falling back to #bbb when
// the theme lookup yields ColorDefault (e.g. a test app with no palette).
func browserControlsColor(app *App) tcell.Color {
	if app != nil {
		if c := GetThemeColors(app.Theme)["browser_controls"]; c != tcell.ColorDefault {
			return c
		}
	}
	return tcell.NewHexColor(0xbbbbbb)
}
