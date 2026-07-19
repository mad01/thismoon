import XCTest

@testable import TossBinCore

final class ConfigTests: XCTestCase {

    private func write(_ yaml: String) throws -> String {
        let path = NSTemporaryDirectory() + "toss-config-\(UUID().uuidString).yaml"
        try yaml.write(toFile: path, atomically: true, encoding: .utf8)
        addTeardownBlock {
            try? FileManager.default.removeItem(atPath: path)
        }
        return path
    }

    // MARK: - Loading

    func testMissingFileReturnsNil() throws {
        let config = try ConfigLoader.load(path: "/nonexistent/toss-bin/config.yaml")
        XCTAssertNil(config)
    }

    func testEmptyFileReturnsEmptyConfig() throws {
        let path = try write("")
        XCTAssertEqual(try ConfigLoader.load(path: path), TossConfig())
    }

    func testFullConfigParses() throws {
        let path = try write(
            """
            protected_paths:
              - /Volumes/backup
            protected_trees:
              - ~/code/archive
            """
        )
        let config = try XCTUnwrap(try ConfigLoader.load(path: path))
        XCTAssertEqual(config.protectedPaths, ["/Volumes/backup"])
        XCTAssertEqual(config.protectedTrees, ["~/code/archive"])
    }

    func testMissingKeysDefaultToEmpty() throws {
        let path = try write("protected_paths: [/Volumes/backup]")
        let config = try XCTUnwrap(try ConfigLoader.load(path: path))
        XCTAssertEqual(config.protectedPaths, ["/Volumes/backup"])
        XCTAssertTrue(config.protectedTrees.isEmpty)
    }

    func testMalformedYAMLThrows() throws {
        let path = try write("protected_paths: [unclosed")
        XCTAssertThrowsError(try ConfigLoader.load(path: path))
    }

    // MARK: - Normalization

    func testNormalizeExpandsTilde() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(ConfigLoader.normalize(["~/code/archive"]), [home + "/code/archive"])
    }

    func testNormalizeStripsTrailingSlashes() {
        XCTAssertEqual(ConfigLoader.normalize(["/Volumes/backup/"]), ["/Volumes/backup"])
        XCTAssertEqual(ConfigLoader.normalize(["/Volumes/backup///"]), ["/Volumes/backup"])
    }

    func testNormalizeDropsEmptiesAndDedupes() {
        XCTAssertEqual(
            ConfigLoader.normalize(["", "/a", "/a/", "/a"]),
            ["/a"]
        )
    }

    func testExtraProtectionsMapsBothLists() {
        let config = TossConfig(
            protectedPaths: ["/Volumes/backup"],
            protectedTrees: ["/Volumes/media/"]
        )
        let extra = ConfigLoader.extraProtections(from: config)
        XCTAssertEqual(extra.exactPaths, ["/Volumes/backup"])
        XCTAssertEqual(extra.treePaths, ["/Volumes/media"])
    }
}
