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
>Channels & RRC

NomadNet includes a built-in client for `*RRC`* (Reticulum Relay Chat), a real-time text chat protocol that runs over Reticulum. The reference RRC server implementation lives at https://github.com/kc1awv/rrcd. Each RRC server is called a `!hub`!, and each hub hosts one or more `!rooms`! (channels) you can join. Hubs are addressed by a Reticulum destination hash, just like nodes and conversations.

The Channels section of NomadNet (accessible from the main menu, or by pressing the corresponding shortcut) is where you manage your hubs, view connection status, and chat in rooms.

>>Joining a hub

To start chatting on a hub you must first add it to your hub list:

>>>
 - Open the `![ Channels ]`! section.
 - Press the shortcut to open the `*New Hub`* dialog (shown in the shortcut bar at the bottom).
 - Enter the `!hub address`! (the destination hash of the hub, typically 16 hex characters / 8 bytes) and an optional display name.
 - Confirm the dialog. The hub will appear in your hub list.
<

You can sometimes receive a hub link from another user or from a node page. Activating such a link will pre-fill the New Hub dialog (and optionally a room name) for you.

>>Connecting and listing rooms

Once a hub is added:

>>>
 - Select the hub in the list and connect to it. Hubs can also be set to auto-reconnect.
 - When connected, run `*/list`* to see public rooms hosted on this hub.
 - Run `*/join <room>`* to join a room. Joined rooms appear under the hub in your channel list.
<

>>Chatting

Inside a room, anything you type that does not start with a `*/`* is sent as a message to everyone in the room. The most common in-room commands are:

>>>
 - `*/help`*  show the full list of commands
 - `*/who`*   list users in the current room
 - `*/nick <name>`*   set your display name on this hub
 - `*/topic <room> [text]`*   view or set the room topic
 - `*/part [room]`*   leave a room (defaults to the current one)
 - `*/quit`*   disconnect from the hub
<

Messages support both `*markdown`* and `*micron`* formatting, depending on your render settings (see the `*Configuration Options`* topic). Because the backtick is the micron control character, you must use the `!broken-bar`! character `*¦`* in place of `*\``* when typing micron formatting in a message. The renderer converts `*¦`* back to `*\``* for display.

>>Mentions and privacy

You will be notified (and the bell may ring, depending on your configuration) when another user mentions your nick with `*@yournick`*. RRC traffic is end-to-end encrypted by Reticulum between you and the hub, but other users in the same room can read what you write there. Treat rooms as semi-public spaces.

>>Hub etiquette

Each hub is operated independently and may have its own rules, MOTD, ban list and operator team. Be respecful, follow the hub's MOTD, and remember that operators can kick, ban and set modes on rooms they own.

