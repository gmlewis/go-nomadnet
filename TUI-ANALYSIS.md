# Nomad Network TUI — Original vs. Go Port: Analysis & Fix Plan

> **Task:** Run the source-of-truth Python `nomadnet` (v1.2.6, urwid) and the Go port
> (`gonomadnet.sh`), exercise each TUI headlessly across keystrokes and terminal sizes,
> record how each renders and behaves, and document how to fix the Go port so it matches
> the original.
>
> **Scope:** analysis only. No existing source files were modified. All driving was done
> against live binaries; captures are reproduced inline below.

---

## 0. Methodology

Both TUIs were driven **headlessly** inside detached `tmux` sessions (`tmux -f /dev/null
new-session -d -x <W> -y <H>`), which provides a real PTY that urwid and tview/tcell both
require. Keystrokes were injected with `tmux send-keys`; the screen was captured with
`tmux capture-pane -p` (plain) and `capture-pane -e` (with ANSI SGR escapes).

Because plain captures hide focus/highlight (urwid and tview express focus via color and
reverse-video, not glyphs), a small ANSI decoder (`/tmp/nn_capture/ansiview.py`) was used
to recover the **style of every cell** — foreground/background RGB, bold, italic,
underline, reverse — so that highlight movement and color application could be observed
without a human at a terminal.

Sizes exercised: **135×32** (the original’s own recommended minimum), **80×24** (small),
and resize probes. Keystrokes exercised: `Up/Down/Left/Right`, `Tab`, `Enter`, `Ctrl-N`,
`Ctrl-Q`, and digit keys.

Config: the original was run with a throwaway `--config` dir so the first-run Guide
appears on demand; the Go port was run with `-t -config <tmpdir>`. Both used their
**default shipped config** (dark theme, 24-bit colormode, Nerd Font glyphs, mouse on,
1 s intro splash) unless noted.

Source-level analysis of both codebases was also performed (Python under
`/opt/homebrew/lib/python3.14/site-packages/nomadnet/`; Go under this repo’s `tui/`,
`cmd/gonomadnet/`, and `nomadnet/`). Line references below point to those sources.

---

## 1. How the ORIGINAL (Python, urwid) functions

### 1.1 Top-level layout

The root is a `urwid.Frame` (`ui/textui/Main.py:39-97`) with three regions:

```
┌─ header  = MenuDisplay  (MenuColumns of [ Name ] buttons) ──────────────┐
│  󰐻 [ Conversations ] [ Network ] [ Channels ] [ Log ] [ Interfaces ] … │
├─ body    = active sub-display’s widget (swapped per page) ───────────────┤
│  …                                                                       │
├─ footer  = active sub-display’s shortcut bar (urwid.Text, swapped) ──────┤
│  [C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  …                │
└──────────────────────────────────────────────────────────────────────────┘
```

- **Header** = `urwid.AttrMap(MenuColumns, "menubar")` — a `urwid.Columns` of `MenuButton`
  (`urwid.Button` with `button_left='['`, `button_right=']'`), so every item renders as
  `[ Name ]` (`Main.py:35-37, 181-211`). A leading `menu_indicator` `Text` shows a Nerd
  Font glyph (`decoration_menu`, or `unread_menu` when there are unread conversations,
  refreshed every 2 s — `Main.py:216-230`).
- **Body** = one of 9 sub-displays (`SubDisplays`, `Main.py:14-30`), swapped via
  `frame.contents["body"] = ...`.
- **Footer** = the active sub-display’s `shortcuts()` `Text`, swapped via
  `update_active_shortcuts` (`Main.py:145-146`). The Conversations page alone has **three
  different shortcut bars** depending on which sub-region (list / message body / editor)
  has focus (`Conversations.py:1765-1781`).

On **first run** the active display is the **Guide**; otherwise **Conversations**
(`Main.py:27-30`). An `IntroDisplay` (big “Nomad Network” banner) is shown for
`intro_time` seconds (default 1 s) then swapped to the main display
(`ui/TextUI.py:223-232`).

### 1.2 The menu / tab system and how it is driven

This is the part the Go port gets most wrong, so it is documented precisely.

- The menu is **not** activated by `Left/Right` from the body. `Left/Right` **inside the
  menu** moves focus across the `[ Name ]` buttons (urwid `Columns` default).
- From the body, pressing **`Up` at the top of any list** sets
  `frame.focus_position = "header"` — i.e. focus escapes *up* into the menu bar. This
  pattern is repeated in every page’s list `keypress` (Conversations, Network, Channels,
  Guide `TopicList`, Interfaces, Log, Config).
- Once the header has focus: **`Left`/`Right`** move between buttons, **`Enter`/`Space`**
  activates the focused button (`on_press` → `show_<page>` or `quit`).
- **`Tab`** or **`Down`** from the menu moves focus back into the body
  (`MenuColumns.keypress`, `Main.py:171-176`).
