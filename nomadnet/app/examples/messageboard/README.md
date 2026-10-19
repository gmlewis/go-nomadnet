# NomadNet Message Board (starter)

This is the Go-native form of the upstream NomadNet `messageboard` example.

Upstream ships two parts: a page (`messageboard.mu`) and a Python script
(`messageboard.py`) that renders the board. The page in that example is itself
an executable Python script that reads the board file, so it cannot run on a
gonomadnet node — gonomadnet does not execute Python pages (it runs `.wasm`
pages in a sandbox instead; see `nomadnet/wasmpages`).

The Go distribution therefore ships:

- `messageboard.mu` — a static Micron starter board you can copy into your
  node's `pages` directory and edit.
- This README.

## Using the starter page

1. Enable node hosting (`enable_node = yes` in the `[node]` section of your
   gonomadnet config).
2. Copy `messageboard.mu` into `storage/pages/` (or reference it from
   `index.mu`).
3. Edit the `lxmf@...` address in the page to point at the peer that collects
   messages.

## Making it dynamic

Write the board as a `.wasm` executable page and serve it instead of the
static `.mu`. The sandbox exposes the request payload (path, identity, form
fields) and returns Micron markup; `assets/wasm-pages/` contains example
guests, and `view-mu -raw assets/wasm-pages/guestbook.wasm` renders one
locally.

This static page is a deliberate divergence from upstream's executable-Python
form and is recorded in `parity-gaps.md` §6.
