import XCTest
@testable import HelloAppRivet

final class HelloAppRivetTests: XCTestCase {
    func testGreetingUsesTheProvidedName() {
        XCTAssertEqual(Greeting.message(for: "Developer"), "Hello, Developer!")
    }
}