- **`Ctrl-Q`** quits from anywhere (`TextUI.unhandled_input`, `ui/TextUI.py:262-266`).
  The `Quit` button also quits (`Main.py:158-168`).
- There are **no digit-prefix menu shortcuts** in the original.

Menu items, in on-screen order (8 items, leading with Conversations):
`[ Conversations ] [ Network ] [ Channels ] [ Log ] [ Interfaces ] [ Config ] [ Guide ] [ Quit ]`
(`Main.py:201-204`; Guide omitted if `hide_guide`).

**Verified by driving the live binary:** from the Conversations page, `Up` → menu,
`Right` ×N → target button, `Enter` → page. All six content pages were captured this way
(§1.4).

### 1.3 Keybindings (comprehensive)

**Global**

| Key | Action | Source |
|---|---|---|
| `ctrl q` | Quit | `ui/TextUI.py:263` |
| `tab` / `down` (from menu) | focus → body | `Main.py:173` |
| `left` / `right` (in menu) | move focus across menu buttons | urwid Columns |
| `enter` / `space` (in menu) | activate focused menu button | urwid Button |
| `up` (at top of a body list) | focus → menu header | per-page list `keypress` |

**Conversations** (`ConversationsArea.keypress`, `Conversations.py:88-117`)
- List: `C-e` peer info · `C-x` delete · `C-n` new · `C-u` ingest URI · `C-r` sync ·
  `C-g` fullscreen · `C-o` sort · `C-p` my LXMF/QR · `tab` → menu.
- Editor (`MessageEdit.keypress`, `Conversations.py:1807-1829`): `C-d` send · `C-p`
  paper msg · `C-f` attach · `C-s` save attachments · `up` at top → message list.
- Body (`ConversationWidget.keypress`, `Conversations.py:2218-2244`): `tab` toggle
  editor↔message list · `C-w` close · `C-u` purge failed · `C-t` toggle title ·
  `C-x` clear history · `C-g` fullscreen · `C-o` sort · `C-a` attach · `C-s` save.

**Network** (`BrowserFrame.keypress` `Browser.py:20-70`; `NetworkLeftPile.keypress`
`Network.py:1598-1617`)
- `C-l` toggle Known Nodes ↔ Announce Stream · `C-g` fullscreen · `C-e` node info ·
  `C-p` reinit LXMF peers · `C-w` disconnect · `C-d` back · `C-f` forward · `C-r` reload ·
  `C-u` URL dialog · `C-s`/`C-b` save node · `C-y` copy URL · `C-x` remove entry.

**Channels** (`ChannelsListArea.keypress`, `Channels.py:352-378`; `RoomMessageEdit.keypress`,
`Channels.py:418-437`; `RoomFrame.keypress`, `Channels.py:522-550`)
- List: `C-n` new hub · `C-a` join room · `C-r` connect · `C-w` disconnect ·
  `C-t` auto-reconnect · `C-e` edit hub · `C-x` remove · `C-y` toggle channel list ·
  `F8` collapse join/part · `tab` → menu.
- Room editor: `tab` nick-complete · `C-d` send · `C-x` leave · `F8` collapse.
- Room body: `C-x` leave · `C-u` toggle users · `C-y` toggle channel list · `F8` ·
  `tab` → editor.

**Interfaces** (`InterfaceFiller.keypress`, `Interfaces.py:1397-1415`)
- `C-a` add · `C-x` remove · `C-e` edit · `C-w` open config editor · `enter` show
  interface. Detail view: `tab`/`shift-tab` cycle focus, `h`/`v` horizontal/vertical
  charts.

**Guide** (`Guide.py`): `TopicList` — `up` at first item → menu; `enter`/click on a
`GuideEntry` opens the topic. `GuideColumns` — `left`/`right` move focus between the
topic list and the reader. `LinkableText` — `left`/`right` move the cursor between styled
“parts”; on a link part, `enter`/click activates it; `up`/`down` scroll; `left` at
position 0 releases focus back to the topic list.

**Log / Config**: embedded `urwid.Terminal` (`tail -fn50 logfile`; `editor configpath`)
with `escape_sequence="up"` so `up` escapes the terminal back to the menu.

**Readline editing** (every `ReadlineEdit`/`ReadlineMixin` field,
`ReadlineEdit.py:54-71`): `C-a`/`C-e` line start/end · `C-u` kill to start · `C-k` kill
to end · `C-w` kill word · `C-l` kill buffer · `C-y` yank · `C-left`/`C-right` word motion.
A module-global kill ring shares kills across widgets.

**List scrolling** (`IndicativeListBox`, `MODIFIER_KEY.NONE`): plain
`up/down/pgup/pgdn/home/end` move the selection; `return_unused_navigation_input=True`
propagates an `up` at the top to the parent (the menu-escape mechanism). `▲`/`▼`
indicators show when the list is scrolled.

### 1.4 Pages and their layouts (live captures)

