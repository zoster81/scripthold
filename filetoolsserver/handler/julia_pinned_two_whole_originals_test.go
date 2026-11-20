package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete Julia declaration expectations were independently inventoried from
// two pinned upstream sources, before consulting development handler output.
func TestJuliaTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JULIA_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("complete independently pinned Julia originals not configured")
	}
	var inventory struct {
		Files []struct {
			File    string `json:"file"`
			Blob    string `json:"blob"`
			Bytes   int64  `json:"bytes"`
			Entries []struct {
				Name             string                                           `json:"name"`
				Kind             string                                           `json:"kind"`
				NativeKind       string                                           `json:"nativeKind"`
				QualifiedName    string                                           `json:"qualifiedName"`
				Owner            string                                           `json:"owner"`
				NameRange        struct{ Start, End struct{ Line, Column int } }  `json:"nameRange"`
				DeclarationRange struct{ Start, End struct{ Line, Column int } }  `json:"declarationRange"`
				SignatureRange   *struct{ Start, End struct{ Line, Column int } } `json:"signatureRange"`
				Signature        string                                           `json:"signature"`
			} `json:"entries"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(juliaTwoWholeOracle), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Files) != 2 {
		t.Fatalf("invalid original project pair: %d", len(inventory.Files))
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	total := 0
	for _, tc := range inventory.Files {
		t.Run(tc.File, func(t *testing.T) {
			path := filepath.Join(root, tc.File)
			st, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !st.Mode().IsRegular() || st.Size() != tc.Bytes {
				t.Fatalf("whole original bytes changed: %+v", st)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(raw), 0)
			_, _ = hash.Write(raw)
			if hex.EncodeToString(hash.Sum(nil)) != tc.Blob {
				t.Fatalf("original upstream Git blob mismatch: %s", tc.File)
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "julia", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 100})
			if err != nil || fail != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(tc.Entries) {
				t.Fatalf("whole-source original coverage expected=%d got=%d failed=%+v err=%v", len(tc.Entries), len(out.Symbols), fail, err)
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.Bytes {
				t.Fatalf("source identity missing: %+v", out.Files[0])
			}
			byLocation := map[string]int{}
			idByQName := map[string]string{}
			seenIDs := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, found := byLocation[loc]; found {
					t.Errorf("duplicate Julia source location %s", loc)
				}
				byLocation[loc] = i
				if s.ID == "" || seenIDs[s.ID] {
					t.Errorf("missing or duplicate Julia symbol ID at %s", loc)
				}
				seenIDs[s.ID] = true
				idByQName[s.QualifiedName] = s.ID
			}
			for _, e := range tc.Entries {
				loc := fmt.Sprintf("%d:%d", e.NameRange.Start.Line, e.NameRange.Start.Column)
				i, found := byLocation[loc]
				if !found {
					t.Errorf("missing source-first original Julia declaration %s at %s", e.Name, loc)
					continue
				}
				s := out.Symbols[i]
				if s.Name != e.Name || string(s.Kind) != e.Kind || s.NativeKind != e.NativeKind || s.QualifiedName != e.QualifiedName || s.ParentQualifiedName != e.Owner ||
					s.NameRange.Start.Line != e.NameRange.Start.Line || s.NameRange.Start.Column != e.NameRange.Start.Column ||
					s.NameRange.End.Line != e.NameRange.End.Line || s.NameRange.End.Column != e.NameRange.End.Column ||
					s.DeclarationRange.Start.Line != e.DeclarationRange.Start.Line || s.DeclarationRange.Start.Column != e.DeclarationRange.Start.Column ||
					s.DeclarationRange.End.Line != e.DeclarationRange.End.Line || s.DeclarationRange.End.Column != e.DeclarationRange.End.Column {
					t.Errorf("authored full Julia source mismatch %s at %s: %+v", e.Name, loc, s)
				}
				if e.SignatureRange != nil && (s.SignatureRange == nil || s.SignatureRange.Start.Line != e.SignatureRange.Start.Line ||
					s.SignatureRange.Start.Column != e.SignatureRange.Start.Column || s.SignatureRange.End.Line != e.SignatureRange.End.Line ||
					s.SignatureRange.End.Column != e.SignatureRange.End.Column || s.Signature != e.Signature) {
					t.Errorf("authored Julia signature mismatch %s: %+v", e.Name, s)
				}
				if e.Owner != "" && idByQName[e.Owner] != "" && s.ParentID != idByQName[e.Owner] {
					t.Errorf("Julia lexical owner ID mismatch %s: %+v", e.Name, s)
				}
				if e.Owner == "" && s.ParentID != "" {
					t.Errorf("invented Julia lexical owner %s: %+v", e.Name, s)
				}
			}
			first := tc.Entries[0].Name
			fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "julia", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
			var symbolID string
			for _, s := range found.Symbols {
				if s.Name == first {
					symbolID = s.ID
				}
			}
			if err != nil || fail != nil || symbolID == "" {
				t.Fatalf("original Julia exact find %s: %+v %v", first, fail, err)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "julia", Encoding: "utf-8", SymbolID: symbolID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("source fingerprint-bound Julia show %s: %+v %v", first, fail, err)
			}
			fail, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "julia", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 100})
			matched := false
			if q.Search != nil {
				for _, m := range q.Search.Matches {
					if m.SymbolID == symbolID && m.Name == first {
						matched = true
					}
				}
			}
			if err != nil || fail != nil || !matched {
				t.Fatalf("original Julia structural search %s: %+v %v", first, fail, err)
			}
			for _, negative := range []string{"ghostJuliaDeclaration", "using", "import"} {
				fail, none, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "julia", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
				if err != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("invented original Julia declaration %s: %+v %v", negative, fail, err)
				}
			}
			total += len(tc.Entries)
		})
	}
	if total != 18 {
		t.Fatalf("incomplete independent Julia source inventory: %d/18", total)
	}
}

const juliaTwoWholeOracle = "{\"files\":[{\"file\":\"julia-compiler-dev-tools.jl\",\"blob\":\"89b902e18d292e3cdab469beb7cd548e3305061d\",\"bytes\":3651,\"entries\":[{\"name\":\"CompilerDevTools\",\"kind\":\"module\",\"nativeKind\":\"module\",\"qualifiedName\":\"CompilerDevTools\",\"owner\":\"\",\"nameRange\":{\"start\":{\"line\":1,\"column\":8},\"end\":{\"line\":1,\"column\":24}},\"declarationRange\":{\"start\":{\"line\":1,\"column\":1},\"end\":{\"line\":1,\"column\":24}},\"signatureRange\":{\"start\":{\"line\":1,\"column\":1},\"end\":{\"line\":1,\"column\":24}},\"signature\":\"module CompilerDevTools\"},{\"name\":\"SplitCacheOwner\",\"kind\":\"struct\",\"nativeKind\":\"mutable-struct\",\"qualifiedName\":\"CompilerDevTools.SplitCacheOwner\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":8,\"column\":16},\"end\":{\"line\":8,\"column\":31}},\"declarationRange\":{\"start\":{\"line\":8,\"column\":1},\"end\":{\"line\":8,\"column\":35}},\"signatureRange\":{\"start\":{\"line\":8,\"column\":1},\"end\":{\"line\":8,\"column\":35}},\"signature\":\"mutable struct SplitCacheOwner end\"},{\"name\":\"SplitCacheInterp\",\"kind\":\"struct\",\"nativeKind\":\"struct\",\"qualifiedName\":\"CompilerDevTools.SplitCacheInterp\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":10,\"column\":8},\"end\":{\"line\":10,\"column\":24}},\"declarationRange\":{\"start\":{\"line\":10,\"column\":1},\"end\":{\"line\":10,\"column\":56}},\"signatureRange\":{\"start\":{\"line\":10,\"column\":1},\"end\":{\"line\":10,\"column\":56}},\"signature\":\"struct SplitCacheInterp <: Compiler.AbstractInterpreter\"},{\"name\":\"SplitCacheInterp\",\"kind\":\"function\",\"nativeKind\":\"function\",\"qualifiedName\":\"CompilerDevTools.SplitCacheInterp.SplitCacheInterp\",\"owner\":\"CompilerDevTools.SplitCacheInterp\",\"nameRange\":{\"start\":{\"line\":17,\"column\":14},\"end\":{\"line\":17,\"column\":30}},\"declarationRange\":{\"start\":{\"line\":17,\"column\":5},\"end\":{\"line\":22,\"column\":72}},\"signatureRange\":{\"start\":{\"line\":17,\"column\":5},\"end\":{\"line\":22,\"column\":72}},\"signature\":\"function SplitCacheInterp(;\\n        world::UInt = Base.get_world_counter(),\\n        owner::SplitCacheOwner = SplitCacheOwner(),\\n        inf_params::Compiler.InferenceParams = Compiler.InferenceParams(),\\n        opt_params::Compiler.OptimizationParams = Compiler.OptimizationParams(),\\n        inf_cache::Compiler.InferenceCache = Compiler.InferenceCache())\"},{\"name\":\"Compiler.InferenceParams\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.InferenceParams\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":27,\"column\":25}},\"declarationRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":27,\"column\":71}},\"signatureRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":27,\"column\":71}},\"signature\":\"Compiler.InferenceParams(interp::SplitCacheInterp) = interp.inf_params\"},{\"name\":\"Compiler.OptimizationParams\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.OptimizationParams\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":28,\"column\":1},\"end\":{\"line\":28,\"column\":28}},\"declarationRange\":{\"start\":{\"line\":28,\"column\":1},\"end\":{\"line\":28,\"column\":74}},\"signatureRange\":{\"start\":{\"line\":28,\"column\":1},\"end\":{\"line\":28,\"column\":74}},\"signature\":\"Compiler.OptimizationParams(interp::SplitCacheInterp) = interp.opt_params\"},{\"name\":\"Compiler.get_inference_world\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.get_inference_world\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":29,\"column\":1},\"end\":{\"line\":29,\"column\":29}},\"declarationRange\":{\"start\":{\"line\":29,\"column\":1},\"end\":{\"line\":29,\"column\":70}},\"signatureRange\":{\"start\":{\"line\":29,\"column\":1},\"end\":{\"line\":29,\"column\":70}},\"signature\":\"Compiler.get_inference_world(interp::SplitCacheInterp) = interp.world\"},{\"name\":\"Compiler.get_inference_cache\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.get_inference_cache\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":30,\"column\":1},\"end\":{\"line\":30,\"column\":29}},\"declarationRange\":{\"start\":{\"line\":30,\"column\":1},\"end\":{\"line\":30,\"column\":74}},\"signatureRange\":{\"start\":{\"line\":30,\"column\":1},\"end\":{\"line\":30,\"column\":74}},\"signature\":\"Compiler.get_inference_cache(interp::SplitCacheInterp) = interp.inf_cache\"},{\"name\":\"Compiler.cache_owner\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.cache_owner\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":31,\"column\":1},\"end\":{\"line\":31,\"column\":21}},\"declarationRange\":{\"start\":{\"line\":31,\"column\":1},\"end\":{\"line\":31,\"column\":62}},\"signatureRange\":{\"start\":{\"line\":31,\"column\":1},\"end\":{\"line\":31,\"column\":62}},\"signature\":\"Compiler.cache_owner(interp::SplitCacheInterp) = interp.owner\"},{\"name\":\"Compiler.codegen_cache\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.Compiler.codegen_cache\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":32,\"column\":1},\"end\":{\"line\":32,\"column\":23}},\"declarationRange\":{\"start\":{\"line\":32,\"column\":1},\"end\":{\"line\":32,\"column\":72}},\"signatureRange\":{\"start\":{\"line\":32,\"column\":1},\"end\":{\"line\":32,\"column\":72}},\"signature\":\"Compiler.codegen_cache(interp::SplitCacheInterp) = interp.codegen_cache\"},{\"name\":\"lookup_method_instance\",\"kind\":\"function\",\"nativeKind\":\"function\",\"qualifiedName\":\"CompilerDevTools.lookup_method_instance\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":44,\"column\":10},\"end\":{\"line\":44,\"column\":32}},\"declarationRange\":{\"start\":{\"line\":44,\"column\":1},\"end\":{\"line\":44,\"column\":44}},\"signatureRange\":{\"start\":{\"line\":44,\"column\":1},\"end\":{\"line\":44,\"column\":44}},\"signature\":\"function lookup_method_instance(f, args...)\"},{\"name\":\"Compiler.transform_result_for_cache\",\"kind\":\"function\",\"nativeKind\":\"function\",\"qualifiedName\":\"CompilerDevTools.Compiler.transform_result_for_cache\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":48,\"column\":10},\"end\":{\"line\":48,\"column\":45}},\"declarationRange\":{\"start\":{\"line\":48,\"column\":1},\"end\":{\"line\":48,\"column\":135}},\"signatureRange\":{\"start\":{\"line\":48,\"column\":1},\"end\":{\"line\":48,\"column\":135}},\"signature\":\"function Compiler.transform_result_for_cache(interp::SplitCacheInterp, result::Compiler.InferenceResult, edges::Compiler.SimpleVector)\"},{\"name\":\"with_new_compiler\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"CompilerDevTools.with_new_compiler\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":65,\"column\":1},\"end\":{\"line\":65,\"column\":18}},\"declarationRange\":{\"start\":{\"line\":65,\"column\":1},\"end\":{\"line\":65,\"column\":113}},\"signatureRange\":{\"start\":{\"line\":65,\"column\":1},\"end\":{\"line\":65,\"column\":113}},\"signature\":\"with_new_compiler(f, args...; owner::SplitCacheOwner = SplitCacheOwner()) = with_new_compiler(f, owner, args...)\"},{\"name\":\"with_new_compiler\",\"kind\":\"function\",\"nativeKind\":\"function\",\"qualifiedName\":\"CompilerDevTools.with_new_compiler\",\"owner\":\"CompilerDevTools\",\"nameRange\":{\"start\":{\"line\":67,\"column\":10},\"end\":{\"line\":67,\"column\":27}},\"declarationRange\":{\"start\":{\"line\":67,\"column\":1},\"end\":{\"line\":67,\"column\":63}},\"signatureRange\":{\"start\":{\"line\":67,\"column\":1},\"end\":{\"line\":67,\"column\":63}},\"signature\":\"function with_new_compiler(f, owner::SplitCacheOwner, args...)\"}]},{\"file\":\"exercism-annalyn-exemplar.jl\",\"blob\":\"7cf8a17e6958703b168c195cff8e6a6bb0639b8b\",\"bytes\":836,\"entries\":[{\"name\":\"can_do_fast_attack\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"can_do_fast_attack\",\"owner\":\"\",\"nameRange\":{\"start\":{\"line\":6,\"column\":1},\"end\":{\"line\":6,\"column\":19}},\"declarationRange\":{\"start\":{\"line\":6,\"column\":1},\"end\":{\"line\":6,\"column\":49}},\"signatureRange\":{\"start\":{\"line\":6,\"column\":1},\"end\":{\"line\":6,\"column\":49}},\"signature\":\"can_do_fast_attack(knight_awake) = !knight_awake\"},{\"name\":\"can_spy\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"can_spy\",\"owner\":\"\",\"nameRange\":{\"start\":{\"line\":13,\"column\":1},\"end\":{\"line\":13,\"column\":8}},\"declarationRange\":{\"start\":{\"line\":13,\"column\":1},\"end\":{\"line\":13,\"column\":101}},\"signatureRange\":{\"start\":{\"line\":13,\"column\":1},\"end\":{\"line\":13,\"column\":101}},\"signature\":\"can_spy(knight_awake, archer_awake, prisoner_awake) = knight_awake || archer_awake || prisoner_awake\"},{\"name\":\"can_signal_prisoner\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"can_signal_prisoner\",\"owner\":\"\",\"nameRange\":{\"start\":{\"line\":20,\"column\":1},\"end\":{\"line\":20,\"column\":20}},\"declarationRange\":{\"start\":{\"line\":20,\"column\":1},\"end\":{\"line\":20,\"column\":84}},\"signatureRange\":{\"start\":{\"line\":20,\"column\":1},\"end\":{\"line\":20,\"column\":84}},\"signature\":\"can_signal_prisoner(archer_awake, prisoner_awake) = prisoner_awake && !archer_awake\"},{\"name\":\"can_free_prisoner\",\"kind\":\"function\",\"nativeKind\":\"compact-function\",\"qualifiedName\":\"can_free_prisoner\",\"owner\":\"\",\"nameRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":27,\"column\":18}},\"declarationRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":28,\"column\":101}},\"signatureRange\":{\"start\":{\"line\":27,\"column\":1},\"end\":{\"line\":28,\"column\":101}},\"signature\":\"can_free_prisoner(knight_awake, archer_awake, prisoner_awake, dog_present) = \\n                    !knight_awake && !archer_awake && prisoner_awake || dog_present && !archer_awake\"}]}]}"
