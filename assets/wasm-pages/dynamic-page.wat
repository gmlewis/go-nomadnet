;; dynamic-page.wat — a .wasm executable page for the Go NomadNet node and
;; browser (go-nomadnet, built with -tags wago).
;;
;; A .wasm file placed in the node's pages directory (default
;; ~/.nomadnetwork/storage/pages/) is an executable page: requests render it
;; through the sandboxed wasm runtime instead of serving it statically. The
;; plugin receives a JSON request payload (path, request_data, link_id,
;; remote_identity, requested_at) and returns Micron markup bytes.
;;
;; What it does: render_page prepends a canned Micron heading and appends the
;; request JSON, so the rendered page provably depends on the request:
;;   >WASM PAGE
;;   ----
;;   {"path":"/page/dynamic.wasm","requested_at":1730000000}
;;
;; Build:  wat2wasm dynamic-page.wat -o dynamic-page.wasm
;; Install: cp dynamic-page.wasm ~/.nomadnetwork/storage/pages/
;; Use:     browse the node (loopback or remote) and request /page/dynamic.wasm
(module
  (memory (export "memory") 1 1)
  ;; Canned Micron prefix lives at offset 2048.
  (data (i32.const 2048) ">WASM PAGE\n----\n")
  ;; The host asks the plugin to reserve guest memory for the request payload
  ;; (offset 4096 keeps it clear of the response buffer at 1024).
  (func $alloc (param i32) (result i32) i32.const 4096)
  (func $render (param $reqPtr i32) (param $reqLen i32) (result i32 i32)
    ;; Copy the canned prefix to the response buffer at 1024.
    i32.const 1024
    i32.const 2048
    i32.const 16
    memory.copy
    ;; Append the request bytes at 1040.
    i32.const 1040
    local.get $reqPtr
    local.get $reqLen
    memory.copy
    ;; Return (ptr=1024, len=16+reqLen).
    i32.const 1024
    i32.const 16
    local.get $reqLen
    i32.add)
  (func (export "wagoplugin_alloc") (param i32) (result i32)
    local.get 0
    call $alloc)
  (func (export "render_page") (param i32 i32) (result i32 i32)
    local.get 0
    local.get 1
    call $render))
