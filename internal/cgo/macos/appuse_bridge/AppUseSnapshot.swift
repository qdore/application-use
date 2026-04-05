import Cocoa
import ApplicationServices
import Foundation
import Vision
import CoreML

// MARK: - Core Types

public class Element {
    let axElement: AXUIElement
    let role: String
    let frame: NSRect
    let title: String
    let label: String
    let identifier: String
    let description: String
    let help: String
    var hint: String?
    var children: [Element] = []
    
    init(axElement: AXUIElement, role: String, frame: NSRect, title: String, label: String = "", identifier: String = "", description: String = "", help: String = "") {
        self.axElement = axElement
        self.role = role
        self.frame = frame
        self.title = title
        self.label = label
        self.identifier = identifier
        self.description = description
        self.help = help
    }
    
    func toDict() -> [String: Any] {
        var dict: [String: Any] = [
            "role": role,
            "title": title,
            "label": label,
            "identifier": identifier,
            "description": description,
            "help": help,
            "frame": [
                "x": frame.origin.x,
                "y": frame.origin.y,
                "width": frame.width,
                "height": frame.height
            ]
        ]
        if let h = hint {
            dict["hint"] = h
        }
        if !children.isEmpty {
            dict["children"] = children.map { $0.toDict() }
        }
        return dict
    }
}

public struct Hint {
    let frame: NSRect
    let text: String
}

// MARK: - Geometry Utils
class GeometryUtils {
    static func convertAXFrameToGlobal(_ axFrame: NSRect) -> NSRect {
        guard let screen = NSScreen.screens.first else { return axFrame }
        let screenHeight = screen.frame.height
        let newY = screenHeight - axFrame.origin.y - axFrame.height
        return NSRect(x: axFrame.origin.x, y: newY, width: axFrame.width, height: axFrame.height)
    }
    
    static func center(_ frame: NSRect) -> NSPoint {
        NSPoint(
            x: frame.origin.x + (frame.size.width / 2),
            y: frame.origin.y + (frame.size.height / 2)
        )
    }
}

// MARK: - Alphabet Hints
class AlphabetHints {
    static func hintStrings(linkCount: Int, hintCharacters: String = "JKHLASDFGYUIOPQWERTNMZXCVB") -> [String] {
        if linkCount == 0 { return [] }
        var hints = [""]
        var offset = 0
        while hints.count - offset < linkCount || hints.count == 1 {
            let hint = hints[offset]
            offset += 1
            for char in hintCharacters {
                hints.append(String(char) + hint)
            }
        }
        return Array(hints[offset...offset+linkCount-1])
            .sorted()
            .map { String($0.reversed()).uppercased() }
    }
}

// MARK: - UI Rendering

class HintText: NSTextField {
    required init(hintTextSize: CGFloat, hintText: String) {
        super.init(frame: .zero)
        self.translatesAutoresizingMaskIntoConstraints = false
        self.stringValue = hintText
        self.font = NSFont.systemFont(ofSize: hintTextSize, weight: .bold)
        self.textColor = .black
        self.isBezeled = false
        self.drawsBackground = true
        self.wantsLayer = true
        self.backgroundColor = .clear
        self.canDrawSubviewsIntoLayer = true
        self.isEditable = false
    }
    required init?(coder: NSCoder) { fatalError() }
}

class HintView: NSView {
    static let borderColor = NSColor.darkGray
    static let backgroundColor = NSColor(red: 255/255, green: 224/255, blue: 112/255, alpha: 1)
    
    let associatedFrame: NSRect
    var hintTextView: HintText!
    let borderWidth: CGFloat = 1.0
    let cornerRadius: CGFloat = 3.0

    required init(frame: NSRect, hintTextSize: CGFloat, hintText: String) {
        self.associatedFrame = frame
        super.init(frame: .zero)
        
        self.hintTextView = HintText(hintTextSize: hintTextSize, hintText: hintText)
        self.addSubview(hintTextView)
        self.wantsLayer = true
        self.layer?.borderWidth = borderWidth
        self.layer?.backgroundColor = HintView.backgroundColor.cgColor
        self.layer?.borderColor = HintView.borderColor.cgColor
        self.layer?.cornerRadius = cornerRadius
        self.translatesAutoresizingMaskIntoConstraints = false
        
        hintTextView.centerYAnchor.constraint(equalTo: self.centerYAnchor).isActive = true
        hintTextView.centerXAnchor.constraint(equalTo: self.centerXAnchor).isActive = true
        
        self.widthAnchor.constraint(equalTo: hintTextView.widthAnchor, constant: 8).isActive = true
        self.heightAnchor.constraint(equalTo: hintTextView.heightAnchor, constant: 4).isActive = true
    }
    required init?(coder: NSCoder) { fatalError() }
}

class HintsWindowController: NSWindowController {
    var hints: [Hint] = []
    var hintViews: [HintView] = []
    
    convenience init(hints: [Hint], screenFrame: NSRect) {
        let window = NSWindow(contentRect: screenFrame, styleMask: .borderless, backing: .buffered, defer: false)
        window.isOpaque = false
        window.backgroundColor = .clear
        window.hasShadow = false
        window.level = .screenSaver
        window.ignoresMouseEvents = true
        window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        
        self.init(window: window)
        self.hints = hints
        
        let contentView = NSView(frame: screenFrame)
        window.contentView = contentView
        
        for hint in hints {
            let globalFrame = GeometryUtils.convertAXFrameToGlobal(hint.frame)
            guard let viewFrame = convertToWindowFrame(globalFrame: globalFrame, window: window) else { continue }
            let view = HintView(frame: hint.frame, hintTextSize: 12, hintText: hint.text)
            
            let center = GeometryUtils.center(viewFrame)
            view.frame.origin = NSPoint(
                x: center.x - (view.intrinsicContentSize.width / 2),
                y: center.y - (view.intrinsicContentSize.height / 2)
            )
            contentView.addSubview(view)
            hintViews.append(view)
        }
    }
    
    func convertToWindowFrame(globalFrame: NSRect, window: NSWindow) -> NSRect? {
        let windowFrame = window.convertFromScreen(globalFrame)
        return window.contentView?.convert(windowFrame, to: nil)
    }
}

