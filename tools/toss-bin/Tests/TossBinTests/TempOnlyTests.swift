import XCTest

@testable import TossBinCore

/// Covers the --temp-only containment check. Every test works in fresh
/// scratch directories: `inside` under the real /tmp root, `outside` under
/// the per-user temporary directory (/var/folders on macOS, outside every
/// root), and a fake home under `outside` for the ~/.tmp root. The check
/// only reads the filesystem; nothing here is trashed.
final class TempOnlyTests: XCTestCase {

    private var inside: String!
    private var outside: String!
    private var fakeHome: String!

    override func setUpWithError() throws {
        var template = Array("/tmp/toss-bin-temp-only-tests.XXXXXX".utf8CString)
        let created = template.withUnsafeMutableBufferPointer { buffer in
            mkdtemp(buffer.baseAddress!)
        }
        XCTAssertNotNil(created, "mkdtemp under /tmp failed")
        inside = String(cString: template)

        outside = FileManager.default.temporaryDirectory
            .appendingPathComponent("toss-bin-temp-only-tests-\(UUID().uuidString)").path
        fakeHome = outside + "/home"
        try FileManager.default.createDirectory(
            atPath: fakeHome + "/.tmp",
            withIntermediateDirectories: true
        )
        FileManager.default.createFile(atPath: inside + "/scratch.txt", contents: nil)
        FileManager.default.createFile(atPath: outside + "/keep.txt", contents: nil)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(atPath: inside)
        try? FileManager.default.removeItem(atPath: outside)
    }

    private func check(_ path: String, cwd: String = "/") -> TempOnly.Refusal? {
        TempOnly.check(path: path, home: fakeHome, cwd: cwd)
    }

    private func isOutside(_ refusal: TempOnly.Refusal?) -> Bool {
        if case .outsideRoots = refusal {
            return true
        }
        return false
    }

    // MARK: - Inside a root

    func testAllowsFileInsideTmp() {
        XCTAssertNil(check(inside + "/scratch.txt"))
    }

    func testAllowsMissingFileInsideTmp() {
        XCTAssertNil(check(inside + "/not-there.txt"))
    }

    func testAllowsPathUnderMissingDirectoryInsideTmp() {
        XCTAssertNil(check(inside + "/gone/build/out.o"))
    }

    func testAllowsPrivateTmpSpelling() {
        let privatePath = "/private" + inside + "/scratch.txt"
        XCTAssertNil(check(privatePath))
    }

    func testAllowsPathInsideVarTmp() {
        XCTAssertNil(check("/var/tmp/toss-bin-test-\(UUID().uuidString)"))
    }

    func testAllowsPathInsideHomeTmp() {
        XCTAssertNil(check(fakeHome + "/.tmp/scratch.txt"))
    }

    func testExpandsTildeAgainstHome() {
        XCTAssertNil(check("~/.tmp/scratch.txt"))
        XCTAssertEqual(
            TempOnly.expandTilde("~/.tmp/a", home: fakeHome),
            fakeHome + "/.tmp/a"
        )
        XCTAssertEqual(TempOnly.expandTilde("~other/a", home: fakeHome), "~other/a")
    }

    func testAllowsRelativePathFromCwdInsideTmp() {
        XCTAssertNil(check("scratch.txt", cwd: inside))
    }

    // MARK: - Outside every root

    func testRefusesPathOutsideRoots() {
        XCTAssertTrue(isOutside(check(outside + "/keep.txt")))
    }

    func testRefusesHomeOutsideItsTmp() {
        XCTAssertTrue(isOutside(check(fakeHome + "/notes.txt")))
        XCTAssertTrue(isOutside(check("~/notes.txt")))
    }

    func testRefusesRoot() {
        XCTAssertEqual(check("/"), .outsideRoots("/"))
    }

    func testRefusesEmptyPath() {
        XCTAssertEqual(check(""), .emptyPath)
    }

    func testRefusesRelativeEscapeFromCwd() {
        XCTAssertTrue(isOutside(check("../../etc/hosts", cwd: inside)))
    }

    // MARK: - The roots themselves

