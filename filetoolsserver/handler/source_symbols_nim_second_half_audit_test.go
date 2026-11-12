package handler

import (
	"os"
	"testing"
)

// Provider 82 uses two further independent Nim origins, excluding the earlier
// compiler and Exercism cases. Every selected original was fully read and
// verified against an immutable upstream Git blob before local handler tests.
func TestSourceSymbolsNimSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Nim project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "nim82-nimble-audit-selected", language: "nim",
			search: "calculateDirSha1Checksum", primary: "src/nimblepkg/checksums.nim",
			negativeFile: "src/nimblepkg/sha1hashes.nim", negativeQuery: "calculateDirSha1Checksum",
			paths: []string{"src/nimblepkg/checksums.nim", "src/nimblepkg/sha1hashes.nim"},
			oracles: []extraAuditEvidence{
				{"src/nimblepkg/checksums.nim", "import common, version, sha1hashes, vcstools, paths, cli"},
				{"src/nimblepkg/checksums.nim", "proc checksumError*(name: string, version: Version,"},
				{"src/nimblepkg/checksums.nim", "proc updateSha1Checksum(checksum: var Sha1State, fileName, filePath: string) ="},
				{"src/nimblepkg/checksums.nim", "proc calculateDirSha1Checksum*(dir: string): Sha1Hash ="},
				{"src/nimblepkg/sha1hashes.nim", "Sha1Hash* = object"},
				{"src/nimblepkg/sha1hashes.nim", "proc initSha1Hash*(value: string): Sha1Hash ="},
				{"src/nimblepkg/sha1hashes.nim", "proc initFromJson*(dst: var Sha1Hash, jsonNode: JsonNode,"},
			},
			targets: []extraAuditSymbol{
				{"src/nimblepkg/checksums.nim", "ChecksumError", 9},
				{"src/nimblepkg/checksums.nim", "checksumError", 11},
				{"src/nimblepkg/checksums.nim", "updateSha1Checksum", 21},
				{"src/nimblepkg/checksums.nim", "calculateDirSha1Checksum", 60},
				{"src/nimblepkg/sha1hashes.nim", "Sha1Hash", 12},
				{"src/nimblepkg/sha1hashes.nim", "invalidSha1Hash", 25},
				{"src/nimblepkg/sha1hashes.nim", "initSha1Hash", 30},
				{"src/nimblepkg/sha1hashes.nim", "initFromJson", 42},
			},
		},
		{
			project: "nim82-chronos-audit-selected", language: "nim",
			search: "tryConsume", primary: "chronos/ratelimit.nim",
			negativeFile: "chronos/timer.nim", negativeQuery: "tryConsume",
			paths: []string{"chronos/ratelimit.nim", "chronos/timer.nim"},
			oracles: []extraAuditEvidence{
				{"chronos/ratelimit.nim", "import ./timer"},
				{"chronos/ratelimit.nim", "TokenBucket* = ref object"},
				{"chronos/ratelimit.nim", "proc refill(bucket: TokenBucket, tokens: int) ="},
				{"chronos/ratelimit.nim", "proc tryConsume*(bucket: TokenBucket, tokens: int, now = Moment.now()): bool ="},
				{"chronos/ratelimit.nim", "proc consume*("},
				{"chronos/ratelimit.nim", "proc replenish*(bucket: TokenBucket, tokens: int, now = Moment.now()) ="},
				{"chronos/timer.nim", "Moment* = object"},
				{"chronos/timer.nim", "Duration* = object"},
				{"chronos/timer.nim", "func toString*(a: timer.Duration, parts = int.high): string ="},
				{"chronos/timer.nim", "proc now*(t: typedesc[Moment]): Moment {.inline.} ="},
			},
			targets: []extraAuditSymbol{
				{"chronos/ratelimit.nim", "ReplenishMode", 18},
				{"chronos/ratelimit.nim", "TokenBucket", 31},
				{"chronos/ratelimit.nim", "refill", 62},
				{"chronos/ratelimit.nim", "mint", 85},
				{"chronos/ratelimit.nim", "tryConsume", 129},
				{"chronos/ratelimit.nim", "consume", 180},
				{"chronos/ratelimit.nim", "replenish", 228},
				{"chronos/timer.nim", "Moment", 113},
				{"chronos/timer.nim", "Duration", 119},
				{"chronos/timer.nim", "toString", 335},
				{"chronos/timer.nim", "now", 387},
				{"chronos/timer.nim", "fromNow", 402},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
