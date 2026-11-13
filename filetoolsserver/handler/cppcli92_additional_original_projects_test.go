package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cppcli92SourceTarget struct {
	file, name, owner, excerpt, near, nearExcerpt, negative string
	line, nearLine                                          int
}

// These two original upstream projects differ from the historical libyaraNET
// and ManagedFFmpeg projects. Both selected sources are pinned Git blobs and
// their target locations are established by independent full source reads.
func TestCPPCLI92AdditionalOriginalProjectNavigation(t *testing.T) {
	ntaglib := os.Getenv("SCRIPTHOLD_REAL_CPPCLI92_NTAGLIB_ROOT")
	physx := os.Getenv("SCRIPTHOLD_REAL_CPPCLI92_PHYSX_ROOT")
	if ntaglib == "" && physx == "" {
		t.Skip("additional independent C++/CLI original projects not configured")
	}
	if ntaglib == "" || physx == "" {
		t.Fatal("both additional independent C++/CLI originals are required")
	}
	cases := []struct {
		name, root, task string
		target           cppcli92SourceTarget
		neighbors        []cppcli92SourceTarget
	}{
		{
			name: "NTagLib cover MIME normalization versus file-signature fallback",
			root: ntaglib, task: "locate MIME normalizer for cover image metadata, not picture type normalizer",
			target: cppcli92SourceTarget{
				file: "NTagLib.cpp", name: "Normalize", owner: "NTagLib.MimeTypes", line: 154,
				excerpt: "static String^ Normalize(String^ mimeType, array<byte>^ data)",
				near:    "GetMimeTypeFromData", nearLine: 183, nearExcerpt: "static String^ GetMimeTypeFromData(array<byte>^ data)",
				negative: "AssemblyTitle",
			},
			neighbors: []cppcli92SourceTarget{{
				file: "NTagLib.cpp", name: "Normalize", owner: "NTagLib.PictureTypes", line: 137,
				excerpt: "static String^ Normalize(String^ pictureType)",
			}},
		},
		{
			name: "PhysX Actor bounding box declaration versus method implementation",
			root: physx, task: "locate GetWorldBounds public contract and corresponding native getter logic",
			target: cppcli92SourceTarget{
				file: "Actor.h", name: "GetWorldBounds", owner: "PhysX.Actor", line: 48,
				excerpt: "Bounds3 GetWorldBounds();", near: "Actor", nearLine: 21,
				nearExcerpt: "public ref class Actor abstract", negative: "Physics::GetWorldBounds",
			},
			neighbors: []cppcli92SourceTarget{{
				file: "Actor.cpp", name: "GetWorldBounds", line: 49,
				excerpt: "Bounds3 Actor::GetWorldBounds()", near: "ToString", nearLine: 54,
				nearExcerpt: "String^ Actor::ToString()",
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := NewHandler([]string{tc.root})
			targetPath := filepath.Join(tc.root, tc.target.file)
			var nearPaths []string
			for _, n := range tc.neighbors {
				nearPaths = append(nearPaths, filepath.Join(tc.root, n.file))
			}
			// The original source establishes the task target, not the parser
			// output. Reject stale or substituted input before handler calls.
			for _, entry := range append([]cppcli92SourceTarget{tc.target}, tc.neighbors...) {
				path := filepath.Join(tc.root, entry.file)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original source not bounded/regular: %s info=%v err=%v", path, info, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(string(data), "\n")
				if entry.line > len(lines) || entry.line < 1 || !strings.Contains(lines[entry.line-1], entry.excerpt) {
					t.Fatalf("independently annotated original target differs %s:%d %q", entry.file, entry.line, entry.excerpt)
				}
				if entry.near != "" && entry.nearExcerpt != "" &&
					(entry.nearLine < 1 || entry.nearLine > len(lines) ||
						!strings.Contains(lines[entry.nearLine-1], entry.nearExcerpt)) {
					t.Fatalf("original nearby declaration differs %s:%d %q", entry.file, entry.nearLine, entry.nearExcerpt)
				}
			}
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{targetPath}, Language: "cpp-cli", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original C++/CLI source digest failed err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{targetPath},
				Language: "cpp-cli", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.FilesParsed != 1 {
				t.Fatalf("original source detailed outline failed err=%v outline=%+v", err, outline)
			}
			targetID := ""
			nearOK := false
			for _, sym := range outline.Symbols {
				if sym.Name == "sealed" || sym.Name == "abstract" {
					t.Errorf("managed class modifier invented as symbol: %+v", sym)
				}
				if sym.Name == tc.target.name && sym.NameRange.Start.Line == tc.target.line &&
					sym.ParentQualifiedName == tc.target.owner {
					targetID = sym.ID
				}
				if sym.Name == tc.target.near && sym.NameRange.Start.Line == tc.target.nearLine {
					nearOK = true
				}
			}
			if targetID == "" || !nearOK {
				t.Fatalf("original target/nearby navigation missing: target=%v near=%v complete=%v truncated=%v total=%d nearby=%+v",
					targetID != "", nearOK, outline.CoverageComplete, outline.Truncated, len(outline.Symbols), outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{targetPath}, Language: "cpp-cli",
				Query: tc.target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 96, FileDetails: "relevant",
			})
			if err != nil || len(found.Files) != 1 {
				t.Fatalf("exact target source finder failed err=%v found=%+v", err, found)
			}
			real := false
			for _, sym := range found.Symbols {
				if sym.ID == targetID && sym.NameRange.Start.Line == tc.target.line {
					real = true
				}
			}
			file := found.Files[0]
			if tc.target.name == "Normalize" && !found.Ambiguous {
				t.Fatal("same-name Normalize declarations must remain distinguishable")
			}
			if !real || file.SourceFingerprint != digest.Digests[0].SourceFingerprint || file.Encoding == "" {
				t.Fatalf("original symbol identity or fingerprint not recoverable: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: targetPath, Language: "cpp-cli", SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, Encoding: file.Encoding, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.target.excerpt) {
				t.Fatalf("fingerprint-checked source excerpt missing err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{targetPath},
				Language: "cpp-cli", Query: tc.target.name, Match: "exact",
				MaxFiles: 1, MaxResults: 96,
			})
			linked := false
			if search.Search != nil {
				for _, hit := range search.Search.Matches {
					if hit.SymbolID == targetID && hit.SourceFingerprint == file.SourceFingerprint {
						linked = true
					}
				}
			}
			if err != nil || search.Search == nil || !linked || search.Coverage.ResultsTruncated {
				t.Fatalf("structural original navigation lacks verified exact target err=%v search=%+v", err, search)
			}
			// Inspect independently known same-name distractors/related files.
			for _, neighbor := range tc.neighbors {
				path := filepath.Join(tc.root, neighbor.file)
				_, adj, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "cpp-cli",
					Query: neighbor.name, Match: "exact", MaxFiles: 1, MaxSymbols: 96,
				})
				neighborFound := false
				for _, sym := range adj.Symbols {
					if sym.NameRange.Start.Line == neighbor.line &&
						(neighbor.owner == "" || sym.ParentQualifiedName == neighbor.owner) {
						neighborFound = true
					}
				}
				if err != nil || !neighborFound {
					t.Fatalf("independent neighbor missing err=%v neighbor=%+v results=%+v", err, neighbor, adj.Symbols)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{targetPath}, Language: "cpp-cli",
				Query: tc.target.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("irrelevant/macro target falsely indexed err=%v negative=%+v", err, negative.Symbols)
			}
			t.Logf("original navigation PASS: symbols=%d ambiguity=%v nearPaths=%d", len(outline.Symbols), found.Ambiguous, len(nearPaths))
		})
	}
}