All captured at **135×32** with default dark/truecolor/Nerd-Font config. Box-drawing is
single-line `┌─┐`; the menu indicator is the Nerd Font glyph `󰐻`.

**Conversations** (default landing page) — fixed-width left list (52 cols) + weighted
right detail, each in its own `LineBox`; sub-tabs `[ Trusted (0) ] [ Untrusted (0) ]`:

```
 󰐻 [ Conversations ] [ Network ] [ Channels ] [ Log ] [ Interfaces ] [ Config ] [ Guide ] [ Quit ]
┌────────────────── Conversations ─────────────────┐┌─────────────────────────────────────────────────────────────────────────────────┐
│[ Trusted (0)           ] [ Untrusted (0)        ]││                                                                                 │
│                        ───                       ││  No conversation selected                                                       │
│             No trusted conversations             ││                                                                                 │
└──────────────────────────────────────────────────┘└─────────────────────────────────────────────────────────────────────────────────┘
[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen
```

**Network** — fixed-width left “Saved Nodes” pane + weighted right “Remote Node” browser
pane, **separate borders**, Nerd Font node glyph `󰙎`, browser shows `Disconnected` with
`← →` nav hints:

```
┌─────────────────── Saved Nodes ──────────────────┐┌────────────────────────────────── Remote Node ──────────────────────────────────┐
│                         󰙎                        ││                                                                                 │
│           Currently, no nodes are saved          ││                                                                                 │
│        Ctrl+L to view the announce stream        ││                                   Disconnected                                  │
│                                                  ││                                       ←  →                                      │
└──────────────────────────────────────────────────┘│                                                                                 │
```

**Channels** — list + room pane (“Select or add a hub to begin”). **Log** — embedded
`tail` of the logfile with timestamps. **Interfaces** — list + detail, rounded border
`╭─╮`, switches to horizontal/vertical charts by width. **Config** — message + “Open
Editor” button launching `$EDITOR` on the config. **Guide** — `Topics` sidebar + reader
(§1.5).

**Dialog (Ctrl-N on Conversations)** — a **true `urwid.Overlay`** painted over the list;
the list remains visible underneath. Fields `Addr`, `Name`, trust radio buttons
`(X) Untrusted / (X) Unknown / ( ) Trusted`, and `< Create >  < Back >` buttons:

```
│ ┌────────────── New Conversation ──────────────┐ │
│ │Addr :                                        │ │
│ │Name :                                        │ │
│ │(X) Untrusted                                 │ │
│ │(X) Unknown                                   │ │
│ │( ) Trusted                                   │ │
│ │< Create            >     < Back             >│ │
│ └──────────────────────────────────────────────┘ │
```

### 1.5 Guide: two-pane Topics + micron reader

```
┌────────────────── Topics ─────────────────┐┌────────────────────────────────────────────────────────────────────────────────────────┐
│                    ───                    ││First Time Information                                                                 ┃│
│Introduction                               ││Hi there. This first run message will only appear once. …                              ┃│
│Concepts & Terminology                     ││To get the most out of Nomad Network, you will need a terminal that supports UTF-8 and  ┃│
│…                                          ││It is recommended to use a terminal size of at least 135x32. …                          ┃│
│Keyboard Shortcuts                         ││                                                                                       ┃│
│…                                          ││     - Cryptography.io by pyca                                                         ┃│
└───────────────────────────────────────────┘└────────────────────────────────────────────────────────────────────────────────────────┘
```

The selected topic carries a **focus highlight** (black fg on ~`#aaa` bg). The reader
renders **micron markup**: section headings are dark-on-light (`#222` on `#bbb`), body
text is `#ddd`, **bold** (`Cryptography.io`), **italic** (`pyca`), links are styled and
**focusable/clickable** (`LinkableText` + `LinkSpec`), `─` dividers, and `#anchor` jumps.

### 1.6 Colors / highlighting

- A 5-tuple urwid palette (16-color, monochrome, 88/256/true-color) per style, registered
  via `screen.register_palette` (`ui/TextUI.py:18-125, 218-219`). Default theme dark,
  colormode **24-bit** (shipped config), so emits `ESC[38;2;r;g;b;48;2;r;g;b`m.
- **Colormode is config-driven** (`NomadNetworkApp.py:952-966`): `monochrome/16/88/256/
  24bit`. `set_colormode` calls `screen.set_terminal_properties(colormode)` and, below
  256, resets/restores the terminal palette (`ui/TextUI.py:254-260`). Micron styles are
  **synthesized per-depth** from the 5-tuple at render time (`MicronParser.make_style`,
  `MicronParser.py:442-591`).
