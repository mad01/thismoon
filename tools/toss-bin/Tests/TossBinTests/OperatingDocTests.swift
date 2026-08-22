import XCTest

@testable import TossBinCore

/// Guards the codegenned operating doc: the generated constant must carry
/// the doc's heading and no unrendered template markers. The Makefile
/// regenerates the constant before every build, so a mismatch here means
/// the generated file was edited by hand or the codegen step was skipped.
final class OperatingDocTests: XCTestCase {
    func testDocStartsWithHeading() {
        XCTAssertTrue(
            operatingDoc.hasPrefix("# operating toss-bin"),
            "operatingDoc must begin with the operating.md heading"
        )
    }

    func testDocHasNoTemplatePlaceholders() {
        XCTAssertFalse(
            operatingDoc.contains("{{"),
            "operatingDoc must not contain template placeholders"
        )
    }
}
