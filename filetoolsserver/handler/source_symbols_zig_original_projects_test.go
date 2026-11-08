package handler

import (
	"os"
	"testing"
)

// Zig original projects are read as evidence only; no third-party code runs.
func TestSourceSymbolsOriginalZigInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Zig corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "zig-neurosnap-zmx", language: "zig",
			search: "socketDir", primary: "src/cfg.zig",
			negativeFile: "src/label.zig", negativeQuery: "socketDir",
			// main.zig is checked as an adjacent source oracle below, but its
			// large selected analysis currently reports partial coverage.
			paths: []string{
				"src/cfg.zig", "src/posix.zig", "src/test.zig", "src/label.zig",
			},
			oracles: []extraAuditEvidence{
				{"src/cfg.zig", "pub const Cfg = @This();"},
				{"src/cfg.zig", "pub fn init(alloc: std.mem.Allocator, io: std.Io) !Cfg {"},
				{"src/cfg.zig", "lib_posix.getenv(\"ZMX_DIR_MODE\")"},
				{"src/cfg.zig", "lib_posix.getenv(\"ZMX_LOG_MODE\")"},
				{"src/cfg.zig", "fn socketDir(alloc: std.mem.Allocator) ![]const u8 {"},
				{"src/cfg.zig", "fn logDir(alloc: std.mem.Allocator) ![]const u8 {"},
				{"src/posix.zig", "pub fn getenv(key: []const u8) ?[:0]const u8 {"},
				{"src/cfg.zig", "test \"Cfg.init uses default modes when env vars are not set\" {"},
				{"src/cfg.zig", "test \"Cfg.init uses custom modes from env vars\" {"},
				{"src/main.zig", "const Cfg = @import(\"cfg.zig\");"},
				{"src/main.zig", "var cfg = try Cfg.init(gpa, io);"},
				{"src/test.zig", "_ = @import(\"cfg.zig\");"},
				{"src/label.zig", "pub fn init(labels: []const u8) LabelIterator {"},
			},
			targets: []extraAuditSymbol{
				{"src/cfg.zig", "Cfg", 4},
				{"src/cfg.zig", "init", 16},
				{"src/cfg.zig", "socketDir", 44},
				{"src/cfg.zig", "logDir", 58},
				{"src/posix.zig", "getenv", 88},
				{"src/cfg.zig", "mkdirAll", 87},
				{"src/label.zig", "init", 78},
			},
		},
		{
			project: "zig-mitchellh-libxev", language: "zig",
			search: "Intrusive", primary: "src/queue_mpsc.zig",
			negativeFile: "src/api.zig", negativeQuery: "Intrusive",
			paths: []string{
				"src/queue_mpsc.zig", "src/ThreadPool.zig", "src/api.zig",
			},
			oracles: []extraAuditEvidence{
				{"src/queue_mpsc.zig", "pub fn Intrusive(comptime T: type) type {"},
				{"src/queue_mpsc.zig", "return struct {"},
				{"src/queue_mpsc.zig", "pub fn init(self: *Self) void {"},
				{"src/queue_mpsc.zig", "pub fn push(self: *Self, v: *T) void {"},
				{"src/queue_mpsc.zig", "pub fn pop(self: *Self) ?*T {"},
				{"src/queue_mpsc.zig", "const prev = @atomicRmw(*T, &self.head, .Xchg, v, .acq_rel);"},
				{"src/queue_mpsc.zig", "test Intrusive {"},
				{"src/queue_mpsc.zig", "const Queue = Intrusive(Elem);"},
				{"src/ThreadPool.zig", "pub fn push(self: *Batch, batch: Batch) void {"},
				{"src/api.zig", "pub fn Xev(comptime be: Backend, comptime T: type) type {"},
			},
			targets: []extraAuditSymbol{
				{"src/queue_mpsc.zig", "Intrusive", 21},
				{"src/queue_mpsc.zig", "init", 32},
				{"src/queue_mpsc.zig", "push", 40},
				{"src/queue_mpsc.zig", "pop", 48},
				{"src/ThreadPool.zig", "push", 128},
				{"src/api.zig", "Xev", 14},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