- Concrete observed styles (truecolor, dark):
  - `menubar` / `shortcutbar`: `#111` on `#bbb` (black on light grey) — the menu and
    footer both have a filled background.
  - `list_focus`: `#111` on `#aaa` — focused list row. `list_off_focus`: `#111` on `#777`
    — selected row when the list is not focused.
  - **Trust-based row colors**: `list_trusted` `#6b2`, `list_untrusted` `#a22`,
    `list_unresponsive` `#b92`, `list_unknown` `#bbb`, each with a `list_focus_*`
    variant. Untrusted/unresponsive peers are background-colored across the whole row.
  - Message headers by state: `msg_header_ok` green, `_caution` amber, `_sent` grey,
    `_propagated`/`_delivered` blue, `_failed` grey; trust banner
    `msg_warning_untrusted` red.
  - RRC/chat: `irc_nick_self` `#6c5`, `irc_nick_peer` `#3cd`, `irc_mention` `#fb4` bold,
    `irc_notice` `#fd3`, etc.
- The **Nerd Font menu indicator** `󰐻` and node glyph `󰙎` render from the `nerdfont`
  glyph set (`ui/TextUI.py:140-172`); `plain`/`unicode` fallbacks exist.
- The “Display Test” is a Guide topic with micron gradient bars + a Nerd Font rendering
  test for visual verification — there is no programmatic auto-detection.

### 1.7 Rendering approach

- `urwid.raw_display.Screen()`, UTF-8 forced, `MainLoop(..., handle_mouse=True)`
  (`ui/TextUI.py:218-229`). Mouse: click menu buttons, list entries, links, pane gutters,
  expand gutters (`MainFrame.mouse_event` tracks focus; `ClickableIcon`,
  `ListEntry`, `LinkableText` emit `click`).
- Widgets reflow to the terminal size; no enforced minimum, but the Guide recommends
  **135×32**. At **80×24** the app still works: the menu bar clips the last items, the
  two-column body survives, and the shortcut bar wraps to two lines:
  ```
  … [C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o]
  Sort  [C-p] My LXMF  [C-g] Fullscreen
  ```
- Threading: `loop.watch_pipe` wakes urwid from RRC/message background threads
  (`Conversations.py:238-275`, `Channels.py:1471-1476`). Periodic `set_alarm_in` jobs
  drive the menu unread indicator, announce stream, sync status, and bandwidth charts.

---

## 2. How the GO PORT (`tview`/`tcell`) currently behaves

### 2.1 Architecture

- **Library:** `rivo/tview` v0.42.0 on `gdamore/tcell/v2` v2.8.1 (`go.mod`) — *not* urwid.
  Entrypoint `cmd/gonomadnet/main.go` → `runTextUI` (`cmd/gonomadnet/textui.go:33`) →
  `tui.NewApp` (`tui/app.go`) → `MainDisplay` (`tui/main.go`).
- Root is a `tview.Flex` (row): **menuBar** `TextView` (1 row) + **content** `tview.Pages`
  + **shortcutBar** `TextView` (1 row) (`tui/main.go:113-117`).
- Input: one app-level `SetInputCapture` → `MainDisplay.handleInput`
  (`tui/main.go:228-296`), plus a `SetInputCapture` on each display’s root flex.

### 2.2 What it actually renders (live capture, 135×32, Network page)

```
 Network  Conversations  Channels  Directory  Map  Log  Config  Interfaces  Guide  Quit
╔═════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗
║                                                               Network                                                               ║
║                                                                                                                                     ║
║Ⓝ IceNet                                            Select an announce or node to view details                                       ║
║11:40:24 — IceNet                                                                                                                    ║
║Ⓟ Web Node                                                                                                                           ║
║11:40:25 — <U+FFFD><U+FFFD>Web Node                                                                                                  ║
║Ⓝ Flummoxx                                                                                                                           ║
║Ⓟ Jouer Zork-1 Francais                                                                                                              ║
║11:40:36 — <U+FFFD><U+FFFD>Jouer Zork-1 Francais<U+FFFD><U+FFFD>                                                                      ║
╚═════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝
[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen
```

Observations vs. the original:

1. **Menu bar — wrong structure.** 10 items (`Network, Conversations, Channels, Directory,
   Map, Log, Config, Interfaces, Guide, Quit`), **leads with Network**, adds `Directory`
   and `Map` (which are not top-level pages in the original), and drops the `[ Name ]`
   button decoration and the leading Nerd Font indicator glyph. The original has 8 items,
   leads with Conversations, and renders `[ Conversations ] [ Network ] …`. The selected
   item is shown bold with `list_focus_bg` (`#aaa`); the rest use `menubar_bg` (`#bbb`).
2. **Body — one box, not two panes.** A single **double-line** `╔═╗` border (the original
   uses single-line `┌─┐`). The Network page has no “Saved Nodes | Remote Node” split;
   the announce list spans the full width and the “Select an announce or node to view
   details” hint is drawn on the same row as the first list item, overlapping it.
3. **UTF-8 glitches.** Names with non-ASCII bytes render as `U+FFFD` replacement chars
   (`<U+FFFD><U+FFFD>Web Node`). The original handles UTF-8 cleanly.
4. **Selection color is hardcoded.** The focused row uses `#666666`
   (`SetSelectedBackgroundColor(0x666666)`), not the theme’s `list_focus_bg` (`#aaa`), and
   there is **no trust-based row coloring** — trust is only an icon prefix (`Ⓝ`/`Ⓟ`).
