#!/data/data/com.termux/files/usr/bin/bash
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# gonomadnet-setup.sh installs into Termux everything the "gonomadnet node"
# Android appliance publishes into shared storage.
#
# It exists because Termux's home directory is another application's private
# data: Android gives one application no way to write into another's, so the
# appliance can put its files in Downloads but only a process running as Termux
# can put them where Termux reads them. This is that process.
#
#   gonomadnet-setup.sh [shared-dir]
#
# The shared directory defaults to the Downloads collection, which is where the
# appliance publishes, and is looked for in all three of the names that place has
# once Termux has been granted access to it. It is an argument so that this can be
# run against a directory of its own when it is tested; the appliance runs it with
# no arguments, which is also what the one line a person pastes has to be.
#
# What it installs, and what it deliberately does not:
#
#   ~/gonomadnet                  the client, always replaced: it is the
#                                 appliance's own binary, not the operator's
#   ~/.shortcuts/gonomadnet       the standalone launcher
#   ~/.shortcuts/gonomadnet+stack the attached launcher
#   ~/.reticulum-stack/config     the attached mode's Reticulum configuration
#   ~/.termux/font.ttf            the Nerd Font the interface draws its glyphs
#                                 from
#   ~/.termux/termux.properties   allow-external-apps, so the appliance's
#                                 buttons can start a session
#
# The terminal colors and the tmux configuration are not installed here: the
# client installs those itself on its first run, from the same directory, and
# two installers for one file is one installer too many.
#
# Everything that is already there is left exactly as it is, except for the
# client binary. A configuration, a font or a properties file on the tablet has
# been put there by somebody who meant it - including the operator, who may well
# have edited it - so this adds what is missing and never "repairs" what is not.
#
# It writes what it did to <shared-dir>/gonomadnet-setup-status.txt as well as
# to its own output. Termux's home directory cannot be read from the appliance
# or from adb, so shared storage is the only channel by which a setup that half
# worked can be told from one that did nothing at all.

set -u

# HOME_DIR and PREFIX are exported by a Termux login shell, but the appliance
# starts this through Termux's RUN_COMMAND service, which is not a login shell,
# so neither can be assumed. The defaults are Termux's own paths.
HOME_DIR="${HOME_DIR:-${HOME:-/data/data/com.termux/files/home}}"
PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"

# Termux's own programs live in $PREFIX/bin and are not on the PATH of a process
# started by RUN_COMMAND.
PATH="$PREFIX/bin:$PATH"
export PATH

SHARED="${1:-}"
if [ -z "$SHARED" ]; then
  if [ -d "$HOME_DIR/storage/downloads" ]; then
    SHARED="$HOME_DIR/storage/downloads"
  else
    SHARED=/sdcard/Download
  fi
fi

STATUS="$SHARED/gonomadnet-setup-status.txt"

# say writes one line to the operator and to the status file. The status file
# cannot be written before storage is readable, and that is not an error worth
# stopping for: the line still reaches whoever ran this.
say() {
  echo "$@"
  echo "$@" >> "$STATUS" 2>/dev/null || true
}

# have reports whether a program is available. Termux ships termux-setup-storage
# and termux-reload-settings with termux-tools, and both are best-effort here.
have() {
  command -v "$1" >/dev/null 2>&1
}

# storage_granted reports whether Termux can read shared storage at all.
#
# It is a different failure from the appliance not having published anything, and
# the two need different advice: "tap Publish files" is useless to somebody who
# has never let Termux see their Downloads.
storage_granted() {
  [ -d "$HOME_DIR/storage/downloads" ] || [ -d "$HOME_DIR/storage" ]
}

# wait_for_storage waits for the storage grant to arrive, up to twenty seconds.
#
# termux-setup-storage is asynchronous: on Android 13 and later it opens the "all
# files access" screen and returns at once, and Termux creates ~/storage when the
# person comes back from it. Without this wait the first run on a new tablet always
# ends in "not found", and the only fix is to run it a second time - which is
# exactly the kind of two-step nobody follows. Twenty seconds covers a person who
# was already looking at the screen, and a person who takes longer gets a report
# that says what happened rather than a wrong one.
wait_for_storage() {
  local waited=0
  while [ "$waited" -lt 20 ] && ! storage_granted; do
    sleep 1
    waited=$((waited + 1))
  done
  storage_granted
}

: > "$STATUS" 2>/dev/null || true
say "gonomadnet setup: installing from $SHARED"

