# nomadnet 1.2.8 (Python) trusted-chat reference behavior

Source-of-truth behavior captured live on 2026-08-23 by running the identical
trusted-chat experiment on **nomadnet 1.2.8 (Python, urwid)** on two machines
(Mac `gonomadnet` tmux session; Linux box `glenn-OMEN-875` `gnomad-linux` tmux
session), both in 24-bit color (`[textui] colormode = 24bit`), sharing the same
`~/.nomadnetwork` identities as the gonomadnet runs. Use this to diff gonomadnet
(Go) behavior and file parity bugs (see `TODO.md` § "Parity bugs found").

All raw `tmux capture-pane -e` screen grabs are in `captures/` (filenames
referenced below). Line numbers / glyph spellings are verbatim from those grabs.

## Setup / identities

- Mac nomadnet LXMF: `2a6105f57145860441a62fe3b2a1352c`
- Linux nomadnet LXMF: `9a2fe7bfd0579a4582cda61509d15551`
- Both connect to the public RNS transport `dfw.us.g00n.cloud:6969` (plus
  AutoInterface), so they share a network path. `gornpath` from either box to
  the other's LXMF hash succeeds (3–4 hops).

## Conversations page layout (where panels appear)

Two-column layout:
- **Left column** = the conversations list (titled "Conversations"), with a
  header row `[ Trusted (N) ✉ M ]   [ Untrusted (N) ✉ M ]` and a `───` divider,
  then peer rows `✓ <name>` + `<activity>`. Width is fixed/narrow (~50 cols).
- **Right column** = the conversation view (wide). When no conversation is
  selected it shows `No conversation selected`. When a conversation is open it
  shows a header line `<name> | ◷ <N> hops` (the peer **display name**, then the
  hop count), a `───` divider, the message list, and the composer at the bottom.
- The **New Conversation** dialog (C-n) is drawn **inside the left column**
  (overlaid on the list), titled "New Conversation", ~48 cols wide.
- The **My LXMF** popup (C-p) is a **wide centered modal** spanning most of the
  right column, titled "QR Code".
- Shortcut bar is the bottom row; it changes per focus region (list bar vs
  composer bar vs messages bar).

## C-p "My LXMF" → "QR Code" modal (capture `01-mac-mylxmf.txt`)

- Modal title: **"QR Code"** (a wide box).
- Renders an **actual QR code** (unicode block glyphs) of the LXMF address.
- Below the QR: the hash as **`< 2a6105f57145860441a62fe3b2a1352c >`** — note
  the **spaces inside the angle brackets**.
- Dismiss: **Space** dismisses the popup. **Esc does NOT dismiss it** (the popup
  stays open; a following C-n was absorbed by the popup rather than opening New
  Conversation). (gonomadnet differs: "LXMF Address" modal, no QR, `<hash>` no
  spaces, dismiss on Esc — see B3/B4.)

## New Conversation (C-n) dialog (capture `02-mac-newconv-dialog.txt`)

- Title "New Conversation". Fields, top to bottom: `Addr :`, `Name :`, a blank
  spacer, three trust radios `(X) Untrusted` / `(X) Unknown` / `( ) Trusted`,
  blank spacer, buttons `< Create >` `< Back >`.
- **Two radios show `(X)` on open** (Untrusted AND Unknown) — this is urwid's
  RadioButton "first True" + explicit-True construction quirk; checking one via
  Space unchecks the others. (gonomadnet reproduces this exactly — NOT a bug.)
- **Field advance key is Down**, not Tab. Tab is consumed by the ReadlineEdit
  (typing after Tab appends to the current field; the field wraps). Down moves
  Addr→Name→Untrusted→Unknown→Trusted. (gonomadnet uses Tab to advance — B5.)
- To select Trusted: from Name, `Down Down Down` to the Trusted radio, then
  `Space` (sets `(X) Trusted`, clears the others).
- To submit: `Down` to the Create button, `Enter`.

## After Create — does NOT auto-open (capture `03-mac-conv-created.txt`)

- On Create, nomadnet adds the peer to the directory/list and **returns to the
  list with the right pane still "No conversation selected"**. The new peer
  appears at the top of the Trusted list (`✓ Linux-OMEN`, `just now`, Trusted
  count +1).
