# Go NomadNet — Port Completion Task List (TDD)

## CRITICAL RULES

- **One function/method per session.** Each task targets a single function, method, or small cohesive unit.
- **Tests first, always.** Write a failing Go test with expected outputs captured from the Python source (run Python in `/tmp` when needed) before implementing.
- **`go test` is the lie detector.** Run `go test ./...` (set `GOCACHE=/tmp/go-cache` on this host) after every change. Never claim completion without passing test output.
- **Never self-assess parity.** The test suite is the only measure of progress.
- **TODO.md must shrink over time.** When a task is completed, REMOVE it entirely. Do not mark "done".
- **Verify against Python** at `/Users/glenn/src/github.com/markqvist/nomadnet`; it is READ-ONLY.
- Every new file starts with the standard copyright header (`copyright_header.txt`).
- Prefer hyphens in filenames, `go fmt`, `any` over `interface{}`, `%v` in fmt strings.
- Do NOT create tracking/progress markdown files. This TODO.md is the only tracker.

## Verification Method

Each task: (1) read the Python function → (2) write a failing Go test with values captured
from Python → (3) implement until the test passes → (4) `go test ./...` is green.

---

## CURRENT STATE SUMMARY (as of this analysis)

- ~40k lines of Go across 14 packages; all existing tests pass.
- **Core library packages are partially implemented but contain major gaps** in
  conversation message I/O, directory persistence, app orchestration, RRC networking,
  and micron interactive rendering.
- **The TUI is largely a non-functional scaffold.** `cmd/gonomadnet/textui.go` contains
  ~25 TODO markers and does not actually wire the UI to the core app — most menu actions
  show placeholder dialogs. The browser does not fetch pages over RNS, channels do not
  connect to hubs, conversations cannot send messages, interfaces show hardcoded data.
- Work proceeds bottom-up: finish core library methods first (they are prerequisites for
  the TUI), then the TUI display widgets, then the wiring in `cmd/gonomadnet`.

---

## E. rrc package — RRC hub networking

### E4. Cross-process parity (integration)
- [ ] Integration test: Go RRC hub sends HELLO, a Python subprocess (via a
  temporary TCP RNS transport) responds with WELCOME.
- [ ] Integration test: Python RRC client sends MSG, Go hub receives and records
  the message.
- [ ] Integration test: Go RRC hub sends MSG, Python RRC client receives it.

---

## F. micron package — interactive rendering (CRITICAL for TUI)

The Go `render.go` only produces plain-text/tview color strings. The Python
`MicronParser.py` (1048 lines) produces interactive urwid widgets with link
navigation, anchors, partials, and color depth. The Go port must produce
equivalent tview primitives.

- [ ] `micron.RenderToTView` — render a parsed document to a list of styled
  `tview` text spans with full color (fg/bg), not just plain text. Test a markup
  fixture with colors/headings/tables against Python `markup_to_attrmaps`.
- [ ] Link rendering — render `"text":target` links as clickable/peekable spans
  carrying the target; verify the link target and display text against Python.
- [ ] Field rendering — render `|label|value|` fields as a two-column layout;
  verify against Python.
- [ ] Anchor rendering — render `[#anchor]` and `[>anchor]` anchors; verify
  against Python.
- [ ] Partial detection/rendering — render `>>partial_name` partials; verify
  against Python `parse_partial` and the partial update flow.
- [ ] Code block rendering — render backtick code spans with a code style.
- [ ] Image rendering — render `{image_data}` image placeholders.

---

## I. tui package — display widgets

Each widget must match the Python `ui/textui/*` behavior. Tests exercise the
widget logic with a mocked `tview.Application` (no real terminal).

### I1. Browser (Python Browser.py, 1848 lines)
- [ ] `BrowserDisplay.retrieveURL` — fetch a page over RNS (link establish,
  request, response); test with a mocked transport asserting request bytes.
- [ ] `BrowserDisplay.loadPage` — render a retrieved Micron page; verify
  against Python `load_page`.
- [ ] `BrowserDisplay.downloadFile` — download a file over RNS; verify against
  Python `download_file`.