# ---------------------------------------------------------------------------
# Shared storage first, because nothing else can be read without it
# ---------------------------------------------------------------------------

# Termux is denied access to shared storage until it has been granted, and what
# grants it is this program: on Android 13 and later it opens the "All files
# access" screen, on earlier versions it asks for the storage permission. It is
# asynchronous, so this waits for the grant to appear before looking for anything.
if ! storage_granted && have termux-setup-storage; then
  say "asking Termux for access to shared storage; accept the prompt"
  termux-setup-storage >/dev/null 2>&1 || say "termux-setup-storage did not complete"
  if wait_for_storage; then
    say "Termux can read shared storage now"
  else
    say "Termux was not given access to shared storage"
  fi
fi

# ---------------------------------------------------------------------------
# Finding what the appliance published
# ---------------------------------------------------------------------------

# find_published prints the first file matching a glob, looking in the directory
# this was told about and then in the two other spellings of the same place.
# Which one works depends on whether termux-setup-storage has ever run.
#
# The name is not the name the appliance asked MediaStore for: Android's type
# table appends the published type's own extension, so a font published as
# "font/otf" arrives as ".otf.ttf". Every lookup here is therefore a glob, and
# never an exact name.
find_published() {
  local pattern="$1" dir name
  for dir in "$SHARED" "$HOME_DIR/storage/downloads" /sdcard/Download; do
    [ -d "$dir" ] || continue
    for name in "$dir"/$pattern; do
      [ -f "$name" ] || continue
      printf '%s\n' "$name"
      return 0
    done
  done
  return 1
}

# report_missing says why a published file was not found, in terms of the button
# that publishes it. A person reading "gonomadnet-client" learns nothing - and
# "tap Publish files" is worse than nothing to somebody who has never let Termux
# read their Downloads, so the two cases are told apart.
report_missing() {
  local what="$1" pattern="$2"
  say "not found: the $what ($pattern in $SHARED)"
  if storage_granted; then
    say "  open the 'gonomadnet node' app and tap 'Publish files for Termux' first"
  else
    say "  Termux cannot read shared storage, so nothing the app publishes is visible"
    say "  run this again and accept the storage prompt when Termux shows it"
  fi
}

# ---------------------------------------------------------------------------
# The client, which everything else is for
# ---------------------------------------------------------------------------

CLIENT="$(find_published 'gonomadnet-client*' || true)"
if [ -z "$CLIENT" ]; then
  # The release asset is named for the target, so a person who downloaded it from
  # the release page instead of letting the appliance publish it still works.
  CLIENT="$(find_published 'gonomadnet-*-linux-arm64' || true)"
fi

if [ -z "$CLIENT" ]; then
  report_missing "gonomadnet client" "gonomadnet-client*"
  say "the client is what runs, so nothing was installed"
  echo
  echo "Nothing was installed: the client binary was not found in $SHARED."
  if storage_granted; then
    echo "Open the 'gonomadnet node' app and tap 'Publish files for Termux'."
  else
    echo "Termux has not been given access to shared storage, so it cannot read"
    echo "anything the app publishes. Run this again and accept the prompt."
  fi
  exit 1
fi

mkdir -p "$HOME_DIR" || {
  say "cannot create $HOME_DIR"
  exit 1
}
if cp "$CLIENT" "$HOME_DIR/gonomadnet" && chmod 755 "$HOME_DIR/gonomadnet"; then
  say "installed the client as $HOME_DIR/gonomadnet (from $CLIENT)"
else
  say "could not install the client from $CLIENT"
  exit 1
fi

# ---------------------------------------------------------------------------
# The launchers
# ---------------------------------------------------------------------------

# Termux:Widget refuses a shortcuts directory that anybody but the operator can
# read, and ignores a target script that is not executable. Both modes are
# installed because which one is wanted depends on whether the appliance's own
# stack is running.
mkdir -p "$HOME_DIR/.shortcuts" && chmod 700 "$HOME_DIR/.shortcuts" || \
  say "could not create $HOME_DIR/.shortcuts"

install_launcher() {
  local pattern="$1" name="$2" source
  source="$(find_published "$pattern" || true)"
  if [ -z "$source" ]; then
    report_missing "$name launcher" "$pattern"
    return 0
  fi
  if cp "$source" "$HOME_DIR/.shortcuts/$name" && chmod 755 "$HOME_DIR/.shortcuts/$name"; then
    say "installed the $name launcher as $HOME_DIR/.shortcuts/$name"
  else
    say "could not install the $name launcher from $source"
  fi
}

