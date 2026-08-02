// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"runtime"
	"testing"
)

func TestGetPortInfo(t *testing.T) {
	t.Parallel()

	ports := GetPortInfo()
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		// Does not panic and returns a slice (possibly empty depending on dev environment)
		_ = ports
	} else {
		if ports != nil {
			t.Errorf("GetPortInfo() on unsupported OS = %v, want nil", ports)
		}
	}
}