5. **Wrong shortcut bar.** The Network page shows the **Conversations** shortcuts
   (`[C-e] Peer Info [C-x] Delete [C-r] Sync …`). The original Network footer is
   `[C-l] Nodes/Announces [C-x] Remove [C-w] Disconnect [C-d] Back [C-f] Forward [C-r]
   Reload [C-u] URL [C-y] Copy [C-g] Fullscreen [C-s / C-b] Save Node`.

### 2.3 Keystroke behavior (live)

- **`Right` switches the whole page** (Network → Conversations). `Left`/`Right` are
  globally hijacked by `MainDisplay.handleInput` to cycle the menu
  (`tui/main.go:247-258`, `return nil` consumes the key). **This is the single biggest
  behavioral regression:** in the original, `Left`/`Right` move focus *between panes
  inside a page* (list ↔ detail, channel list ↔ users). In the Go port they can never do
  that — they change the entire page.
- **`Down`/`Up`** are not intercepted at the app level, so they do navigate `tview.List`
  rows natively (verified: the highlight moves in the Network announce list). But because
  each page is a single list+detail rather than a real multi-pane Columns, lateral
  navigation is impossible.
- **Digit keys `1`–`9`/`0`** select menu pages directly (`tui/main.go` `KeyRune` cases) —
  a feature the original does **not** have, and one that collides with any future
  per-page digit shortcuts.
- **`Ctrl-Q`, `Esc`, `q`** quit (`tui/main.go`). `Esc` quitting globally also conflicts
  with the original, where `Esc` closes dialogs and `Ctrl-Q` is the only global quit.
- **`Tab`** is passed through (`tui/main.go:260`) but **no display implements Tab
  focus-cycling between sub-widgets** (the original’s `tab` toggles editor↔message list,
  moves to menu, nick-completes, etc.).

### 2.4 Colors / highlighting

- `tui/theme.go` holds two static maps (`darkColors`/`lightColors`, ~50 named entries) of
  **hardcoded 24-bit hex** `tcell.NewHexColor` values. `RegisterThemeStyles`
  (`tui/theme.go:283-287`) is an explicit **no-op**:
  ```go
  func RegisterThemeStyles(app *tview.Application, theme int) {
      // This function is a placeholder for theme initialization
  }
  ```
- There is **no color-depth detection / fallback**. tcell negotiates truecolor and emits
  24-bit SGR regardless of terminal capability; on a 16/256-color terminal the colors
  degrade poorly. The `nomadnet/micron/color-depth.go` package *does* implement
  `monoColor`/`lowColor`/`highColor` matching the Python palette-depth helpers, but **the
  TUI never calls it**; `tui/micron-view.go`’s `mapColor` just maps a handful of names to
  tview tags and passes the rest through.
- No `list_off_focus` (selected-but-unfocused) style, no trust-based row backgrounds, no
  unread blink (the `StartUnreadBlink` goroutine locks/unlocks and never redraws —
  `tui/main.go:330-345`).

### 2.5 Micron rendering

- Parser `nomadnet/micron/micron.go` produces an AST; renderer `tui/micron-view.go`
  flattens it into **one tview color-tagged string** in a single `TextView`.
- **Links are inert** yellow text — no `LinkableText` equivalent, no target carried, no
  enter/click activation, no “Link to …” footer peek.
