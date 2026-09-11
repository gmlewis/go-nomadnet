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

//go:build !wago || (!linux && !darwin && !windows) || (!amd64 && !arm64)

// This file is the stub .wasm page renderer, compiled whenever the wago
// runtime is not linked into the build (no -tags wago, or an unsupported
// platform). Node and browser callers treat the stub as "not available" and
// keep serving .wasm page files statically, which was the behavior before
// executable pages landed.

package wasmpages

import "errors"

// ErrWagoNotLinked reports render requests on a binary built without the
// wago runtime.
var ErrWagoNotLinked = errors.New("wasm page rendering not compiled in (build with -tags wago)")

// Enabled reports whether .wasm pages render through the sandbox; the stub
// build always reports false.
func Enabled() bool { return false }

// Render always fails in the stub build.
func Render(_ string, _ PageRequest) ([]byte, error) {
	return nil, ErrWagoNotLinked
}
