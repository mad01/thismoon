import XCTest

@testable import TossBinCore

/// Covers the --temp-only move. Every test works in fresh scratch
/// directories: `inside` under the real /tmp root, and `outside` under the
/// per-user temporary directory (/var/folders on macOS, outside every root),
/// which also holds a scratch home whose trash the entries move into. The
/// real ~/.Trash is never touched.
final class TempOnlyMoveTests: XCTestCase {

    private var inside: String!
    private var outside: String!
    private var home: String!
    private var trashPath: String!
    private let date = "2026-01-02"
    private let fileManager = FileManager.default

    override func setUpWithError() throws {
        var template = Array("/tmp/toss-bin-temp-only-move-tests.XXXXXX".utf8CString)
        let created = template.withUnsafeMutableBufferPointer { buffer in
            mkdtemp(buffer.baseAddress!)
        }
        XCTAssertNotNil(created, "mkdtemp under /tmp failed")
        inside = String(cString: template)

        outside = fileManager.temporaryDirectory
            .appendingPathComponent("toss-bin-temp-only-move-tests-\(UUID().uuidString)").path
        home = outside + "/home"
        trashPath = home + "/.Trash/my-trash/" + date
        try fileManager.createDirectory(atPath: home, withIntermediateDirectories: true)
        try fileManager.createDirectory(atPath: inside + "/real", withIntermediateDirectories: true)
        fileManager.createFile(atPath: inside + "/real/f.txt", contents: nil)
        fileManager.createFile(atPath: outside + "/f.txt", contents: nil)
    }

    override func tearDownWithError() throws {
        try? fileManager.removeItem(atPath: inside)
        try? fileManager.removeItem(atPath: outside)
    }

    private func locate(_ path: String) throws -> TempOnly.Location {
        try TempOnly.locate(path: path, home: outside, cwd: "/").get()
    }

    private func openTrash(home trashHome: String? = nil) -> Result<TempOnlyMove.Directory, TempOnlyMove.Failure> {
        TempOnlyMove.openTrash(home: trashHome ?? home, date: date)
    }

    private func moveToTrash(
        _ location: TempOnly.Location,
        home trashHome: String? = nil
    ) -> Result<String, TempOnlyMove.Failure> {
        openTrash(home: trashHome).flatMap { trash in
            TempOnlyMove.prepare(location).flatMap { TempOnlyMove.move($0, into: trash) }
        }
    }

    private func exists(_ path: String) -> Bool {
        (try? fileManager.attributesOfItem(atPath: path)) != nil
    }

    private func isRealDirectory(_ path: String) -> Bool {
        (try? fileManager.attributesOfItem(atPath: path))?[.type] as? FileAttributeType == .typeDirectory
    }

    /// A directory outside every root that a planted trash link points at.
    private func linkTarget() throws -> String {
        let target = outside + "/elsewhere"
        try fileManager.createDirectory(atPath: target, withIntermediateDirectories: true)
        return target
    }

    /// The planted link is refused, nothing lands where it points, and the
    /// entry stays put.
    private func assertTrashLinkRefused(_ name: String, target: String, file: StaticString = #filePath, line: UInt = #line) throws {
        let result = try moveToTrash(locate(inside + "/real/f.txt"))
        XCTAssertEqual(result.failure, .symlinkedTrash(name), file: file, line: line)
        XCTAssertEqual(try fileManager.contentsOfDirectory(atPath: target), [], file: file, line: line)
        XCTAssertTrue(exists(inside + "/real/f.txt"), file: file, line: line)
    }

    // MARK: - The move

    func testMovesEntryFromCheckedDirectory() throws {
        let result = try moveToTrash(locate(inside + "/real/f.txt"))
        XCTAssertEqual(result, .success("f.txt"))
        XCTAssertFalse(exists(inside + "/real/f.txt"))
        XCTAssertTrue(exists(trashPath + "/f.txt"))
    }

