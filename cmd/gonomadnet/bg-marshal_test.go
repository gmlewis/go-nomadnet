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

package main

import (
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
)

func TestUIChangeCallbackMarshaling(t *testing.T) {
	t.Parallel()

	a := &app.App{}

	var wg sync.WaitGroup
	wg.Add(1)
	called := false

	a.SetUIChangeCallback(func() {
		called = true
		wg.Done()
	})

	// Simulate background goroutine firing callback (e.g. inbound announce or message)
	go func() {
		a.UIChangeCallback()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !called {
			t.Error("UIChangeCallback was not called")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UIChangeCallback")
	}
}