    func testRefusesEachRootItself() {
        for root in ["/tmp", "/tmp/", "/private/tmp", "/var/tmp", "/private/var/tmp", "~/.tmp"] {
            guard case .tempRoot = check(root) else {
                XCTFail("\(root) should be refused as a temp root")
                continue
            }
        }
    }

    func testRefusesRootReachedThroughDotDot() {
        XCTAssertEqual(check("/tmp/../tmp"), .tempRoot("/private/tmp"))
    }

    func testRefusesDotEntries() {
        XCTAssertEqual(check(inside + "/."), .dotEntry)
        XCTAssertEqual(check(inside + "/sub/.."), .dotEntry)
    }

    // MARK: - Escapes through .. and symlinks

    func testRefusesDotDotEscape() {
        XCTAssertTrue(isOutside(check("/tmp/../Users/x")))
        XCTAssertTrue(isOutside(check(inside + "/../../etc/hosts")))
    }

    func testRefusesSymlinkedParentEscape() throws {
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/link-to-outside",
            withDestinationPath: outside
        )
        XCTAssertTrue(isOutside(check(inside + "/link-to-outside/keep.txt")))
    }

    func testRefusesSymlinkToHomeAsParent() throws {
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/link-to-home",
            withDestinationPath: NSHomeDirectory()
        )
        XCTAssertTrue(isOutside(check(inside + "/link-to-home/file")))
    }

    /// `..` after a symlinked directory climbs from the link's target, the
    /// way the kernel resolves it; a lexical check would call this inside.
    func testRefusesDotDotAfterSymlinkedDirectory() throws {
        try FileManager.default.createDirectory(
            atPath: outside + "/sub",
            withIntermediateDirectories: true
        )
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/sub-link",
            withDestinationPath: outside + "/sub"
        )
        XCTAssertTrue(isOutside(check(inside + "/sub-link/../keep.txt")))
    }

    func testRefusesHomeTmpThatIsASymlinkOutside() throws {
        let home = outside + "/linked-home"
        try FileManager.default.createDirectory(atPath: home, withIntermediateDirectories: true)
        try FileManager.default.createSymbolicLink(
            atPath: home + "/.tmp",
            withDestinationPath: outside
        )
        let refusal = TempOnly.check(path: home + "/.tmp/keep.txt", home: home, cwd: "/")
        XCTAssertTrue(isOutside(refusal))
    }

    func testRefusesDanglingSymlinkedParent() throws {
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/dangling",
            withDestinationPath: outside + "/does-not-exist-yet"
        )
        XCTAssertEqual(check(inside + "/dangling/file"), .unresolvable)
    }

    func testRefusesDotDotUnderMissingDirectory() {
        XCTAssertEqual(check(inside + "/gone/../../etc/hosts"), .unresolvable)
    }

    // MARK: - The final component is judged where it lives

    func testAllowsSymlinkInsidePointingOutside() throws {
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/link-out",
            withDestinationPath: outside + "/keep.txt"
        )
        XCTAssertNil(check(inside + "/link-out"))
    }

    func testAllowsSymlinkToDirectoryWithTrailingSlash() throws {
        try FileManager.default.createSymbolicLink(
            atPath: inside + "/dir-link",
            withDestinationPath: outside
        )
        XCTAssertNil(check(inside + "/dir-link/"))
    }

    // MARK: - The roots are fixed

    func testRootsAreTheBuiltInSet() {
        XCTAssertEqual(TempOnly.rootNames, ["/tmp", "/var/tmp", "~/.tmp"])
        let roots = TempOnly.roots(home: fakeHome)
        XCTAssertEqual(roots.count, 5)
        XCTAssertEqual(Array(roots.prefix(4)), ["/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"])
        // The home is resolved (/var/folders becomes /private/var/folders);
        // .tmp itself is not.
        XCTAssertTrue(roots[4].hasSuffix(fakeHome + "/.tmp"), roots[4])
    }

    func testAccountHomeIgnoresHomeVariable() {
        let saved = ProcessInfo.processInfo.environment["HOME"]
        setenv("HOME", outside, 1)
        defer {
            if let saved {
                setenv("HOME", saved, 1)
            } else {
                unsetenv("HOME")
            }
        }
        XCTAssertNotEqual(TempOnly.accountHome(), outside)
    }
}