// MARK: - Vision OCR Helper

/// Singleton to handle one-pass full-screen OCR to avoid redundant screencapture calls.
class ScreenOCR {
    static let shared = ScreenOCR()
    
    private var cachedResults: [(text: String, frame: NSRect)] = []
    private var hasPerformedOCR = false
    private(set) var lastCapturedImage: CGImage?
    
    func reset() {
        cachedResults = []
        hasPerformedOCR = false
        lastCapturedImage = nil
    }
    
    /// Synchronously performs a single-pass OCR on the specified window frame.
    /// Also saves the capture to current_position.png.
    func performWindowOCR(winFrame: NSRect) {
        guard !hasPerformedOCR else { return }
        hasPerformedOCR = true
        
        let winX = winFrame.origin.x
        let winY = winFrame.origin.y
        let winW = winFrame.width
        let winH = winFrame.height
        
        let imgPath = "/tmp/application-use-current.png"
        let rect = CGRect(x: winX, y: winY, width: winW, height: winH)
        
        // Use CoreGraphics to capture the screen region directly instead of screencapture CLI,
        // which has bugs with negative coordinates on multi-monitor setups.
        guard let cgImage = CGWindowListCreateImage(rect, .optionOnScreenOnly, kCGNullWindowID, .boundsIgnoreFraming) else {
            return
        }
        lastCapturedImage = cgImage
        
        // Save to current_position.png for caching/UI purposes
        let nsImage = NSImage(cgImage: cgImage, size: NSSize(width: winW, height: winH))
        if let tiffRep = nsImage.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiffRep) {
            if let data = bitmap.representation(using: .png, properties: [:]) {
                try? data.write(to: URL(fileURLWithPath: imgPath))
            }
        }
        
        let semaphore = DispatchSemaphore(value: 0)
        let request = VNRecognizeTextRequest { [weak self] req, _ in
            defer { semaphore.signal() }
            guard let observations = req.results as? [VNRecognizedTextObservation] else { return }
            
            for observation in observations {
                guard let topCandidate = observation.topCandidates(1).first else { continue }
                
                // Vision normalized box: origin at bottom-left, y-up, 0..1 (relative to the image)
                let box = observation.boundingBox
                
                // Convert to AX screen space (absolute):
                // normalizedX * windowWidth + windowOriginX
                let axX = winX + (box.origin.x * winW)
                // Vision Y is bottom-up within the image. AX Y is top-down from screen top.
                // windowOriginY + (windowHeight - (normalizedY + normalizedHeight) * windowHeight)
                let axY = winY + (winH - (box.origin.y + box.size.height) * winH)
                let axWidth = box.size.width * winW
                let axHeight = box.size.height * winH
                
                let axFrame = NSRect(x: axX, y: axY, width: axWidth, height: axHeight)
                self?.cachedResults.append((text: topCandidate.string, frame: axFrame))
            }
        }
        
        request.recognitionLevel = .accurate
        var languages = ["zh-Hans", "zh-Hant", "en-US"]
        for lang in Locale.preferredLanguages.reversed() {
            if !languages.contains(lang) {
                languages.insert(lang, at: 0)
            }
        }
        request.recognitionLanguages = languages
        request.usesLanguageCorrection = true
        
        let handler = VNImageRequestHandler(cgImage: cgImage, options: [:])
        try? handler.perform([request])
        semaphore.wait()
    }
    
    /// Returns concatenated text for all OCR blocks that overlap significantly with the target frame.
    func textAt(frame: NSRect) -> String {
        // Cached results should already be populated by performWindowOCR in trigger_appuse_snapshot
        let matches = cachedResults.filter { block in
            // Use intersection to see if the text block is mostly within the element
            let intersection = block.frame.intersection(frame)
            if intersection.isEmpty { return false }
            
            // If the intersection covers a good chunk of the text block, it's a match
            let intersectionArea = intersection.width * intersection.height
            let blockArea = block.frame.width * block.frame.height
            return intersectionArea > (blockArea * 0.5)
        }
        
        return matches.map { $0.text }
            .joined(separator: " ")
    }

    /// Returns all OCR blocks that fall within the specified frame.
    func allResults(within targetFrame: NSRect) -> [(text: String, frame: NSRect)] {
        return cachedResults.filter { targetFrame.intersects($0.frame) }
    }
}

/// Fallback to OCR the screen region covered by `frame` (AX coordinate space).
func ocrElementFrame(_ frame: NSRect) -> String {
    return ScreenOCR.shared.textAt(frame: frame)
}

// MARK: - CoreML Icon Detection

class IconDetector {
    static let shared = IconDetector()
    
    private var vnModel: VNCoreMLModel?
    private var cachedResults: [(confidence: Float, frame: NSRect)] = []
    private var hasPerformed = false
    
    private init() {
        loadModel()
    }
    
    private func loadModel() {
        // Check environment variable first
        if let envPath = ProcessInfo.processInfo.environment["ICON_DETECT_MODEL_PATH"] {
            if tryLoadModel(at: envPath) { return }
        }
        
        // Search relative to executable
        let execPath = CommandLine.arguments[0]
        let execDir = (execPath as NSString).deletingLastPathComponent
        
        let candidates = [
            (execDir as NSString).appendingPathComponent("../omniparser_icon_detect/model_v1_5.mlpackage"),
            (execDir as NSString).appendingPathComponent("omniparser_icon_detect/model_v1_5.mlpackage"),
            (execDir as NSString).appendingPathComponent("models/icon_detect_v1_5.mlpackage"),
            NSString(string: "~/.application-use/models/icon_detect_v1_5.mlpackage").expandingTildeInPath,
        ]
        
        for path in candidates {
            if tryLoadModel(at: path) { return }
        }
    }
    
