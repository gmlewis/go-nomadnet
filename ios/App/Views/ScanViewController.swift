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

import AVFoundation
import UIKit

/// The screen the camera is read on.
///
/// A dialog in the client would rather read an address from a QR code than have it typed, and
/// an iOS application has no way to reach a camera except through its own interface. This is
/// that interface: a full-screen preview, and the first QR code that decodes is handed back
/// through ``onFinish``.
///
/// AVFoundation is the whole of the dependence here — no package, no third party, the
/// platform's own camera. The decode is the platform's too: `AVCaptureMetadataOutput` asks
/// the device's own detector for QR codes and hands back the payload, so there is no image
/// processing in this app at all, and nothing to unit-test that a device would not test
/// better.
///
/// The camera is opened in `viewWillAppear` and released in `viewWillDisappear`, which is what
/// keeps it from being held while this screen is not on the display: an iOS app that holds the
/// camera while it is in the background is an app the system will take it away from.
final class ScanViewController: UIViewController {

    /// What the camera read, or nil if the reader backed out.
    var onFinish: ((String?) -> Void)?

    /// The capture session, once it has been made.
    private var session: AVCaptureSession?

    /// The layer the preview is drawn in.
    private var previewLayer: AVCaptureVideoPreviewLayer?

    /// What the reader is told to do, or why they cannot.
    private let hint = UILabel()

