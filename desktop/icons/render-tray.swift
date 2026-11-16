import AppKit
import CoreGraphics
import Foundation

// Farm's three stacked screen silhouettes, derived from the application mark.
// The exposed rear screens remain separate through transparent negative space.
// Keep the application icon untouched; this source renders menu-bar assets only.
func screenPaths() -> [CGPath] {
    let back = CGMutablePath()
    back.move(to: CGPoint(x: 139, y: 184))
    back.addCurve(to: CGPoint(x: 103, y: 217), control1: CGPoint(x: 119, y: 181), control2: CGPoint(x: 103, y: 196))
    back.addLine(to: CGPoint(x: 103, y: 329))
    back.addCurve(to: CGPoint(x: 137, y: 365), control1: CGPoint(x: 103, y: 349), control2: CGPoint(x: 116, y: 360))
    back.addLine(to: CGPoint(x: 142, y: 367))
    back.addLine(to: CGPoint(x: 142, y: 185))
    back.closeSubpath()

    let middle = CGMutablePath()
    middle.move(to: CGPoint(x: 199, y: 144))
    middle.addCurve(to: CGPoint(x: 154, y: 185), control1: CGPoint(x: 175, y: 139), control2: CGPoint(x: 154, y: 158))
    middle.addLine(to: CGPoint(x: 154, y: 350))
    middle.addCurve(to: CGPoint(x: 193, y: 391), control1: CGPoint(x: 154, y: 373), control2: CGPoint(x: 170, y: 386))
    middle.addLine(to: CGPoint(x: 207, y: 394))
    middle.addLine(to: CGPoint(x: 207, y: 145))
    middle.closeSubpath()

    let front = CGMutablePath()
    front.move(to: CGPoint(x: 263, y: 109))
    front.addCurve(to: CGPoint(x: 219, y: 149), control1: CGPoint(x: 239, y: 105), control2: CGPoint(x: 219, y: 124))
    front.addLine(to: CGPoint(x: 219, y: 369))
    front.addCurve(to: CGPoint(x: 263, y: 408), control1: CGPoint(x: 219, y: 394), control2: CGPoint(x: 237, y: 412))
    front.addLine(to: CGPoint(x: 365, y: 386))
    front.addCurve(to: CGPoint(x: 399, y: 343), control1: CGPoint(x: 386, y: 382), control2: CGPoint(x: 399, y: 366))
    front.addLine(to: CGPoint(x: 399, y: 172))
    front.addCurve(to: CGPoint(x: 365, y: 128), control1: CGPoint(x: 399, y: 147), control2: CGPoint(x: 386, y: 132))
    front.closeSubpath()
    return [back, middle, front]
}

func render(_ size: Int, to url: URL) throws {
    let byteCount = size * size * 4
    let pixels = UnsafeMutablePointer<UInt8>.allocate(capacity: byteCount)
    defer { pixels.deallocate() }
    pixels.initialize(repeating: 0, count: byteCount)
    guard let context = CGContext(data: pixels, width: size, height: size, bitsPerComponent: 8,
        bytesPerRow: size * 4, space: CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { fatalError("Cannot create tray drawing context") }
    context.setAllowsAntialiasing(true)
    context.setShouldAntialias(true)
    context.translateBy(x: 0, y: CGFloat(size))
    context.scaleBy(x: CGFloat(size) / 340, y: -CGFloat(size) / 340)
    context.translateBy(x: -80, y: -90)
    context.setFillColor(CGColor(gray: 1, alpha: 1))
    for path in screenPaths() { context.addPath(path); context.fillPath() }

    // Encode straight RGBA: RGB is always white; edge smoothing lives in alpha.
    guard let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bitmapFormat: .alphaNonpremultiplied,
        bytesPerRow: size * 4, bitsPerPixel: 32), let rgba = bitmap.bitmapData else { fatalError("Cannot allocate tray bitmap") }
    for index in stride(from: 0, to: byteCount, by: 4) {
        rgba[index] = 255; rgba[index + 1] = 255; rgba[index + 2] = 255
        rgba[index + 3] = pixels[index + 3]
    }
    guard let png = bitmap.representation(using: .png, properties: [:]) else { fatalError("Cannot encode tray PNG") }
    try png.write(to: url, options: .atomic)
    print("Rendered \(url.path) \(size)x\(size)")
}

let directory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
try render(36, to: directory.appendingPathComponent("tray-white.png"))
try render(1024, to: directory.appendingPathComponent("tray-white-source.png"))