    private func tryLoadModel(at path: String) -> Bool {
        let fm = FileManager.default
        guard fm.fileExists(atPath: path) else { return false }
        
        let url = URL(fileURLWithPath: path)
        
        // Use a cached compiled model to avoid recompiling every time
        let cacheDir = (NSTemporaryDirectory() as NSString).appendingPathComponent("application-use-icon-detect")
        let compiledPath = (cacheDir as NSString).appendingPathComponent("icon_detect_v1_5.mlmodelc")
        let compiledURL = URL(fileURLWithPath: compiledPath)
        
        var useCache = false
        if fm.fileExists(atPath: compiledPath) {
            if let srcAttrs = try? fm.attributesOfItem(atPath: path),
               let cacheAttrs = try? fm.attributesOfItem(atPath: compiledPath),
               let srcMod = srcAttrs[.modificationDate] as? Date,
               let cacheMod = cacheAttrs[.modificationDate] as? Date {
                useCache = cacheMod >= srcMod
            }
        }
        
        do {
            let mlModel: MLModel
            if useCache {
                mlModel = try MLModel(contentsOf: compiledURL)
            } else {
                let tempCompiled = try MLModel.compileModel(at: url)
                try? fm.createDirectory(atPath: cacheDir, withIntermediateDirectories: true)
                try? fm.removeItem(at: compiledURL)
                try fm.copyItem(at: tempCompiled, to: compiledURL)
                mlModel = try MLModel(contentsOf: compiledURL)
            }
            vnModel = try VNCoreMLModel(for: mlModel)
            return true
        } catch {
            fputs("IconDetector: Failed to load model at \(path): \(error)\n", stderr)
            return false
        }
    }
    
    var isAvailable: Bool { vnModel != nil }
    
    func reset() {
        cachedResults = []
        hasPerformed = false
    }
    
    func detectIcons(cgImage: CGImage, winFrame: NSRect) {
        guard !hasPerformed else { return }
        hasPerformed = true
        guard let vnModel = vnModel else { return }
        
        let winX = winFrame.origin.x
        let winY = winFrame.origin.y
        let winW = winFrame.width
        let winH = winFrame.height
        
        let semaphore = DispatchSemaphore(value: 0)
        let request = VNCoreMLRequest(model: vnModel) { [weak self] req, error in
            defer { semaphore.signal() }
            guard let results = req.results as? [VNRecognizedObjectObservation] else { return }
            
            for observation in results {
                let box = observation.boundingBox
                // Vision normalized box: origin bottom-left, y-up, 0..1
                // Convert to AX screen space (origin top-left, y-down)
                let axX = winX + box.origin.x * winW
                let axY = winY + (1.0 - box.origin.y - box.height) * winH
                let axW = box.width * winW
                let axH = box.height * winH
                
                let conf = observation.confidence
                let frame = NSRect(x: axX, y: axY, width: axW, height: axH)
                self?.cachedResults.append((confidence: conf, frame: frame))
            }
        }
        request.imageCropAndScaleOption = .scaleFill
        
        let handler = VNImageRequestHandler(cgImage: cgImage, options: [:])
        try? handler.perform([request])
        semaphore.wait()
    }
    
    func allResults(within targetFrame: NSRect) -> [(confidence: Float, frame: NSRect)] {
        return cachedResults.filter { targetFrame.intersects($0.frame) }
    }
}

// MARK: - Deduplication Helpers (IoU / Containment)

func computeIntersectionArea(_ a: NSRect, _ b: NSRect) -> CGFloat {
    let x1 = max(a.origin.x, b.origin.x)
    let y1 = max(a.origin.y, b.origin.y)
    let x2 = min(a.origin.x + a.width, b.origin.x + b.width)
    let y2 = min(a.origin.y + a.height, b.origin.y + b.height)
    if x2 <= x1 || y2 <= y1 { return 0 }
    return (x2 - x1) * (y2 - y1)
}

func computeIoU(_ a: NSRect, _ b: NSRect) -> CGFloat {
    let inter = computeIntersectionArea(a, b)
    if inter == 0 { return 0 }
    let unionArea = a.width * a.height + b.width * b.height - inter
    if unionArea <= 0 { return 0 }
    return inter / unionArea
}

func computeContainment(_ a: NSRect, _ b: NSRect) -> CGFloat {
    let inter = computeIntersectionArea(a, b)
    if inter == 0 { return 0 }
    let smallerArea = min(a.width * a.height, b.width * b.height)
    if smallerArea <= 0 { return 0 }
    return inter / smallerArea
}

// MARK: - Element Extractor

class ElementExtractor {
    static func getAXAttribute(_ element: AXUIElement, _ attribute: String) -> CFTypeRef? {
        var value: CFTypeRef?
        let error = AXUIElementCopyAttributeValue(element, attribute as CFString, &value)
        if error == .success { return value }
        return nil
    }

    static func getAXArrayAttribute(_ element: AXUIElement, _ attribute: String) -> [AXUIElement]? {
        var value: CFTypeRef?
        let error = AXUIElementCopyAttributeValue(element, attribute as CFString, &value)
        if error == .success { return value as? [AXUIElement] }
        return nil
    }

    static func isHintable(_ element: AXUIElement) -> Bool {
        let role = getAXAttribute(element, kAXRoleAttribute) as? String ?? ""
        if role == "AXWindow" || role == "AXScrollArea" { return false }
        
        let inputRoles: Set<String> = ["AXTextField", "AXTextArea", "AXSearchField", "AXComboBox"]
        if inputRoles.contains(role) { return true }

        var names: CFArray?
        let error = AXUIElementCopyActionNames(element, &names)
        if error == .success, let actions = names as? [String] {
            let ignoredActions: Set<String> = ["AXShowMenu", "AXScrollToVisible", "AXShowDefaultUI", "AXShowAlternateUI"]
            return Set(actions).subtracting(ignoredActions).count > 0
        }
        return false
    }

    static func getFrame(_ element: AXUIElement) -> NSRect {
        var value: CFTypeRef?
        AXUIElementCopyAttributeValue(element, "AXFrame" as CFString, &value)
        var frame: CGRect = .zero
        if let value = value {
            AXValueGetValue(value as! AXValue, .cgRect, &frame)
        }
        return frame
    }

