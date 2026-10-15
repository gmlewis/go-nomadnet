#!/usr/bin/env bash
# Snap every tmux session's window to the size of its attached client(s)
# and refresh each client so it repaints at the new size.
set -eu

sessions=$(tmux list-sessions -F '#{session_name}') || true
if [[ -z $sessions ]]; then
    echo "no tmux sessions" >&2
    exit 0
fi

for session in $sessions; do
    # Snap the window to its client by setting window-size "latest": tmux re-fits
    # the window to the attached client(s) at once, and keeps re-fitting on every
    # later terminal resize.
    #
    # Deliberately NOT "resize-window -a": that implicitly pins the window to
    # window-size manual, and from then on tmux never re-fits it at all. A pinned
    # window can sit one row taller than the space under a top status bar
    # (status-position top takes a row), which clips the application's bottom line
    # while capture-pane still reports it - a footer that "disappears" and a
    # session that looks like it will not resize. "latest" overrides any such
    # leftover pin, and is what "-a" was immediately overwritten with anyway.
    if ! tmux set-window-option -t "$session:" window-size latest; then
        printf 'could not fit %s\n' "$session" >&2
    fi

    # Refresh every client attached to this session, reporting the geometry a
    # clipping session would show up in (window one row taller than the client).
    geo=$(tmux display-message -p -t "$session:" \
              'client=#{client_height} window=#{window_height}' 2>/dev/null || true)
    for tty in $(tmux list-clients -F '#{session_name} #{client_tty}' \
                     | awk -v s="$session" '$1 == s {print $2}'); do
        if tmux refresh-client -t "$tty"; then
            printf 'resized+refreshed %-18s %s  (%s)\n' "$session" "$tty" "$geo"
        else
            printf 'could not refresh %s (%s)\n' "$tty" "$session" >&2
        fi
    done
done
