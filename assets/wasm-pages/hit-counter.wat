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

;; hit-counter.wat -- a gonomadnet executable page that counts page visits.
;;
;; WHAT IT DOES
;;   On every render it reads a 4-byte little-endian visit counter from its
;;   plugin scratch store through rns.kv_get, increments it, writes it back
;;   through rns.kv_set, and renders one line of Micron markup carrying the new
;;   count in decimal:
;;
;;     You are visitor 7 to this page.
;;
;;   The counter lives on disk, not in the module: gonomadnet compiles, runs,
;;   and releases a fresh sandbox instance for every request, so only the KV
;;   store survives between visits.
;;
;; ONE COUNTER PER PAGE
;;   A page embeds this module as a Micron partial and names itself in the
;;   partial's fields, so the module counts that page and no other:
;;
;;     `{<node-hash>:/page/hit-counter.wasm`0`page=index.mu}
;;
;;   gonomadnet sends a partial's "k=v" field as request_data var_k=v, so the
;;   module sees var_page=index.mu and stores its count under the key
;;   "index.mu". Each page that embeds the partial with its own name therefore
;;   gets its own independent counter, like a 1990s page counter. The refresh
;;   slot "0" means "fetch once": the count increments when the page is opened,
;;   never on a timer.
;;
;;   With no var_page — a partial that names no page, or a visitor opening the
;;   module directly — the count falls back to the module's own default key
;;   "hits" and counts the whole site, and the rendered line says so:
;;
;;     You are visitor 7 to this site.
;;
;; QUIET MODE
;;   A partial carrying a "quiet" field with any value counts its visit and
;;   renders nothing, so a page can trigger a counter that it does not show:
;;
;;     `{<node-hash>:/page/hit-counter.wasm`0`quiet=1}
;;
;;   With no "page" field a quiet partial still counts the whole site, which is
;;   how a page bumps the node's visitor total without printing it among its own
;;   text. A quiet partial that names a page counts only that page, still
;;   silently. An empty "quiet" value is not a quiet partial: like "page", the
;;   field only counts when it carries a value.
;;
;; BUILD
;;   wat2wasm hit-counter.wat -o hit-counter.wasm
;;
;; INSTALL
;;   Copy hit-counter.wasm into ~/.nomadnetwork/storage/pages/ on the node.
;;   gonomadnet renders .wasm pages only when built with -tags wago.
;;
;; STATE
;;   Each counter is a 4-byte little-endian file in the page's own scratch
;;   store under ~/.nomadnetwork/storage/pages/data/hit-counter/. The key is the
;;   counted page's name ("index.mu"), or "hits" when the partial names no page.
;;   Deleting a key's file resets that counter; the store is capped at 10 MiB.
;;
;; SECURITY
;;   request_data is attacker-controlled, so var_page is used only as a store
;;   key: it is length-capped, may not contain a path separator or a Micron
;;   metacharacter, and the module never re-emits it. The rendered count is
;;   generated digits, never input.

