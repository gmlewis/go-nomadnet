# TODO — open work

The mechanical task list, and nothing else. Working rules, the verification
surfaces and the source locations live in the `parity` skill and in `AGENTS.md`;
the test suite is the progress tracker. Delete a task when it is done.

## Outstanding items

- **Deploy the 2026-09-03 evening Channels fixes to the fleet.** The four
  fixes (the channels-list selection follows the shown room + the multi-hub
  info-panel lookup, the /who-reply member-set replacement + the 60 s silent
  membership reconciliation, the pinned all-rooms greeting MOTD, and the
  Users-pane no-selection-highlight under the "N users" count —
  `tui/room-widget.go`, pinned by `tui/users-pane-selection_test.go`) are in
  this working tree, uncommitted — the agent never commits or pushes. Commit +
  push go-nomadnet, then rebuild and restart `./gonomadnet.sh` on all six fleet
  nodes: `local`, `glenn-OMEN-875`, `glenn-nano2gb`, `glenn-mac-mini-m2`,
  `raspberrypi`, `glenn-kamrui`.

  Expect after reconnect: every node shows the same live member count
  (reconciles within ~60 s) with NO highlighted row under "N users", the
  opened room's row carries the selection highlight, and every room shows
  the hub's greeting MOTD.
