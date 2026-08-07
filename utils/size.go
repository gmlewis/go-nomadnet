// Copyright 2026 Glenn Lewis. All rights reserved.

package utils

import (
	"strconv"
	"strings"
)

// ParseSize parses "WxH" (e.g. "135x32") or stty's "H W" ("rows cols") into a
// width and height. ok=false if the string is not a recognizable size.
func ParseSize(s string) (w, h int, ok bool) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "x") {
		parts := strings.SplitN(s, "x", 2)
		if len(parts) != 2 {
			return 0, 0, false
		}
		ww, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		hh, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || ww <= 0 || hh <= 0 {
			return 0, 0, false
		}
		return ww, hh, true
	}
	// stty size: "rows cols".
	parts := strings.Fields(s)
	if len(parts) == 2 {
		hh, err1 := strconv.Atoi(parts[0])
		ww, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || ww <= 0 || hh <= 0 {
			return 0, 0, false
		}
		return ww, hh, true
	}
	return 0, 0, false
}
