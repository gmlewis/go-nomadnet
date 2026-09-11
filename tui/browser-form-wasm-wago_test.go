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

//go:build wago && (linux || darwin || windows) && (amd64 || arm64)

package tui

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-nomadnet/nomadnet/wasmpages"
	"github.com/gmlewis/go-reticulum/testutils"
)

// formSubmitPage is a Micron page whose form submits to a .wasm executable
// page: a text field named "name", a checkbox whose value is the "topic"
// field's value supplied by the submit link, and a submit link that names the
// collected fields and carries a var_* entry of its own.
//
//	>Guestbook                            (line 0: heading)
//	Name: `<name`glenn>                   (line 1: text field "name")
//	`[Sign`/page/guestbook.wasm`name|topic=wasm]
//	                                      (line 2: submit link, Fields)
const formSubmitPage = ">Guestbook\nName: `<name`glenn>\n`[Sign`/page/guestbook.wasm`name|topic=wasm]"

// wasmFormPageWasm mirrors the wasmpages package's dynamic page fixture: it
// prepends a canned Micron prefix and echoes the serialized request payload.
var wasmFormPageWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x0d, 0x02,
	0x60, 0x01, 0x7f, 0x01, 0x7f,
	0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f,
	0x03, 0x03, 0x02, 0x00, 0x01,
	0x05, 0x04, 0x01, 0x01, 0x01, 0x01,
	0x07, 0x2b, 0x03,
	0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
	0x10, 'w', 'a', 'g', 'o', 'p', 'l', 'u', 'g', 'i', 'n', '_', 'a', 'l', 'l', 'o', 'c', 0x00, 0x00,
	0x0b, 'r', 'e', 'n', 'd', 'e', 'r', '_', 'p', 'a', 'g', 'e', 0x00, 0x01,
	0x0a, 0x29, 0x02,
	0x05, 0x00, 0x41, 0x80, 0x20, 0x0b,
	0x21, 0x00, 0x41, 0x80, 0x08, 0x41, 0x80, 0x10, 0x41, 0x10, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x90, 0x08, 0x20, 0x00, 0x20, 0x01, 0xfc, 0x0a, 0x00, 0x00, 0x41, 0x80, 0x08, 0x41, 0x10, 0x20, 0x01, 0x6a, 0x0b,
	0x0b, 0x17, 0x01, 0x00, 0x41, 0x80, 0x10, 0x0b, 0x10,
	'>', 'W', 'A', 'S', 'M', ' ', 'P', 'A', 'G', 'E', '\n', '-', '-', '-', '-', '\n',
}

// TestBrowserFormSubmitToWasmPageLoopback pins the whole form-submit flow for a
// loopback .wasm page: a Micron form's field values (collectFields) plus the
// submit link's var_* suffix (ParseURL) reach the rendered executable page as
// its request_data map, exactly as they would over a remote link.
func TestBrowserFormSubmitToWasmPageLoopback(t *testing.T) {
	t.Parallel()

	pages := testutils.TempDir(t, "nomadnet-tui-form-wasm")
	wasmPath := filepath.Join(pages, "guestbook.wasm")
	if err := os.WriteFile(wasmPath, wasmFormPageWasm, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, bd := newFieldTestBrowser(t, formSubmitPage)

	// Locate the form's submit link and read the fields it names.
	var linkFields string
	for _, n := range micron.Parse(formSubmitPage) {
		if n.Type == micron.NodeLink && strings.HasSuffix(n.LinkURL, "/page/guestbook.wasm") {
			linkFields = n.LinkFields
		}
	}
	if linkFields != "name|topic=wasm" {
		t.Fatalf("submit link fields = %q, want %q", linkFields, "name|topic=wasm")
	}

	// Type the user's name into the field, as the browser overlay does.
	fieldLine := findLine(bd, "Name:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	bd.lineFields[fieldLine][0].editor.SetText("glenn")

	requestData := bd.collectFields(linkFields)
	if requestData["field_name"] != "glenn" {
		t.Fatalf("collected request data = %v, want field_name=glenn", requestData)
	}
	if requestData["var_topic"] != "wasm" {
		t.Fatalf("collected request data = %v, want var_topic=wasm from the link's k=v entry", requestData)
	}

	destHash := make([]byte, 16)
	for i := range destHash {
		destHash[i] = byte(0x11)
	}
	target := hex.EncodeToString(destHash) + ":/page/guestbook.wasm"
	gotDest, path, merged, err := browser.ParseURL(target, nil, requestData)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	if !bytes.Equal(gotDest, destHash) {
		t.Errorf("parsed destination = %x, want %x", gotDest, destHash)
	}
	if path != "/page/guestbook.wasm" {
		t.Errorf("parsed path = %q, want /page/guestbook.wasm", path)
	}
	if merged["var_topic"] != "wasm" || merged["field_name"] != "glenn" {
		t.Fatalf("merged request data = %v, want the submitted field and var_ entry", merged)
	}

	identityHash := make([]byte, 32)
	for i := range identityHash {
		identityHash[i] = byte(0x24)
	}
	rendered := browser.ServeLocalPageWithCaller(pages, path, identityHash, merged)
	if !strings.HasPrefix(string(rendered), ">WASM PAGE\n----\n") {
		t.Fatalf("rendered page = %q, want the canned prefix", rendered)
	}
	var req wasmpages.PageRequest
	if err := json.Unmarshal(rendered[len(">WASM PAGE\n----\n"):], &req); err != nil {
		t.Fatalf("rendered suffix does not decode as the request JSON: %v (%q)", err, rendered)
	}
	if got := req.RequestData; len(got) != 2 || got["field_name"] != "glenn" || got["var_topic"] != "wasm" {
		t.Errorf("rendered request_data = %v, want the submitted field and var_ entry", got)
	}
	if req.LinkID != "loopback" {
		t.Errorf("link_id = %q, want loopback", req.LinkID)
	}
}

// TestBrowserFormSubmitPlainLinkNoRequestData pins that a link naming no
// fields collects nil request data, so a plain navigation keeps the page-cache
// path and sends no request_data at all.
func TestBrowserFormSubmitPlainLinkNoRequestData(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, formSubmitPage)
	if rd := bd.collectFields(""); rd != nil {
		t.Errorf("collectFields on a plain link = %v, want nil", rd)
	}
}
