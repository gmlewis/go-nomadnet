# Micron `` `L `` — the Location (Plus Code) extension

**Status:** specification. This document defines the extension; it does not
change the Micron parser, and a page that does not use `` `L `` renders exactly
as it does today.

**Scope:** `go-nomadnet/nomadnet/micron`.

---

## 1. Why

NomadNet pages travel over Reticulum, where a reader may be on a 1 kbps LoRa
link with no map, no web browser, and no geocoder. A page that needs to say
*where* something is — a repeater site, a trailhead, a supply cache, a
shelter — therefore cannot rely on a click-through to a map service.

An Open Location Code (Plus Code) solves the transport problem: ten characters
name a 13.9 m by 13.9 m area anywhere on Earth, they are short enough to read
out over a voice link, and they decode offline. What a Plus Code does not do is
tell a reader how far away it is, or in which direction.

The `` `L `` extension closes that gap. It renders a Plus Code, and — when the
client knows its own position — the distance and bearing to it, from data the
client already has. Nothing leaves the node.

## 2. Syntax

```text
`L<code>`L
`L<code>|<format>`L
```

- The directive opens with a backtick and a capital `L`, and closes with a
  backtick and a capital `L`. This matches Micron's existing extension form
  (`` `F `` fields, `` `{ `` partials), so a reader who knows one knows all of
  them.
- `<code>` is a **full** Open Location Code with no padding: eight to eleven
  significant characters, with the `+` separator in its standard position
  (`` `+` `` after the eighth character, or after the fourth for a shortened
  code, in which case see §6).
- `<format>` is one of the four format tokens in §4. With no `|`, the default
  format is `%default`.
- Neither side may contain a backtick. A `|` is only a separator when it appears
  before the first backtick; anything after the format token is ignored.

### 2.1 Grammar

```abnf
location-directive = "`L" code [ "|" format ] "`L"
code               = 2*15( olc-char )
olc-char           = %x30-39 / %x41-5A  ; 0-9, A-Z (the OLC alphabet excludes I, L, O, U)
format             = "%c" / "%d" / "%b" / "%ll" / "%default"
```

A code is validated before it is rendered; an invalid one is not an extension
at all (§7).

## 3. Semantics

1. Parse the code with a full Open Location Code decoder. The code identifies
   an **area**; the point used for a distance or a bearing is the area's centre.
2. If the client knows its own position, compute:
   - the great-circle distance from the reader to the code's centre, on a
     sphere of radius 6 371 000 m (the Haversine formula); and
   - the initial bearing from the reader to the code's centre.
3. Render according to the format token.
4. If the client does **not** know its own position, or the code is malformed,
   render the raw code text (§6). An extension must never fail a page: the
   reader still learns the code, which is the part that does not depend on the
   client.

Distances are rendered in meters below 1 000 m (`850 m`), to one decimal place
in kilometers below 1 000 km (`3.2 km`), and as whole kilometers at or above
1 000 km (`8967 km`). Bearings are rendered as three digits plus a sixteen-point
compass name (`048° NE`).

## 4. Formats

| Token | Renders | Example |
|-------|---------|---------|
| *(none)*, `%default` | code, then distance and bearing when known | `849VCWC8+R9 (3.2 km, bearing 048° NE)` |
| `%c` | the code alone | `849VCWC8+R9` |
| `%d` | the distance alone, when known | `3.2 km` |
| `%b` | the bearing alone, when known | `048° NE` |
| `%ll` | the centre as signed decimal degrees | `37.421937, -122.084062` |

With no known position, `%d` and `%b` render nothing at all (not a placeholder):
a page laid out in columns should collapse, not print `unknown distance`. The
default format degrades to the bare code, and `%ll` still renders, because a
coordinate needs no reference point.

## 5. Interaction

- **TUI:** the rendered text is a clickable link. Activating it opens the
  client's location actions: copy the code, copy the coordinate, or show compass
  guidance when a position is known.
- **GUI:** the same actions, plus opening the code in whatever map application
  the host provides, when there is one.
- **Copying** always copies the **code**, never the rendered sentence, so a code
  copied from a page stays a code.

## 6. Shortened codes

A shortened code (four to seven characters) omits the digits that identify the
region and is only meaningful next to a reference location. The extension does
**not** silently recover one: doing so needs a reference point, and a client
that guesses wrong places the answer in the wrong hemisphere.

A shortened code is therefore rendered exactly as it was written, and the
default, `%d`, and `%b` formats add nothing. `%ll` is not available for a
shortened code and renders the code text instead. A page that wants a
recoverable location writes a full code — which the producing bot does by
construction.

## 7. Backward compatibility

- A client that does not know the extension displays the raw text
  `` `L849VCWC8+R9`L `` … which is not graceful. To make degradation graceful,
  a producer writes the code as ordinary text and wraps it:

  ```text
  Repeater: 849VCWC8+R9 `L849VCWC8+R9`L
  ```

  An old client renders `Repeater: 849VCWC8+R9 `L849VCWC8+R9`L`; a client that
  knows the extension renders the second half as the live location. Producers
  that do not care about old clients may write the directive alone.
- An **invalid or unparseable** code makes the whole directive unrecognized, and
  the parser leaves the raw text in place. This is the rule every Micron
  extension follows: an extension that cannot be understood is text, never an
  error.
- `%c`, `%d`, `%b`, and `%ll` are the whole format vocabulary. An unknown token
  makes the directive unrecognized rather than silently rendering the default,
  so a typo in a page is visible during development.

## 8. Producer side

`gorrcbot`'s `loc`, `proj`, `sos`, `checkin`, and `sitrep` commands all resolve
positions offline and print ten-character Plus Codes produced by a full
implementation of the Open Location Code specification, checked against the
reference implementation's own test data. A page generator that takes a Plus
Code from any of them can wrap it in `` `L `` and be sure the code is full,
unpadded, and in the canonical upper-case alphabet.

## 9. Worked examples

A reader at 37.4220 N, 122.0841 W (the Googleplex) reading a page that carries a
code for the Eiffel Tower:

```text
`L8FW4V75V+8R`L
```

renders as

```text
8FW4V75V+8R (8967 km, bearing 033° NE)
```

The same page read with no known position renders:

```text
8FW4V75V+8R
```

A formatter that wants only a distance, and a page that wants the coordinate:

```text
`L8FW4V75V+8R|%d`L        →  8967 km
`L8FW4V75V+8R|%ll`L       →  48.858312, 2.294563
```

## 10. Reference values

The distance and bearing above were produced with the formulas in §3 and the
Earth radius in §3, so an implementation can be checked against them:

The code `8FW4V75V+8R` decodes to the area whose centre is 48.858312 N,
2.294563 E. The code `849VCWC8+R9` decodes to the area whose centre is
37.4220625 N, 122.0840625 W.

| Reader | Target centre | Distance | Bearing | Rendered |
|--------|---------------|----------|---------|----------|
| 37.4220 N, 122.0841 W | 48.858312 N, 2.294563 E | 8 967 033 m | 33.39° | `8FW4V75V+8R (8967 km, bearing 033° NE)` |
| 51.5000 N, 0.1200 W | 48.858312 N, 2.294563 E | 340 318 m | 148.72° | `8FW4V75V+8R (340.3 km, bearing 149° SSE)` |
| -33.8568 S, 151.2153 E | 37.4220625 N, 122.0840625 W | 11 952 709 m | 56.23° | `849VCWC8+R9 (11953 km, bearing 056° NE)` |