- [ ] `BrowserDisplay.partialReceived` / `partialProgressed` / `partialFailed` —
  partial update handlers; verify against Python.
- [ ] `BrowserDisplay.updatePartials` — periodic partial refresh; verify against
  Python `update_partials`.
- [ ] `BrowserDisplay.cachePage` / `getCached` / `cleanCache` / `uncachePage` —
  cache management; verify against Python (Go cache exists; audit limits/eviction).
- [ ] `BrowserDisplay.identify` / `disconnect` — link identity and disconnect;
  verify against Python.
- [ ] `BrowserDisplay.saveNodeDialog` — save-node dialog; verify against Python.
- [ ] `BrowserDisplay.urlDialog` — URL entry dialog; verify against Python.
- [ ] `BrowserDisplay.markedLink` / `copyUrl` — link marking and clipboard copy;
  verify against Python.
- [ ] `BrowserDisplay.responseReceived` / `requestFailed` / `requestTimeout` —
  request lifecycle callbacks; verify against Python.
- [ ] `BrowserDisplay.responseProgressed` — progress bar updates; verify against
  Python.
- [ ] `BrowserDisplay.statusText` — status line text; verify against Python.
- [ ] `BrowserDisplay.jumpToAnchor` — scroll to an anchor; verify against Python.

### I2. Conversations (Python Conversations.py, 3093 lines)
- [ ] `ConversationsDisplay.updateListbox` — build the conversation list;
  verify formatting against Python `update_listbox`.
- [ ] `ConversationsDisplay.conversationListWidget` — single list row widget;
  verify against Python `conversation_list_widget`.
- [ ] `ConversationWidget` — full conversation view (trust banner, messages,
  editor, footer); verify against Python `ConversationWidget`.
- [ ] `ConversationWidget.buildTrustBanner` — trust banner; verify against
  Python `_build_trust_banner`.
- [ ] `ConversationWidget.blockPeer` / unblock — verify against Python
  `_block_peer`/`_on_block_click`.
- [ ] `ConversationWidget.sendMessage` — send the editor content; verify against
  Python `send_message`.
- [ ] `ConversationWidget.attachFile` / `fileBrowserClosed` — file attachment
  flow; verify against Python.
- [ ] `ConversationWidget.saveFocusedAttachments` — save selected attachments;
  verify against Python.
- [ ] `ConversationWidget.paperMessage` (print_qr/save_qr/save_uri) — paper
  message flow; verify against Python `paper_message`.
- [ ] `LXMessageWidget` — render a single message (header, body, state, progress);
  verify against Python `LXMessageWidget`.
- [ ] `LXMessageWidget.progressPoll` — outbound message progress polling;
  verify against Python `_poll_progress`.
- [ ] `ClickableAttachment` / `FileBrowserDialog` — attachment save dialog and
  file browser; verify against Python.
- [ ] `ConversationsDisplay.syncConversations` / `updateSyncDialog` — sync flow
  and progress; verify against Python.
- [ ] `ConversationsDisplay.ingestLXMURI` — ingest an `lxm://` URI; verify
  against Python `ingest_lxm_uri` (URL parsing + message creation).
- [ ] `ConversationsDisplay.newConversation` — new conversation dialog; verify
  against Python `new_conversation`.
- [ ] `ConversationsDisplay.editSelectedInDirectory` — peer info editor; verify
  against Python `edit_selected_in_directory`.
- [ ] `ConversationsDisplay.toggleFullscreen` — fullscreen toggle; verify
  against Python.

### I3. Channels (Python Channels.py, 2285 lines)
- [ ] `ChannelsDisplay.composeListWidgets` — build the hub/room list; verify
  against Python `_compose_list_widgets`.
- [ ] `RoomWidget` — full room view (messages, users pane, editor); verify
  against Python `RoomWidget`.
- [ ] `RoomWidget.sendMessage` — send a room message; verify against Python.
- [ ] `RoomWidget.handleSlashCommand` — slash command dispatcher; verify each
  command against Python `_handle_slash_command`.
- [ ] `RoomWidget.updateMessages` / `appendMessage` — message list updates;
  verify against Python.
