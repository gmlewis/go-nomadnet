/*
 * Copyright 2026 Glenn Lewis. All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

/*
 * bridge.h declares the appliance's Go half for Swift.
 *
 * It is written by hand rather than taken from the header cgo generates, because
 * cgo only generates one for the exports of a main package and these live in a
 * library package — every one of them is reachable, and this is the file that says
 * so. The names, argument order and status codes here are the C ABI the two halves
 * of the appliance share; ios/bridge is the Go side of the same contract, and its
 * tests assert the behavior behind each of these.
 *
 * The implementation is linked from the static archive the build script produces
 * from ./cmd/gonomadnet with GOOS=ios (see scripts/build-ios-app.sh).
 */

#ifndef GONOMADNET_BRIDGE_H
#define GONOMADNET_BRIDGE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Status codes every entry point returns.
 *
 * A status alone cannot say why an appliance would not come up, so a failure also
 * records its reason, which GonomadnetLastError hands back.
 */
#define GONOMADNET_OK 0               /* the call did what it says */
#define GONOMADNET_ALREADY_RUNNING 1  /* the start or stop was not needed */
#define GONOMADNET_NOT_RUNNING 2      /* there was nothing to stop, or to feed */
#define GONOMADNET_BAD_ARGUMENT 3     /* a configuration the bridge refuses */
#define GONOMADNET_FAILED 4           /* the call failed; see GonomadnetLastError */
#define GONOMADNET_SHUT_DOWN 5        /* the appliance has already been shut down */

/*
 * GonomadnetStart brings the whole appliance up: the Reticulum transport, the sensor feed,
 * the RRC hub and the bot, in that order. The first thing the client and the bot read is
 * the feed, and the transport owns the shared instance everything else attaches to.
 *
 * configDir     the client's own configuration directory, where its configuration, its
 *               storage and the local hub's identity live
 * rnsConfigDir  the Reticulum configuration directory that owns the shared instance
 * clientRNSDir  the Reticulum configuration directory the hub and the bot attach to the
 *               shared instance through. It is not the transport's: a second user of the
 *               transport's directory would try to own an instance that is already owned.
 * feedPort      the loopback port the sensor feed binds; 0 means the pinned 37430
 *
 * The hub's identity and its published address are made here, because the hub has no
 * bootstrap run of its own to make them in, and its address is how the bot and the client
 * find the appliance's own hub at all.
 */
int GonomadnetStart(const char *configDir, const char *rnsConfigDir, const char *clientRNSDir,
                    int feedPort);

/*
 * GonomadnetStop shuts the appliance down and releases every goroutine, socket and
 * temporary file it holds.
 *
 * It is safe to call more than once, and safe to call on an appliance that never
 * started. It blocks until the console session has released its socket, so a
 * relaunch finds the path free.
 */
int GonomadnetStop(void);

/*
 * GonomadnetStartConsole listens for the app to attach to a console and then runs
 * the NomadNet client on it, inside this process.
 *
 * socketPath  the Unix socket the app dials; at most 103 bytes, because that is all
 *             the kernel's sun_path holds. iOS's own temporary directory is long
 *             enough for a careless name to reach that limit, so keep it short.
 * cols, rows  the terminal's initial size; 0 means 80 by 24. The app sends a RESIZE
 *             frame as soon as it is attached, so this is only the starting size.
 *
 * It returns as soon as the listener is bound. The session ends the way every other
 * one does: the client exits, a 0x03 EXIT frame carries its status over the socket,
 * and the connection closes. A second call replaces the session rather than being
 * refused, because the app rebuilds its console when the reader reopens it.
 */
int GonomadnetStartConsole(const char *socketPath, uint16_t cols, uint16_t rows);

/*
 * GonomadnetSetScanEndpoint records where the app's camera scanner is listening, and
 * the token a request for a scan must carry.
 *
 * It is called before GonomadnetStartConsole, because the client is told both when it
 * starts: a client told an endpoint afterwards would have a Scan button that did
 * nothing until the console was reopened. An empty endpoint means the app has no
 * scanner, and the client then draws no Scan button at all — which is what a desktop
 * terminal and a device without a camera both get.
 *
 * The endpoint is the host:port of a loopback listener, and the token is hex.
 */
int GonomadnetSetScanEndpoint(const char *endpoint, const char *token);

/*
 * GonomadnetFeedLocation publishes one GNSS fix to every sensor-feed subscriber, as
 * NMEA-0183 sentences.
 *
 * A measurement the device did not take is passed as NaN, which is how NMEA writes
 * it: an empty field. A height nobody measured is therefore never reported as
 * standing at sea level. A NaN latitude or longitude makes the fix invalid, and the
 * sentences carry the void status rather than a stale position.
 *
 * ts is seconds since the Unix epoch, or 0 when the receiver did not say when the
 * fix was taken.
 */
int GonomadnetFeedLocation(double lat, double lon, double alt, double hAcc,
                           double speed, double course, double ts);

/*
 * GonomadnetFeedHeading publishes one orientation reading, as a single NMEA-0183
 * heading sentence.
 *
 * A heading is published only when the device measured one: pass NaN for a heading
 * that was withheld, which is what a device whose reference axis is near vertical
 * reports. Zero degrees is due north, and a reader cannot tell the two apart.
 *
 * The true heading wins when it is known, because a reader stores one heading and
 * the true one is the one a person reads off a map.
 *
 * yaw, pitch, roll and isBackAxis are accepted because they are part of the ABI the
 * Swift half computes, and a heading sentence has no field to put a tilt in, so the
 * bridge drops them rather than inventing one.
 */
int GonomadnetFeedHeading(double magnetic, double trueHeading, double yaw,
                          double pitch, double roll, int isBackAxis);

/*
 * GonomadnetLastError is why the last call failed, as a NUL-terminated string, or the
 * empty string when it did not. The caller owns it and frees it with
 * GonomadnetFreeError.
 */
char *GonomadnetLastError(void);

/* GonomadnetFreeError releases a string GonomadnetLastError returned. */
void GonomadnetFreeError(char *message);

/* GonomadnetVersion is the Go NomadNet version the appliance runs. */
char *GonomadnetVersion(void);

#ifdef __cplusplus
}
#endif

#endif /* GONOMADNET_BRIDGE_H */
