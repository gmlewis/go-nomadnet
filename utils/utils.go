// Copyright 2026 Glenn Lewis. All rights reserved.

// Package utils provides the shared tmux remote-control and terminal-screen
// parsing primitives used by the gonomadnet test harnesses
// (cmd/run-tmux-test-suite and cmd/test-conversations) to drive a live
// gonomadnet TUI and actively verify its state — instead of blindly sleeping
// and grepping pane output like the earlier bash scripts did.
//
// The package is split across these files:
//
//   - utils.go  (this file) holds only the package documentation you are
//     reading; it carries no code.
//   - tmux.go   wraps tmux remote-control: creating/destroying a session,
//     sending keys, capturing the pane (plain and with ANSI escapes), reading
//     the terminal cursor position, resizing, and polling a View until a
//     condition holds.
//   - screen.go parses `tmux capture-pane -e -p` output (which embeds real
//     ANSI SGR escapes) into a 2D cell grid ([Screen]) and exposes query
//     helpers on [View] that let a driver assert the TUI's state by color and
//     cursor position as well as plain text.
//   - size.go  parses size strings ("WxH" or stty's "H W") via [ParseSize].
//
// # Overview
//
// A harness drives a TUI in two layers. First it creates a detached tmux
// session running the TUI with [NewSession], then it sends keystrokes with
// [Session.SendKeys] (tmux key names like "Enter", "C-l") or literal text with
// [Session.SendLiteral], pausing for the app to redraw. After each keystroke it
// captures the current state and verifies it, instead of sleeping a fixed
// duration: a [View] bundles the decoded [Screen] grid with the terminal
// cursor position, and [Session.Wait] polls until a condition on the View
// holds (or times out). Failures are logged with the observed state and the
// run continues — never aborting — so a single log records the full sequence.
//
// # Screen model
//
// [Screen] is the decoded 2D grid of the visible pane. Each [Cell] carries its
// rune and the SGR style (foreground/background color, bold, reverse,
// underline). Colors are normalized: the terminal default is colorDefault
// (-1); everything else is a packed 0xRRGGBB truecolor value regardless of how
// the app emitted it (8-color, 256-color, or truecolor SGR). This means a test
// can assert "this row's background is the list-focus color" portably, without
// caring which color-depth path the app took to render it.
//
// [Screen.RowText] returns a row as plain text (rune-less cells become
// spaces); [Screen.FullText] returns the whole pane as text. RowText is the
// workhorse for plain-text queries (border titles, "Retrieving", "URL: ",
// error strings) where color is irrelevant; the per-cell color on Rows is used
// where it is relevant (focus detection, message-header state glyphs).
//
// # View queries
//
// [View] holds the Screen plus the cursor position (CursorX/CursorY/CursorOK).
// Its methods answer the questions the harnesses actually ask:
//
//   - [View.ActivePage] — which top-level page is showing
//     (Conversations/Network/Channels/Log/Interfaces/Config/Guide).
//   - [View.MenuFocusedButton] — the focused menu-bar button index, or whether
//     the menu bar has focus at all.
//   - [View.ListSelectedRows] / [View.FirstSelectedRow] — the selected rows of
//     a list, detected by the full-line focus-background color.
//   - [View.BrowserState] — the Network browser pane state
//     (disconnected/retrieving/rendered) and current URL.
//   - [View.GuideTopicRendered] / [View.GuideSelectedTopic] — Guide topic
//     navigation state.
//   - [View.CursorOnAnnounceNodeRow], [View.BrowserPaneSig],
//     [View.BrowserFooterLink], [View.ReaderPaneSig] — finer Network/Reader
//     state used by the network/guide suite.
//
// The free functions [HasBorderTitle], [IsAnnounceTabOrFilter], and
// [AnnounceNodeName] are exported for the same reason: the harnesses call them
// from outside the per-View methods. The remaining View query methods and the
// internal SGR parser (parseScreen, applySGR, the color tables) are
// unexported; they are implementation details of the screen model.
//
// # Usage by a harness
//
// A harness is a *main* program (not a library): it owns one or more
// [Session] values, wraps each in a driver struct with helper methods (send /
// snapshot / view / waitFor / assert / step), and runs phases that navigate
// the TUI and assert its state. The non-fatal assert pattern — log the
// observed state and continue — is what lets a single run produce a complete
// diagnostic log even when an early step diverges. See cmd/run-tmux-test-suite
// and cmd/test-conversations for the canonical driver shapes.
package utils
