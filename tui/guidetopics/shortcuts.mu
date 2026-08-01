# // Copyright 2026 Glenn Lewis. All rights reserved.
# //
# // This program is free software: you can redistribute it and/or modify
# // it under the terms of the GNU General Public License as published by
# // the Free Software Foundation, either version 3 of the License, or
# // (at your option) any later version.
# //
# // This program is distributed in the hope that it will be useful,
# // but WITHOUT ANY WARRANTY; without even the implied warranty of
# // MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# // GNU General Public License for more details.
# //
# // You should have received a copy of the GNU General Public License
# // along with this program. If not, see <https://www.gnu.org/licenses/>.
>Keyboard Shortcuts

The different sections of the program has a number of keyboard shortcuts mapped, that makes operating and navigating the program easier. The following lists details all mapped shortcuts.

>>`!Conversations Window`!
>>>Conversation List
 - Ctrl-N   Start a new conversation
 - Ctrl-E   Display and edit selected peer info
 - Ctrl-X   Delete conversation
 - Ctrl-R   Open LXMF syncronisation dialog
 - Ctrl-U   Ingest LXMF URI
 - Ctrl-O   Toggle sort mode
 - Ctrl-P   Display own LXMF address
 - Ctrl-G   Toggle fullscreen

>>>Message Editor
 - Ctrl-D   Send message
 - Ctrl-P   Compose paper message
 - Ctrl-T   Toggle message title field
 - Ctrl-F   Attach file
 - Ctrl-S   Save focused attachment
 - Tab      Switch focus to message list

>>>Message List
 - Ctrl-W   Close conversation
 - Ctrl-U   Purge failed messages
 - Ctrl-O   Toggle sort mode
 - Ctrl-X   Clear conversation history
 - Ctrl-G   Toggle fullscreen conversation
 - Ctrl-S   Save focused attachment
 - Tab      Switch focus to message editor

>>`!Channels Window`!
>>>Channel List
 - Ctrl-N   Add a new hub
 - Ctrl-A   Add (join) a room
 - Ctrl-R   Connect to selected hub
 - Ctrl-W   Disconnect from selected hub
 - Ctrl-T   Toggle auto-reconnect for selected hub
 - Ctrl-E   Edit selected hub
 - Ctrl-X   Remove selected hub
 - F8       Toggle join/part collapse

>>>Message Editor
 - Ctrl-D   Send message
 - Ctrl-X   Leave current room
 - F8       Toggle join/part collapse
 - Tab      Complete nickname

>>>Message List
 - Ctrl-X   Leave current room
 - Ctrl-U   Toggle users pane
 - Ctrl-Y   Toggle channel list
 - F8       Toggle join/part collapse
 - Tab      Switch focus to message editor

>>`!Input Field Editing`!
All text input fields support readline-style editing shortcuts. When an input
field is focused, these take precedence over any window shortcut mapped to the
same key (so, for example, Ctrl-U edits the line rather than toggling a pane):
 - Ctrl-A      Move to beginning of line
 - Ctrl-E      Move to end of line
 - Ctrl-U      Delete from cursor to beginning of line
 - Ctrl-K      Delete from cursor to end of line
 - Ctrl-W      Delete previous word (whitespace-delimited)
 - Ctrl-L      Delete the entire buffer
 - Ctrl-Y      Paste (yank) most recently deleted text
 - Ctrl-Left   Move backward one word
 - Ctrl-Right  Move forward one word

Text deleted with Ctrl-U, Ctrl-K, Ctrl-W or Ctrl-L is placed in a shared yank
buffer that Ctrl-Y pastes back, so text can be moved between input fields.

>>`!Network Window`!
>>>Browser
 - Ctrl-D   Back
 - Ctrl-F   Forward
 - Ctrl-R   Reload page
 - Ctrl-U   Open URL entry dialog
 - Ctrl-S   Save connected node
 - Ctrl-G   Toggle fullscreen browser window
 - Ctrl-W   Disconnect from node

>>>Announce Stream
 - Ctrl-L   Switch to Known Nodes list
 - Ctrl-X   Delete selected announce
 - Ctrl-P   Display peered LXMF Propagation Nodes

>>>Known Nodes
 - Ctrl-L   Switch to Announce Stream
 - Ctrl-X   Delete selected node entry
 - Ctrl-P   Display peered LXMF Propagation Nodes

>>>Peered LXMF Propagation Nodes
 - Ctrl-L   Switch to Announce Stream or Known Nodes
 - Ctrl-X   Break peering with selected node entry
 - Ctrl-R   Request immediate delivery sync of unhandled LXMs
