// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
// PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// this program. If not, see <https://www.gnu.org/licenses/>.

import ApplianceKit
import UIKit

/// The camera, as the scan listener sees it.
///
/// `ScanServer` is what speaks the protocol and it lives in `ApplianceKit`, where it is
/// tested on a machine with no camera. All that is left for the app is the part that needs
/// a device: putting a camera on the screen and saying what it read.
///
/// The listener calls ``requestScan(completion:)`` on the thread that is holding the
/// client's connection open, so this hops to the main queue — presenting a view controller
/// from another queue is not a thing UIKit does — and the answer travels back through the
/// completion handler, which the listener is waiting on.
final class ScanPresenter: ScanRequesting {

    /// Presents the scanner and reports what it read, or nil if the reader backed out.
    func requestScan(completion: @escaping (String?) -> Void) {
        DispatchQueue.main.async {
            self.present(completion: completion)
        }
    }

    /// present puts the scanner on the screen, from the view controller that is showing.
    @MainActor
    private func present(completion: @escaping (String?) -> Void) {
        guard let host = ScanPresenter.visibleViewController() else {
            // Nothing to present from is nothing to scan with. It is answered rather than
            // ignored: the client is waiting, and a reader left with a Scan button that
            // never returns is worse than one that leaves their typing alone.
            completion(nil)
            return
        }

        let scanner = ScanViewController()
        scanner.onFinish = { payload in
            host.dismiss(animated: true) { completion(payload) }
        }
        // Full screen rather than a sheet: the whole of the display is the camera, which is
        // what makes aiming a code at an iPad quick.
        scanner.modalPresentationStyle = .fullScreen
        host.present(scanner, animated: true)
    }

    /// The view controller to present the scanner from.
    ///
    /// It is found through the active scene rather than through
    /// `UIApplication.shared.keyWindow`, which is deprecated and, on a scene-based app, is
    /// not the window anything is actually in. The topmost presented controller is the one
    /// to present from: presenting over a controller that is already presenting something
    /// is a request UIKit quietly ignores.
    private static func visibleViewController() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let window = scenes
            .first { $0.activationState == .foregroundActive }?
            .windows.first { $0.isKeyWindow } ?? scenes.first?.windows.first
        guard var controller = window?.rootViewController else {
            return nil
        }
        while let presented = controller.presentedViewController {
            controller = presented
        }
        return controller
    }
}
