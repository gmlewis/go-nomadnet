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
;;   Renders a Micron form (a name field, a message field, and a submit link
;;   that posts the form back to this page) first, so that a visitor never has
;;   to scroll past the whole history to sign it, and then the guestbook
;;   entries newest-first below a divider. A submission is appended to the
;;   on-disk store and then the page re-renders with the new entry at the top
;;   of that history:
;;
;;     >Guestbook
;;
;;     Name:    [24-cell box]
;;     Message: [32-cell box]
;;     `[Sign the guestbook`:/page/guestbook.wasm`name|message]
;;
;;     [usage tip: the editing keys and how to submit]
;;
;;     ----
;;
;;     Ada: second entry
;;     Glenn: hello from the wasm sandbox
;;
;;   gonomadnet compiles, runs, and releases a fresh sandbox instance for every
;;   request, so only the KV store survives between visits.
;;
;; FORM MARKUP
;;   Micron spells a field `<NAME`VALUE>, where VALUE is the field's pre-defined
;;   content — not a hint or a label. Typing appends to that content (the
;;   markup guide's own example shows it as "Pre-defined data", and a
;;   pre-filled field is how a page offers a default value to keep or edit).
;;   The form therefore carries its labels as ordinary page text and leaves the
;;   fields empty:
;;
;;     Name: `B444`<name`>`b
;;     Message: `B444`<32|message`>`b
;;
;;   The `B444` background tag is what makes an empty field visible: a text
;;   field occupies its declared width (24 columns by default, 32 for the
;;   message field set by the width flag before the name), so its background
;;   paints the whole box whether or not it holds text.
;;
;;   The block carries a short usage tip for first-time visitors: which keys
;;   edit a field, and how to submit. That tip is ordinary page text — a form's
;;   instructions for its visitors belong in the page, not in its fields. Those
;;   readline keys do reach a field because the browser gives a field in edit
;;   mode the first refusal of every key, matching the priority Python's urwid
;;   gives the focused widget over the surrounding shortcut layers. A blank
;;   line and a `----` divider close the block, separating the form from the
;;   history below it.
;;
;; VISITS
;;   Every render counts one visit to the page in its own store under key "v",
;;   and the line it prints after the usage tip reports that count:
;;
;;     Guestbook has been visited 7 times.
;;
;;   The count is the page's own, because a page's KV store is private to it: a
;;   page cannot read or write another module's store. The node's site-wide
;;   counter therefore lives in hit-counter.wasm, reached the only way a page can
;;   reach another page — a Micron partial, embedded at the end of that same
;;   line with the quiet flag:
;;
;;     `{:/page/hit-counter.wasm`0`quiet=1}
;;
;;   An empty destination means the node serving this page, so the partial needs
;;   no node hash baked in. It names no counted page, so it adds this visit to
;;   the site-wide total, and "quiet=1" makes the counter render nothing, so the
;;   guestbook triggers that counter without displaying it. Install
;;   hit-counter.wasm beside this module; a node without it shows its client's
;;   partial-load error in place of the (invisible) partial.
;;
;; TIMESTAMPS
;;   An entry is stored as "<unix-seconds>\t<name>: <message>\n", taken from the
;;   request's own "requested_at" field, so the store keeps the instant and
;;   nothing about how it should be spelled. The render wraps those seconds in
;;   the timestamp construct
;;
;;     `T1789178907`T
;;
;;   which a client that understands it renders in the reader's own timezone:
;;   gonomadnet formats the seconds with the local zone and the default format
;;   "Fri Sep 11, 2026 9:08:27PM EST". A page names its own strftime format when
;;   it wants a different spelling:
;;
;;     `T1789178907|%Y-%m-%d %H:%M`T
;;
;;   Unix seconds mean UTC, and only the client that parses the page knows the
;;   reader's timezone, so localizing anywhere else would show every visitor the
;;   sender's clock instead of their own. Python's NomadNet has no such
;;   construct: it drops the marker character and renders the payload as text,
;;   so a reader on the original client sees the raw seconds. Entries stored
;;   before this page stamped them carry no "<seconds>\t" prefix, and entries
;;   whose prefix is not a run of digits are rendered exactly as stored.
;;
;; SUBMISSION MECHANICS
;;   The fields above are named "name" and "message", and the submit link's
;;   third backtick segment ("name|message") names those fields to collect. On
;;   submit the browser reads the live widget values and sends them as
;;   request_data: field_name and field_message (gonomadnet's Micron field
;;   machinery prefixes each collected field with "field_"). This module scans
;;   the request JSON for those two keys, and also accepts var_name /
;;   var_message so a hand-written link that carries the values directly in its
;;   fields suffix still works.
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
  ;; the block every render opens with: the form, its usage tip, and the
  ;; divider before the entries                                          len=571
  (data (i32.const 1152) "Name: `B444`<name`>`b\0aMessage: `B444`<32|message`>`b\0a`[Sign the guestbook`:/page/guestbook.wasm`name|message]\0a\0a`!Using the form`!: the selected field has the keyboard, so just type. `!Down`! / `!Up`! (or `!Tab`!) move between fields, and `!Enter`! on the `!Sign the guestbook`! line submits what you typed. Every field is a readline-style editor: `!Ctrl-A`! / `!Ctrl-E`! start / end of the line, `!Ctrl-U`! clears back to the start, `!Ctrl-K`! clears to the end, `!Ctrl-W`! deletes a word, `!Ctrl-L`! clears the field, and `!Ctrl-Y`! pastes back what you cleared.\0a\0a----\0a\0a")
  ;; request-JSON scan keys for the collected Micron form fields
  (data (i32.const 2048) "\"field_name\":\"")                     ;; len=14
  (data (i32.const 2080) "\"field_message\":\"")                  ;; len=17
  ;; request-JSON scan keys for a fields suffix that carries values directly
  (data (i32.const 2112) "\"var_name\":\"")                       ;; len=12
  (data (i32.const 2144) "\"var_message\":\"")                    ;; len=15

  ;; request-JSON key for the request's own unix seconds. The host writes the
  ;; value as a JSON number, so the page copies its digits verbatim instead of
  ;; parsing an integer and formatting it again.                    len=15
  (data (i32.const 2208) "\"requested_at\":")

  ;; The visit line: a count of this page's own visits, then the quiet partial
  ;; that adds this visit to the node's site-wide counter without printing it.
  ;;                                       "Guestbook has been visited " len=27
  (data (i32.const 2240) "Guestbook has been visited ")
  (data (i32.const 2304) " times.")                                     ;; len=7
  (data (i32.const 2336) " time.")                                      ;; len=6
  ;;                                       "`{:/page/hit-counter.wasm`0`quiet=1}"
  (data (i32.const 2368) "`{:/page/hit-counter.wasm`0`quiet=1}")        ;; len=36
  ;; The visit counter's own store key.                             len=1
  (data (i32.const 2440) "v")

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
  ;; $scan_digits copies a JSON number field's decimal digits. The caller names
  ;; the key (quotes and colon included, as the host writes it) and the
  ;; destination, and gets back how many digits were copied: 0 when the key is
  ;; absent or carries no digits, never more than cap. The digits are stored
  ;; verbatim, which is what keeps unix seconds lossless in both directions.
  (func $scan_digits (param $start i32) (param $len i32) (param $key i32) (param $key_len i32)
                     (param $out i32) (param $cap i32) (result i32)
    (local $i i32)      ;; candidate start offset
    (local $limit i32)  ;; last offset at which the key can still fit
    (local $p i32)      ;; read cursor inside the digits
    (local $n i32)      ;; digits copied so far
    (local $c i32)      ;; current byte
    (local $res i32)
    i32.const 0
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
          block $digits_done
            loop $digits
              local.get $n
              local.get $cap
              i32.ge_s
              br_if $digits_done
              local.get $p
              local.get $n
              i32.add
              i32.load8_u
              local.set $c
              ;; Anything outside 0-9 ends the number.
              local.get $c
              i32.const 0x30
              i32.lt_u
              br_if $digits_done
              local.get $c
              i32.const 0x39
              i32.gt_u
              br_if $digits_done
              local.get $out
              local.get $n
              i32.add
              local.get $c
              i32.store8
              local.get $n
              i32.const 1
              i32.add
              local.set $n
              br $digits
            end
          end
          local.get $n
          local.set $res
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

  ;; $stamp_prefix_len reports how many decimal digits prefix a stored entry of
  ;; the form "<seconds>\t<text>": the digit count when a tab follows a
  ;; non-empty run of digits, and 0 otherwise — an entry stored before
  ;; timestamps, or a message that happens to contain a tab. The renderer emits
  ;; the timestamp construct around those digits and drops the tab; everything
  ;; else is copied exactly as stored.
  (func $stamp_prefix_len (param $ptr i32) (param $len i32) (result i32)
    (local $i i32)
    (local $c i32)
    (local $res i32)
    i32.const 0
    local.set $res
    block $done
      loop $next
        local.get $i
        local.get $len
        i32.ge_s
        br_if $done
        local.get $ptr
        local.get $i
        i32.add
        i32.load8_u
        local.set $c
        local.get $c
        i32.const 9
        i32.eq
        if
          local.get $i
          local.set $res
          br $done
        end
        local.get $c
        i32.const 0x30
        i32.lt_u
        br_if $done
        local.get $c
        i32.const 0x39
        i32.gt_u
        br_if $done
        local.get $i
        i32.const 1
        i32.add
        local.set $i
        br $next
      end
    end
    local.get $res)

  ;; $write_digits(value, out) -> digit count, writing value's decimal digits at
  ;; out least-significant digit first. The caller reads them back in reverse,
  ;; which avoids dividing by a power of ten just to find the leading digit.
  (func $write_digits (param $value i32) (param $out i32) (result i32)
    (local $v i32)  ;; remaining value
    (local $i i32)  ;; write cursor
    local.get $value
    local.set $v
    local.get $out
    local.set $i
    loop $digits
      local.get $i
      local.get $v
      i32.const 10
      i32.rem_u
      i32.const 48
      i32.add
      i32.store8
      local.get $i
      i32.const 1
      i32.add
      local.set $i
      local.get $v
      i32.const 10
      i32.div_u
      local.set $v
      local.get $v
      br_if $digits
    end
    local.get $i
    local.get $out
    i32.sub)

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
    (local $stamp_len i32) ;; digits of the request's unix seconds
    (local $off i32)       ;; where the name starts inside the entry buffer
    (local $stamp i32)     ;; digits prefixing a stored entry when rendering
    (local $visits i32)    ;; visits to this page, including this one
    (local $digits i32)    ;; decimal digits of the visit count

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
      ;; Stamp the entry with the request's unix seconds: the host writes the
      ;; value as a JSON number, so its digits are copied verbatim and prefixed
      ;; to the line with a tab. Unix seconds mean UTC, and only the client that
      ;; reads the page knows the reader's timezone, so the store keeps the
      ;; seconds and each reader's client renders them locally.
      local.get $req_ptr
      local.get $req_len
      i32.const 2208
      i32.const 15
      i32.const 9216
      i32.const 20
      call $scan_digits
      local.set $stamp_len
      i32.const 9216
      local.get $stamp_len
      i32.add
      local.set $off
      local.get $stamp_len
      i32.const 0
      i32.gt_s
      if
        local.get $off
        i32.const 9
        i32.store8
        local.get $off
        i32.const 1
        i32.add
        local.set $off
      end

      local.get $off
      i32.const 8192
      local.get $name_len
      memory.copy
      ;; ": " after the name
      local.get $off
      local.get $name_len
      i32.add
      i32.const 0x203a
      i32.store16
      ;; the message, then a newline
      local.get $off
      local.get $name_len
      i32.add
      i32.const 2
      i32.add
      i32.const 8448
      local.get $msg_len
      memory.copy
      local.get $off
      local.get $name_len
      i32.add
      i32.const 2
      i32.add
      local.get $msg_len
      i32.add
      i32.const 10
      i32.store8
      local.get $off
      i32.const 9216
      i32.sub
      local.get $name_len
      i32.add
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

    ;; response += the form block and its usage tip. The form opens the page so
    ;; no visitor has to scroll past the whole history to sign it; whatever is
    ;; below the divider is history, newest first.
    local.get $out
    i32.const 1152
    i32.const 565
    memory.copy
    local.get $out
    i32.const 565
    i32.add
    local.set $out

    ;; Count this visit. The counter is the page's own 4-byte little-endian slot
    ;; under key "v"; a missing key leaves the zeroed slot as-is, so the first
    ;; visit reads 1. The store is per page, which is exactly why the site-wide
    ;; count below is reached through a partial instead.
    i32.const 2440
    i32.const 1
    i32.const 128
    i32.const 4
    call $kv_get
    drop
    i32.const 128
    i32.load
    i32.const 1
    i32.add
    local.set $visits
    i32.const 128
    local.get $visits
    i32.store
    i32.const 2440
    i32.const 1
    i32.const 128
    i32.const 4
    call $kv_set
    drop

    ;; response += the visit line: "Guestbook has been visited <n> times." and
    ;; the quiet site-counter partial, which renders nothing at all.
    local.get $out
    i32.const 2240
    i32.const 27
    memory.copy
    local.get $out
    i32.const 27
    i32.add
    local.set $out

    local.get $visits
    i32.const 144
    call $write_digits
    local.set $digits
    i32.const 0
    local.set $i
    block $digits_done
      loop $copy_digits
        local.get $i
        local.get $digits
        i32.ge_u
        br_if $digits_done
        local.get $out
        local.get $i
        i32.add
        i32.const 144
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
        br $copy_digits
      end
    end
    local.get $out
    local.get $digits
    i32.add
    local.set $out

    ;; The first visit is phrased in the singular, like any counter a visitor
    ;; reads.
    local.get $visits
    i32.const 1
    i32.eq
    if
      local.get $out
      i32.const 2336
      i32.const 6
      memory.copy
      local.get $out
      i32.const 6
      i32.add
      local.set $out
    else
      local.get $out
      i32.const 2304
      i32.const 7
      memory.copy
      local.get $out
      i32.const 7
      i32.add
      local.set $out
    end

    local.get $out
    i32.const 2368
    i32.const 36
    memory.copy
    local.get $out
    i32.const 36
    i32.add
    i32.const 10
    i32.store8
    local.get $out
    i32.const 37
    i32.add
    local.set $out

    ;; response += the divider that separates the form from the history.
    local.get $out
    i32.const 1717
    i32.const 6
    memory.copy
    local.get $out
    i32.const 6
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
            ;; An entry stored as "<seconds>\t<text>" renders as the timestamp
            ;; construct followed by the text, so each reader's client shows
            ;; those seconds in that reader's own timezone. Any other entry is
            ;; copied verbatim, which is how entries stored before timestamps
            ;; keep rendering exactly as they were written.
            i32.const 10240
            local.get $n
            call $stamp_prefix_len
            local.set $stamp
            local.get $stamp
            i32.const 0
            i32.gt_s
            if
              local.get $out
              i32.const 0x60
              i32.store8
              local.get $out
              i32.const 1
              i32.add
              i32.const 0x54
              i32.store8
              local.get $out
              i32.const 2
              i32.add
              i32.const 10240
              local.get $stamp
              memory.copy
              local.get $out
              i32.const 2
              i32.add
              local.get $stamp
              i32.add
              i32.const 0x60
              i32.store8
              local.get $out
              i32.const 3
              i32.add
              local.get $stamp
              i32.add
              i32.const 0x54
              i32.store8
              local.get $out
              i32.const 4
              i32.add
              local.get $stamp
              i32.add
              i32.const 0x20
              i32.store8
              local.get $out
              i32.const 5
              i32.add
              local.get $stamp
              i32.add
              i32.const 10240
              local.get $stamp
              i32.const 1
              i32.add
              i32.add
              local.get $n
              local.get $stamp
              i32.const 1
              i32.add
              i32.sub
              memory.copy
              local.get $out
              i32.const 5
              i32.add
              local.get $stamp
              i32.add
              local.get $n
              local.get $stamp
              i32.const 1
              i32.add
              i32.sub
              i32.add
              local.set $out
            else
              local.get $out
              i32.const 10240
              local.get $n
              memory.copy
              local.get $out
              local.get $n
              i32.add
              local.set $out
            end
          end
          local.get $i
          i32.const 1
          i32.sub
          local.set $i
          br $entries
        end
      end
    end

    i32.const 12288
    local.get $out
    i32.const 12288
    i32.sub)
  (export "render_page" (func $render)))
