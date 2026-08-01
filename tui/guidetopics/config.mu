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
>Configuration Options

To change the configuration of Nomad Network, you must edit the configuration file. If you did not manually specify a config path when you started the program, Nomad Net will look for a configuration in the folllowing directories:

  `!/etc/nomadnetwork`!
  `!~/.config/nomadnetwork`!
  `!~/.nomadnetwork`!

If no existing configuration file is found, one will be created at `!~/.nomadnetwork/config`! by default. The default configuration file contains comments on all the different configuration options present, and explains their possible settings.

You can open the configuration file in any text-editor, and change the options. You can also use the editor built in to this program, under the `![ Config ]`! menu item. If the built-in editor does not gain focus, and your navigation keys are not working, try hitting enter or space, which should focus the editor and let you navigate the text.

For reference, all the configuration options are listed and explained here as well. The configuration is divided into different sections, each with their own options.

>> Logging Section

This section hold configuration directives related to logging output, and is delimited by the `![logging]`! header in the configuration file. Available directives, along with their default values, are as follows:

>>>
`!loglevel = 4`!
>>>>
Sets the verbosity of the log output. Must be an integer from 0 through 7.
>>>>>
0: Log only critical information
1: Log errors and lower log levels
2: Log warnings and lower log levels
3: Log notices and lower log levels
4: Log info and lower (this is the default)
5: Verbose logging
6: Debug logging
7: Extreme logging
<

>>>
`!destination = file`!
>>>>
Determines the output destination of logged information. Must be `!file`! or `!console`!.
<

>>>
`!logfile = ~/.nomadnetwork/logfile`!
>>>>
Path to the log file. Must be a writable filesystem path.
<

>> Client Section

This section hold configuration directives related to the client behaviour and user interface of the program. It is delimited by the `![client]`! header in the configuration file. Available directives, along with their default values, are as follows:

>>>
`!enable_client = yes`!
>>>>
Determines whether the client part of the program should be started on launch. Must be a boolean value.
<

>>>
`!user_interface = text`!
>>>>
Selects which interface to use. Currently, only the `!text`! interface is available.
<

>>>
`!downloads_path = ~/Downloads`!
>>>>
Sets the filesystem path to store downloaded files in.
<

>>>
`!notify_on_new_message = yes`!
>>>>
Sets whether to output a notification character (bell or flash) to the terminal when a new message is received.
<

>>>
`!announce_at_start = yes`!
>>>>
Determines whether your LXMF address is automatically announced when the program starts. Must be a boolean value.
<

>>>
`!announce_interval = 360`!
>>>>
Determines how often, in minutes, your LXMF address is announced on the network. Defaults to 6 hours.
<

>>>
`!try_propagation_on_send_fail = yes`!
>>>>
When this option is enabled, and sending a message directly to a peer fails, Nomad Network will instead deliver the message to the propagation network, for later retrieval by the recipient.
<

>>>
`!periodic_lxmf_sync = yes`!
>>>>
Whether the program should periodically download messages from available propagation nodes in the background.
<

>>>
`!lxmf_sync_interval = 360`!
>>>>
The number of minutes between each automatic sync. The default is equal to 6 hours.
<

>>>
`!lxmf_sync_limit = 8`!
>>>>
On low-bandwidth networks, it can be useful to limit the amount of messages downloaded in each sync. The default is 8. Set to 0 to download all available messages every time a sync occurs.
<

>>>
`!required_stamp_cost = None`!
>>>>
You can specify a required stamp cost for inbound messages to be accepted. Specifying a stamp cost will require untrusted senders that message you to include a cryptographic stamp in their messages. Performing this operation takes the sender an amount of time proportional to the stamp cost. As a rough estimate, a stamp cost of 8 will take less than a second to compute, and a stamp cost of 20 could take several minutes, even on a fast computer.
<

>>>
`!accept_invalid_stamps = False`!
>>>>
You can signal stamp requirements to senders, but still accept messages with invalid stamps by setting this option to True.
<

>>>
`!max_accepted_size = 500`!
>>>>
The maximum accepted unpacked size for messages received directly from other peers, specified in kilobytes. Messages larger than this will be rejected before the transfer begins.
<

