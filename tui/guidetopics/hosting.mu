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
>Hosting a Node

To host a node on the network, you must enable it in the configuration file, by setting the `*enable_node`* directive to `*yes`*. You should also configure the other node-related parameters such as the node name and announce interval settings. Once node hosting has been enabled in the configuration, Nomad Network will start hosting your node as soon as the program is launched, and other peers on the network will be able to connect and interact with content on your node.

By default, no content is defined, apart from a short placeholder home page. To learn how to add your own content, read on.

>>Distributed Message Store

All nodes on the network will automatically participate in a distributed message store that allows users to exchange messages, even when they are not connected to the network at the same time.

When Nomad Network is configured to host a node, by default it also configures itself as an LXMF Propagation Node, and automatically discovers and peers with other propagation nodes on the network. This process is completely automatic and requires no configuration from the node operator.

`!However`!, if there is already an abundance of Propagation Nodes on the network, or the operator simply wishes to host a pageserving-only node, Propagation Node hosting can be disabled in the configuration file.

To view LXMF Propagation nodes that are currently peered with your node, go to the `![ Network ]`! part of the program and press `!Ctrl-P`!. In the list of peered Propagation Nodes, it is possible to:

 - Immediately break peering with a node by pressing `!Ctrl-X`!
 - Request an immediate delivery sync of all unhandled messages for a node, by pressing `!Ctrl-R`!

The distributed message store is resilient to intermittency, and will remain functional as long as at least one node remains on the network. Nodes that were offline for a time will automatically be synced up to date when they regain connectivity.

>>Pages

Nomad Network nodes can host pages similar to web pages, that other peers can read and interact with. Pages are written in a compact markup language called `*micron`*. To learn how to write formatted pages with micron, see the `*Markup`* section of this guide (which is, itself, written in micron). Pages can be linked together with hyperlinks, that can also link to pages (or other resources) on other nodes.

To add pages to your node, place micron files in the `*pages`* directory of your Nomad Network programs `*storage`* directory. By default, the path to this will be `!~/.nomadnetwork/storage/pages`!. You should probably create the file `!index.mu`! first, as this is the page that will get served by default to a connecting peer.

You can control how long a peer will cache your pages by including the cache header in a page. To do so, the first line of your page must start with `!#!c=X`!, where `!X`! is the cache time in seconds. To tell the peer to always load the page from your node, and never cache it, set the cache time to zero. You should only do this if there is a real need, for example if your page displays dynamic content that `*must`* be updated at every page view. The default caching time is 12 hours. In most cases, you should not need to include the cache control header in your pages.

>> Dynamic Pages

You can use a preprocessor such as PHP, bash, Python (or whatever you prefer) to generate dynamic pages and fully interactive applications running over Nomad Network. To do so, just set executable permissions on the relevant page file, and be sure to include the interpreter at the beginning of the file, for example `!#!/usr/bin/python3`!.

Data from fields and link variables will be passed to these scripts or programs as environment variables, and can simply be read by any method for accessing such.

In the `!examples`! directory, you can find various small examples for the use of this feature. The currently included examples are:

 - A messageboard that receives messages over LXMF, contributed by trippcheng
 - A simple demonstration on how to create fields and read entered data in node-side scripts

By default, you can find the examples in `!~/.nomadnetwork/examples`!. If you build something neat, that you feel would fit here, you are more than welcome to contribute it.

>>Authenticating Users

Sometimes, you don't want everyone to be able to view certain pages or execute certain scripts. In such cases, you can use `*authentication`* to control who gets to run certain requests.

To enable authentication for any page, simply add a new file to your pages directory with ".allowed" added to the file-name of the page. If your page is named "secret_page.mu", just add a file named "secret_page.mu.allowed".

For each user allowed to access the page, add a line to this file, containing the hash of that user's primary identity. Users can find their own identity hash in the `![ Network ]`! part of the program, under `!Local Peer Info`!. If you want to allow access for three different users, your file would look like this:

`Faaa
`=
d454bcdac0e64fb68ba8e267543ae110
2b9ff3fb5902c9ca5ff97bdfb239ef50
7106d5abbc7208bfb171f2dd84b36490
`=
``

You can also dynamically generate this list, by making the file executable, and writing a script (in whatever language you want), that prints the list to stdout. Every time someone tries to request the page, Nomad Network will check the allowed identities list, and only grant access to allowed users.

By default, Nomad Network connects anonymously to all nodes. To be able to identify, and access restricted pages, you must allow identifying on a per-node basis. To allow identifying when connecting to a node, you must go to the `!Known Nodes`! list in the `![ Network ]`! part of the program, and enable the `!Identify When Connecting`! checkbox under `!Node Info`!.

>>Files

Like pages, you can place files you want to make available in the `!~/.nomadnetwork/storage/files`! directory. To let a peer download a file, you should create a link to it in one of your pages.

>>Links and URLs

Links to pages and resources in Nomad Network use a simple URL format. Here is an example:

`!18176ffddcc8cce1ddf8e3f72068f4a6:/page/index.mu`!

The first part is the 10 byte destination address of the node (represented as readable hexadecimal), followed by the `!:`! character. Everything after the `!:`! represents the request path.

By convention, Nomad Network nodes map all hosted pages under the `!/page`! path, and all hosted files under the `!/file`! path. You can create as many subdirectories for pages and files as you please, and they will be automatically mapped to corresponding request paths.

You can omit the destination address of the node, if you are referring to a local page or file. You must still keep the `!:`! character. In such a case, the URL to a page could look like this:

`!:/page/other_page.mu`!

The URL to a local file could look like this:

`!:/file/document.pdf`!

Links can be inserted into micron documents. See the `*Markup`* section of this guide for info on how to do so.

