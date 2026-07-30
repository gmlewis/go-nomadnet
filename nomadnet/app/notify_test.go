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
	"bytes"
	"testing"
)

func TestNotifyMessageReceived(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	var buf bytes.Buffer
	a.notifyWriter = &buf

	a.UIMode = UINone
	a.NotifyMessageReceived()
	if buf.Len() != 0 {
		t.Fatal("no bell expected in non-text mode")
	}

	a.UIMode = UIText
	a.NotifyMessageReceived()
	if got := buf.String(); got != "\a" {
		t.Fatalf("got %q, want bell", got)
	}
}
