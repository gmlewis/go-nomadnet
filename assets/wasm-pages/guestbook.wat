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

;; guestbook.wat -- a gonomadnet executable page with a working Micron form.
;;
;; WHAT IT DOES
;;   Renders the guestbook entries newest-first, followed by a Micron form
;;   (a name field, a message field, and a submit link that posts the form
;;   back to this page). A submission is appended to the on-disk store and
;;   then the page re-renders with the new entry at the top:
;;
;;     >Guestbook
;;
;;     Glenn: hello from the wasm sandbox
;;     Ada: second entry
;;     ----
;;
;;     `<name`Your name>
;;     `<message`Your message>
;;     `[Sign the guestbook`:/page/guestbook.wasm`name|message]
;;
;;   gonomadnet compiles, runs, and releases a fresh sandbox instance for every
;;   request, so only the KV store survives between visits.
;;
;; SUBMISSION MECHANICS
;;   Micron spells a field `<NAME`VALUE>, so the fields above are named "name"
;;   and "message", and the submit link's third backtick segment
;;   ("name|message") names those fields to collect. On submit the browser
;;   reads the live widget values and sends them as request_data: field_name
;;   and field_message (gonomadnet's Micron field machinery prefixes each
;;   collected field with "field_"). This module scans the request JSON for
;;   those two keys, and also accepts var_name / var_message so a hand-written
;;   link that carries the values directly in its fields suffix still works.
;;
;; BUILD
;;   wat2wasm guestbook.wat -o guestbook.wasm
;;
;; INSTALL
;;   Copy guestbook.wasm into ~/.nomadnetwork/storage/pages/ on the node and
;;   add a Micron link to <node-destination-hash>:/page/guestbook.wasm.
;;   gonomadnet renders .wasm pages only when built with -tags wago.
;;
;; STATE
;;   Under the page's own scratch store, ~/.nomadnetwork/storage/pages/data/
;;   guestbook/:
;;     n   -- 4-byte little-endian entry count
;;     e0, e1, ... -- one file per entry, holding that entry's rendered
;;          Micron line. Deleting the whole directory resets the guestbook.
;;
;;   Each entry is stored as its finished Micron line rather than as a
;;   "|"-joined name/message pair, so rendering needs no delimiter scan: the
;;   host only ever appends stored bytes. That keeps the newest-first loop to
;;   a kv_get plus a memory.copy per entry.
;;
;; SECURITY
;;   request_data is attacker-controlled and the host does not sanitize it, so
;;   this module rejects a submission whose name or message contains a backtick
;;   (0x60, Micron's formatting-mode character), a left bracket (0x5b, which
;;   opens a link), or any control byte below 0x20. Storage is where that
;;   matters, because the rendered page re-emits stored bytes verbatim; a
;;   rejected submission stores nothing and simply re-renders the page.
;;   RESIDUAL RISK: the store is written only through this page's own
;;   validation, but the store files themselves are plain files on the node's
;;   disk -- anyone who can write to them can inject markup into this page.
;;   Values are also capped (64 bytes for the name, 256 for the message) so a
;;   single request cannot grow the store without bound.

(module
  ;; rns.kv_get(key_ptr, key_len, out_ptr, out_cap) -> bytes written
  ;;   0 = key absent, -1 = out_cap too small
  (import "rns" "kv_get" (func $kv_get (param i32 i32 i32 i32) (result i32)))
  ;; rns.kv_set(key_ptr, key_len, val_ptr, val_len) -> status (0 = ok)
  (import "rns" "kv_set" (func $kv_set (param i32 i32 i32 i32) (result i32)))

  (memory 1 1)
  (export "memory" (memory 0))

  ;; --- Static data. The trailing "len=N" comment on each segment is the byte
  ;; length the code passes to the host and to memory.copy. ---

  ;; key "n" (the entry count)                                     len=1
  (data (i32.const 64) "n")
  ;; key prefix "e" (each entry's key is "e" + decimal index)      len=1
  (data (i32.const 80) "e")
  ;; page heading                                                  len=12
  (data (i32.const 1024) ">Guestbook\0a\0a")
  ;; shown when the guestbook is empty                             len=16
  (data (i32.const 1088) "No entries yet.\0a")
  ;; the form block that closes every render                       len=106
  (data (i32.const 1152) "\0a----\0a\0a`<name`Your name>\0a`<message`Your message>\0a`[Sign the guestbook`:/page/guestbook.wasm`name|message]\0a")
  ;; request-JSON scan keys for the collected Micron form fields
  (data (i32.const 2048) "\"field_name\":\"")                     ;; len=14
  (data (i32.const 2080) "\"field_message\":\"")                  ;; len=17
  ;; request-JSON scan keys for a fields suffix that carries values directly
  (data (i32.const 2112) "\"var_name\":\"")                       ;; len=12
  (data (i32.const 2144) "\"var_message\":\"")                    ;; len=15

  ;; --- Buffer layout ---
  ;;   96    : the 4-byte little-endian entry count (kv_get/kv_set buffer)
  ;;   112   : the "e<digits>" key scratch (1 + up to 10 digits)
  ;;   4096  : request payload, placed at the top of memory by $alloc
  ;;   8192  : extracted name (cap 64)
  ;;   8448  : extracted message (cap 256)
  ;;   9216  : the entry line under construction (cap 512)
  ;;   10240 : kv_get buffer for one stored entry (cap 512)
  ;;   12288 : the response under construction (cap 8192)

  ;; $alloc places the request payload at the top of linear memory so it can
  ;; never overlap the scratch buffers above, whose offsets are fixed. A
  ;; payload too large for the remaining space fails the host's own bounds
  ;; check before render_page runs.
  (func $alloc (param $len i32) (result i32)
    i32.const 65536
    local.get $len
    i32.sub
    i32.const 8
    i32.sub
    i32.const -8
    i32.and)
  (export "wagoplugin_alloc" (func $alloc))

  ;; $key_at(pos, key, key_len) -> 1 when the key_len bytes at pos equal key.
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
  ;; value that follows it into out. A backslash escape is dropped, keeping
  ;; the character it escaped, so a value containing a quote round-trips.
  ;; Returns -1 when the key is absent, the value exceeds cap, or the value
  ;; carries a byte that could inject Micron markup (see SECURITY above).
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
              ;; A backslash escape: the value is kept only when the escaped
              ;; character stands for itself ("\"", "\\", "\/"). Any other escape
              ;; (\n, \t, \uXXXX) denotes a byte this page refuses to store.
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
              ;; Reject Micron markup injection and control bytes.
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

  ;; $utoa(value, dst) -> digit count. Writes the decimal digits of value into
  ;; dst least-significant first; callers that need the usual order call
  ;; $reverse.
  (func $utoa (param $value i32) (param $dst i32) (result i32)
    (local $v i32)
    (local $n i32)
    local.get $value
    local.set $v
    loop $digits
      local.get $dst
      local.get $n
      i32.add
      local.get $v
      i32.const 10
      i32.rem_u
      i32.const 48
      i32.add
      i32.store8
      local.get $n
      i32.const 1
      i32.add
      local.set $n
      local.get $v
      i32.const 10
      i32.div_u
      local.set $v
      local.get $v
      br_if $digits
    end
    local.get $n)

  ;; $reverse(dst, n) flips the n bytes at dst in place.
  (func $reverse (param $dst i32) (param $n i32)
    (local $i i32)
    (local $j i32)
    (local $tmp i32)
    block $done
      loop $rev
        local.get $i
        local.get $n
        i32.const 1
        i32.shr_u
        i32.ge_u
        br_if $done
        local.get $n
        i32.const 1
        i32.sub
        local.get $i
        i32.sub
        local.set $j
        local.get $dst
        local.get $i
        i32.add
        i32.load8_u
        local.set $tmp
        local.get $dst
        local.get $i
        i32.add
        local.get $dst
        local.get $j
        i32.add
        i32.load8_u
        i32.store8
        local.get $dst
        local.get $j
        i32.add
        local.get $tmp
        i32.store8
        local.get $i
        i32.const 1
        i32.add
        local.set $i
        br $rev
      end
    end)

  ;; $entry_key(index) -> key length, with the key at 112: "e" + decimal.
  (func $entry_key (param $index i32) (result i32)
    (local $digits i32)
    i32.const 112
    i32.const 101 ;; 'e'
    i32.store8
    local.get $index
    i32.const 113
    call $utoa
    local.set $digits
    i32.const 113
    local.get $digits
    call $reverse
    local.get $digits
    i32.const 1
    i32.add)

  ;; render_page(req_ptr, req_len) -> (response_ptr, response_len).
  (func $render (param $req_ptr i32) (param $req_len i32) (result i32 i32)
    (local $count i32)     ;; entries stored
    (local $name_len i32)
    (local $msg_len i32)
    (local $key_len i32)
    (local $entry_len i32)
    (local $i i32)
    (local $n i32)
    (local $out i32)       ;; response write cursor
    (local $head_end i32)  ;; where the form block starts

    ;; count = le32(store["n"]); a missing key leaves the zeroed slot as-is.
    i32.const 64
    i32.const 1
    i32.const 96
    i32.const 4
    call $kv_get
    drop
    i32.const 96
    i32.load
    local.set $count

    ;; Extract the submitted name and message. The browser's collected form
    ;; fields arrive as field_name / field_message; a hand-written fields
    ;; suffix carrying values arrives as var_name / var_message.
    local.get $req_ptr
    local.get $req_len
    i32.const 2048
    i32.const 14
    i32.const 8192
    i32.const 64
    call $scan
    local.set $name_len
    local.get $name_len
    i32.const 0
    i32.lt_s
    if
      local.get $req_ptr
      local.get $req_len
      i32.const 2112
      i32.const 12
      i32.const 8192
      i32.const 64
      call $scan
      local.set $name_len
    end
    local.get $req_ptr
    local.get $req_len
    i32.const 2080
    i32.const 17
    i32.const 8448
    i32.const 256
    call $scan
    local.set $msg_len
    local.get $msg_len
    i32.const 0
    i32.lt_s
    if
      local.get $req_ptr
      local.get $req_len
      i32.const 2144
      i32.const 15
      i32.const 8448
      i32.const 256
      call $scan
      local.set $msg_len
    end

    ;; Append the entry when a submission carried both parts. Building the
    ;; finished line here is what makes the render loop delimiter-free.
    local.get $name_len
    i32.const 0
    i32.gt_s
    local.get $msg_len
    i32.const 0
    i32.gt_s
    i32.and
    if
      i32.const 9216
      i32.const 8192
      local.get $name_len
      memory.copy
      ;; ": " after the name
      i32.const 9216
      local.get $name_len
      i32.add
      i32.const 0x203a
      i32.store16
      ;; the message, then a newline
      i32.const 9216
      local.get $name_len
      i32.add
      i32.const 2
      i32.add
      i32.const 8448
      local.get $msg_len
      memory.copy
      i32.const 9216
      local.get $name_len
      i32.add
      i32.const 2
      i32.add
      local.get $msg_len
      i32.add
      i32.const 10
      i32.store8
      local.get $name_len
      local.get $msg_len
      i32.add
      i32.const 3
      i32.add
      local.set $entry_len

      ;; kv_set("e<count>", entry)
      local.get $count
      call $entry_key
      local.set $key_len
      i32.const 112
      local.get $key_len
      i32.const 9216
      local.get $entry_len
      call $kv_set
      drop

      ;; kv_set("n", count + 1)
      local.get $count
      i32.const 1
      i32.add
      local.set $count
      i32.const 96
      local.get $count
      i32.store
      i32.const 64
      i32.const 1
      i32.const 96
      i32.const 4
      call $kv_set
      drop
    end

    ;; response = heading
    i32.const 12288
    i32.const 1024
    i32.const 12
    memory.copy
    i32.const 12288
    i32.const 12
    i32.add
    local.set $out

    local.get $count
    i32.eqz
    if
      local.get $out
      i32.const 1088
      i32.const 16
      memory.copy
      local.get $out
      i32.const 16
      i32.add
      local.set $out
    else
      ;; newest first: index count-1 down to 0
      local.get $count
      i32.const 1
      i32.sub
      local.set $i
      block $entries_done
        loop $entries
          local.get $i
          i32.const 0
          i32.lt_s
          br_if $entries_done
          local.get $i
          call $entry_key
          local.set $key_len
          i32.const 112
          local.get $key_len
          i32.const 10240
          i32.const 512
          call $kv_get
          local.set $n
          local.get $n
          i32.const 0
          i32.gt_s
          if
            local.get $out
            i32.const 10240
            local.get $n
            memory.copy
            local.get $out
            local.get $n
            i32.add
            local.set $out
          end
          local.get $i
          i32.const 1
          i32.sub
          local.set $i
          br $entries
        end
      end
    end

    ;; response += form block
    local.get $out
    i32.const 1152
    i32.const 106
    memory.copy
    local.get $out
    i32.const 106
    i32.add
    local.set $head_end

    i32.const 12288
    local.get $head_end
    i32.const 12288
    i32.sub)
  (export "render_page" (func $render)))