    static func extractTree(from root: AXUIElement) -> [Element] {
        var results: [Element] = []
        let role = getAXAttribute(root, kAXRoleAttribute) as? String ?? ""
        
        if isHintable(root) {
            let title = getAXAttribute(root, "AXTitle") as? String ?? ""
            let subrole = getAXAttribute(root, "AXSubrole") as? String ?? ""
            let description = getAXAttribute(root, "AXDescription") as? String ?? ""
            let label = getAXAttribute(root, "AXLabel") as? String ?? ""
            let identifier = getAXAttribute(root, "AXIdentifier") as? String ?? ""
            let help = getAXAttribute(root, "AXHelp") as? String ?? ""
            let value = getAXAttribute(root, "AXValue") as? String ?? ""

            // Choose the best candidate for the display name (fallback chain)
            var displayName = title
            if displayName.isEmpty { displayName = subrole }
            if displayName.isEmpty { displayName = description }
            if displayName.isEmpty { displayName = label }
            if displayName.isEmpty { displayName = identifier }
            if displayName.isEmpty { displayName = help }
            if displayName.isEmpty { displayName = value }

            let frame = getFrame(root)
            if frame.width > 0 && frame.height > 0 {
                // Last resort: OCR the element's visible region using Vision
                // Also trigger OCR if the current name is a generic AppKit identifier starting with _NS:
                if displayName.isEmpty || displayName.hasPrefix("_NS") || displayName == "unnamed" {
                    let ocrResult = ocrElementFrame(frame)
                    if !ocrResult.isEmpty {
                        displayName = ocrResult + " (via OCR)"
                    }
                }
                let node = Element(axElement: root, role: role, frame: frame, title: displayName, label: label, identifier: identifier, description: description, help: help)
                if let children = getAXArrayAttribute(root, kAXChildrenAttribute) {
                    for child in children {
                        node.children.append(contentsOf: extractTree(from: child))
                    }
                }
                results.append(node)
                return results
            }
        }
        
        if let children = getAXArrayAttribute(root, kAXChildrenAttribute) {
            for child in children {
                results.append(contentsOf: extractTree(from: child))
            }
        }
        return results
    }
    
    static func flatten(_ tree: [Element]) -> [Element] {
        var flat: [Element] = []
        for node in tree {
            flat.append(node)
            flat.append(contentsOf: flatten(node.children))
        }
        return flat
    }
    
    static func isEditable(_ element: AXUIElement) -> Bool {
        let role = getAXAttribute(element, kAXRoleAttribute) as? String ?? ""
        let inputRoles: Set<String> = ["AXTextField", "AXTextArea", "AXSearchField", "AXComboBox"]
        if inputRoles.contains(role) { return true }
        
        var isSettable: DarwinBoolean = false
        if AXUIElementIsAttributeSettable(element, kAXValueAttribute as CFString, &isSettable) == .success {
            return isSettable.boolValue
        }
        return false
    }
}

// Global references
var activeOverlayWindowController: HintsWindowController?
var pendingHints: [Hint] = []

// MARK: - Frontmost Window Detection

func getFrontmostWindowInfo(appElement: AXUIElement) -> [String: Any]? {
    guard let windowValue = ElementExtractor.getAXAttribute(appElement, kAXMainWindowAttribute) else { return nil }
    let window = windowValue as! AXUIElement
    
    let role = ElementExtractor.getAXAttribute(window, kAXRoleAttribute) as? String ?? ""
    let title = ElementExtractor.getAXAttribute(window, kAXTitleAttribute) as? String ?? ""
    
    var frameDict: [String: Any] = [:]
    var frameValue: CFTypeRef?
    if AXUIElementCopyAttributeValue(window, "AXFrame" as CFString, &frameValue) == .success,
       let val = frameValue {
        var cgRect: CGRect = .zero
        AXValueGetValue(val as! AXValue, .cgRect, &cgRect)
        frameDict = ["x": cgRect.origin.x, "y": cgRect.origin.y, "width": cgRect.width, "height": cgRect.height]
    }
    
    var result: [String: Any] = ["role": role, "title": title]
    if !frameDict.isEmpty { result["frame"] = frameDict }
    return result
}

// MARK: - Caret Detection

func getFocusedCaretScreenRect() -> CGRect? {
    let systemWideElement = AXUIElementCreateSystemWide()
    var focusedElement: CFTypeRef?
    
    let result = AXUIElementCopyAttributeValue(systemWideElement, kAXFocusedUIElementAttribute as CFString, &focusedElement)
    
    if result != .success || focusedElement == nil {
        return nil
    }
    
    let axElement = focusedElement as! AXUIElement
    
    // 1. Try AXTextInsertionPointLineRect (supported by some apps)
    var insertionPointRect: CFTypeRef?
    let rectResult = AXUIElementCopyAttributeValue(axElement, "AXTextInsertionPointLineRect" as CFString, &insertionPointRect)
    
    if rectResult == .success, let rectValue = insertionPointRect {
        var rect = CGRect.zero
        AXValueGetValue(rectValue as! AXValue, .cgRect, &rect)
        if rect.width > 0 || rect.height > 0 {
            return rect
        }
    }
    
    // 2. Fallback: Use AXSelectedTextRange + AXBoundsForRange (Standard for NSTextView/AXTextArea)
    var selectedRangeValue: CFTypeRef?
    if AXUIElementCopyAttributeValue(axElement, kAXSelectedTextRangeAttribute as CFString, &selectedRangeValue) == .success {
        var boundsValue: CFTypeRef?
        // Use the selected range to get its screen bounds
        let parameterizedResult = AXUIElementCopyParameterizedAttributeValue(
            axElement, 
            kAXBoundsForRangeParameterizedAttribute as CFString, 
            selectedRangeValue!, 
            &boundsValue
        )
        
        if parameterizedResult == .success, let rectValue = boundsValue {
            var rect = CGRect.zero
            AXValueGetValue(rectValue as! AXValue, .cgRect, &rect)
            // For a cursor, the width might be small or zero in some reports, 
            // but the height and position should be correct.
            return rect
        }
    }
    
    return nil
}

// MARK: - CGO Exports

