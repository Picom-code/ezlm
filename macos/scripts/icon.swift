// Renders EZLM's app icon: black squircle, "EZ" grey + "LM" white in heavy mono.
import AppKit

let size: CGFloat = 1024
let img = NSImage(size: NSSize(width: size, height: size))
img.lockFocus()
let rect = NSRect(x: 0, y: 0, width: size, height: size).insetBy(dx: 100, dy: 100)
let path = NSBezierPath(roundedRect: rect, xRadius: 185, yRadius: 185)
NSColor.black.setFill(); path.fill()
NSColor(white: 0.18, alpha: 1).setStroke(); path.lineWidth = 6; path.stroke()

let font = NSFont.monospacedSystemFont(ofSize: 300, weight: .heavy)
let s = NSMutableAttributedString(string: "EZ", attributes: [.font: font, .foregroundColor: NSColor(white: 0.42, alpha: 1), .kern: -14])
s.append(NSAttributedString(string: "LM", attributes: [.font: font, .foregroundColor: NSColor(white: 0.93, alpha: 1), .kern: -14]))
let b = s.size()
s.draw(at: NSPoint(x: (size - b.width) / 2 + 7, y: (size - b.height) / 2 + 10))

// Lilac underline block: the "signal" accent.
NSColor(red: 0.725, green: 0.659, blue: 1, alpha: 1).setFill()
for i in 0..<10 { NSRect(x: 232 + CGFloat(i) * 58, y: 300, width: 46, height: 22).fill() }
img.unlockFocus()

let tiff = img.tiffRepresentation!
let png = NSBitmapImageRep(data: tiff)!.representation(using: .png, properties: [:])!
try! png.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
