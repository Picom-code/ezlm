import XCTest
@testable import EZLM

/// A chat's stream dies when the Mac wakes from sleep, or when the model
/// server restarts under it. These check when asking again is safe, and when
/// the words already on screen must be kept instead.
final class ReconnectTests: XCTestCase {
    private func err(_ code: URLError.Code) -> URLError { URLError(code) }

    func testAsksAgainOnlyBeforeTheFirstWord() {
        // A dead socket or a server that isn't up yet: nothing was written, so a retry is free.
        XCTAssertTrue(ChatModel.retryable(err(.networkConnectionLost), text: "", thinking: "", tries: 0))
        XCTAssertTrue(ChatModel.retryable(err(.cannotConnectToHost), text: "", thinking: "", tries: 2))
        XCTAssertTrue(ChatModel.retryable(err(.timedOut), text: "", thinking: "", tries: 0))
        XCTAssertTrue(ChatModel.retryable(err(.notConnectedToInternet), text: "", thinking: "", tries: 0))

        // Anything already on screen stays: a retry would replace it.
        XCTAssertFalse(ChatModel.retryable(err(.networkConnectionLost), text: "Hello", thinking: "", tries: 0))
        XCTAssertFalse(ChatModel.retryable(err(.networkConnectionLost), text: "", thinking: "Hmm", tries: 0))

        // Bounded, so a server that is really gone doesn't spin forever.
        XCTAssertFalse(ChatModel.retryable(err(.networkConnectionLost), text: "", thinking: "", tries: 3))
    }

    func testStopsAndOtherErrorsAreNotRetried() {
        // Stop (esc) cancels the task: that is not a dropped connection.
        XCTAssertFalse(ChatModel.retryable(err(.cancelled), text: "", thinking: "", tries: 0))
        XCTAssertFalse(ChatModel.retryable(err(.badURL), text: "", thinking: "", tries: 0))
        XCTAssertFalse(ChatModel.retryable(err(.badServerResponse), text: "", thinking: "", tries: 0))
    }
}
