package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type assembly105Target struct {
	file, name, kind, excerpt string
	line                      int
}

// These two independent original Assembly projects check bounded navigation,
// not assembled program behavior or inferred cross-file execution semantics.
func TestAssembly105AdditionalOriginalProjectNavigation(t *testing.T) {
	mal := os.Getenv("SCRIPTHOLD_REAL_ASSEMBLY105_MAL_ROOT")
	samples := os.Getenv("SCRIPTHOLD_REAL_ASSEMBLY105_SAMPLES_ROOT")
	if mal == "" && samples == "" {
		t.Skip("additional independent original Assembly Git projects not configured")
	}
	if mal == "" || samples == "" {
		t.Fatal("both original Assembly project roots are required")
	}
	type sourceEvidence struct{ file, excerpt string }
	cases := []struct {
		name, root, prefix, negativeFile, negativeName string
		selected, code                                 []string
		evidence                                       []sourceEvidence
		targets                                        []assembly105Target
	}{
		{
			name: "MAL NASM environment bindings and evaluator entrypoints",
			root: mal, prefix: "impls/nasm", negativeFile: "step2_eval.asm", negativeName: "mov",
			selected: []string{"env.asm", "step2_eval.asm", "macros.mac", "README.md"},
			code:     []string{"env.asm", "step2_eval.asm"},
			evidence: []sourceEvidence{
				{"env.asm", "%include \"macros.mac\""},
				{"env.asm", "call map_set"},
				{"env.asm", "jmp env_get"},
				{"step2_eval.asm", "%include \"reader.asm\""},
				{"step2_eval.asm", "jmp read_str"},
				{"step2_eval.asm", "call eval"},
				{"macros.mac", "%macro static_symbol 2"},
				{"README.md", "# x86_64 NASM implementation"},
			},
			targets: []assembly105Target{
				{"env.asm", "env_new", "label", "env_new:", 37},
				{"env.asm", "env_get", "label", "env_get:", 266},
				{"step2_eval.asm", "eval_ast", "label", "eval_ast:", 155},
				{"step2_eval.asm", "rep_seq", "label", "rep_seq:", 605},
				{"step2_eval.asm", "_start", "label", "_start:", 634},
			},
		},
		{
			name: "Sample Programs x86-64 reverse, parity and message handling",
			root: samples, prefix: "archive/x/x86-64", negativeFile: "reverse-string.asm", negativeName: "SYS_MMAP",
			selected: []string{"reverse-string.asm", "even-odd.asm", "hello-world.asm"},
			code:     []string{"reverse-string.asm", "even-odd.asm", "hello-world.asm"},
			evidence: []sourceEvidence{
				{"reverse-string.asm", "CALL strlen"},
				{"reverse-string.asm", "CALL reverseString"},
				{"even-odd.asm", "cmp bl, '9'"},
				{"even-odd.asm", "even_len equ $-even_msg"},
				{"hello-world.asm", "mov rdx, message_len"},
			},
			targets: []assembly105Target{
				{"reverse-string.asm", "strlen", "label", "strlen:", 26},
				{"reverse-string.asm", "reverseString", "label", "reverseString:", 54},
				{"even-odd.asm", "even_len", "constant", "even_len equ $-even_msg", 5},
				{"even-odd.asm", "_start", "label", "_start:", 15},
				{"hello-world.asm", "message_len", "constant", "message_len equ $-message", 3},
				{"hello-world.asm", "_start", "label", "_start:", 8},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sources := make(map[string]string, len(tc.selected))
			for _, file := range tc.selected {
				full := filepath.Join(tc.root, filepath.FromSlash(tc.prefix), file)
				info, err := os.Lstat(full)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original Assembly source unavailable or unbounded %s: info=%v err=%v", file, info, err)
				}
				raw, err := os.ReadFile(full)
				if err != nil {
					t.Fatal(err)
				}
				sources[file] = string(raw)
			}
			for _, evidence := range tc.evidence {
				if !strings.Contains(sources[evidence.file], evidence.excerpt) {
					t.Fatalf("original Assembly source relation changed %s: %q", evidence.file, evidence.excerpt)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(sources[target.file], "\n")
				if target.line < 1 || target.line > len(lines) || !strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original Assembly declaration changed %s:%d %s", target.file, target.line, target.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			digests, ids := make(map[string]string), make(map[string]map[string]bool)
			for _, file := range tc.code {
				full := filepath.Join(tc.root, filepath.FromSlash(tc.prefix), file)
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Paths: []string{full}, Language: "assembly", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
					digest.Digests[0].SourceFingerprint == "" {
					t.Fatalf("Assembly source digest %s: err=%v digest=%+v", file, err, digest)
				}
				digests[file] = digest.Digests[0].SourceFingerprint
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{full},
					Language: "assembly", MaxFiles: 1, MaxSymbols: 512,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("Assembly source outline %s: err=%v outline=%+v", file, err, outline)
				}
				ids[file] = make(map[string]bool)
				for _, sym := range outline.Symbols {
					ids[file][sym.ID] = true
					if sym.Name == "mov" || sym.Name == "include" {
						t.Fatalf("instruction/directive invented as declaration: %+v", sym)
					}
				}
			}
			for _, target := range tc.targets {
				full := filepath.Join(tc.root, filepath.FromSlash(tc.prefix), target.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{full}, Language: "assembly",
					Query: target.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 512, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Assembly exact find %s:%d: err=%v found=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Name == target.name && string(sym.Kind) == target.kind && sym.NameRange.Start.Line == target.line {
						id = sym.ID
					}
				}
				if id == "" || !ids[target.file][id] {
					t.Fatalf("Assembly declaration not safely located %s:%d: %+v", target.file, target.line, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digests[target.file] {
					t.Fatalf("Assembly source fingerprint missing: %+v", file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: full, Language: "assembly", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 16384,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("Assembly fingerprint-bound excerpt %s:%d: err=%v show=%+v", target.file, target.line, err, shown.Show)
				}
				_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{full},
					Language: "assembly", Query: target.name, Match: "exact",
					MaxFiles: 1, MaxResults: 256,
				})
				linked := false
				if search.Search != nil {
					for _, hit := range search.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
					t.Fatalf("Assembly structural search mismatch %s:%d: err=%v query=%+v", target.file, target.line, err, search)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find",
				Paths:     []string{filepath.Join(tc.root, filepath.FromSlash(tc.prefix), tc.negativeFile)},
				Language:  "assembly", Query: tc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("Assembly instruction/macro operand invented as declaration %s: err=%v symbols=%+v", tc.negativeName, err, negative.Symbols)
			}
			t.Logf("Assembly original navigation: %d selected source files; %d exact targets", len(tc.selected), len(tc.targets))
		})
	}
}
