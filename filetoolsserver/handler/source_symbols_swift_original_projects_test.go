package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent Swift source-navigation tasks. The original projects are
// inspected as data; their Swift code, HTTP requests, and tests are not executed.
func TestSourceSymbolsOriginalSwiftInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Swift corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "swift-alamofire-alamofire", language: "swift",
			search: "shouldRetry", primary: "Source/Features/RetryPolicy.swift",
			negativeFile:  "Source/Features/RequestInterceptor.swift",
			negativeQuery: "shouldRetry",
			paths: []string{
				"Source/Features/RetryPolicy.swift",
				"Tests/RetryPolicyTests.swift",
				"Source/Features/RequestInterceptor.swift",
			},
			oracles: []extraAuditEvidence{
				{"Source/Features/RetryPolicy.swift", "open class RetryPolicy: @unchecked Sendable, RequestInterceptor {"},
				{"Source/Features/RetryPolicy.swift", "public static let defaultRetryLimit: UInt = 2"},
				{"Source/Features/RetryPolicy.swift", "public static let defaultExponentialBackoffScale: Double = 0.5"},
				{"Source/Features/RetryPolicy.swift", "open func retry(_ request: Request,"},
				{"Source/Features/RetryPolicy.swift", "request.retryCount < retryLimit, shouldRetry(request: request, dueTo: error)"},
				{"Source/Features/RetryPolicy.swift", "open func shouldRetry(request: Request, dueTo error: any Error) -> Bool {"},
				{"Source/Features/RetryPolicy.swift", "retryableHTTPStatusCodes.contains(statusCode)"},
				{"Tests/RetryPolicyTests.swift", "func testThatRetryPolicyRetriesRequestsBelowRetryLimit() {"},
				{"Tests/RetryPolicyTests.swift", "XCTAssertEqual(results[0]?.delay, 0.5)"},
				{"Tests/RetryPolicyTests.swift", "func testThatRetryPolicyRetriesIdempotentRequests() {"},
				{"Tests/RetryPolicyTests.swift", "func testThatRetryPolicyRetriesRequestsWithRetryableStatusCodes() {"},
				{"Source/Features/RequestInterceptor.swift", "public protocol RequestRetrier: Sendable {"},
				{"Source/Features/RequestInterceptor.swift", "func retry(_ request: Request, for session: Session, dueTo error: any Error, completion:"},
			},
			targets: []extraAuditSymbol{
				{"Source/Features/RetryPolicy.swift", "retry", 308},
				{"Source/Features/RetryPolicy.swift", "shouldRetry", 326},
				{"Tests/RetryPolicyTests.swift", "testThatRetryPolicyRetriesRequestsBelowRetryLimit", 182},
				{"Tests/RetryPolicyTests.swift", "testThatRetryPolicyRetriesIdempotentRequests", 222},
				{"Tests/RetryPolicyTests.swift", "testThatRetryPolicyRetriesRequestsWithRetryableStatusCodes", 251},
				{"Source/Features/RequestInterceptor.swift", "RequestRetrier", 101},
			},
		},
		{
			project: "swift-collections", language: "swift",
			search: "encode", primary: "Sources/DequeModule/Deque/Deque+Codable.swift",
			negativeFile:  "Sources/DequeModule/Deque/Deque+Equatable.swift",
			negativeQuery: "encode",
			paths: []string{
				"Sources/DequeModule/Deque/Deque+Codable.swift",
				"Tests/DequeTests/DequeTests.swift",
				"Sources/DequeModule/Deque/Deque.swift",
				"Sources/DequeModule/Deque/Deque+Equatable.swift",
			},
			oracles: []extraAuditEvidence{
				{"Sources/DequeModule/Deque/Deque.swift", "public struct Deque<Element> {"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "extension Deque: Encodable where Element: Encodable {"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "public func encode(to encoder: Encoder) throws {"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "var container = encoder.unkeyedContainer()"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "extension Deque: Decodable where Element: Decodable {"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "public init(from decoder: Decoder) throws {"},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "self.reserveCapacity(count)"},
				{"Tests/DequeTests/DequeTests.swift", "func test_Encodable() throws {"},
				{"Tests/DequeTests/DequeTests.swift", "let encoded = try MinimalEncoder.encode(deque)"},
				{"Tests/DequeTests/DequeTests.swift", "func test_Decodable() throws {"},
				{"Tests/DequeTests/DequeTests.swift", "let decoded = try MinimalDecoder.decode(input, as: Deque<Int>.self)"},
				{"Sources/DequeModule/Deque/Deque+Equatable.swift", "extension Deque: Equatable where Element: Equatable {"},
				{"Sources/DequeModule/Deque/Deque+Equatable.swift", "public static func ==(left: Self, right: Self) -> Bool {"},
			},
			targets: []extraAuditSymbol{
				{"Sources/DequeModule/Deque/Deque.swift", "Deque", 85},
				{"Sources/DequeModule/Deque/Deque+Codable.swift", "encode", 24},
				{"Tests/DequeTests/DequeTests.swift", "test_Encodable", 104},
				{"Tests/DequeTests/DequeTests.swift", "test_Decodable", 122},
				{"Tests/DequeTests/DequeTests.swift", "test_Equatable_Hashable", 80},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
	// Swift initializers are indexed by the enclosing type name, while the
	// source excerpt spells the declaration as init(...), not Deque(...).
	t.Run("swift-collections-decode-initializer", func(t *testing.T) {
		root := filepath.Join(corpus, "swift-collections")
		path := filepath.Join(root, "Sources", "DequeModule", "Deque", "Deque+Codable.swift")
		requireAdditionalOriginalFile(t, path)
		h := NewHandler([]string{root})
		failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Query: "Deque", Match: "exact",
			Language: "swift", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
		})
		if err != nil || failed != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
			t.Fatalf("Swift constructor find failed: %v, tool=%+v, result=%+v", err, failed, found)
		}
		for _, symbol := range found.Symbols {
			if symbol.Kind != "constructor" || symbol.Name != "Deque" || symbol.NameRange.Start.Line != 40 {
				continue
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: symbol.ID,
				SourceFingerprint: found.Files[0].SourceFingerprint,
				Language:          "swift", Encoding: "utf-8", MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "public init(from decoder: Decoder) throws") {
				t.Fatalf("Swift decode initializer excerpt failed: %v, tool=%+v, result=%+v", err, failed, shown.Show)
			}
			return
		}
		t.Fatalf("original Swift decode initializer not indexed at line 40: %+v", found.Symbols)
	})
}