>>>
`!compact_announce_stream = yes`!
>>>>
With this option enabled, Nomad Network will only display one entry in the announce stream per destination. Older announces are culled when a new one arrives.
<

>>>
`!compose_in_markdown = yes`!
>>>>
Configures whether sent messages are composed in markdown format.
<

>> RRC Section

This section hold configuration directives related to the RRC client behaviour. It is delimited by the `![rrc]`! header in the configuration file. Available directives, along with their default values, are as follows:

>>>
`!history_per_room_cap = 500`!
>>>>
Maximum number of messages retained per room in the in-memory scrollback buffer, and the number of messages restored from on-disk history at startup. The on-disk log itself is appended to indefinitely; this cap only controls how much backlog is visible. Set to 0 to keep every message in memory.
<

>>>
`!filter_loaded_history = yes`!
>>>>
Whether to filter notices and system message when initially loading room history.
<

>>>
`!ephemeral_notices = 10`!
>>>>
If enabled, notices and system messages will disappear from the history after a while. 0 disables, otherwise sets the timeout in minutes.
<

>>>
`!render_markdown = yes`!
>>>>
Whether or not to render markdown formatting in messages.
<

>>>
`!render_micron = yes`!
>>>>
Whether or not to render micron formatting in messages. When using micron in messages, use the ¦ character in place of backticks.
<

>>>
`!nick_colors = yes`!
>>>>
Whether or not to render RRC nicks in distinct colors based on identity hash.
<

>>>
`!mention_color = None`!
>>>>
Sets a specific color for mentions of your nick, for example FFBB44.
<

>>>
`!color_mention_timestamps = yes`!
>>>>
Whether or not to color timestamps of messages you were mentioned in.
<

>>>
`!justify_msgs = yes`!
`!space_msgs = no`!
`!show_gutters = no`!
>>>>
Rendering layout options for messages.
<

>> Text UI Section

This section hold configuration directives related to the look and feel of the text-based user interface of the program. It is delimited by the `![textui]`! header in the configuration file. Available directives, along with their default values, are as follows:

>>>
`!intro_time = 1`!
>>>>
Number of seconds to display the intro screen. Set to 0 to disable the intro screen.
<

>>>
`!intro_text = Nomad Network`!
>>>>
The text to display on the intro screen.
<

>>>
`!editor = editor`!
>>>>
What editor program to use when launching a text editor from within the program. Defaults to the `!editor`! alias, which in turn will use the default editor of the operating system.
<

>>>
`!glyphs = unicode`!
>>>>
Determines what set of glyphs the program uses for rendering the user interface.
>>>>>
The `!plain`! set only uses ASCII characters, and should work on all terminals, but is rather boring.
The `!unicode`! set uses more interesting glyphs and icons, and should work on most terminals. This is the default.
The `!nerdfont`! set allows using a much wider range of glyphs, icons and graphics, and should be enabled if you are using a Nerd Font in your terminal.
<

>>>
`!mouse_enabled = yes`!
>>>>
Determines whether the program should react to mouse/touch input. Must be a boolean value.
<

>>>
`!sanitize_names = yes`!
>>>>
This option allows sanitizing announced names before they are displayed in the user interface.
<

>>>
`!clipboard_copy = no`!
>>>>
If your terminal supports it, you can enable copy-to-clipboard functionality using OSC52 escape sequences.
<

>>>
`!hide_guide = no`!
>>>>
This option allows hiding the `![ Guide ]`! section of the program.
<

>>>
`!animation_interval = 1`!
>>>>
Sets the animation refresh rate for certain animations and graphics in the program. Must be an integer.
<

>>>
`!colormode = 256`!
>>>>
Tells the program what color palette is supported by the terminal. Most terminals support `!256`! colors. If your terminal supports full-color / RGB-mode, set to `!24bit`!. Available options:
>>>>>
`!monochrome`! Single-color (black/white) palette, for monochrome displays
`!16`! Low-color mode for really old-school terminals
`!88`! Standard palletised color-mode for terminals
`!256`! Almost all modern terminals support this mode
`!24bit`! Most new terminals support this full-color mode
<

>>>
`!theme = dark`!
>>>>
What color theme to use. Set it to match your terminal theme. Can be either `!dark`! or `!light`!.
<

>> Node Section

