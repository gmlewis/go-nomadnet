# TODO — open work

The mechanical task list, and nothing else. Working rules, the verification
surfaces and the source locations live in the `parity` skill and in `AGENTS.md`;
the test suite is the progress tracker. Delete a task when it is done.

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