/// Step 1: Extract accessibility tree + cursor info. Returns JSON.
/// Does NOT show the overlay yet — call show_appuse_overlay() after screenshots are taken.
@_cdecl("trigger_appuse_snapshot")
public func trigger_appuse_snapshot() -> UnsafeMutablePointer<CChar>? {
    ScreenOCR.shared.reset()
    IconDetector.shared.reset()
    let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
    guard AXIsProcessTrustedWithOptions(options) else {
        return strdup("{\"error\": \"Accessibility permission denied\"}")
    }

    guard let targetApp = NSWorkspace.shared.frontmostApplication else {
        return strdup("{\"error\": \"No frontmost application\"}")
    }

    let pid = targetApp.processIdentifier
    let appElement = AXUIElementCreateApplication(pid)

    var ocrResults: [(text: String, frame: NSRect)] = []
    var windowRect: NSRect?
    if let windowInfo = getFrontmostWindowInfo(appElement: appElement) {
        if let winFrame = windowInfo["frame"] as? [String: Any],
           let x = winFrame["x"] as? CGFloat, let y = winFrame["y"] as? CGFloat,
           let w = winFrame["width"] as? CGFloat, let h = winFrame["height"] as? CGFloat {
            let rect = NSRect(x: x, y: y, width: w, height: h)
            windowRect = rect
            ScreenOCR.shared.performWindowOCR(winFrame: rect)
            // OCR results are already filtered to be within rect by allResults(within:)
            ocrResults = ScreenOCR.shared.allResults(within: rect)
            // Run icon detection on the same captured image
            if let cgImage = ScreenOCR.shared.lastCapturedImage {
                IconDetector.shared.detectIcons(cgImage: cgImage, winFrame: rect)
            }
        }
    }

    guard let mainWindowValue = ElementExtractor.getAXAttribute(appElement, kAXMainWindowAttribute) else {
        return strdup("{\"error\": \"No main window\"}")
    }

    let rootNodes = ElementExtractor.extractTree(from: mainWindowValue as! AXUIElement)
    var flatHintable = ElementExtractor.flatten(rootNodes)

    // Filter elements to be strictly within the window frame
    if let winRect = windowRect {
        flatHintable = flatHintable.filter { winRect.contains($0.frame) }
        ocrResults = ocrResults.filter { winRect.contains($0.frame) }
    }

    // Deduplicate OCR results. If within 10px of AX element, discard.
    var filteredOCR: [(text: String, frame: NSRect)] = []
    for ocr in ocrResults {
        let ocrCenter = GeometryUtils.center(ocr.frame)
        var isDuplicate = false
        for ax in flatHintable {
            let axCenter = GeometryUtils.center(ax.frame)
            let dx = Double(ocrCenter.x - axCenter.x)
            let dy = Double(ocrCenter.y - axCenter.y)
            if sqrt(dx*dx + dy*dy) <= 10.0 {
                isDuplicate = true
                break
            }
        }
        if !isDuplicate {
            filteredOCR.append(ocr)
        }
    }

    // Icon detection results + dedup against AX elements (IoU >= 0.3 or containment >= 0.7)
    var filteredIcons: [(confidence: Float, frame: NSRect)] = []
    if let winRect = windowRect {
        var iconResults = IconDetector.shared.allResults(within: winRect)
        iconResults = iconResults.filter { winRect.contains($0.frame) }
        for icon in iconResults {
            var isDuplicate = false
            for ax in flatHintable {
                let iou = computeIoU(icon.frame, ax.frame)
                let containment = computeContainment(icon.frame, ax.frame)
                if iou >= 0.3 || containment >= 0.7 {
                    isDuplicate = true
                    break
                }
            }
            if !isDuplicate {
                filteredIcons.append(icon)
            }
        }
    }

    let totalCount = flatHintable.count + filteredOCR.count + filteredIcons.count
    let hintStrings = AlphabetHints.hintStrings(linkCount: totalCount)

    pendingHints = []
    var currentHintIndex = 0
    
    // Assign hints to AX elements
    for element in flatHintable {
        let hintText = hintStrings[currentHintIndex]
        element.hint = hintText
        pendingHints.append(Hint(frame: element.frame, text: hintText))
        currentHintIndex += 1
    }

    // Assign hints to OCR elements
    var ocrJson: [[String: Any]] = []
    for ocr in filteredOCR {
        let hintText = hintStrings[currentHintIndex]
        let ocrNode = [
            "name": ocr.text,
            "hint": hintText,
            "frame": ["x": ocr.frame.origin.x, "y": ocr.frame.origin.y, "width": ocr.frame.width, "height": ocr.frame.height]
        ] as [String: Any]
        ocrJson.append(ocrNode)
        
        pendingHints.append(Hint(frame: ocr.frame, text: hintText))
        currentHintIndex += 1
    }

    // Assign hints to Icon elements
    var iconJson: [[String: Any]] = []
    for icon in filteredIcons {
        let hintText = hintStrings[currentHintIndex]
        let iconNode = [
            "confidence": icon.confidence,
            "hint": hintText,
            "frame": ["x": icon.frame.origin.x, "y": icon.frame.origin.y, "width": icon.frame.width, "height": icon.frame.height]
        ] as [String: Any]
        iconJson.append(iconNode)
        
        pendingHints.append(Hint(frame: icon.frame, text: hintText))
        currentHintIndex += 1
    }

    // Return JSON now; overlay will be shown by show_appuse_overlay() after Go takes screenshots.
    var jsonDict: [String: Any] = [
        "appName": targetApp.localizedName ?? "Unknown",
        "bundleID": targetApp.bundleIdentifier ?? "",
        "elements": rootNodes.map { $0.toDict() },
        "ocrElements": ocrJson,
        "iconElements": iconJson
    ]
    if let windowInfo = getFrontmostWindowInfo(appElement: appElement) {
        jsonDict["frontmostWindow"] = windowInfo
    }

    if let caretRect = getFocusedCaretScreenRect() {
        jsonDict["caret"] = [
            "x": caretRect.origin.x,
            "y": caretRect.origin.y,
            "width": caretRect.size.width,
            "height": caretRect.size.height
        ]
    }

    if let jsonData = try? JSONSerialization.data(withJSONObject: jsonDict, options: []),
       let jsonString = String(data: jsonData, encoding: .utf8) {
        return strdup(jsonString)
    }
    return strdup("{}")
}

