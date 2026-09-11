;; Copyright 2026 Glenn Lewis. All rights reserved.
;;
;; This program is free software: you can redistribute it and/or modify
;; it under the terms of the GNU General Public License as published by
;; the Free Software Foundation, either version 3 of the License, or
;; (at your option) any later version.
;;
;; This program is distributed in the hope that it will be useful,
;; but WITHOUT ANY WARRANTY; without even the implied warranty of
;; MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
;; GNU General Public License for more details.
;;
;; You should have received a copy of the GNU General Public License
;; along with this program. If not, see <https://www.gnu.org/licenses/>.

;; hit-counter.wat -- a gonomadnet executable page that counts its own visits.
;;
;; WHAT IT DOES
;;   On every render it reads the 4-byte little-endian visit counter from its
;;   plugin scratch store through rns.kv_get, increments it, writes it back
;;   through rns.kv_set, and renders Micron markup carrying the new count in
;;   decimal:
;;
;;     >Hit Counter
;;     Visits: 7
;;     ----
;;
;;   The counter lives on disk, not in the module: gonomadnet compiles, runs,
;;   and releases a fresh sandbox instance for every request, so only the KV
;;   store survives between visits.
;;
;; BUILD
;;   wat2wasm hit-counter.wat -o hit-counter.wasm
;;
;; INSTALL
;;   Copy hit-counter.wasm into ~/.nomadnetwork/storage/pages/ on the node and
;;   add a Micron link to <node-destination-hash>:/page/hit-counter.wasm.
;;   gonomadnet renders .wasm pages only when built with -tags wago.
;;
;; STATE
;;   The visit count is stored under the key "hits" in the page's own scratch
;;   store at ~/.nomadnetwork/storage/pages/data/hit-counter/hits (4 bytes,
;;   little-endian). Deleting that file resets the counter; the store is
;;   capped at 10 MiB per page.
;;
;; SECURITY
;;   The module never emits request data, so a visitor cannot inject markup
;;   through this page. request_data is attacker-controlled in general: other
;;   pages must treat it as untrusted and never re-emit it unescaped.

(module
  ;; rns.kv_get(key_ptr, key_len, out_ptr, out_cap) -> bytes written
  ;;   0 = key absent, -1 = out_cap too small
  (import "rns" "kv_get" (func $kv_get (param i32 i32 i32 i32) (result i32)))
  ;; rns.kv_set(key_ptr, key_len, val_ptr, val_len) -> status (0 = ok)
  (import "rns" "kv_set" (func $kv_set (param i32 i32 i32 i32) (result i32)))

  (memory 1 1)
  (export "memory" (memory 0))

  ;; Key "hits" at 64.
  (data (i32.const 64) "hits")
  ;; Response prefix ">Hit Counter\nVisits: " at 128 (21 bytes).
  (data (i32.const 128) ">Hit Counter\0aVisits: ")
  ;; Response suffix "\n----\n" at 256 (6 bytes).
  (data (i32.const 256) "\0a----\0a")

  ;; The counter's little-endian 4-byte value lives at 512 while the host
  ;; calls kv_get/kv_set; the LSB-first digit scratch buffer starts at 700.
  (func $counter_addr (result i32) i32.const 512)
  (func $digit_scratch (result i32) i32.const 700)

  ;; wagoplugin_alloc hands out the fixed request buffer at 4096.
  (func $alloc (param $len i32) (result i32) i32.const 4096)
  (export "wagoplugin_alloc" (func $alloc))

  ;; render_page(req_ptr, req_len) -> (response_ptr, response_len).
  ;; The request payload is not needed: the visit count is the only input.
  (func $render (param $req_ptr i32) (param $req_len i32) (result i32 i32)
    (local $count i32)   ;; the incremented visit count
    (local $value i32)   ;; remaining value while extracting digits
    (local $digits i32)  ;; number of decimal digits
    (local $i i32)       ;; reversal cursor

    ;; count = le32(store["hits"]) + 1. A missing key leaves the counter
    ;; slot untouched; a fresh guest instance starts zeroed, so absent == 0.
    i32.const 64
    i32.const 4
    call $counter_addr
    i32.const 4
    call $kv_get
    drop
    call $counter_addr
    i32.load
    i32.const 1
    i32.add
    local.set $count

    ;; Persist the new count before rendering.
    call $counter_addr
    local.get $count
    i32.store
    i32.const 64
    i32.const 4
    call $counter_addr
    i32.const 4
    call $kv_set
    drop

    ;; Extract the decimal digits least-significant first into the scratch
    ;; buffer, then reverse them into the response.
    local.get $count
    local.set $value
    call $digit_scratch
    local.set $i
    loop $digits
      local.get $i
      local.get $value
      i32.const 10
      i32.rem_u
      i32.const 48
      i32.add
      i32.store8
      local.get $i
      i32.const 1
      i32.add
      local.set $i
      local.get $value
      i32.const 10
      i32.div_u
      local.set $value
      local.get $value
      br_if $digits
    end
    local.get $i
    call $digit_scratch
    i32.sub
    local.set $digits

    ;; response = prefix + digits (reversed) + suffix, starting at 1024.
    i32.const 1024
    i32.const 128
    i32.const 21
    memory.copy

    i32.const 0
    local.set $i
    block $reversed
      loop $reverse
        local.get $i
        local.get $digits
        i32.ge_u
        br_if $reversed
        i32.const 1024
        i32.const 21
        i32.add
        local.get $i
        i32.add
        call $digit_scratch
        local.get $digits
        i32.const 1
        i32.sub
        local.get $i
        i32.sub
        i32.add
        i32.load8_u
        i32.store8
        local.get $i
        i32.const 1
        i32.add
        local.set $i
        br $reverse
      end
    end

    i32.const 1024
    i32.const 21
    i32.add
    local.get $digits
    i32.add
    i32.const 256
    i32.const 6
    memory.copy

    i32.const 1024
    i32.const 21
    local.get $digits
    i32.add
    i32.const 6
    i32.add)
  (export "render_page" (func $render)))
