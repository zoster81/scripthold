package handler

import (
	"os"
	"testing"
)

// Provider 63: independent original Zig project origins, distinct from the
// first-stage zmx and libxev cases. Both pinned upstream revisions were fully
// read before defining the expected navigation and the adjacent distractors.
// Third-party Zig source and its inline tests are inspected, never executed.
func TestSourceSymbolsZigSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional pinned original Zig projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "zig63-std-audit-selected", language: "zig",
			search: "PriorityQueue", primary: "lib/std/priority_queue.zig",
			negativeFile: "lib/std/priority_dequeue.zig", negativeQuery: "PriorityQueue",
			paths: []string{"lib/std/priority_queue.zig", "lib/std/priority_dequeue.zig"},
			oracles: []extraAuditEvidence{
				{"lib/std/priority_queue.zig", "pub fn PriorityQueue(comptime T: type, comptime Context: type, comptime compareFn:"},
				{"lib/std/priority_queue.zig", "pub fn init(allocator: Allocator, context: Context) Self {"},
				{"lib/std/priority_queue.zig", "pub fn add(self: *Self, elem: T) !void {"},
				{"lib/std/priority_queue.zig", "fn siftUp(self: *Self, start_index: usize) void {"},
				{"lib/std/priority_queue.zig", "pub fn removeIndex(self: *Self, index: usize) T {"},
				{"lib/std/priority_queue.zig", "pub fn update(self: *Self, elem: T, new_elem: T) !void {"},
				{"lib/std/priority_queue.zig", "test \"add and remove min heap\" {"},
				{"lib/std/priority_queue.zig", "test \"update min heap\" {"},
				{"lib/std/priority_queue.zig", "test \"update after remove\" {"},
				{"lib/std/priority_dequeue.zig", "pub fn PriorityDequeue(comptime T: type, comptime Context: type, comptime compareFn:"},
				{"lib/std/priority_dequeue.zig", "pub fn removeMaxOrNull(self: *Self) ?T {"},
				{"lib/std/priority_dequeue.zig", "test \"add and remove max\" {"},
			},
			targets: []extraAuditSymbol{
				{"lib/std/priority_queue.zig", "init", 27},
				{"lib/std/priority_queue.zig", "add", 42},
				{"lib/std/priority_queue.zig", "removeIndex", 95},
				{"lib/std/priority_queue.zig", "update", 230},
				{"lib/std/priority_queue.zig", "lessThan", 292},
				{"lib/std/priority_dequeue.zig", "peekMin", 134},
				{"lib/std/priority_dequeue.zig", "removeMaxOrNull", 168},
			},
		},
		{
			project: "zig63-ghostty-audit-selected", language: "zig",
			search: "TerminalSearch", primary: "src/terminal/search/terminal.zig",
			negativeFile: "src/terminal/search.zig", negativeQuery: "TerminalSearch",
			paths: []string{"src/terminal/search/terminal.zig", "src/terminal/search.zig"},
			oracles: []extraAuditEvidence{
				{"src/terminal/search/terminal.zig", "pub const TerminalSearch = struct {"},
				{"src/terminal/search/terminal.zig", "pub fn init("},
				{"src/terminal/search/terminal.zig", "pub fn feed("},
				{"src/terminal/search/terminal.zig", "if (updated) self.stale_viewport_matches = true;"},
				{"src/terminal/search/terminal.zig", "pub fn viewportMatches("},
				{"src/terminal/search/terminal.zig", "if (!self.stale_viewport_matches) return self.viewport_matches.items;"},
				{"src/terminal/search/terminal.zig", "fn clearViewportMatches(self: *TerminalSearch) void {"},
				{"src/terminal/search/terminal.zig", "pub fn select("},
				{"src/terminal/search/terminal.zig", "test \"viewport matches are cached until the next feed\" {"},
				{"src/terminal/search/terminal.zig", "test \"select after active screen removal\" {"},
				{"src/terminal/search/terminal.zig", "test \"feed after complete discovers prepended snapshot history\" {"},
				{"src/terminal/search.zig", "pub const Terminal = @import(\"search/terminal.zig\").TerminalSearch;"},
				{"src/terminal/search.zig", "pub const Viewport = @import(\"search/viewport.zig\").ViewportSearch;"},
			},
			targets: []extraAuditSymbol{
				{"src/terminal/search/terminal.zig", "init", 82},
				{"src/terminal/search/terminal.zig", "status", 170},
				{"src/terminal/search/terminal.zig", "feed", 257},
				{"src/terminal/search/terminal.zig", "viewportMatches", 372},
				{"src/terminal/search/terminal.zig", "clearViewportMatches", 399},
				{"src/terminal/search/terminal.zig", "select", 416},
				{"src/terminal/search.zig", "Terminal", 8},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