/// Step 2: Show the hint overlay. Call this AFTER screenshots have been taken.
@_cdecl("show_appuse_overlay")
public func show_appuse_overlay() {
    let hintsToShow = pendingHints
    DispatchQueue.main.async {
        if let existing = activeOverlayWindowController {
            existing.close()
        }
        let frame = NSScreen.main?.frame ?? NSRect(x: 0, y: 0, width: 1920, height: 1080)
        let overlay = HintsWindowController(hints: hintsToShow, screenFrame: frame)
        overlay.showWindow(nil)
        activeOverlayWindowController = overlay
    }
}

@_cdecl("clear_appuse_snapshot")
public func clear_appuse_snapshot() {
    pendingHints = []
    DispatchQueue.main.async {
        if let existing = activeOverlayWindowController {
            existing.close()
            activeOverlayWindowController = nil
        }
    }
}

// MARK: - Actions

@_cdecl("click_at")
public func click_at(x: Double, y: Double) {
    let point = CGPoint(x: x, y: y)
    print("Clicking at: \(point)")
    
    // 1. Try AXPress first
    let systemWide = AXUIElementCreateSystemWide()
    var element: AXUIElement?
    if AXUIElementCopyElementAtPosition(systemWide, Float(x), Float(y), &element) == .success,
       let target = element {
        let result = AXUIElementPerformAction(target, kAXPressAction as CFString)
        if result == .success {
            print("AXPress success at \(point)")
            return
        }
    }
    
    // 2. Fallback to raw mouse click
    mouseClick(at: point)
}

@_cdecl("double_click_at")
public func double_click_at(x: Double, y: Double) {
    let point = CGPoint(x: x, y: y)
    print("Double-clicking at: \(point)")
    mouseDoubleClick(at: point)
}

@_cdecl("right_click_at")
public func right_click_at(x: Double, y: Double) {
    let point = CGPoint(x: x, y: y)
    print("Right-clicking at: \(point)")
    mouseRightClick(at: point)
}

@_cdecl("fill_at")
public func fill_at(x: Double, y: Double, text: UnsafePointer<CChar>) -> Bool {
    let textStr = String(cString: text)
    
    // 1. Backup current clipboard content
    let pasteboard = NSPasteboard.general
    var savedItems: [[String: Data]] = []
    if let items = pasteboard.pasteboardItems {
        for item in items {
            var itemData: [String: Data] = [:]
            for type in item.types {
                if let data = item.data(forType: type) {
                    itemData[type.rawValue] = data
                }
            }
            savedItems.append(itemData)
        }
    }

    // 2. Set the target text to system clipboard
    pasteboard.clearContents()
    pasteboard.setString(textStr, forType: .string)
    
    // 3. If coordinates are provided, click the target to set focus
    if x >= 0 && y >= 0 {
        click_at(x: x, y: y)
        // Short delay to allow focus to shift
        Thread.sleep(forTimeInterval: 0.2)
    }
    
    // 4. Simulate Command + V (v key code: 0x09)
    let source = CGEventSource(stateID: .combinedSessionState)
    let vKeyCode: CGKeyCode = 0x09
    
    // Setting .maskCommand on the V key events is sufficient for a paste action
    let vDown = CGEvent(keyboardEventSource: source, virtualKey: vKeyCode, keyDown: true)
    vDown?.flags = .maskCommand
    
    let vUp = CGEvent(keyboardEventSource: source, virtualKey: vKeyCode, keyDown: false)
    vUp?.flags = .maskCommand
    
    vDown?.post(tap: .cgSessionEventTap)
    vUp?.post(tap: .cgSessionEventTap)
    
    // 5. Short delay and then restore the previous clipboard content
    // Increased to 400ms to ensure the target app has time to process the paste event
    Thread.sleep(forTimeInterval: 0.4)
    
    pasteboard.clearContents()
    for itemData in savedItems {
        let item = NSPasteboardItem()
        for (typeRaw, data) in itemData {
            item.setData(data, forType: NSPasteboard.PasteboardType(typeRaw))
        }
        pasteboard.writeObjects([item])
    }
    
    return true
}

func typeString(_ string: String) {
    let source = CGEventSource(stateID: .combinedSessionState)
    
    // For each utf16 code point (roughly characters)
    for codePoint in string.utf16 {
        let keyDown = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: true)
        var utf16CodePoint = codePoint
        keyDown?.keyboardSetUnicodeString(stringLength: 1, unicodeString: &utf16CodePoint)
        keyDown?.post(tap: .cgSessionEventTap)
        
        let keyUp = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: false)
        keyUp?.keyboardSetUnicodeString(stringLength: 1, unicodeString: &utf16CodePoint)
        keyUp?.post(tap: .cgSessionEventTap)
        
        // Minor sleep to ensure the system processes the keystrokes
        usleep(1000) // 1ms
    }
}

func mouseClick(at point: CGPoint) {
    let source = CGEventSource(stateID: .combinedSessionState)
    let move = CGEvent(mouseEventSource: source, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left)
    let clickDown = CGEvent(mouseEventSource: source, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)
    let clickUp = CGEvent(mouseEventSource: source, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)
    
    move?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.1)
    clickDown?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.05)
    clickUp?.post(tap: .cghidEventTap)
}

func mouseDoubleClick(at point: CGPoint) {
    let source = CGEventSource(stateID: .combinedSessionState)
    let move = CGEvent(mouseEventSource: source, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left)
    
    let click1Down = CGEvent(mouseEventSource: source, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)
    let click1Up = CGEvent(mouseEventSource: source, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)
    click1Down?.setIntegerValueField(.mouseEventClickState, value: 1)
    click1Up?.setIntegerValueField(.mouseEventClickState, value: 1)
    
    let click2Down = CGEvent(mouseEventSource: source, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)
    let click2Up = CGEvent(mouseEventSource: source, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)
    click2Down?.setIntegerValueField(.mouseEventClickState, value: 2)
    click2Up?.setIntegerValueField(.mouseEventClickState, value: 2)
    
    move?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.1)
    
    click1Down?.post(tap: .cghidEventTap)
    click1Up?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.1)
    click2Down?.post(tap: .cghidEventTap)
    click2Up?.post(tap: .cghidEventTap)
}

