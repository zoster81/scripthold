package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSymbolsRoutesDistinctFormatsAndExplicitDialects(t *testing.T) {
	autoRoot := filepath.Join(canonicalHandlerTestDir(t), "auto")
	if err := os.MkdirAll(autoRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	autoFiles := map[string]string{
		"script.vbs":   "Function VBScriptBox()\nEnd Function\n",
		"free.bi":      "Namespace Free\nSub FreeBasicBox()\nEnd Sub\nEnd Namespace\n",
		"pure.pb":      "Module Pure\nProcedure PureBasicBox()\nEndProcedure\nEndModule\n",
		"module.fs":    "let FSharpBox value = value\n",
		"code.il":      ".assembly CILModule {}\n.class public CILBox {}\n",
		"script.ps1":   "function PowerShellBox { 1 }\n",
		"classic.asp":  "<%@ Language=\"VBScript\" %>\n<% Function ASPBox() : End Function %>\n",
		"page.aspx":    "<%@ Page Language=\"C#\" %>\n<script runat=\"server\">\npublic void WebFormsBox() {}\n</script>\n",
		"page.cshtml":  "@functions {\npublic void RazorBox() {}\n}\n",
		"Widget.razor": "@code {\nprivate void BlazorBox() {}\n}\n",
		"View.xaml":    "<Window x:Class=\"Demo.XamlBox\" xmlns:x=\"http://schemas.microsoft.com/winfx/2006/xaml\"><Grid x:Name=\"Root\" /></Window>\n",
	}
	for name, content := range autoFiles {
		if err := os.WriteFile(filepath.Join(autoRoot, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{filepath.Dir(autoRoot)})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{autoRoot}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 512,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("auto outline err=%v toolErr=%+v", err, toolErr)
	}
	if outline.FilesConsidered != len(autoFiles) || outline.FilesParsed != len(autoFiles) || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 6 auto outline summary = %+v", outline)
	}
	wantLanguages := map[string]bool{
		"vbscript": false, "freebasic": false, "purebasic": false, "fsharp": false, "cil": false, "powershell": false,
		"classic-asp": false, "aspnet-webforms": false, "razor": false, "blazor": false, "xaml": false,
	}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 6 auto file routing = %+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed Phase 6 language %s: %+v", language, outline.Files)
		}
	}
	for _, name := range []string{"VBScriptBox", "Free.FreeBasicBox", "Pure.PureBasicBox", "FSharpBox", "CILModule.CILBox", "PowerShellBox", "ASPBox", "WebFormsBox", "RazorBox", "BlazorBox", "Demo.XamlBox", "Demo.XamlBox.Root"} {
		found := false
		for _, symbol := range outline.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 6 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	explicitRoot := filepath.Join(filepath.Dir(autoRoot), "explicit")
	if err := os.MkdirAll(explicitRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	explicit := []struct {
		filename, language, text, want string
	}{
		{"vb6.bas", "vb6", "Attribute VB_Name = \"VB6Module\"\nPublic Sub VB6Box()\nEnd Sub\n", "VB6Module.VB6Box"},
		{"vba.bas", "vba", "Attribute VB_Name = \"VBAModule\"\nPublic Function VBABox()\nEnd Function\n", "VBAModule.VBABox"},
		{"qbasic.bas", "qbasic", "SUB QBasicBox()\nEND SUB\n", "QBasicBox"},
		{"classic.bas", "classic-basic", "10 DEF FNClassicBox(X)=X\n", "FNClassicBox"},
		{"cppcli.cpp", "cpp-cli", "public ref class CPPCLIBox {};\n", "CPPCLIBox"},
		{"jsnet.js", "jscript-net", "package Demo { public class JScriptNetBox { public function Run() {} } }\n", "Demo.JScriptNetBox"},
	}
	for _, tc := range explicit {
		path := filepath.Join(explicitRoot, tc.filename)
		if err := os.WriteFile(path, []byte(tc.text), 0o600); err != nil {
			t.Fatal(err)
		}
		toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: tc.language, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 64,
		})
		if err != nil || toolErr != nil {
			t.Fatalf("explicit %s err=%v toolErr=%+v", tc.language, err, toolErr)
		}
		if result.FilesParsed != 1 || result.FilesSkipped != 0 || !result.CoverageComplete || len(result.Files) != 1 || result.Files[0].Detection.Language != tc.language {
			t.Fatalf("explicit %s result=%+v", tc.language, result)
		}
		found := false
		for _, symbol := range result.Symbols {
			if symbol.QualifiedName == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("explicit %s missing %s; symbols=%+v", tc.language, tc.want, result.Symbols)
		}
	}
}

