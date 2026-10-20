// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import SwiftUI

/// GonomadnetApp is the appliance's entry point.
///
/// The one thing it is careful about is the appliance's lifetime. A Reticulum transport
/// holds sockets, a shared instance and a hub's identity, and an iOS app can be
/// backgrounded, killed and relaunched at any moment; leaving a transport behind is how a
/// relaunch ends up with a transport that reports every interface disconnected because
/// another one still holds the port. So the stack is brought down when the app goes away,
/// and it is brought down through the same path whether the reader pressed Stop or the
/// system is taking the app off the screen.
@main
struct GonomadnetApp: App {
    /// The appliance, as the interface sees it.
    @StateObject private var model = ApplianceModel()

    /// The scene's phase, which is what says the app is going away.
    @Environment(\.scenePhase) private var scenePhase

    /// Whether the appliance was running when the app was last backgrounded.
    ///
    /// It is what makes returning to the app return to the appliance rather than starting
    /// one the reader never asked for: the scene goes inactive for a permission alert and
    /// for the app switcher, and restarting on every interruption would also ask for
    /// location before the reader ever pressed Start Sensors.
    @State private var resumeStack = false
    @State private var resumeSensors = false

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .preferredColorScheme(.dark)
        }
        .onChange(of: scenePhase) { phase in
            switch phase {
            case .background:
                // The console goes first and the stack second, which is the order the
                // appliance's own shutdown uses: the client stops drawing before the
                // transport it draws from goes away.
                resumeStack = model.state.isRunning
                resumeSensors = model.sensorsRunning
                model.terminal?.stop()
                model.supervisor.stop()
            case .active:
                // What was running comes back, with the reader's own settings: a device
                // whose reader turned the sensors off comes back with them off.
                if resumeStack, model.enableStack, !model.state.isRunning {
                    model.toggleStack()
                }
                if resumeSensors, model.enableSensors, !model.sensorsRunning {
                    model.toggleSensors()
                }
                resumeStack = false
                resumeSensors = false
            default:
                break
            }
        }
    }
}
