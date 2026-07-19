import XCTest

@testable import TossBinCore

final class ExtraProtectionsTests: XCTestCase {

    private let extra = PathSafety.ExtraProtections(
        exactPaths: ["/Volumes/backup"],
        treePaths: ["/Volumes/media"]
    )

    func testExtraExactPathBlocksThePathItself() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Volumes/backup", extra: extra),
            .protectedSystemPath("/Volumes/backup")
        )
    }

    func testExtraExactPathAllowsContents() {
        XCTAssertNil(PathSafety.validate(path: "/Volumes/backup/old.txt", extra: extra))
    }

    func testExtraTreeBlocksPathAndDescendants() {
        XCTAssertEqual(
            PathSafety.validate(path: "/Volumes/media", extra: extra),
            .protectedSystemPath("/Volumes/media")
        )
        XCTAssertEqual(
            PathSafety.validate(path: "/Volumes/media/movies/film.mp4", extra: extra),
            .protectedSystemPath("/Volumes/media")
        )
    }

    func testExtraTreeDoesNotBlockSiblings() {
        XCTAssertNil(PathSafety.validate(path: "/Volumes/mediastore", extra: extra))
    }

    func testBuiltInsStayBlockedWithExtras() {
        XCTAssertNotNil(PathSafety.validate(path: "/System", extra: extra))
        XCTAssertNotNil(PathSafety.validate(path: "/tmp", extra: extra))
    }

    func testDuplicatingABuiltInIsHarmless() {
        let duplicated = PathSafety.ExtraProtections(
            exactPaths: ["/tmp"],
            treePaths: ["/System"]
        )
        XCTAssertEqual(
            PathSafety.validate(path: "/tmp", extra: duplicated),
            .protectedSystemPath("/tmp")
        )
        XCTAssertNotNil(PathSafety.validate(path: "/System/Library", extra: duplicated))
    }

    func testTreeAllowListExemptionSurvivesExtras() {
        XCTAssertNil(PathSafety.validate(path: "/usr/local/bin/tool", extra: extra))
    }

    func testNoExtrasMatchesDefaultBehavior() {
        XCTAssertNil(PathSafety.validate(path: "/Volumes/backup", extra: .none))
    }
}