- **The conversation is NOT auto-opened.** To open it: move the list cursor onto
  the peer and press `Enter` (or `Space`) — this fires the list item's "click"
  signal → `display_conversation`. (gonomadnet auto-opens after Create — B1.)

## Opening a conversation (capture `04-mac-conv-opened.txt`)

- List selection callback is a no-op; conversations open via the list item's
  "click" signal (Enter/Space) which calls `display_conversation(source_hash)`.
- Opened view header: **`Linux-OMEN | ◷ 2 hops`** — the peer **display name**,
  then `◷` and the hop count. (gonomadnet shows `<hash>` here — B2.)
- Composer is at the bottom of the right column; shortcut bar becomes
  `[C-d] Send  [C-p] Paper Msg  [C-t] Title  [C-f] Attach  [C-s] Save  [Tab] ↑ Messages`.
- `Tab` from the composer moves focus up to the messages list (bar becomes
  `[C-s] Save [C-u] Purge [C-o] Sort [C-x] Clear History [C-g] Fullscreen [C-w] Close [Tab] ↓ Editor`).

## Sending a message (capture `06-mac-sent.txt`)

- Type in the composer, `C-d` to send.
- The outgoing message appears **immediately** in the message list:
  `✓ → just now | 2026-08-23 16:20:31 ⚿` then the message text on the next line.
  - Leading marker **`✓`**, direction arrow **`→`** (outgoing), relative time,
    `|` timestamp, trailing status glyph **`⚿`**.
  - (gonomadnet outgoing marker is `↑` not `✓` — B7.)

## Receiving a message on the remote side (capture `08…/12-linux-mac-conv-messages.txt`)

- The incoming message makes the peer appear in the remote list with an unread
  badge: `✓ Mac ✉ (2)`. The Trusted/Untrusted tab counts gain a `✉ N` unread.
- Opening the conversation shows the incoming message:
  `✓ ← 7m ago | 2026-08-23 16:20:31 ⚿` then the text.
  - Leading marker **`✓`**, direction arrow **`←`** (incoming), `⚿` status.
  - **No "Unknown Origin" warning** because the peer is Trusted. (For an
    untrusted/unknown sender nomadnet shows `⚠ ← Unknown Origin` instead of the
    `✓ ← …` line — trust-dependent, see R3.)

## "Delete conversation" (C-x) retains the directory entry (R1)

- Deleting a conversation (C-x → "Delete conversation with <name>" → `< Yes >`)
  removes the conversation/messages and drops the peer from the list (Trusted
  count −1) but **retains the directory entry (name + trust)**.
- A subsequent incoming message from that peer **re-creates the conversation
  under the retained entry**, reusing the name and trust (it reappeared as
  `✓ Mac` Trusted, not as a fresh Unknown/Untrusted entry).

## Replying — path known, sends immediately (capture `13-linux-reply-sent.txt`)

- Because the Mac nomadnet announced its LXMF destination, the Linux side had
  learned a path: header `Mac | ◷ 2 hops`.
- Reply (type + C-d) sends and the outgoing message appears immediately:
  `✓ → just now | 2026-08-23 16:28:42 ⚿` + text. (gonomadnet reply never sent —
  it showed `Mac | ◷ unknown` and no outgoing message appeared — B6/B8.)

## Open conversation auto-refreshes on incoming (R2, capture `14-mac-reply-arrival.txt`)

- With the Mac's Linux-OMEN conversation already open, the Linux reply arrived
  and **appeared live in the open conversation** without re-selecting:
  `✓ ← just now | 2026-08-23 16:28:42 ⚿` + text, appended under the Mac's
  outgoing `✓ → 8m ago …`. The list unread badge also updated.

## Glyph / marker summary (nomadnet 1.2.8)

- Trusted peer (list): `✓ <name>`
- Outgoing message: `✓ → <rel> | <ts> ⚿`
- Trusted incoming message: `✓ ← <rel> | <ts> ⚿`
- Untrusted/unknown incoming: `⚠ ← Unknown Origin` (then `just now | <ts> !`)
- Hop indicator in conversation header: `◷ <N> hops` (or `◷ unknown` when no
  path is known)
