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

## The Android appliance

- Capture a screenshot of the `/msg gobot` path asked from the client's own
  Channels view on the tablet.
- Compare standalone and attached mode battery drain under a controlled test.
- Capture heading data with `/sdcard/Download/exp-sensors.sh`, then read
  `summary.txt` and `markers.txt` from the newest
  `/sdcard/Download/experiment-*` directory. Source:
  `android/termux/sensorlog-main.go`.
- Record the two failure strings of the app's client launch, which needs
  `com.termux.permission.RUN_COMMAND` hand-granted and `allow-external-apps =
  true` in Termux's `~/.termux/termux.properties`.
- Rotate the daemons' log files.
- Support more than one transport interface in the appliance, including removing
  one; today it holds a single hub address.