    func testMovesDirectoryWhole() throws {
        let location = try locate(inside + "/real")
        let entry = try TempOnlyMove.prepare(location).get()
        XCTAssertTrue(entry.isDirectory)
        let trash = try openTrash().get()
        XCTAssertEqual(TempOnlyMove.move(entry, into: trash), .success("real"))
        XCTAssertTrue(exists(trashPath + "/real/f.txt"))
    }

    func testFinalSymlinkMovesAsLink() throws {
        try fileManager.createSymbolicLink(atPath: inside + "/link", withDestinationPath: outside + "/f.txt")
        let entry = try TempOnlyMove.prepare(locate(inside + "/link")).get()
        XCTAssertFalse(entry.isDirectory)
        let trash = try openTrash().get()
        XCTAssertEqual(TempOnlyMove.move(entry, into: trash), .success("link"))
        XCTAssertEqual(try fileManager.destinationOfSymbolicLink(atPath: trashPath + "/link"), outside + "/f.txt")
        XCTAssertTrue(exists(outside + "/f.txt"))
    }

    func testNameClashNeverOverwrites() throws {
        try fileManager.createDirectory(atPath: trashPath, withIntermediateDirectories: true)
        fileManager.createFile(atPath: trashPath + "/f.txt", contents: Data("kept".utf8))
        try fileManager.createSymbolicLink(atPath: trashPath + "/f.1.txt", withDestinationPath: "/nonexistent")
        let result = try moveToTrash(locate(inside + "/real/f.txt"))
        XCTAssertEqual(result, .success("f.2.txt"))
        XCTAssertEqual(fileManager.contents(atPath: trashPath + "/f.txt"), Data("kept".utf8))
        XCTAssertNotNil(try? fileManager.destinationOfSymbolicLink(atPath: trashPath + "/f.1.txt"))
    }

    func testMissingEntryAndMissingParent() throws {
        XCTAssertEqual(TempOnlyMove.prepare(try locate(inside + "/real/gone.txt")).failure, .missing)
        XCTAssertEqual(TempOnlyMove.prepare(try locate(inside + "/gone/build/out.o")).failure, .missing)
    }

    func testRegularFileAsParentReadsAsMissing() throws {
        XCTAssertEqual(TempOnlyMove.prepare(try locate(inside + "/real/f.txt/x")).failure, .missing)
    }

    // MARK: - Swaps between the check and the move

    /// The check resolved the parent to `real`; a process that can write
    /// /tmp then replaces `real` with a symlink to an outside directory.
    func testRefusesParentSwappedForSymlinkAfterCheck() throws {
        let location = try locate(inside + "/real/f.txt")
        try fileManager.moveItem(atPath: inside + "/real", toPath: inside + "/real.old")
        try fileManager.createSymbolicLink(atPath: inside + "/real", withDestinationPath: outside)

        XCTAssertEqual(moveToTrash(location).failure, .symlinkedParent("real"))
        XCTAssertTrue(exists(outside + "/f.txt"))
        XCTAssertFalse(exists(trashPath + "/f.txt"))
    }

    /// The caller wrote a symlinked parent that pointed inside; repointing
    /// it at an outside directory after the check changes nothing, because
    /// the move walks the directory the check resolved.
    func testRepointedSymlinkedParentStillMovesFromCheckedDirectory() throws {
        try fileManager.createSymbolicLink(atPath: inside + "/w", withDestinationPath: inside + "/real")
        let location = try locate(inside + "/w/f.txt")
        XCTAssertTrue(location.directory.hasSuffix("/real"), location.directory)
        try fileManager.removeItem(atPath: inside + "/w")
        try fileManager.createSymbolicLink(atPath: inside + "/w", withDestinationPath: outside)

        XCTAssertEqual(moveToTrash(location), .success("f.txt"))
        XCTAssertFalse(exists(inside + "/real/f.txt"))
        XCTAssertTrue(exists(outside + "/f.txt"))
    }