- Direction arrows `→` (out) / `←` (in) match between runtimes; the leading
  status glyph and the QR modal are the main divergences (see TODO.md B2/B3/B7).

## Network tab / browsing (captures `15`–`21`)

- **Tab switching is by mouse-click on the menu bar, not keyboard.** Keyboard
  `Up`-at-top of the Conversations list did NOT move focus to the menu header in
  nomadnet 1.2.8 (the `IndicativeListBox` does not return `"up"` unhandled at
  the top, so the `ConversationsArea` Up→header transition never fires). An
  **SGR-1006 mouse click** on the `[ Network ]` menu button (the `✉` is 3 bytes
  but 1 column, so `[ Network ]` is at column 21–31; click col ~27, row 1) opens
  the Network tab: `tmux send-keys -l $'\033[<0;27;1 M'` press + `$'\033[<0;27;1 m'`
  release. Verify whether gonomadnet's keyboard Up-at-top→menu works — candidate
  parity item (TODO.md B9).
- Network tab layout: left column "Saved Nodes" (or "Announce Stream", toggled
  by `C-l`), right column the browser pane. Shortcut bar: `[C-l] Nodes/Announces
  [C-x] Remove [C-w] Disconnect [C-d] Back [C-f] Forward [C-r] Reload [C-u] URL
  [C-g] Fullscreen [C-s / C-b] Save Node`.
- `C-u` (URL) and `C-l` (toggle) are handled by `NetworkLeftPile.keypress`
  (Network.py:1599) — they only fire when the **left pile has focus**. If a
  Local-Peer/Node-Info panel is shown in the right pane, `C-u` instead opens
  that panel; click the left list first to focus it.
- **Announce Stream** (`C-l` with left pane focused): header "Announce Stream",
  tab bar `[ Nodes ] [ Peers ] [ Propagation Nodes (256) ]`, a `Search:` filter
  + `Show: Name`/`Show: Dest` toggle, then a scrolling list of recent announces
  (`HH:MM:SS Ⓝ <name>`).
- **Browsing by URL** (captures `19` Mac→Linux, `21` Linux→Mac): focus left pane
  → `C-u` → "Enter URL" dialog (`URL :` input + `< Cancel > < Go >`) → type
  `<nodehash>:/page/index.mu` → `Tab Tab` to `Go` → `Enter`. The browser opens
  in the right pane: URL bar `Ⓝ <hash>:/page/index.mu`, a `┄┄┄` divider, then
  the rendered Micron page. 24-bit throughout (4711–4746 RGB cells on the page).
- **Both nodes serve the identical `index.mu`**: Mac node
  `bc37348ec27fafad10f3fd2e92ecf5f5` and Linux node
  `cbae8e4890c9ca51d32b349d860fa977` both render the same page — heading
  "Welcome to gonomadnet", body "This node is running gonomadnet — a Go
  implementation of the Nomad Network client and node.", plus a large QR code.
- **Node hash ≠ LXMF hash.** The nomadnetwork.node destination hash is distinct
  from the lxmf.delivery hash (same identity, different app/aspect). Compute
  with `gornid -i ~/.nomadnetwork/storage/identity -H nomadnetwork.node`
  (calibrate with `-H lxmf.delivery` → must match the known LXMF hash).

## Channels / RRC (captures `22`–`38`)

- Opened via mouse-click on the `[ Channels ]` menu button (col ~40, row 1;
  same mouse-click tab-switch as Network — keyboard Up-at-top does not fire).
- Layout: left column titled "Channels" (the hubs list, with rooms indented
  under each hub), right column the chat view. With no hubs: left shows
  `No hubs yet. Press Ctrl-N to add one.`, right shows
  `Select or add a hub to begin`.
- Shortcut bar (no room open): `[C-n] New Hub  [C-a] Add Room  [C-r] Connect
  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove`.
- **New Hub dialog** (`C-n`, left pane focused — click it first): titled
  "New Hub", fields `Hub address :` + `Display name:`, buttons `< Add > < Back >`.
  The hub address is the hub's `rrc.hub` destination hash (RRC default dest name
  is `rrc.hub`, `nomadnet/RRC.py:97`). Advance fields with `Down`, `< Add >` to
  save (it's several Downs past the name field — there's a "Show more options"
  button + divider between the fields and the Save/Add button row).