- [ ] `RoomWidget.leaveRoom` — leave-room flow; verify against Python.
- [ ] `_messageWidget` — render a single chat message; verify against Python.
- [ ] `ChannelsDisplay.newHubDialog` / `confirmNewHubDialog` / `editHubDialog` /
  `joinRoomDialog` — dialogs; verify against Python.
- [ ] `ChannelsDisplay.showUserInfo` — user info dialog; verify against Python.
- [ ] `ChannelsDisplay.autoconnect` — auto-connect on startup; verify against
  Python `_maybe_autoconnect`.

### I4. Network (Python Network.py, 1974 lines)
- [ ] `NetworkDisplay` — full left pane (announce stream, known nodes, LXMF
  peers, local peer, node info, network stats) with tab switching; verify layout
  against Python `NetworkDisplay`.
- [ ] `AnnounceStream` — announce stream list with search and display-mode
  toggle; verify against Python `AnnounceStream`.
- [ ] `AnnounceStreamEntry` — single announce entry widget; verify against
  Python `AnnounceStreamEntry`.
- [ ] `AnnounceInfo` — announce detail dialog; verify against Python
  `AnnounceInfo`.
- [ ] `KnownNodes` — known nodes list; verify against Python `KnownNodes`.
- [ ] `NodeEntry` — single known-node entry; verify against Python `NodeEntry`.
- [ ] `KnownNodeInfo` — known-node detail dialog; verify against Python
  `KnownNodeInfo`.
- [ ] `LXMFPeers` / `LXMFPeerEntry` — LXMF propagation peers list; verify
  against Python.
- [ ] `LocalPeer` — local peer info panel; verify against Python `LocalPeer`.
- [ ] `NodeInfo` — node info panel; verify against Python `NodeInfo`.
- [ ] `NetworkStats` — network statistics panel; verify against Python
  `NetworkStats`.

### I5. Interfaces (Python Interfaces.py, 3214 lines)
- [ ] `InterfaceDisplay` — full interface list with add/edit/show/remove and
  config-editor actions; verify layout against Python `InterfaceDisplay`.
- [ ] `SelectableInterfaceItem` — single interface row; verify against Python.
- [ ] `ShowInterface` — interface detail view with bandwidth charts; verify
  against Python `ShowInterface`.
- [ ] `AddInterfaceView` / `EditInterfaceView` — interface add/edit forms with
  per-type fields; verify against Python.
- [ ] `RNodeCalculator` widget — verify field updates against Python.
- [ ] `InterfaceBandwidthChart` — bandwidth sparkline chart; verify against
  Python `InterfaceBandwidthChart`.
- [ ] `getPortInfo` / `getPortField` — serial port detection; verify against
  Python.
- [ ] `openConfigEditor` — open the RNS config in an external editor; verify
  against Python.

### I6. Guide (Python Guide.py, 1937 lines)
- [ ] `GuideDisplay` — full guide with topic list and reader; verify layout
  against Python `GuideDisplay`.
- [ ] `TopicList` — topic list widget; verify against Python `TopicList`.
- [ ] `GuideEntry` — topic entry widget; verify against Python `GuideEntry`.
- [ ] `GuideLinkDelegate.handleLink` — guide link handling; verify against
  Python `GuideLinkDelegate.handle_link`.
- [ ] `jumpToAnchor` — anchor navigation; verify against Python.
- [ ] Guide content topics — verify each topic's Micron content matches the
  Python guide source.

### I7. MicronParser TUI bridge (Python MicronParser.py)
- [ ] `LinkableText` — tview text widget with cursor navigation (next/prev part,
  peek link, handle link); verify against Python `LinkableText`.
- [ ] `LinkSpec` — link attribute spec; verify against Python `LinkSpec`.
- [ ] `markupToAttrmaps` (tview equivalent) — render Micron markup to tview
  styled spans; verify against Python `markup_to_attrmaps` for a fixture set.

### I8. Main / TextUI / ReadlineEdit / Helpers / Extras / Config / Log
- [ ] `MainDisplay` — top-level menu bar + sub-display switching; verify menu
  keys and shortcuts against Python `Main.py`.
