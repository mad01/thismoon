// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "toss-bin",
    platforms: [
        .macOS(.v10_15)
    ],
    products: [
        .executable(
            name: "toss-bin",
            targets: ["toss-bin"]
        )
    ],
    dependencies: [
        .package(url: "https://github.com/jpsim/Yams.git", from: "5.1.0")
    ],
    targets: [
        .executableTarget(
            name: "toss-bin",
            dependencies: ["TossBinCore"]
        ),
        .target(
            name: "TossBinCore",
            dependencies: ["Yams"]
        ),
        .testTarget(
            name: "TossBinTests",
            dependencies: ["TossBinCore"]
        )
    ]
)