func mouseRightClick(at point: CGPoint) {
    let source = CGEventSource(stateID: .combinedSessionState)
    let move = CGEvent(mouseEventSource: source, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left)
    let clickDown = CGEvent(mouseEventSource: source, mouseType: .rightMouseDown, mouseCursorPosition: point, mouseButton: .right)
    let clickUp = CGEvent(mouseEventSource: source, mouseType: .rightMouseUp, mouseCursorPosition: point, mouseButton: .right)
    
    move?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.1)
    clickDown?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.05)
    clickUp?.post(tap: .cghidEventTap)
}

@_cdecl("search_apps")
public func search_apps() -> UnsafeMutablePointer<CChar>? {
    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: "/usr/bin/mdfind")
    proc.arguments = ["kMDItemContentType == 'com.apple.application-bundle'"]
    let pipe = Pipe()
    proc.standardOutput = pipe
    
    // Add stderr capture for debugging
    let errPipe = Pipe()
    proc.standardError = errPipe
    
    do {
        try proc.run()
    } catch {
        fputs("Failed to run mdfind: \(error)\n", stderr)
        return strdup("[]")
    }
    
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    let errData = errPipe.fileHandleForReading.readDataToEndOfFile()
    proc.waitUntilExit()
    
    if !errData.isEmpty {
        // fputs("mdfind stderr\n", stderr)
    }
    
    guard let output = String(data: data, encoding: .utf8) else {
        return strdup("[]")
    }
    
    let paths = output.components(separatedBy: .newlines).filter { !$0.isEmpty }
    
    var apps: [[String: String]] = []
    let fm = FileManager.default
    
    for path in paths {
        // Exclude system/internal apps
        if path.contains("/.Trash/") { continue }
        if path.contains("/Library/PrivateFrameworks/") || 
           path.contains("/Library/SystemExtensions/") ||
           path.contains("/Frameworks/") ||
           path.contains("/usr/libexec/") {
            continue
        }
        
        // Only allow /System/Library apps if they are core user-facing utilities (in CoreServices)
        if path.hasPrefix("/System/Library/") && !path.contains("/CoreServices/") {
            continue
        }
        
        let url = URL(fileURLWithPath: path)
        let fileName = url.deletingPathExtension().lastPathComponent
        var displayName = fileName
        
        if let item = MDItemCreate(kCFAllocatorDefault, path as CFString),
           let name = MDItemCopyAttribute(item, kMDItemDisplayName) as? String {
            displayName = name
        } else {
            displayName = fm.displayName(atPath: path)
        }
        
        apps.append(["name": displayName, "fileName": fileName, "path": path])
    }
    
    if let jsonData = try? JSONSerialization.data(withJSONObject: apps, options: []),
       let jsonString = String(data: jsonData, encoding: .utf8) {
        return strdup(jsonString)
    }
    return strdup("[]")
}

@_cdecl("open_app_at_path")
public func open_app_at_path(_ path: UnsafePointer<CChar>) -> Bool {
    let appPath = String(cString: path)
    let url = URL(fileURLWithPath: appPath)
    return NSWorkspace.shared.open(url)
}