- [ ] `TextUI` app setup — themes, palettes, glyph sets, intro screen; verify
  theme/palette values against Python `TextUI.py`.
- [ ] `ReadlineEdit` — verify ctrl-a/e/u/k/w/y/l and history against Python
  `ReadlineEdit.py`.
- [ ] `Helpers` — OSC52 clipboard (`CopyToClipboard`) and `ClickableIcon`;
  verify clipboard escape sequence against Python `Helpers.py`.
- [ ] `Extras` — intro/splash screen content; verify against Python `Extras.py`.
- [ ] `ConfigDisplay` — config path display and editor launch; verify against
  Python `Config.py`.
- [ ] `LogDisplay` — log file viewer with tailing; verify against Python
  `Log.py`.

---

## J. cmd/gonomadnet — wiring the TUI to the core (CRITICAL)

`textui.go` has ~25 TODO markers. Each must be replaced with a real call into
the app/core. Implement bottom-up as the underlying app methods land.

- [ ] Wire `networkDisplay` to live RNS announce/node/peer data (replace the
  hardcoded interfaces list).
- [ ] Wire network "Navigate" to open the real browser over RNS.
- [ ] Wire network "Delete selected entry" to `Directory.RemoveAnnounce…`.
- [ ] Wire network "Save node" to `Directory.Remember`.
- [ ] Wire network "Show peers" to the LXMF peers list from the router.
- [ ] Wire `conversationsDisplay.OnDeleteConv` to `App.DeleteConversation`.
- [ ] Wire `conversationsDisplay.OnNewConv` to `App.CreateDirectoryEntry`.
- [ ] Wire `conversationsDisplay.OnShowQR` to the app identity LXMF address/QR.
- [ ] Wire `conversationsDisplay.OnEditPeerInfo` to `Directory.Remember`.
- [ ] Wire `conversationsDisplay.OnIngestURI` to `Conversation.Ingest` of an LXM URI.
- [ ] Wire `conversationsDisplay.OnSync` to `App.RequestLXMFSync` with a progress
  dialog.
- [ ] Wire block/unblock/ping peer callbacks to `App.BlockDestination`/
  `UnblockDestination` and the RNS link ping.
- [ ] Wire `channelsDisplay` to a real `rrc.RRCManager` from the app.
- [ ] Wire `channelsDisplay.OnNewHub` to `RRCManager.AddHub`.
- [ ] Wire `channelsDisplay.OnJoinRoom` to `RRCHub.JoinRoom`.
- [ ] Wire `channelsDisplay.OnRemoveHub` to `RRCManager.RemoveHub`.
- [ ] Wire `channelsDisplay.OnEditHub` to `RRCHub` display-name update.
- [ ] Wire `channelsDisplay.OnConnect`/`OnDisconnect` to `RRCHub.Connect`/
  `Disconnect`.
- [ ] Wire `channelsDisplay.OnToggleAutoReconnect` to
  `RRCHub.SetAutoReconnect`.
- [ ] Wire `interfacesDisplay` to the real RNS `TransportSystem` interface list.
- [ ] Wire `interfacesDisplay.OnAddInterface` to RNS config addition.
- [ ] Wire `interfacesDisplay.OnConfigEditor` to launch the external editor on
  the RNS config file.
- [ ] Wire `browserDisplay` to a real `Browser` over RNS (page fetch, history,
  cache, save-node, identify, disconnect).
- [ ] Wire `browserDisplay.OnToggleFullscreen` to the main display fullscreen
  toggle.
- [ ] Wire the splash/quit display to the real intro content and graceful
  shutdown.
- [ ] End-to-end smoke test: start the daemon (`-daemon`) with a temp config,
  verify it registers destinations and stays alive until SIGTERM.

---

## K. Cross-cutting / integration

- [ ] Integration test: Go node serves a Micron page, a Python node fetches it
  over a temporary TCP RNS transport and the bytes match.
- [ ] Integration test: Go app receives an LXMF message from a Python sender
  and ingests it into a conversation identical to Python's on-disk layout.
- [ ] `go vet ./...` and `gofmt -l .` are clean.
- [ ] All packages have godoc package comments in a same-named file.