(module
  ;; rns.kv_get(key_ptr, key_len, out_ptr, out_cap) -> bytes written
  ;;   0 = key absent, -1 = out_cap too small
  (import "rns" "kv_get" (func $kv_get (param i32 i32 i32 i32) (result i32)))
  ;; rns.kv_set(key_ptr, key_len, val_ptr, val_len) -> status (0 = ok)
  (import "rns" "kv_set" (func $kv_set (param i32 i32 i32 i32) (result i32)))

  (memory 1 1)
  (export "memory" (memory 0))

  ;; Default key "hits" at 64 (4 bytes).
  (data (i32.const 64) "hits")
  ;; The request-data key the browser sends for a partial's "page=" entry:
  ;; "var_page":" is 12 bytes.
  (data (i32.const 80) "\22var_page\22:\22")
  ;; The request-data key a partial sends for a "quiet=" entry, which counts the
  ;; visit without rendering the count: "var_quiet":" is 13 bytes.
  (data (i32.const 208) "\22var_quiet\22:\22")
  ;; Response prefix "You are visitor " at 96 (16 bytes).
  (data (i32.const 96) "You are visitor ")
  ;; Response suffix " to this page.\n" at 128 (15 bytes) for a page counter,
  ;; and " to this site.\n" at 192 (15 bytes) for the keyless site-wide one.
  (data (i32.const 128) " to this page.\0a")
  (data (i32.const 192) " to this site.\0a")

  (func $counter_addr (result i32) i32.const 512)
  (func $digit_scratch (result i32) i32.const 700)
  (func $key_addr (result i32) i32.const 256)
  (func $scan_addr (result i32) i32.const 320)
  (func $default_key (result i32) i32.const 64)
  (func $var_page_key (result i32) i32.const 80)
  (func $var_quiet_key (result i32) i32.const 208)
  (func $response_addr (result i32) i32.const 1024)
  (func $page_suffix (result i32) i32.const 128)
  (func $site_suffix (result i32) i32.const 192)
  (func $suffix_len (result i32) i32.const 15)
  ;; The prefix length, so the digit and suffix offsets cannot drift
  ;; from the data segment above.
  (func $prefix_len (result i32) i32.const 16)

  ;; $key_at(pos, key, key_len) -> 1 when [pos, pos+key_len) equals key.
  (func $key_at (param $pos i32) (param $key i32) (param $key_len i32) (result i32)
    (local $k i32)
    (local $res i32)
    i32.const 1
    local.set $res
    i32.const 0
    local.set $k
    block $done
      loop $cmp
        local.get $k
        local.get $key_len
        i32.ge_u
        br_if $done
        local.get $pos
        local.get $k
        i32.add
        i32.load8_u
        local.get $key
        local.get $k
        i32.add
        i32.load8_u
        i32.ne
        if
          i32.const 0
          local.set $res
          br $done
        end
        local.get $k
        i32.const 1
        i32.add
        local.set $k
        br $cmp
      end
    end
    local.get $res)

  ;; $scan(start, len, key, key_len, out, cap) -> bytes copied, or -1.
  ;; Finds the first key in [start, start+len), then copies the JSON string
  ;; value that follows it into out, resolving the escapes "\"", "\\" and "\/"
  ;; and rejecting any other escape. Returns -1 when the key is absent, the
  ;; value exceeds cap, or the value carries a byte unfit to be a store key: a
  ;; control byte, a Micron backtick or bracket, or a path separator.
  (func $scan (param $start i32) (param $len i32) (param $key i32) (param $key_len i32)
              (param $out i32) (param $cap i32) (result i32)
    (local $i i32)      ;; candidate start offset
    (local $limit i32)  ;; last offset at which the key can still fit
    (local $p i32)      ;; read cursor inside the value
    (local $n i32)      ;; bytes copied so far
    (local $c i32)      ;; current byte
    (local $res i32)
    i32.const -1
    local.set $res
    local.get $len
    local.get $key_len
    i32.sub
    local.set $limit
    i32.const 0
    local.set $i
    block $done
      loop $next
        local.get $i
        local.get $limit
        i32.gt_s
        br_if $done
        local.get $start
        local.get $i
        i32.add
        local.get $key
        local.get $key_len
        call $key_at
        if
          local.get $start
          local.get $i
          i32.add
          local.get $key_len
          i32.add
          local.set $p
          i32.const 0
          local.set $n
          block $value_done
            loop $chars
              local.get $p
              i32.load8_u
              local.set $c
              ;; A closing quote ends the value.
              local.get $c
              i32.const 0x22
              i32.eq
              if
                local.get $n
                local.set $res
                br $value_done
              end
              ;; A backslash escape is kept only when the escaped character
              ;; stands for itself ("\"", "\\", "\/").
              local.get $c
              i32.const 0x5c
              i32.eq
              if
                local.get $p
                i32.const 1
                i32.add
                local.set $p
                local.get $p
                i32.load8_u
                local.set $c
                local.get $c
                i32.const 0x22
                i32.eq
                local.get $c
                i32.const 0x5c
                i32.eq
                i32.or
                local.get $c
                i32.const 0x2f
                i32.eq
                i32.or
                i32.eqz
                if
                  br $done
                end
              end
              ;; Reject Micron markup injection, control bytes, and the path
              ;; separators a store key may not contain.
              local.get $c
              i32.const 0x60
              i32.eq
              local.get $c
              i32.const 0x5b
              i32.eq
              i32.or
              local.get $c
              i32.const 0x20
              i32.lt_u
              i32.or
              local.get $c
              i32.const 0x2f
              i32.eq
              i32.or
              local.get $c
              i32.const 0x5c
              i32.eq
              i32.or
              if
                br $done
              end
              ;; Refuse to overrun the caller's buffer.
              local.get $n
              local.get $cap
              i32.ge_u
              if
                br $done
              end
              local.get $out
              local.get $n
              i32.add
              local.get $c
              i32.store8
              local.get $n
              i32.const 1
              i32.add
              local.set $n
              local.get $p
              i32.const 1
              i32.add
              local.set $p
              br $chars
            end
          end
          br $done
        end
        local.get $i
        i32.const 1
        i32.add
        local.set $i
        br $next
      end
    end
    local.get $res)

  ;; wagoplugin_alloc hands out the fixed request buffer at 4096.
  (func $alloc (param $len i32) (result i32) i32.const 4096)
  (export "wagoplugin_alloc" (func $alloc))

  ;; render_page(req_ptr, req_len) -> (response_ptr, response_len).
  (func $render (param $req_ptr i32) (param $req_len i32) (result i32 i32)
    (local $n i32)       ;; bytes scanned from var_page, or -1
    (local $key_len i32) ;; the effective key length
    (local $count i32)   ;; the incremented visit count
    (local $value i32)   ;; remaining value while extracting digits
    (local $digits i32)  ;; number of decimal digits
    (local $i i32)       ;; digit cursor
    (local $suffix i32)  ;; the wording for whichever counter is being served

    ;; The key defaults to "hits" and is replaced by the counted page's name
    ;; when the embedding partial named one. With no page named the count is
    ;; the site's total, so the wording follows the mode too: a partial that
    ;; names no page demonstrates the module's own default key.
    call $key_addr
    call $default_key
    i32.const 4
    memory.copy
    i32.const 4
    local.set $key_len
    call $site_suffix
    local.set $suffix

    local.get $req_ptr
    local.get $req_len
    call $var_page_key
    i32.const 12
    call $scan_addr
    i32.const 64
    call $scan
    local.set $n

    block $keep_default
      local.get $n
      i32.const 0
      i32.le_s
      br_if $keep_default
      call $key_addr
      call $scan_addr
      local.get $n
      memory.copy
      local.get $n
      local.set $key_len
      call $page_suffix
      local.set $suffix
    end

    ;; count = le32(store[key]) + 1. A missing key leaves the counter slot
    ;; untouched; a fresh guest instance starts zeroed, so absent == 0.
    call $key_addr
    local.get $key_len
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
    call $key_addr
    local.get $key_len
    call $counter_addr
    i32.const 4
    call $kv_set
    drop

    ;; A quiet caller counts its visit but renders nothing, which is how a page
    ;; triggers a counter it does not display: the partial substitutes to no
    ;; text at the place the page embedded it. The flag is read after the count
    ;; is persisted, so a quiet visit counts exactly like a visible one, and the
    ;; value is capped at one byte because only "some value" matters here.
    local.get $req_ptr
    local.get $req_len
    call $var_quiet_key
    i32.const 13
    call $scan_addr
    i32.const 1
    call $scan
    i32.const 0
    i32.gt_s
    if
      call $response_addr
      i32.const 0
      return
    end

    ;; Extract the decimal digits least-significant first into the scratch
    ;; buffer, then reverse them into the response.
    local.get $count
    local.set $value
    call $digit_scratch
    local.set $i
    loop $digits_loop
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
      br_if $digits_loop
    end
    local.get $i
    call $digit_scratch
    i32.sub
    local.set $digits

    ;; response = prefix + count + suffix, starting at 1024.
    call $response_addr
    i32.const 96
    call $prefix_len
    memory.copy

    i32.const 0
    local.set $i
    block $reversed
      loop $reverse
        local.get $i
        local.get $digits
        i32.ge_u
        br_if $reversed
        call $response_addr
        call $prefix_len
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

    call $response_addr
    call $prefix_len
    i32.add
    local.get $digits
    i32.add
    local.get $suffix
    call $suffix_len
    memory.copy

    call $response_addr
    call $prefix_len
    local.get $digits
    i32.add
    call $suffix_len
    i32.add)
  (export "render_page" (func $render)))
