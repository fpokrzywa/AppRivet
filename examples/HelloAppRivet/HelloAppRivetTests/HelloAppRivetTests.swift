import XCTest
@testable import AppRivet_HelloAppRivet

final class HelloAppRivetTests: XCTestCase {
    func testGreetingUsesTheProvidedName() {
        XCTAssertEqual(Greeting.message(for: "Developer"), "Hello, Developer!")
    }
}
