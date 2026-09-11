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

// Package wasmpages renders .wasm executable pages through sandboxed in-process
// wasm plugins (the wago runtime), mirroring Python NomadNet's executable-page
// behavior (Node.py:161-175 and Browser.py:1306-1316 run an executable page as
// a subprocess with request metadata as environment variables; the Go sandbox
// replaces that subprocess with the deny-by-default wasm ABI
// render_page(req_ptr, req_len) -> (resp_ptr, resp_len)).
//
// The request payload is a JSON object (PageRequest) and the response is Micron
// markup bytes. Each render compiles, runs, and releases its own plugin
// instance, so on-disk page edits take effect on the next request and no state
// survives between requests. Built without -tags wago the package compiles as
// a stub (Enabled reports false, every Render fails with ErrWagoNotLinked) and
// callers keep serving .wasm files statically.
//
// The runtime is linked only on Linux, Darwin, and Windows on amd64/arm64;
// every other platform keeps the stub.
package wasmpages

import "encoding/json"

// PageRequest is the JSON payload passed to a .wasm page plugin's
// render_page export. It carries only owned leaf data: the request path, the
// form request data (Python's field_*/var_* request_data map), and hex link /
// remote identity metadata.
type PageRequest struct {
	// Path is the full request path (e.g. "/page/dynamic.wasm").
	Path string `json:"path"`
	// RequestData carries the request's field_*/var_* entries (nil when the
	// request carried none).
	RequestData map[string]string `json:"request_data,omitempty"`
	// LinkID is the hex-encoded RNS link ID, empty when not linked.
	LinkID string `json:"link_id,omitempty"`
	// RemoteIdentity is the hex-encoded remote identity hash, empty when the
	// request came from an unidentified peer.
	RemoteIdentity string `json:"remote_identity,omitempty"`
	// RequestedAt is the request time in Unix seconds.
	RequestedAt int64 `json:"requested_at"`
}

// payload encodes the request the way the plugin host passes it to the guest.
func (req PageRequest) payload() []byte {
	data, err := json.Marshal(req)
	if err != nil {
		return []byte("{}")
	}
	return data
}