- **Add Room dialog** (`C-a`): titled "Add Room", `Room : #` input (type the room
  name after the `#`), `[ ] Keyed room (+k)` checkbox, `< Join > < Back >`.
- **Hub connect + room join (live, public hub ShadowLink TSK-0,
  hash `c3fd74541dafc8b7077bc8a25a2c2302`, room `#teskeslab`):**
  - After `C-r` Connect, the hub entry shows `✓ ShadowLink` and the joined room
    `#teskeslab` appears indented under it in the left list.
  - The hub is reachable in 2 hops via the existing public RNS interfaces
    (`gornpath c3fd74541…` → 2 hops via Local shared instance) — a dedicated
    transport interface is NOT required for this hub.
  - Chat header (right pane): `#teskeslab ┄ ShadowLink TSK-0 v0.3.2
    (ShadowLink) | Connected` — `#room ┄ <hub software> (<hub display name>) |
    Connected`, on a light background (black on `#d7d7d7`).
  - System messages on join: `[17:08:23] → You joined #teskeslab` and
    `[17:08:23] ℹ room teskeslab: unregistered; mode=(none); topic=(none)`.
  - **Send flow** (`C-d`): outgoing chat message renders as
    `[17:08:48]  <Go port of NomadNet> test from …` — `[HH:MM:SS]  <nick> text`.
  - Message bar (room open): `[C-d] Send  [C-x] Leave  [F8] Collapse
    [Tab] Complete Nick`.
- **Colors (24-bit, 15 RGB on the chat view):** timestamp `[HH:MM:SS]` in
  `#888888` (gray); nick `<…>` in a per-nick palette color (here `#bbaa00` for
  "Go port of NomadNet"); message body in `#dddddd` (body_text). Nick palette:
  `nick_self`, `nick_peer`, and a 24-entry `nick_colors` list
  (`nomadnet/ui/textui/Channels.py:24-45`). `@nick` and bare nick mentions are
  scanned and highlighted (`nick_mention` attr, Channels.py:124-186).
- **Members panel:** not visibly present with a single occupant; `F8` Collapse
  did not reveal a separate members list in this room (verify on a busier room).
- No gonomadnet diff for RRC yet (the gonomadnet run only exercised
  Conversations); diff when we switch runtimes.

## Interfaces (captures `24`–`27`)

- Opened via mouse-click on the `[ Interfaces ]` menu button (col ~62, row 1).
- **List view**: a vertical stack of bordered boxes (`╭…╰`), one per RNS
  interface. Each box: `○   🖧  <name>`, then `Status:   <Enabled|Disabled>
  | <Connected|Disconnected>`, `Type:     <AutoInterface|TCPClientInterface|…>`,
  a `---` divider, `TX:   <n> bytes   RX:   <n> bytes` (humanized: `17.2 KB`,
  `450.0 KB`, etc.). Mac and Linux show the same 7-interface template
  (Default Interface AutoInterface + 6 TCPClientInterface: Michmesh Testnet,
  Beleth RNS Hub, g00n Cloud Dallas, mobilefabrik TCP, Quortal TCP Node, Sydney
  RNS) with different enabled/connected state and TX/RX traffic.
- List shortcut bar: `[C-a] Add Interface [C-e] Edit Interface [C-x] Remove
  Interface [Enter] Show Interface [C-w] Open Text Editor`.
- **Show Interface detail** (`Enter` on a selected interface — click the box
  first, then `Enter`): a `===` top border, then `Type: 🖧 <type>`,
  `Status: ● <Enabled|Disabled> | <Connected|Disconnected>`, `TX:/RX:` bytes,
  a `---` divider, then an `RX Traffic (60s)` ASCII line chart (y-axis `N bps`,
  x-axis time, drawn with `┼┤` and `─`), a `---` divider, and (off-screen) a TX
  chart. Bottom buttons `< Back > < Disable/Enable >`.
  - Detail shortcut bar: `[Up/Down] Navigate [Tab] Switch Focus [h] Horizontal
    Charts [v] Vertical Charts`.