- **No input fields** (`` `< ``), **no tables**, **no partials**, **no anchor jumps**
  (`jumpToAnchor` is missing though `ExtractAnchors` exists in `tui/guide-links.go`).
- **All-bold Guide bug:** on the Guide page every content line is rendered bold
  (`[::-]`-style) instead of normal body text with only headings emphasized:
  ```
  ║Introduction                                Welcome to Nomad Network                                                 ║
  ║Concepts & Terminology                      NomadNet is a peer-to-peer information sharing system built on Reticulum.  ║
  ║Channels & RRC                              It provides encrypted messaging, relay chat, and a decentralized web of    ║
  ```
  The original uses `body_text` (`#ddd`, normal) for paragraphs and a distinct heading
  style for titles.

### 2.6 Pages status

| Page | Go port status | Notes |
|---|---|---|
| Network | Partial | Real announces via `app.GetAnnounces()`. No two-pane split, no LXMF Peers / LocalPeer / NodeInfo / NetworkStats panels. |
| Conversations | Partial | List + Trusted/Untrusted tabs (digit-prefixed `1`/`2`) + detail `TextView`. **No editor, no send, no trust banner, no attachments.** |
| Channels | Stub | `NewChannelsDisplay(app, nil)` — nil room data; no hub/RRC; shortcuts pop “TODO” dialogs. |
| Directory | **Unwired** | Menu item exists but `textui.go` never calls `SetDisplay("directory", …)`; shows generic placeholder. A real `DirectoryDisplay` widget exists but is unused. |
| Map | **Unwired + not in original** | Menu item exists, no widget, not wired, and not a top-level page in the original. |
| Log | Partial | Static `tailFile` read; no live tail. |
| Config | Partial | Prints the config path; no in-app editor. |
| Interfaces | Partial | **Hardcoded** sample data (`Michmesh Testnet …`); no add/edit/show forms, no real RNS enumeration. |
| Guide | Partial | Two-pane list+content (closer to original) but all-bold content, ~10 hardcoded topics, no anchor jump. |
| Browser | Stub | `LoadURL`/`Back`/`Forward` exist but **no RNS fetch**; history is empty. |
| Quit | Splash | Maps to `NewIntroDisplay` splash, not a quit action. |

### 2.7 Dialogs / popups

- `tui/dialog.go` `ShowDialog` uses `app.SetRoot(dialog, true)` — it **replaces the root
  primitive** instead of overlaying. The underlying screen state is lost and not restored
  on dismiss. The original uses `urwid.Overlay` (true modal, background preserved).
- **Live proof (Ctrl-N on Conversations):** the entire screen becomes just
  ```
  Address (hex hash):
                               Save                                Cancel
  ```
  No Conversations list underneath, and the form is stripped down (no Name field, no
  trust radios) vs. the original’s Addr/Name/Untrusted-Unknown-Trusted/Create/Back.
- `Esc` dismisses via `DialogLineBox.InputHandler`; but because `Esc` also quits globally
  at the app level, the dismiss-vs-quit precedence is fragile.

### 2.8 Build / run status

- `go build ./...` and `go vet ./...` **pass** (clean).
- `bash gonomadnet.sh` **runs and renders** (captured above). It initializes Reticulum
  and pipes real announce data into the Network list.
- During this analysis the Go tmux session **terminated on a window resize** (the
  80×24 resize probe killed the process). The original survives resize and reflows. This
  is an additional robustness gap worth investigating (a panic in a `Resize`/`Draw` path,
  or the concurrent edits to the repo at the time).

---

## 3. Side-by-side comparison

| Aspect | Original (urwid) | Go port (tview) |
|---|---|---|
| Root layout | Frame: header menu / body sub-page / footer shortcuts | Flex: menuBar TextView / Pages / shortcutBar TextView |
| Menu items | 8, `[ Conversations ] … [ Quit ]`, leading indicator glyph | 10, plain words, leads with Network, adds Directory+Map, no glyph |
| Menu activation | `Up`→menu, `Left/Right` across buttons, `Enter` | `Left/Right` globally cycles pages; digits `1-9`; `Enter` not used |
| `Left/Right` in body | move focus between panes | **switch entire page (hijacked)** |
| Page body | per-page two-pane (fixed list + weighted detail), separate single-line borders | single double-line box; list+detail overlap |
| Focus highlight | `list_focus` `#111/#aaa`; `list_off_focus`; trust-colored rows | hardcoded `#666` selected bg; no trust coloring |
| Shortcut bar | per-page and per-focus-region (3 on Conversations) | Conversations shortcuts shown on every page (bug) |
| Colors | 5-tuple palette, depth-aware (mono/16/256/true) | hardcoded 24-bit; `RegisterThemeStyles` no-op; no depth detection |
| Micron | structured AttrMap rows; clickable links; fields; tables; partials; anchors | one flat TextView; inert links; no fields/tables/partials; all-bold bug |
| Dialogs | `urwid.Overlay` (background preserved); Esc closes | `SetRoot` root-swap (background lost); stripped forms |
| Quit | `Ctrl-Q` (and Quit button) | `Ctrl-Q`, `Esc`, `q` (Esc conflicts with dialog close) |
| UTF-8 / glyphs | clean; Nerd Font indicator + node glyphs | `U+FFFD` glitches; circled-letter icons |
| Resize | reflows (survives 80×24) | process died on resize during testing |
| Mouse | click menu, list, links, gutters | menu-bar click only (13/14 interactions missing per PARITY-REPORT) |

---

## 4. HOW TO FIX the Go port (prioritized, concrete)

### P0 — Stop breaking the original’s core interaction model

1. **Stop hijacking `Left/Right` globally.** In `tui/main.go:247-258`, do **not** consume
   `Left/Right` to cycle the menu. Instead, replicate the original’s model:
   - `Up` at the top of a page’s primary list → focus the menu bar.
   - `Left/Right` **within the menu bar** move between items; `Enter` activates.
   - `Tab`/`Down` from the menu → focus the body.
   - `Left/Right` **within a page** move focus between its panes (list ↔ detail,
     channel list ↔ users, topic list ↔ reader).
   - Keep `Ctrl-Q` as the only global quit; remove `Esc`/`q` as global quits so `Esc` can
     resume its role as the universal dialog-closer.
   - Remove the `1`–`9` digit menu shortcuts (or repurpose them only inside pages) so
     they don’t collide with per-page digit shortcuts later.

2. **Fix the menu structure** (`tui/theme.go:296-307`, `tui/main.go`):
   - 8 items in the original order: `Conversations, Network, Channels, Log, Interfaces,
     Config, Guide, Quit`.
   - Render each as `[ Name ]` to match the original (or at least keep the brackets).
   - Add the leading menu-indicator glyph from the configured glyph set
     (`decoration_menu` / `unread_menu`) and implement the 2 s unread-blink job so it
     actually redraws (`tui/main.go:330-345`).
   - `Directory` and `Map` are **not** top-level pages in the original — either remove
     them from the menu or demote them to sub-views (the original’s Map/Directory
     displays are placeholders inside their own pages). At minimum, do not present them as
     peers to the real pages.

3. **Make dialogs true overlays, not root-swaps** (`tui/dialog.go`):
   - Replace `ShowDialog`’s `app.SetRoot(dialog, true)` with a modal overlay that
     preserves the underlying screen — e.g. `tview.NewModal`-style or a `pages` layer
     that paints the dialog on top of the current page and restores it on dismiss.
   - Restore the full original dialog forms: New Conversation must have `Addr`, `Name`,
     and trust radios `( ) Untrusted / ( ) Unknown / ( ) Trusted` plus `Create`/`Back`.
   - Wire `Esc` to close the dialog (not quit the app) via the `DialogLineBox` handler,
     and ensure dismiss restores the previous focus/page.

### P1 — Layout, shortcuts, and color/highlight parity

4. **Per-page two-pane layouts.** Rebuild each content page as a `tview.Flex` (column)
   with a fixed-width left list and a weighted right detail, each in its own
   **single-line** `┌─┐` bordered `tview.Box` (not `╔═╗`):
   - Conversations: list (52) + detail, with `[ Trusted (0) ] [ Untrusted (0) ]` sub-tabs
     (no digit prefixes).
   - Network: “Saved Nodes” (52) + “Remote Node” browser; add the Announce Stream /
     Known Nodes toggle (`Ctrl-L`) and LXMF Peers panel.
   - Channels: list (36) + room; Users pane toggle (`Ctrl-U`).
   - Implement `Ctrl-G` fullscreen toggle (left width → 0) per page.
   - Fix the Network overlap: the “Select an announce…” hint must live in the right
     detail pane, not on the same row as list items.

5. **Wire the correct shortcut bar per page.** In `cmd/gonomadnet/textui.go:195` and
   `tui/main.go:131-136`, stop unconditionally returning
   `conversationsDisplay.GetShortcutText()`. Each display must supply its own shortcut
   text matching the original (see §1.3), and Conversations must switch between its three
   bars based on which sub-region has focus.

6. **Color/highlighting parity** (`tui/theme.go`, `tui/micron-view.go`):
   - Wire `nomadnet/micron/color-depth.go` into the TUI: detect terminal color depth via
     tcell and select mono/16/256/true variants per style (delete the `RegisterThemeStyles`
     no-op and implement it). Make `colormode` config-driven like the original.
   - Use the theme’s `list_focus_bg` (`#aaa`) for selected rows, not `#666666`. Implement
     `list_off_focus` for selected-but-unfocused rows.
   - Apply **trust-based row backgrounds** (`list_trusted` `#6b2`, `list_untrusted`
     `#a22`, `list_unresponsive` `#b92`) across the whole row, not just an icon prefix.
   - Apply `menubar`/`shortcutbar` background fill and the menu selected-vs-unselected
     distinction; render the Nerd Font indicator/node glyphs from the configured glyph
     set (with `plain`/`unicode` fallbacks).

### P1/P2 — Micron interactivity

7. **Render micron as structured, focusable content** (`tui/micron-view.go`):
   - Instead of one flat `TextView` string, produce a list of styled rows (tview
     `TextView` per row, or a `List`/`Flex` of `Text` widgets) so per-line focus and
     link cursoring work.
   - Implement `LinkableText`/`LinkSpec` equivalents: links are focusable parts;
     `Left/Right` move the cursor between parts; `Enter`/click activates the link;
     `Up/Down` scroll; `Left` at position 0 releases focus back to the topic list; a
     2 s key-timeout shows/hides the cursor and peeks the focused link in the footer
     (“Link to …”).
   - Render input fields (`` `< `` → `tview.InputField`/`CheckBox`/`RadioButton`),
     tables, partials, and `#anchor` jumps (`jumpToAnchor`).
   - Fix the **all-bold Guide content bug**: body text must be normal (`body_text`),
     only headings emphasized; use the heading styles (`heading1/2/3…`) by section depth.

### P2 — Per-widget key handlers, page completeness, robustness

8. **Fine-grained per-sub-widget input handlers.** The original dispatches keys per
   sub-widget (`ConversationsArea`, `ConversationWidget`, `ChannelsListArea`,
   `RoomMessageEdit`, `UsersBox`, `BrowserFrame`, `InterfaceFiller`, …). The Go port
   only has coarse per-display `SetInputCapture`. Add the missing handlers (PARITY-REPORT
   §1 lists them as MISSING), including `Tab` focus cycling, nick-completion
   (`RoomMessageEdit` `Tab`), and `ReadlineMixin` kill/yank keys (`C-a/e/u/k/w/l/y`,
   `C-left/right`) shared via a global kill ring.

9. **Page completeness:**
   - **Conversations:** message editor with send (`C-d`), paper msg (`C-p`), attach
     (`C-f`), save (`C-s`); trust banner; purge failed (`C-u`); QR dialog (`C-p`).
   - **Browser:** actually fetch micron pages over Reticulum (`LoadURL` → RNS retrieve);
     back/forward/reload (`C-d/C-f/C-r`); URL dialog (`C-u`); save node (`C-s/C-b`).
   - **Channels:** connect to a hub via RRC; room widget with message list and users
     pane; join/part collapse (`F8`).
   - **Interfaces:** enumerate real RNS interfaces; add/edit/show forms; bandwidth charts
     (horizontal/vertical by width).
   - **Config:** launch `$EDITOR` on the config (embedded terminal or spawn) and return.
   - **Log:** live `tail -f` instead of a static read.

10. **Mouse parity:** click list entries, links, pane gutters, and expand gutters
    (PARITY-REPORT: 13/14 mouse interactions missing).

11. **Robustness / polish:**
    - Fix the **UTF-8 replacement-char glitch** (decode announce/source names as UTF-8,
      not byte-wise).
    - Use **single-line `┌─┐` borders** to match the original’s look (or make the glyph
      set configurable: `plain`/`unicode`/`nerdfont`).
    - Investigate and fix the **resize crash** observed during testing (the process died
      on `tmux resize-window`); the original reflows cleanly.
    - Add the 1 s **intro splash** (`Extras.IntroDisplay`) for parity.
    - Respect the **135×32 recommendation** (the Guide already mentions it); ensure
      small terminals reflow rather than break.

---

## 5. Captures (reproducible artifacts)

Driving harness: `/tmp/nn_capture/harness.sh` (tmux PTY + `send-keys` + `capture-pane`).
ANSI/style decoder: `/tmp/nn_capture/ansiview.py`. Captures live under
`/tmp/nn_capture/{orig,orig_pages,orig_dialog,orig_kb,orig_sizes,goport}/`.

Key frames referenced above:
- Original first-run Guide: `orig/guide_135x32_00.txt`
- Original Network/Channels/Log/Interfaces/Config/Guide pages: `orig_pages/pg_135x32_{03,06,09,12,15,18}_*.txt`
- Original New Conversation dialog: `orig_dialog/dlg_135x32_01_esc.txt`
- Original at 80×24: `orig_sizes/small_80x24_00_esc.txt`
- Go port Network page: `goport/go_135x32_00_esc.txt`
- Go port after `Right` (page switch): `goport/go_135x32_01_right_esc.txt`
- Go port Guide page: `goport/go_135x32_03_guide_esc.txt`
- Go port Ctrl-N “dialog” (root-swap): `goport/go_135x32_04_dialog_esc.txt`

To reproduce a capture:
```bash
tmux -f /dev/null new-session -d -x 135 -y 32 -s nn \
  "nomadnet --config /tmp/cfg; sleep 60"
sleep 5                                   # let it boot
tmux send-keys -t nn Up Right Enter       # menu → Network
tmux capture-pane -t nn -p -e > frame.txt # -e keeps SGR colors
/opt/homebrew/bin/python3 /tmp/nn_capture/ansiview.py frame.txt   # decode styles
```

---

## 6. Bottom line

The Go port **compiles and runs** and successfully streams real Reticulum announce data
into a list — it is not a dead scaffold. But as a TUI it does **not** behave like the
original:

- The **interaction model is broken at the app level**: `Left/Right` switch pages
  instead of moving focus between panes, so every multi-pane navigation the original
  supports is impossible.
- The **menu is the wrong shape** (10 items, wrong order, no `[ ]` decoration, no
  indicator glyph, two non-existent top-level pages).
- **Dialogs destroy the screen** (root-swap) instead of overlaying, and are stripped of
  fields.
- **Highlighting is wrong**: hardcoded selection color, no trust-based row coloring, no
  color-depth adaptation, wrong shortcut bar on most pages.
- **Micron is non-interactive flat text** with an all-bold rendering bug; links, fields,
  tables, and anchors don’t work.
- Most pages are **stubs** (Channels, Browser, Directory, Map, Config, Interfaces) and
  **UTF-8 rendering glitches** and a **resize crash** remain.

The fix plan in §4 is ordered so that the P0 items (§4.1–4.3) alone would transform the
port from “feels nothing like the original” to “correct core interaction model with real
dialogs,” and the P1 items (§4.4–4.7) would restore visual and micron parity. The P2
items close the remaining per-widget and page-completeness gaps.