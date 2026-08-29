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
    # Snap the window to the smallest attached client's size.
    # resize-window implicitly sets window-size to manual; restore
    # "latest" right after so future terminal resizes auto-fit again.
    if ! tmux resize-window -a -t "$session:"; then
        printf 'could not resize %s\n' "$session" >&2
    fi
    tmux set-window-option -t "$session:" window-size latest

    # Refresh every client attached to this session.
    for tty in $(tmux list-clients -F '#{session_name} #{client_tty}' \
                     | awk -v s="$session" '$1 == s {print $2}'); do
        if tmux refresh-client -t "$tty"; then
            printf 'resized+refreshed %-18s %s\n' "$session" "$tty"
        else
            printf 'could not refresh %s (%s)\n' "$tty" "$session" >&2
        fi
    done
done