    /// Whether a code has already been read, so a second one cannot be reported.
    private var finished = false

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        buildInterface()
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        startCamera()
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        stopCamera()
    }

    /// The preview fills the screen, with the hint and the way out over its lower edge.
    private func buildInterface() {
        hint.text = NSLocalizedString(
            "scan.hint",
            value: "Point the camera at a QR code.\nThe address will appear in the field you pressed Scan in.",
            comment: "What the scanner says while the reader is aiming it",
        )
        hint.textColor = .white
        hint.numberOfLines = 0
        hint.textAlignment = .center
        hint.backgroundColor = UIColor(white: 0, alpha: 0.6)
        hint.translatesAutoresizingMaskIntoConstraints = false

        let cancel = UIButton(type: .system)
        cancel.setTitle(
            NSLocalizedString("scan.cancel", value: "Cancel", comment: "The way out of the scanner"),
            for: .normal,
        )
        cancel.setTitleColor(.white, for: .normal)
        cancel.titleLabel?.font = .preferredFont(forTextStyle: .headline)
        cancel.backgroundColor = UIColor(white: 0, alpha: 0.6)
        cancel.addTarget(self, action: #selector(cancelTapped), for: .touchUpInside)
        cancel.translatesAutoresizingMaskIntoConstraints = false

        view.addSubview(hint)
        view.addSubview(cancel)
        NSLayoutConstraint.activate([
            hint.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            hint.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            hint.bottomAnchor.constraint(equalTo: cancel.topAnchor),
            cancel.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            cancel.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            cancel.bottomAnchor.constraint(equalTo: view.safeAreaLayoutGuide.bottomAnchor),
            cancel.heightAnchor.constraint(equalToConstant: 52),
        ])
    }

    /// Cancel is one answer, and so is a code that was read: both end this screen.
    @objc private func cancelTapped() {
        finish(with: nil)
    }

    /// startCamera asks for the camera, and opens it if it is allowed.
    private func startCamera() {
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            openCamera()
        case .notDetermined:
            // The prompt is asynchronous, so the answer arrives after this screen is already
            // up; the hint says what is happening in the meantime.
            hint.text = NSLocalizedString(
                "scan.asking",
                value: "Waiting for permission to use the camera…",
                comment: "Shown while the system asks whether the appliance may use the camera",
            )
            AVCaptureDevice.requestAccess(for: .video) { [weak self] granted in
                DispatchQueue.main.async {
                    if granted {
                        self?.openCamera()
                    } else {
                        self?.refuseCamera()
                    }
                }
            }
        default:
            refuseCamera()
        }
    }

    /// refuseCamera says why there is no picture, and stays until the reader dismisses it.
    ///
    /// A refused camera permission has no other symptom anywhere in this appliance: the reader
    /// pressed a button in a dialog, and then nothing happened. So the reason is said here,
    /// on the screen they are already looking at, and it stays — Settings is the only place a
    /// refusal can be undone, and a message that dismissed itself would leave them with a
    /// button that does nothing and nothing saying why.
    private func refuseCamera() {
        hint.text = NSLocalizedString(
            "scan.refused",
            value: "gonomadnet cannot use the camera, so it cannot read a QR code.\n\nAllow Camera for this app in Settings to scan an address instead of typing it.",
            comment: "Why the scanner has nothing to show",
        )
    }

    /// openCamera builds the session, starts it, and begins looking for QR codes.
    private func openCamera() {
        guard session == nil else {
            return
        }
        guard let device = AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: .back)
            ?? AVCaptureDevice.default(for: .video) else {
            hint.text = NSLocalizedString(
                "scan.unavailable",
                value: "The camera could not be started.\n\nType the address by hand, or try again.",
                comment: "Why the scanner has nothing to show",
            )
            return
        }

        do {
            let session = AVCaptureSession()
            session.sessionPreset = .high
            let input = try AVCaptureDeviceInput(device: device)
            guard session.canAddInput(input) else {
                hint.text = NSLocalizedString(
                    "scan.unavailable",
                    value: "The camera could not be started.\n\nType the address by hand, or try again.",
                    comment: "Why the scanner has nothing to show",
                )
                return
            }
            session.addInput(input)

            let output = AVCaptureMetadataOutput()
            guard session.canAddOutput(output) else {
                return
            }
            session.addOutput(output)
            // Only QR codes are looked for, which is what keeps the detector quick: asked for
            // every symbology it knows, it spends its time on the ones a Reticulum address is
            // never written in.
            output.setMetadataObjectsDelegate(self, queue: DispatchQueue.main)
            output.metadataObjectTypes = [.qr]

            let preview = AVCaptureVideoPreviewLayer(session: session)
            preview.videoGravity = .resizeAspectFill
            preview.frame = view.bounds
            view.layer.insertSublayer(preview, at: 0)
            previewLayer = preview

            // Starting a session blocks until the camera is running, which is long enough to
            // drop frames on the main thread; the hint is put back before it starts.
            hint.text = NSLocalizedString(
                "scan.hint",
                value: "Point the camera at a QR code.\nThe address will appear in the field you pressed Scan in.",
                comment: "What the scanner says while the reader is aiming it",
            )
            self.session = session
            DispatchQueue.global(qos: .userInitiated).async {
                session.startRunning()
            }
        } catch {
            hint.text = NSLocalizedString(
                "scan.unavailable",
                value: "The camera could not be started.\n\nType the address by hand, or try again.",
                comment: "Why the scanner has nothing to show",
            )
        }
    }

    /// stopCamera releases the camera.
    private func stopCamera() {
        guard let session else {
            return
        }
        self.session = nil
        previewLayer?.removeFromSuperlayer()
        previewLayer = nil
        DispatchQueue.global(qos: .userInitiated).async {
            session.stopRunning()
        }
    }

    /// finish hands the reader's answer back, once.
    private func finish(with payload: String?) {
        guard !finished else {
            return
        }
        finished = true
        onFinish?(payload)
    }

    /// The preview is drawn in a layer, which does not follow the view's own resizing.
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        previewLayer?.frame = view.bounds
    }
}

/// The camera's own answer: a detected code, one per scan.
extension ScanViewController: AVCaptureMetadataOutputObjectsDelegate {
    func metadataOutput(
        _ output: AVCaptureMetadataOutput,
        didOutput metadataObjects: [AVMetadataObject],
        from connection: AVCaptureConnection,
    ) {
        for object in metadataObjects {
            guard let code = object as? AVMetadataMachineReadableCodeObject,
                  let payload = code.stringValue,
                  !payload.isEmpty else {
                continue
            }
            // The text of the code exactly as it was read. It is the client that decides
            // whether it is an address, which is what makes that rule testable where the
            // protocol is — and what lets a code carrying anything else be refused there
            // rather than here.
            finish(with: payload)
            return
        }
    }
}