- **Add Interface dialog** (`C-a` from the list): titled "Select Interface
  Type", a list of types: Auto Interface, TCP Client Interface, TCP Server
  Interface, I2P Interface, `ᚱ  RNodes` (group), RNode Interface, RNode Multi
  Interface, AX.25 KISS Interface, … (selecting one opens the type-specific
  field form).
- **Edit Interface** (`C-e`) and **Open Text Editor** (`C-w`, opens the config
  in an embedded terminal editor) not yet exercised — capture when needed.
- No gonomadnet diff attempted for Channels/Interfaces yet (the gonomadnet run
  so far only exercised Conversations); diff these against gonomadnet when we
  switch runtimes.

## Previously-unknown node — first contact (Mac ↔ Mac Mini, never conversed)
(captures `40`–`46`)

A clean control (two nodes that had never conversed) to verify the
trust-contamination-sensitive bugs. Mac Mini M2 (`glenn-mac-mini-m2`, nomadnet
1.2.8 via `pip3 install nomadnet`, `colormode = 24bit`, on the same public RNS
transports) is the previously-unknown node; its LXMF is
`712ffbfdb82c7fe60d0c5fa163ad2955`. The Mac's LXMF is `2a6105…` and its
self-announced display name is "Go port of NomadNet".

- **B1 re-confirmed clean:** after `C-n` Create (Mac adds Mac Mini as Trusted),
  nomadnet 1.2.8 still does **NOT** auto-open — right pane "No conversation
  selected", `✓ MacMini` added to Trusted (5→6). So B1 is NOT a
  trust-contamination artifact; it holds for a never-conversed node.
- **Opening the new conversation** required selecting the peer in the list and
  pressing Enter (clicking the `✓ MacMini` row opened it). Header:
  `MacMini | ◷ 2 hops` — the manually-set name (B2: nomadnet uses the name).
- **Send (Mac→Mac Mini):** outgoing `✓ → just now | 2026-08-23 17:26:54 ⚿`
  (B7: nomadnet `✓ →`). Delivered to the Mac Mini (on-disk conversation
  `…/conversations/2a6105…` created).
- **On the previously-unknown side (Mac Mini), the new peer lands in the
  UNTRUSTED list** (click the `[ Untrusted (N) ]` tab to see it): `? Go port of
  NomadNet` — the `?` is the **unknown-trust** symbol and the name is the peer's
  **announce-derived display name** (not a manually-set name) — with
  `<2a6105f57145860441a62fe3b2a1352c>  (1)` (hash + unread count).
- **R3 refined (untrusted-peer warning):** opening the conversation shows a
  **header warning line** ` This peer isn't trusted yet.` (with a warning glyph)
  directly under the header `Go port of NomadNet |  unknown`, then the message
  `✓ ← 2m ago | 2026-08-23 17:26:54 ` + text. So nomadnet 1.2.8's untrusted
  warning is a **header banner**, and the message marker is still the normal
  `✓ ← …` (NOT the inline `⚠ ← Unknown Origin` + `!` status that gonomadnet
  showed). This is a concrete R3 parity difference to record.
- **Composer is active even for an untrusted peer** (bar `[C-d] Send …`) —
  nomadnet lets you reply to an untrusted peer without first trusting them.
- **B6 confirmed gonomadnet-specific (the big one):** the Mac Mini had **no
  path** to the Mac (header `Go port of NomadNet |  unknown`). On sending the
  reply (`C-d`), nomadnet Python **learned the path on send** — the header
  flipped to `Go port of NomadNet |  2 hops` — and the reply delivered
  (`✓ → just now | 2026-08-23 17:30:06 `, arrived on the Mac as
  `✓ ← just now | 2026-08-23 17:30:06 ⚿`). In the gonomadnet run the Linux side
  stayed `Mac | ◷ unknown` and the reply never sent. So gonomadnet fails to
  resolve/send a path on reply where nomadnet Python succeeds.
- **R2 confirmed:** the Mac's open MacMini conversation auto-refreshed when the
  reply arrived (no re-select needed).