install_launcher 'gonomadnet-standalone*' 'gonomadnet'
install_launcher 'gonomadnet-stack*' 'gonomadnet+stack'

# ---------------------------------------------------------------------------
# The attached mode's Reticulum configuration
# ---------------------------------------------------------------------------

STACK_CONFIG="$(find_published 'reticulum-stack-config*' || true)"
STACK_DEST="$HOME_DIR/.reticulum-stack/config"
if [ -e "$STACK_DEST" ]; then
  say "kept the Reticulum configuration already at $STACK_DEST"
elif [ -z "$STACK_CONFIG" ]; then
  report_missing "Reticulum configuration" "reticulum-stack-config*"
else
  mkdir -p "$HOME_DIR/.reticulum-stack" && cp "$STACK_CONFIG" "$STACK_DEST" && chmod 600 "$STACK_DEST" &&
    say "installed the attached mode's Reticulum configuration as $STACK_DEST" ||
    say "could not install the Reticulum configuration from $STACK_CONFIG"
fi

# ---------------------------------------------------------------------------
# The terminal font
# ---------------------------------------------------------------------------

FONT="$(find_published '*NerdFont*' || true)"
FONT_DEST="$HOME_DIR/.termux/font.ttf"
if [ -e "$FONT_DEST" ]; then
  say "kept the terminal font already at $FONT_DEST"
elif [ -z "$FONT" ]; then
  report_missing "terminal font" "*NerdFont*"
else
  mkdir -p "$HOME_DIR/.termux" && cp "$FONT" "$FONT_DEST" &&
    say "installed the terminal font as $FONT_DEST (from $FONT)" ||
    say "could not install the terminal font from $FONT"
fi

# ---------------------------------------------------------------------------
# Termux's own settings
# ---------------------------------------------------------------------------

# allow-external-apps is what lets the appliance's "Open gonomadnet" buttons and
# its home-screen shortcut start a session: without it Termux ignores a command
# sent by another application, silently. It is the one setting this appliance
# cannot do without, and Android gives it no way to set it - the file is inside
# Termux's private data - so it is set here, by the only process that can.
#
# The edit adds to the file rather than rewriting it. The file holds the
# operator's own Termux settings, including the extra-keys row a tablet needs to
# type an escape, and a setup script that discards them has broken the terminal
# it was trying to fix.
PROPERTIES="$HOME_DIR/.termux/termux.properties"
SETTING='allow-external-apps = true'
mkdir -p "$HOME_DIR/.termux" || say "could not create $HOME_DIR/.termux"
if [ -f "$PROPERTIES" ] && grep -qE '^[[:space:]]*allow-external-apps[[:space:]]*=' "$PROPERTIES"; then
  if grep -qE '^[[:space:]]*allow-external-apps[[:space:]]*=[[:space:]]*true' "$PROPERTIES"; then
    say "allow-external-apps is already set in $PROPERTIES"
  elif sed "s|^[[:space:]]*allow-external-apps[[:space:]]*=.*|$SETTING|" "$PROPERTIES" > "$PROPERTIES.new" &&
    mv "$PROPERTIES.new" "$PROPERTIES"; then
    say "changed allow-external-apps to true in $PROPERTIES"
  else
    rm -f "$PROPERTIES.new"
    say "could not set allow-external-apps in $PROPERTIES; add this line to it by hand:"
    say "  $SETTING"
  fi
else
  if printf '\n%s\n' "$SETTING" >> "$PROPERTIES" 2>/dev/null; then
    say "set allow-external-apps in $PROPERTIES"
  else
    say "could not write $PROPERTIES; add this line to it by hand:"
    say "  $SETTING"
  fi
fi

# ---------------------------------------------------------------------------
# Adopt the font, if Termux can be told
# ---------------------------------------------------------------------------

# A font and a palette are read into a running terminal, so writing the file is
# not enough to adopt it: without this the person sees the old font until the
# next time they restart Termux, which is the difference between a fix and a
# puzzle. It is best-effort - the font is installed either way.
if have termux-reload-settings; then
  if termux-reload-settings >/dev/null 2>&1; then
    say "asked Termux to reload its settings, so the font takes effect now"
  else
    say "the font is installed, but Termux did not reload its settings; restart Termux to see it"
  fi
else
  say "termux-reload-settings is not available, so restart Termux to pick up the font"
fi

say "done: run ~/gonomadnet -t in Termux, or tap the gonomadnet icon"
exit 0
