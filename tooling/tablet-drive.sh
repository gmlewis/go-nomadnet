#!/usr/bin/env bash
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# tablet-drive.sh — drive the gonomadnet appliance on the tablet and capture what
# the client drew, so a screen-rotation (or keyboard) defect can be checked
# without a person at the device.
#
# A rotation is what the appliance sends as a window-size frame, and what the
# client answers with is what `tablet-measure.py` reads back out of a capture.
# This script is the other half: it takes deterministic control of the rotation,
# raises and drops the soft keyboard, taps, and takes captures in the raw form
# that needs nothing but the standard library to measure.
#
#   SERIAL=R95Y4073WYZ ./tablet-drive.sh setup          # take control (saves what it found)
#   ./tablet-drive.sh rot portrait
#   ./tablet-drive.sh shot guide-portrait
#   ./tablet-drive.sh restore                           # put the device back
#
# Commands:
#   setup                 save the display settings found and pin the rotation
#   restore               put the saved settings back (and report if none were saved)
#   state                 the current rotation, soft keyboard and focus, in one line
#   rot portrait|landscape
#   ime up|down           a tap raises the soft keyboard; Back drops it
#   tap X Y               a tap inside the console is a mouse event to the client
#   key [KEYCODE...]      a key COMBINATION, e.g. `key KEYCODE_CTRL_LEFT KEYCODE_Y`
#   shot NAME [NAME...]   capture raw into $SHOTS and measure each one
#   alive                 the client and console-host processes, and nothing else
#
# Three traps, each of which has cost a session:
#
#   * A TAP ON THE CONSOLE RAISES THE SOFT KEYBOARD, and the keyboard takes half a
#     portrait screen's height. Dismiss it with Back — the input method consumes
#     that keypress, so it never reaches the activity — and a second Back then
#     closes the console, and a third leaves the app. The key strip is drawn only
#     while the keyboard is up, which is also the one probe that reports the view
#     height (see tablet-measure.py).
#   * PLAIN `input keyevent` DOES NOT REACH THE CLIENT: DPAD and ENTER do nothing,
#     while `input keycombination KEYCODE_CTRL_LEFT ...` does. Drive the client
#     with taps and control keys.
#   * A CONTROL KEY IS SWALLOWED BY THE MENU BAR while it holds the focus, which
#     is where a menu-bar tap leaves it (Ctrl-Y on the Channels page does nothing
#     until the focus is in the body; a rotation leaves it in the body, a menu tap
#     does not). `state` reports which side of that line you are on.
#
# Nothing here connects to a hub or a node: a tap selects, a rotation re-lays out,
# and neither joins a room or dials a peer. Do not send Ctrl-r (Connect) or Ctrl-a
# (Add Room) to the Channels page — that is the operator's traffic, not a test.
#
# See tooling/README.md ("Driving the tablet") and the android-tablet skill.

set -uo pipefail

SHOTS="${SHOTS:-/tmp/gonomadnet-shots}"
STATE="$SHOTS/device-state.txt"
SERIAL="${SERIAL:-${ANDROID_SERIAL:-}}"

usage() { sed -n '4,20p' "$0" | sed 's/^# \{0,1\}//'; }

# adb_ refuses to run without a serial: the tablet is normally reachable both over
# USB and over the network, and without -s every command fails with "more than one
# device/emulator" — or, worse, quietly drives the wrong one.
adb_() {
  if [ -z "$SERIAL" ]; then
    echo "tablet-drive.sh: set SERIAL (or ANDROID_SERIAL) to the tablet's serial or address" >&2
    exit 2
  fi
  adb -s "$SERIAL" "$@"
}

saved() { adb_ shell settings get system "$1"; }

case "${1:-}" in
  setup)
    mkdir -p "$SHOTS"
    if [ ! -f "$STATE" ]; then
      {
        echo "accelerometer_rotation=$(saved accelerometer_rotation)"
        echo "user_rotation=$(saved user_rotation)"
        echo "screen_off_timeout=$(saved screen_off_timeout)"
      } > "$STATE"
      echo "saved the device's settings to $STATE"
    else
      echo "using the settings already saved in $STATE"
    fi
    # Auto-rotate off, or the sensor overrides user_rotation and every "rotate" is
    # a request rather than an instruction. Kept awake for the same reason: a
    # dimming screen blanks the captures and the measurements report a state they
    # cannot see.
    adb_ shell settings put system accelerometer_rotation 0
    adb_ shell settings put system screen_off_timeout 1800000
    ;;
  restore)
    if [ ! -f "$STATE" ]; then
      echo "tablet-drive.sh: no saved settings at $STATE — run 'setup' first, or set them by hand" >&2
      exit 2
    fi
    while IFS='=' read -r key value; do
      [ -n "$key" ] || continue
      adb_ shell settings put system "$key" "$value"
    done < "$STATE"
    echo "restored the device's settings from $STATE"
    ;;
  state)
    echo "rotation:   auto=$(saved accelerometer_rotation) user=$(saved user_rotation)"
    echo "screen:     $(adb_ shell wm size | tr -d '\r' | tail -1)"
    echo "keyboard:   $(adb_ shell dumpsys input_method | grep -m1 -o 'mInputShown=[a-z]*')"
    echo "focus:      $(adb_ shell dumpsys window | grep -m1 -o 'mCurrentFocus=Window{[^}]*}')"
    ;;
  rot)
    case "${2:-}" in
      portrait) adb_ shell settings put system user_rotation 0 ;;
      landscape) adb_ shell settings put system user_rotation 1 ;;
      *) echo "tablet-drive.sh: rot takes portrait or landscape" >&2; exit 2 ;;
    esac
    sleep 3
    ;;
  ime)
    case "${2:-}" in
      up) adb_ shell input tap 600 900 ;;
      down) adb_ shell input keyevent KEYCODE_BACK ;;
      *) echo "tablet-drive.sh: ime takes up or down" >&2; exit 2 ;;
    esac
    sleep 2
    ;;
  tap)
    adb_ shell input tap "$2" "$3"
    sleep 2
    ;;
  key)
    shift
    adb_ shell input keycombination "$@"
    sleep 2
    ;;
  shot)
    mkdir -p "$SHOTS"
    shift
    for name in "$@"; do
      if ! adb_ exec-out screencap > "$SHOTS/$name.raw"; then
        echo "tablet-drive.sh: capturing $name failed" >&2
        exit 1
      fi
      python3 "$(dirname "$0")/tablet-measure.py" "$SHOTS/$name.raw"
    done
    ;;
  alive)
    adb_ shell "ps -A -o PID,PPID,NAME | grep -iE 'gonomadnetclient|gorcons' || echo 'no client and no console host: the console is closed'"
    ;;
  *)
    usage
    exit 2
    ;;
esac
