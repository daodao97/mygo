// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Roundtrip",
    platforms: [.macOS(.v13), .iOS(.v16)],
    products: [.library(name: "Roundtrip", type: .dynamic, targets: ["Roundtrip"])],
    dependencies: [],
    targets: [
        .target(name: "Roundtrip"),
    ],
    swiftLanguageModes: [.v5]
)
