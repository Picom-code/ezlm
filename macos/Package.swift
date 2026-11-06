// swift-tools-version:5.10
import PackageDescription

let package = Package(
    name: "EZLM",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(name: "EZLM", path: "Sources/EZLM"),
        .testTarget(name: "EZLMTests", dependencies: ["EZLM"], path: "Tests/EZLMTests"),
    ]
)