This section holds configuration directives related to the node hosting. It is delimited by the `![node]`! header in the configuration file. Available directives, along with example values, are as follows:

>>>
`!enable_node = no`!
>>>>
Determines whether the node server should be started on launch. Must be a boolean value, and is turned off by default.
<

>>>
`!node_name = DisplayName's Node`!
>>>>
Defines what the announced name of the node should be.
<

>>>
`!announce_at_start = yes`!
>>>>
Determines whether your node is automatically announced on the network when the program starts. Must be a boolean value.
<

>>>
`!announce_interval = 360`!
>>>>
Determines how often, in minutes, your node is announced on the network. Defaults to 6 hours.
<

>>>
`!pages_path = ~/.nomadnetwork/storage/pages`!
>>>>
Determines where the node server will look for hosted pages. Must be a readable filesystem path.
<

>>>
`!page_refresh_interval = 0`!
>>>>
Determines the interval in minutes for rescanning the hosted pages path. By default, this option is disabled, and the pages path will only be scanned on startup.
<

>>>
`!files_path = ~/.nomadnetwork/storage/files`!
>>>>
Determines where the node server will look for downloadable files. Must be a readable filesystem path.
<

>>>
`!file_refresh_interval = 0`!
>>>>
Determines the interval in minutes for rescanning the hosted files path. By default, this option is disabled, and the files path will only be scanned on startup.
<

>>>
`!disable_propagation = yes`!
>>>>
When Nomad Network is hosting a node, it can also run an LXMF propagation node. If there is already a large amount of propagation nodes on the network, or you simply want to run a pageserving-only node, you can disable running a propagation node.
<

>>>
`!message_storage_limit = 2000`!
>>>>
Configures the maximum amount of storage, in megabytes, that the LXMF Propagation Node will use to store messages.
<

>>>
`!max_transfer_size = 256`!
>>>>
The maximum accepted transfer size per incoming propagation transfer, in kilobytes. This also sets the upper limit for the size of single messages accepted onto this propagation node. If a node wants to propagate a larger number of messages to this node, than what can fit within this limit, it will prioritise sending the smallest, newest messages first, and try with any remaining messages at a later point.
<

>>>
`!prioritise_destinations = 41d20c727598a3fbbdf9106133a3a0ed, d924b81822ca24e68e2effea99bcb8cf`!
>>>>
Configures the LXMF Propagation Node to prioritise storing messages for certain destinations. If the message store reaches the specified limit, LXMF will prioritise keeping messages for destinations specified with this option. This setting is optional, and generally you do not need to use it.
<

>>>
`!max_peers = 25`!
>>>>
Configures the maximum number of other nodes the LXMF Propagation Node will automatically peer with. The default is 50, but can be lowered or increased according to available resources.
<

>>>
`!static_peers = e17f833c4ddf8890dd3a79a6fea8161d, 5a2d0029b6e5ec87020abaea0d746da4`!
>>>>
Configures the LXMF Propagation Node to always maintain propagation node peering with the specified list of destination hashes.
<

>> Printing Section

This section holds configuration directives related to printing. It is delimited by the `![printing]`! header in the configuration file. Available directives, along with example values, are as follows:

>>>
`!print_messages = no`!
>>>>
Determines whether messages should be printed upon arrival. Must be a boolean value, and is turned off by default.
<

>>>
`!message_template = ~/.nomadnetwork/print_template_msg.txt`!
>>>>
Determines where the template for printed messages is found. Must be a filesystem path. If you set this path to a non-existing file, an example will be generated in the specified location.
<

>>>
`!print_from = 76fe5751a56067d1e84eef3e88eab85b, trusted`!
>>>>
Determines from which destinations messages are printed. Can be a list of destinations hashes, the keyword "trusted", or "everywhere".
<

>>>
`!print_command = lp -d PRINTER_NAME -o cpi=16 -o lpi=8`!
>>>>
Specifies the command that Nomad Network uses to print the message. Defaults to "lp". The above example works well for small thermal-roll printers.
<

>Ignoring Destinations

If you encounter peers or nodes on the network, that you would rather not see in your client, you can add them to the `!~/.nomadnetwork/ignored`! file. To ignore nodes or peers, add one 32-character hexadecimal destination hash per line to the file. To unignore one again, simply remove the corresponding entry from the file and restart Nomad Network.
