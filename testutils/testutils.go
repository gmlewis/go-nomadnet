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

// Package testutils provides shared helper functions for tests across this
// repository. It mirrors the testutils package in go-reticulum so the two
// repositories share a common convention for guarding slow tests.
package testutils

import "testing"

// SkipShortIntegration skips a test when -short is in effect. The -short
// integration run (scripts/test-integration.sh -short) is a fast feedback loop
// where every test should run in well under 5 seconds by definition; any test
// that cannot meet that budget (a full-package type check, a live network
// round-trip, a cross-process tmux harness, ...) calls this at its top so the
// short run stays quick while the full unit and full integration runs still
// execute the test.
func SkipShortIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
}
