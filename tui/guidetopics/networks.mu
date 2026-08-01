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
>Network Configuration

Nomad Network uses the Reticulum Network Stack for communication and encryption. This means that it will use any interfaces and communications channels already defined in your Reticulum configuration.

Reticulum supports using many kinds of devices as networking interfaces, and allows you to mix and match them in any way you choose. The number of distinct network topologies you can create with Reticulum is more or less endless, but common to them all is that you will need to define one or more interfaces for Reticulum to use.

If you have not changed the default Reticulum configuration, which should be located at `!~/.reticulum/config`!, you will have one interface active right now. With it, you should be able to communicate with any other peers and systems that exist on your local ethernet or WiFi network, if your computer is connected to one, and most probably nothing else outside of that.

To learn how to configure your Reticulum setup to use LoRa radios, packet radio or other interfaces, or connect to other Reticulum networks via the Internet, the best places to start is to read the relevant parts of the Reticulum Manual, which can be found on GitHub:

`c`_https://markqvist.github.io/Reticulum/manual/interfaces.html`_
`l

If you don't currently have access to the Internet, you can generate a configuration file full of examples of all the supported interface types, by using the command `!rnsd --exampleconfig`!. Using those examples, it should be possible to get a working setup going.

For future reference, you can download the Reticulum Manual in PDF format here:

`c`_https://github.com/markqvist/Reticulum/raw/master/docs/Reticulum%20Manual.pdf`_
`l

It might be nice to keep that handy when you are not connected to the Internet, as it is full of information and examples that are also very relevant to Nomad Network.

>The Distributed Backbone

If you have Internet access, and just want to get started experimenting, you can connected to the distributed global Reticulum backbone. For more information about this, see the Reticulum Manual, or sites such as https://directory.rns.recips or https://rmap.world.

If you connect to the public backbone, you can leave nomadnet running for a while and wait for it to receive announces from other nodes on the network that host pages or services, or you can try connecting directly to some nodes listed here:

To browse pages on a node that is not currently known, open the URL dialog in the `![ Network ]`! section of the program by pressing `!Ctrl+U`!, paste or enter the address and select `!< Go >`! or press enter. Nomadnet will attempt to discover and connect to the requested node. You can save the currently connected node by pressing `!Ctrl+S`!.