    /// The walk starts at `/` and takes the root's own real path, so a root
    /// component swapped for a symlink is refused too.
    func testRefusesSwappedComponentAboveTheEntry() throws {
        try fileManager.createDirectory(atPath: inside + "/a/b", withIntermediateDirectories: true)
        fileManager.createFile(atPath: inside + "/a/b/f.txt", contents: nil)
        try fileManager.createDirectory(atPath: outside + "/b", withIntermediateDirectories: true)
        fileManager.createFile(atPath: outside + "/b/f.txt", contents: nil)
        let location = try locate(inside + "/a/b/f.txt")
        try fileManager.moveItem(atPath: inside + "/a", toPath: inside + "/a.old")
        try fileManager.createSymbolicLink(atPath: inside + "/a", withDestinationPath: outside)

        XCTAssertEqual(moveToTrash(location).failure, .symlinkedParent("a"))
        XCTAssertTrue(exists(outside + "/b/f.txt"))
    }

    // MARK: - The trash directory

    func testOpenTrashCreatesWhatIsMissing() throws {
        XCTAssertFalse(exists(home + "/.Trash"))
        _ = try openTrash().get()
        XCTAssertTrue(isRealDirectory(trashPath))
    }

    /// The home is the trusted start and may sit behind a symlink; it is
    /// resolved once, and the entry lands in the real home's trash.
    func testHomeBehindSymlinkIsResolvedOnce() throws {
        let linkedHome = outside + "/home-link"
        try fileManager.createSymbolicLink(atPath: linkedHome, withDestinationPath: home)
        XCTAssertEqual(try moveToTrash(locate(inside + "/real/f.txt"), home: linkedHome), .success("f.txt"))
        XCTAssertTrue(exists(trashPath + "/f.txt"))
    }

    func testRefusesSymlinkedDotTrash() throws {
        let target = try linkTarget()
        try fileManager.createSymbolicLink(atPath: home + "/.Trash", withDestinationPath: target)
        try assertTrashLinkRefused(".Trash", target: target)
    }

    func testRefusesSymlinkedMyTrash() throws {
        let target = try linkTarget()
        try fileManager.createDirectory(atPath: home + "/.Trash", withIntermediateDirectories: true)
        try fileManager.createSymbolicLink(atPath: home + "/.Trash/my-trash", withDestinationPath: target)
        try assertTrashLinkRefused("my-trash", target: target)
    }

    func testRefusesSymlinkedDatedDirectory() throws {
        let target = try linkTarget()
        try fileManager.createDirectory(atPath: home + "/.Trash/my-trash", withIntermediateDirectories: true)
        try fileManager.createSymbolicLink(atPath: trashPath, withDestinationPath: target)
        try assertTrashLinkRefused(date, target: target)
    }

    /// mkdirat never follows a link, so a dangling one does not get its
    /// target created.
    func testRefusesDanglingTrashLinkWithoutCreatingItsTarget() throws {
        let target = outside + "/not-yet"
        try fileManager.createDirectory(atPath: home + "/.Trash", withIntermediateDirectories: true)
        try fileManager.createSymbolicLink(atPath: home + "/.Trash/my-trash", withDestinationPath: target)
        XCTAssertEqual(try moveToTrash(locate(inside + "/real/f.txt")).failure, .symlinkedTrash("my-trash"))
        XCTAssertFalse(exists(target))
        XCTAssertTrue(exists(inside + "/real/f.txt"))
    }

    func testRefusesRegularFileOnTheWayToTheTrash() throws {
        try fileManager.createDirectory(atPath: home + "/.Trash", withIntermediateDirectories: true)
        fileManager.createFile(atPath: home + "/.Trash/my-trash", contents: nil)
        XCTAssertEqual(openTrash().failure, .trashNotDirectory("my-trash"))
    }

    // MARK: - Trash names

    func testTrashNameCandidates() {
        XCTAssertEqual(TrashName.candidate("f.txt", attempt: 0), "f.txt")
        XCTAssertEqual(TrashName.candidate("f.txt", attempt: 2), "f.2.txt")
        XCTAssertEqual(TrashName.candidate("notes", attempt: 1), "notes.1")
        XCTAssertEqual(TrashName.candidate("a.tar.gz", attempt: 1), "a.tar.1.gz")
    }
}

private extension Result {
    var failure: Failure? {
        if case .failure(let failure) = self {
            return failure
        }
        return nil
    }
}
