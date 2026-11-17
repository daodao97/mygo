// swift-tools-version: 6.0
import PackageDescription

// The Swift of the native module sqlitecore, for iOS only: on macOS the
// module is its Zig. It links SQLite 3.53.4, which build-xcframework.sh
// builds from the amalgamation that the Zig build pins, with its options.
let package = Package(
    name: "Sqlitecore",
    platforms: [.iOS(.v16)],
    products: [.library(name: "Sqlitecore", type: .dynamic, targets: ["Sqlitecore"])],
    dependencies: [],
    targets: [
        .binaryTarget(
            name: "CSQLite",
            url: "https://github.com/egoist/mygo/releases/download/native-sqlitecore/CSQLite-3.53.4.xcframework.zip",
            checksum: "5a48e398289f68487c6515fe7cd6b3be71217980a8b4e215772bf58ee5f8c177"
        ),
        .target(name: "Sqlitecore", dependencies: ["CSQLite"]),
    ],
    swiftLanguageModes: [.v5]
)