@_cdecl("get_bundle_identifier")
public func get_bundle_identifier(_ path: UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>? {
    let appPath = String(cString: path)
    if let bundle = Bundle(path: appPath), let bundleID = bundle.bundleIdentifier {
        return strdup(bundleID)
    }
    return nil
}

@_cdecl("app_has_window")
public func app_has_window(_ bundleID: UnsafePointer<CChar>) -> Bool {
    let bid = String(cString: bundleID)
    let apps = NSRunningApplication.runningApplications(withBundleIdentifier: bid)
    guard let app = apps.first else { return false }
    
    let pid = app.processIdentifier
    let appElement = AXUIElementCreateApplication(pid)
    var windowList: CFTypeRef?
    let result = AXUIElementCopyAttributeValue(appElement, kAXWindowsAttribute as CFString, &windowList)
    
    if result == .success, let windows = windowList as? [AXUIElement], !windows.isEmpty {
        return true
    }
    // Fallback: check CGWindowList for visible windows if AX fails or is empty
    let options = CGWindowListOption(arrayLiteral: .excludeDesktopElements, .optionOnScreenOnly)
    let windowInfos = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []
    for info in windowInfos {
        if let winPID = info[kCGWindowOwnerPID as String] as? Int32, winPID == pid {
            // Check if it's a normal window (usually has a name or layer 0)
            let layer = info[kCGWindowLayer as String] as? Int ?? 0
            if layer == 0 {
                return true
            }
        }
    }
    
    return false
}

@_cdecl("activate_app")
public func activate_app(_ bundleID: UnsafePointer<CChar>) -> Bool {
    let bid = String(cString: bundleID)
    let apps = NSRunningApplication.runningApplications(withBundleIdentifier: bid)
    guard let app = apps.first else {
        fputs("activate_app: App not running for \(bid)\n", stderr)
        return false
    }
    // activateIgnoringOtherApps is deprecated in macOS 14.0, using activateAllWindows instead.
    return app.activate(options: [.activateAllWindows])
}

@_cdecl("terminate_app")
public func terminate_app(_ bundleID: UnsafePointer<CChar>) -> Bool {
    let bid = String(cString: bundleID)
    let apps = NSRunningApplication.runningApplications(withBundleIdentifier: bid)
    guard let app = apps.first else {
        fputs("terminate_app: App not running for \(bid)\n", stderr)
        return false
    }
    return app.terminate()
}

@_cdecl("get_window_frame")
public func get_window_frame(_ bundleID: UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>? {
    let bid = String(cString: bundleID)
    var targetApp: NSRunningApplication?
    
    if bid.isEmpty {
        targetApp = NSWorkspace.shared.frontmostApplication
    } else {
        targetApp = NSRunningApplication.runningApplications(withBundleIdentifier: bid).first
    }
    
    guard let app = targetApp else { return nil }
    let appElement = AXUIElementCreateApplication(app.processIdentifier)
    
    if let windowInfo = getFrontmostWindowInfo(appElement: appElement),
       let winFrame = windowInfo["frame"] as? [String: Any],
       let x = winFrame["x"] as? CGFloat, let y = winFrame["y"] as? CGFloat,
       let w = winFrame["width"] as? CGFloat, let h = winFrame["height"] as? CGFloat {
        let frameStr = "\(Int(x)),\(Int(y)),\(Int(w)),\(Int(h))"
        return strdup(frameStr)
    }
    return nil
}

@_cdecl("save_area_screenshot")
public func save_area_screenshot(_ path: UnsafePointer<CChar>, _ frame: UnsafePointer<CChar>) -> Bool {
    let imgPath = String(cString: path)
    let region = String(cString: frame)
    
    if region.isEmpty {
        // Fallback to screencapture for full screen (works fine without -R)
        let proc = Process()
        proc.launchPath = "/usr/sbin/screencapture"
        proc.arguments = ["-x", imgPath]
        proc.launch()
        proc.waitUntilExit()
        return proc.terminationStatus == 0
    }
    
    // Parse "x,y,w,h" string
    let parts = region.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }
    guard parts.count == 4,
          let x = Double(parts[0]), let y = Double(parts[1]),
          let w = Double(parts[2]), let h = Double(parts[3]) else {
        return false
    }
    
    let rect = CGRect(x: x, y: y, width: w, height: h)
    guard let cgImage = CGWindowListCreateImage(rect, .optionOnScreenOnly, kCGNullWindowID, .boundsIgnoreFraming) else {
        return false
    }
    
    let nsImage = NSImage(cgImage: cgImage, size: NSSize(width: w, height: h))
    guard let tiffRep = nsImage.tiffRepresentation,
          let bitmap = NSBitmapImageRep(data: tiffRep),
          let data = bitmap.representation(using: .png, properties: [:]) else {
        return false
    }
    
    do {
        try data.write(to: URL(fileURLWithPath: imgPath))
        return true
    } catch {
        return false
    }
}

private let keyMap: [String: UInt16] = [
    "a": 0, "b": 11, "c": 8, "d": 2, "e": 14, "f": 3, "g": 5, "h": 4, "i": 34,
    "j": 38, "k": 40, "l": 37, "m": 46, "n": 45, "o": 31, "p": 35, "q": 12,
    "r": 15, "s": 1, "t": 17, "u": 32, "v": 9, "w": 13, "x": 7, "y": 16, "z": 6,
    "0": 29, "1": 18, "2": 19, "3": 20, "4": 21, "5": 23, "6": 22, "7": 26, "8": 28, "9": 25,
    "enter": 36, "return": 36, "esc": 53, "escape": 53, "tab": 48, "space": 49,
    "backspace": 51, "delete": 51, "up": 126, "down": 125, "left": 123, "right": 124,
    "f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97, "f7": 98, "f8": 100,
    "f9": 101, "f10": 109, "f11": 103, "f12": 111,
    "home": 115, "end": 119, "pageup": 116, "pagedown": 121
]

@_cdecl("send_key")
public func send_key(_ key: UnsafePointer<CChar>) -> Bool {
    let rawKeyStr = String(cString: key).lowercased()
    let parts = rawKeyStr.split(separator: "+").map { String($0).trimmingCharacters(in: .whitespaces) }
    
    var keyCode: UInt16 = 0
    var flags = CGEventFlags()
    var keyFound = false
    
    for part in parts {
        switch part {
        case "cmd", "command": flags.insert(.maskCommand)
        case "shift": flags.insert(.maskShift)
        case "alt", "opt", "option": flags.insert(.maskAlternate)
        case "ctrl", "control": flags.insert(.maskControl)
        default:
            if let code = keyMap[part] {
                keyCode = code
                keyFound = true
            } else if let code = UInt16(part) {
                keyCode = code
                keyFound = true
            }
        }
    }
    
    guard keyFound else {
        print("Error: Key not found for '\(rawKeyStr)'")
        return false
    }
    
    print("Sending key \(keyCode) with flags \(flags.rawValue) for '\(rawKeyStr)'")
    
    let source = CGEventSource(stateID: .combinedSessionState)
    let keyDown = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true)
    keyDown?.flags = flags
    keyDown?.post(tap: .cgSessionEventTap)
    
    // Tiny delay between down and up
    usleep(10000) // 10ms
    
    let keyUp = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false)
    keyUp?.flags = flags
    keyUp?.post(tap: .cgSessionEventTap)
    
    return true
}

@_cdecl("scroll_at")
public func scroll_at(_ x: Double, _ y: Double, _ dx: Double, _ dy: Double) {
    let source = CGEventSource(stateID: .combinedSessionState)
    let point = CGPoint(x: x, y: y)
    
    // 1. Move mouse to the target area first
    let moveEvent = CGEvent(mouseEventSource: source, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left)
    moveEvent?.post(tap: .cgSessionEventTap)
    
    // 1.5. Click to ensure focus
    // let clickDown = CGEvent(mouseEventSource: source, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)
    // let clickUp = CGEvent(mouseEventSource: source, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)
    // clickDown?.post(tap: .cgSessionEventTap)
    // clickUp?.post(tap: .cgSessionEventTap)
    
    // 2. Small delay to ensure the system registers the mouse position and focus
    usleep(50000) // 50ms
    
    // 3. Post scroll event
    // wheel1 is vertical, wheel2 is horizontal
    let scrollEvent = CGEvent(scrollWheelEvent2Source: source, units: .pixel, wheelCount: 2, wheel1: Int32(dy), wheel2: Int32(dx), wheel3: 0)
    scrollEvent?.post(tap: .cgSessionEventTap)
}

@_cdecl("check_accessibility_permission")
public func check_accessibility_permission(prompt: Int) -> Bool {
    let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: prompt != 0] as CFDictionary
    return AXIsProcessTrustedWithOptions(options)
}

@_cdecl("check_screen_recording_permission")
public func check_screen_recording_permission() -> Bool {
    if #available(macOS 10.15, *) {
        return CGPreflightScreenCaptureAccess()
    }
    return true
}