// A DIM SHARED statement after a TYPE block still declares every variable.
func TestSourceSymbolsQBasicDimSharedAfterTypeAndSegmentDirective(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := canonicalHandlerTestDir(t)
			path := filepath.Join(root, "program.bas")
			source := "TYPE Player\n  x AS INTEGER\nEND TYPE\nDIM SHARED mapwidth, mapheight, numscripts AS INTEGER\nDEF SEG = VARSEG(mapwidth)\nDEF SEG\nSUB MovePlayer\nEND SUB\n"
			source = strings.ReplaceAll(source, "\n", tc.eol)
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})
			toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "qbasic", Encoding: "utf-8", MaxSymbols: 32,
			})
			if err != nil || toolErr != nil {
				t.Fatalf("handler err=%v toolErr=%+v", err, toolErr)
			}
			if !result.CoverageComplete || result.FilesParsed != 1 || len(result.Symbols) != 6 {
				t.Fatalf("unexpected coverage/symbols: %+v", result)
			}
			want := map[string]struct {
				kind string
				line int
			}{
				"Player": {"type", 1}, "Player.x": {"field", 2},
				"mapwidth": {"variable", 4}, "mapheight": {"variable", 4}, "numscripts": {"variable", 4},
				"MovePlayer": {"function", 7},
			}
			for _, symbol := range result.Symbols {
				v, ok := want[symbol.QualifiedName]
				if !ok || string(symbol.Kind) != v.kind || symbol.DeclarationRange.Start.Line != v.line {
					t.Fatalf("unexpected symbol: %+v", symbol)
				}
				delete(want, symbol.QualifiedName)
			}
			if len(want) != 0 {
				t.Fatalf("missing symbols: %+v", want)
			}
		})
	}
}

// Type-first DIM from real QB64 sources must not create an "AS" declaration.
func TestSourceSymbolsBasicTypeFirstDimAfterType(t *testing.T) {
	for _, language := range []string{"qbasic", "classic-basic", "freebasic"} {
		t.Run(language, func(t *testing.T) {
			root := canonicalHandlerTestDir(t)
			path := filepath.Join(root, "types.bas")
			source := "TYPE Vector\n  x AS SINGLE\nEND TYPE\nDIM AS LONG g1, g2\nDIM AS _UNSIGNED LONG index, totalParticles\nSUB Run\n  DIM AS SINGLE dx, dy\nEND SUB\n"
			if language == "freebasic" {
				source = strings.ReplaceAll(source, "AS _UNSIGNED LONG", "AS LONG")
			}
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})
			toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: language, Encoding: "utf-8", MaxSymbols: 32,
			})
			if err != nil || toolErr != nil {
				t.Fatalf("handler: %v %+v", err, toolErr)
			}
			if !result.CoverageComplete || result.FilesParsed != 1 || result.FilesSkipped != 0 {
				t.Fatalf("incomplete: %+v", result)
			}
			names := map[string]struct {
				kind string
				line int
			}{
				"Vector": {"type", 1}, "Vector.x": {"field", 2},
				"g1": {"variable", 4}, "g2": {"variable", 4},
				"index": {"variable", 5}, "totalParticles": {"variable", 5},
				"Run": {"function", 6}, "Run.dx": {"variable", 7}, "Run.dy": {"variable", 7},
			}
			if len(result.Symbols) != len(names) {
				t.Fatalf("symbols=%+v", result.Symbols)
			}
			for _, symbol := range result.Symbols {
				expected, ok := names[symbol.QualifiedName]
				if !ok || string(symbol.Kind) != expected.kind || symbol.DeclarationRange.Start.Line != expected.line {
					t.Fatalf("unexpected symbol: %+v", symbol)
				}
				delete(names, symbol.QualifiedName)
			}
			if len(names) != 0 {
				t.Fatalf("missing: %v", names)
			}
		})
	}
}

func TestSourceSymbolsBasicDefSegOriginalFormIsNotAFunction(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "segment.bas")
	original := "DIM mz%(50, 50)\nDEF SEG = VARSEG(mz%(0, 0))\nDEF SEG\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	for _, language := range []string{"qbasic", "classic-basic", "freebasic", "vb6", "vba", "vbscript", "purebasic"} {
		t.Run(language, func(t *testing.T) {
			toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: language, Encoding: "utf-8", MaxSymbols: 32,
			})
			if err != nil || toolErr != nil {
				t.Fatalf("outline err=%v toolErr=%+v", err, toolErr)
			}
			if !result.CoverageComplete || result.FilesParsed != 1 || result.FilesSkipped != 0 {
				t.Fatalf("unexpected coverage: %+v", result)
			}
			for _, symbol := range result.Symbols {
				if symbol.Kind == "function" || symbol.Name == "SEG" {
					t.Fatalf("DEF SEG invented a function: %+v", symbol)
				}
			}
		})
	}
}
