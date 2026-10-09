#!/data/data/com.termux/files/usr/bin/bash
# exp-sensors.sh records the tablet's own position and heading while it is carried
# about, into /sdcard/Download, so the measurements can be read back afterwards
# without a cable.
#
# It exists because the useful experiments are physical ones: the tablet has to be
# rotated, carried outside and handled, and a USB cable is in the way while that
# happens. Nothing here needs a host.
set -u

H=/data/data/com.termux/files/home
OUT="/sdcard/Download/experiment-$(date +%Y%m%d-%H%M%S)"

# The collector has to live in Termux's own home: files on /sdcard are not executable.
mkdir -p "$OUT" 2>/dev/null || true
cp /sdcard/Download/sensorlog "$H/sensorlog" 2>/dev/null || true
chmod 755 "$H/sensorlog" 2>/dev/null || true
if [ ! -x "$H/sensorlog" ]; then
  echo "could not install the collector at $H/sensorlog"
  echo "re-copy it with:  cp /sdcard/Download/sensorlog ~/sensorlog"
  exit 1
fi

# Keep the CPU and Wi-Fi awake for the whole run, or Android suspends the node as
# soon as the screen goes off and the recording stops without saying so.
termux-wake-lock >/dev/null 2>&1 || true

# The appliance has to be publishing its feed. If it is not, say so plainly rather
# than recording an empty file.
#
# The check uses bash's /dev/tcp, so this script MUST be started with bash:
#
#     bash /sdcard/Download/exp-sensors.sh
#
# Termux's /bin/sh is mksh, which has no /dev/tcp — and reading /proc/net/tcp instead
# is not an option either, because Android denies it to an untrusted app:
#     /proc/net/tcp: Permission denied
# Both were tried. This is the one that works.
if ! (exec 3<>/dev/tcp/127.0.0.1/37430) 2>/dev/null; then
  cat <<'MSG'

  The gonomadnet appliance is not publishing its sensor feed.

  Open the "gonomadnet node" app first. Its notification should say
  "live position and heading". Then run this again.

MSG
  exit 1
fi

cat <<MSG

  ================= gonomadnet sensor experiment =================

  Recording to:  $OUT

  What to do:

   1. Put the tablet on its stand, standing up, screen facing you.  It
      must be STANDING, not lying flat, or there will be no heading.

   2. Watch the line below. It updates twice a second.  "no position
      yet" is normal at first; "hdg <number> true" is which way you are
      facing.

   3. Turn the whole stand a quarter turn (90 degrees) CLOCKWISE, then
      type  r  and press Enter.  Do this four times, so it goes right
      around.

   4. Take the folio case OFF, turn the stand right around again the same
      way (four quarter turns, typing  r  each time), then type  c.

   5. Put the case back on and type  c  again.

   6. Go outside, well away from buildings, and hold the tablet up with
      plenty of sky above it.

   7. The moment the position line stops saying "no position yet", type
      f  and press Enter.

   8. Stay outside about five more minutes, then type  q  and press
      Enter to finish.

  Anything else you type is kept as a note, so add comments freely.

  ===============================================================

MSG

exec "$H/sensorlog" "$OUT" tcp://127.0.0.1:37430
