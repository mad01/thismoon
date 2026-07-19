import XCTest
@testable import TossBinCore

final class SafetyTests: XCTestCase {

    // MARK: - System Path Protection

    func testBlocksRootPath() {
        XCTAssertEqual(
            PathSafety.validate(path: "/"),
            .protectedSystemPath("/")
        )
    }

    func testBlocksSystemDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/System"),
            .protectedSystemPath("/System")
        )
    }

    func testBlocksLibraryDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Library"),
            .protectedSystemPath("/Library")
        )
    }

    func testBlocksApplicationsDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Applications"),
            .protectedSystemPath("/Applications")
        )
    }

    func testBlocksUsersDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Users"),
            .protectedSystemPath("/Users")
        )
    }

    func testBlocksBinDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/bin"),
            .protectedSystemPath("/bin")
        )
    }

    func testBlocksUsrDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/usr"),
            .protectedSystemPath("/usr")
        )
    }

    func testBlocksEtcDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/etc"),
            .protectedSystemPath("/etc")
        )
    }

    func testBlocksVarDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/var"),
            .protectedSystemPath("/var")
        )
    }

    func testBlocksVolumesDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Volumes"),
            .protectedSystemPath("/Volumes")
        )
    }

    func testBlocksPrivateDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: "/private"),
            .protectedSystemPath("/private")
        )
    }

    // MARK: - Home Directory Protection

    func testBlocksHomeDirectory() {
        XCTAssertEqual(
            PathSafety.validate(path: NSHomeDirectory()),
            .homeDirectoryItself
        )
    }

    func testBlocksHomeTilde() {
        // Expand ~ and test
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: home),
            .homeDirectoryItself
        )
    }

    func testBlocksHomeTrashDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: "\(home)/.Trash"),
            .protectedHomePath(".Trash")
        )
    }

    func testBlocksHomeLibraryDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: "\(home)/Library"),
            .protectedHomePath("Library")
        )
    }

    func testBlocksHomeDocumentsDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: "\(home)/Documents"),
            .protectedHomePath("Documents")
        )
    }

    func testBlocksHomeDesktopDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: "\(home)/Desktop"),
            .protectedHomePath("Desktop")
        )
    }

    func testBlocksHomeDownloadsDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertEqual(
            PathSafety.validate(path: "\(home)/Downloads"),
            .protectedHomePath("Downloads")
        )
    }

    // MARK: - Allowed Paths

    func testAllowsFileInsideProtectedDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertNil(PathSafety.validate(path: "\(home)/Documents/somefile.txt"))
    }

    func testAllowsFileInsideLibraryCaches() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertNil(PathSafety.validate(path: "\(home)/Library/Caches/something"))
    }

    func testAllowsRegularFilePath() {
        XCTAssertNil(PathSafety.validate(path: "/tmp/somefile.txt"))
    }

    func testAllowsUserProjectDirectory() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertNil(PathSafety.validate(path: "\(home)/code/myproject"))
    }

    // MARK: - Edge Cases

    func testBlocksEmptyPath() {
        XCTAssertEqual(
            PathSafety.validate(path: ""),
            .emptyPath
        )
    }

    func testHandlesTrailingSlash() {
        XCTAssertEqual(
            PathSafety.validate(path: "/System/"),
            .protectedSystemPath("/System")
        )
    }

    func testBlocksPathTraversalToRoot() {
        // /tmp/../ resolves to /
        XCTAssertNotNil(PathSafety.validate(path: "/tmp/.."))
    }

    func testBlocksPathTraversalToSystem() {
        // This would resolve to /System
        XCTAssertNotNil(PathSafety.validate(path: "/tmp/../System"))
    }

    // MARK: - Symlink / Private Path Resolution

    func testBlocksPrivateEtc() {
        XCTAssertNotNil(PathSafety.validate(path: "/private/etc"))
    }

    func testBlocksPrivateVar() {
        XCTAssertNotNil(PathSafety.validate(path: "/private/var"))
    }

    func testBlocksPrivateTmp() {
        XCTAssertNotNil(PathSafety.validate(path: "/private/tmp"))
    }

    func testBlocksTrailingSlashOnPrivateEtc() {
        XCTAssertNotNil(PathSafety.validate(path: "/private/etc/"))
    }

    // MARK: - Tree Deny List (subtree blocked)

    func testBlocksSystemLibrary() {
        XCTAssertNotNil(PathSafety.validate(path: "/System/Library"))
    }

    func testBlocksBinSh() {
        XCTAssertNotNil(PathSafety.validate(path: "/bin/sh"))
    }

    func testBlocksSbinChild() {
        XCTAssertNotNil(PathSafety.validate(path: "/sbin/fsck"))
    }

    func testAllowsPrivateEtcHosts() {
        // /private/etc/hosts resolves to /etc/hosts via macOS firmlinks;
        // /etc is exact-only so children are allowed
        XCTAssertNil(PathSafety.validate(path: "/private/etc/hosts"))
    }

    func testBlocksCoresChild() {
        XCTAssertNotNil(PathSafety.validate(path: "/cores/core.1234"))
    }

    func testBlocksDocumentRevisions() {
        XCTAssertNotNil(PathSafety.validate(path: "/.DocumentRevisions-V100"))
    }

    func testBlocksDocumentRevisionsChild() {
        XCTAssertNotNil(PathSafety.validate(path: "/.DocumentRevisions-V100/db-V1"))
    }

    func testBlocksPKInstallSandbox() {
        XCTAssertNotNil(PathSafety.validate(path: "/.PKInstallSandboxManager"))
    }

    func testBlocksPKInstallSandboxSystemSoftware() {
        XCTAssertNotNil(PathSafety.validate(path: "/.PKInstallSandboxManager-SystemSoftware"))
    }

    func testBlocksMobileBackups() {
        XCTAssertNotNil(PathSafety.validate(path: "/.MobileBackups"))
    }

    func testBlocksFileAnchor() {
        XCTAssertNotNil(PathSafety.validate(path: "/.file"))
    }

    // /usr is tree-protected but /usr/local is exempted (Homebrew)
    func testBlocksUsrBin() {
        XCTAssertNotNil(PathSafety.validate(path: "/usr/bin"))
    }

    func testBlocksUsrLib() {
        XCTAssertNotNil(PathSafety.validate(path: "/usr/lib"))
    }

    func testBlocksUsrShare() {
        XCTAssertNotNil(PathSafety.validate(path: "/usr/share"))
    }

    // MARK: - Exact Deny List (children allowed)

    func testAllowsUsrLocal() {
        XCTAssertNil(PathSafety.validate(path: "/usr/local"))
    }

    func testAllowsUsrLocalBin() {
        XCTAssertNil(PathSafety.validate(path: "/usr/local/bin/mytool"))
    }

    func testAllowsEtcHosts() {
        XCTAssertNil(PathSafety.validate(path: "/etc/hosts"))
    }

    func testAllowsVarLog() {
        XCTAssertNil(PathSafety.validate(path: "/var/log"))
    }

    func testAllowsSafariAppSymlink() {
        // Safari.app is a symlink into the SIP-protected cryptex. Symlinks
        // are validated by where the link lives (/Applications, children
        // allowed), not the target — and SIP blocks the actual move anyway.
        XCTAssertNil(PathSafety.validate(path: "/Applications/Safari.app"))
    }

    func testAllowsUserInstalledApp() {
        // Non-system apps in /Applications should be allowed
        XCTAssertNil(PathSafety.validate(path: "/Applications/SomeUserApp.app"))
    }

    func testAllowsOptHomebrew() {
        XCTAssertNil(PathSafety.validate(path: "/opt/homebrew"))
    }

    // MARK: - Symlinks (validate the link's location, not its target)

    private func makeTempDir() throws -> URL {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("toss-bin-tests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir
    }

    func testAllowsSymlinkPointingAtProtectedHomeDirectory() throws {
        let dir = try makeTempDir()
        defer { try? FileManager.default.removeItem(at: dir) }
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let link = dir.appendingPathComponent("docs-link")
        try FileManager.default.createSymbolicLink(
            atPath: link.path,
            withDestinationPath: home + "/Documents"
        )
        XCTAssertNil(PathSafety.validate(path: link.path))
    }

    func testAllowsSymlinkPointingAtProtectedSystemTree() throws {
        let dir = try makeTempDir()
        defer { try? FileManager.default.removeItem(at: dir) }
        let link = dir.appendingPathComponent("bin-link")
        try FileManager.default.createSymbolicLink(
            atPath: link.path,
            withDestinationPath: "/usr/bin"
        )
        XCTAssertNil(PathSafety.validate(path: link.path))
    }

    func testAllowsBrokenSymlink() throws {
        let dir = try makeTempDir()
        defer { try? FileManager.default.removeItem(at: dir) }
        let link = dir.appendingPathComponent("broken-link")
        try FileManager.default.createSymbolicLink(
            atPath: link.path,
            withDestinationPath: "/nonexistent-target-for-toss-bin-tests"
        )
        XCTAssertNil(PathSafety.validate(path: link.path))
    }

    // MARK: - Deny List Completeness

    func testAllSystemPathsInDenyLists() {
        let requiredExact = [
            "/",
            "/Library",
            "/Applications",
            "/Users",
            "/Volumes",
            "/etc",
            "/var",
            "/tmp",
        ]
        for path in requiredExact {
            XCTAssertTrue(
                PathSafety.denyList.contains(path),
                "Deny list should contain \(path)"
            )
        }

        let requiredTree = [
            "/System",
            "/bin",
            "/sbin",
            "/usr",
            "/private",
            "/cores",
            "/.DocumentRevisions-V100",
            "/.PKInstallSandboxManager",
            "/.PKInstallSandboxManager-SystemSoftware",
            "/.MobileBackups",
            "/.file",
        ]
        for path in requiredTree {
            XCTAssertTrue(
                PathSafety.treeDenyList.contains(path),
                "Tree deny list should contain \(path)"
            )
        }
    }

    func testAllHomePathsInDenyList() {
        let requiredPaths = [
            ".Trash",
            "Library",
            "Documents",
            "Desktop",
            "Downloads",
            "Pictures",
            "Music",
            "Movies",
        ]

        for path in requiredPaths {
            XCTAssertTrue(
                PathSafety.homeDenyList.contains(path),
                "Home deny list should contain \(path)"
            )
        }
    }
}
