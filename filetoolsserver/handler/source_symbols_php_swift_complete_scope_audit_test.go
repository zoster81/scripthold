package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Original CodeIgniter and Swift sources were read in full before independently
// enumerating recognized declarations. These are structural inventories, not
// runtime, Swift conditional-compilation, or full grammar certifications.
// Swift nested function declarations and enum case members are individually
// inventoried; closure literals, local value bindings and compiler conditions
// are not treated as independently indexed declarations in this selected scope.
func TestPHPAndSwiftOriginalCompleteSupportedDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original-source corpus not configured")
	}
	type originalDecl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	cases := []struct {
		project, language string
		files             []string
		declarations      []originalDecl
	}{
		{project: "php54-codeigniter-audit-selected", language: "php", files: []string{"app/Config/Routes.php", "app/Config/Routing.php", "app/Config/Filters.php", "app/Controllers/BaseController.php", "app/Controllers/Home.php"}, declarations: []originalDecl{
			{"app/Config/Routing.php", 12, "Config", "namespace", "namespace", ""},
			{"app/Config/Routing.php", 19, "Routing", "class", "class", "Config"},
			{"app/Config/Routing.php", 31, "$routeFiles", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 42, "$defaultNamespace", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 51, "$defaultController", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 60, "$defaultMethod", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 69, "$translateURIDashes", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 87, "$override404", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 97, "$autoRoute", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 106, "$useControllerAttributes", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 115, "$prioritize", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 123, "$multipleSegmentsOneParam", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 137, "$moduleRoutes", "property", "property", "Config.Routing"},
			{"app/Config/Routing.php", 148, "$translateUriToCamelCase", "property", "property", "Config.Routing"},
			{"app/Config/Filters.php", 3, "Config", "namespace", "namespace", ""},
			{"app/Config/Filters.php", 16, "Filters", "class", "class", "Config"},
			{"app/Config/Filters.php", 27, "$aliases", "property", "property", "Config.Filters"},
			{"app/Config/Filters.php", 52, "$required", "property", "property", "Config.Filters"},
			{"app/Config/Filters.php", 73, "$globals", "property", "property", "Config.Filters"},
			{"app/Config/Filters.php", 98, "$methods", "property", "property", "Config.Filters"},
			{"app/Config/Filters.php", 109, "$filters", "property", "property", "Config.Filters"},
			{"app/Controllers/BaseController.php", 3, "App\\Controllers", "namespace", "namespace", ""},
			{"app/Controllers/BaseController.php", 21, "BaseController", "class", "class", "App\\Controllers"},
			{"app/Controllers/BaseController.php", 33, "initController", "method", "method-definition", "App\\Controllers.BaseController"},
			{"app/Controllers/Home.php", 3, "App\\Controllers", "namespace", "namespace", ""},
			{"app/Controllers/Home.php", 5, "Home", "class", "class", "App\\Controllers"},
			{"app/Controllers/Home.php", 7, "index", "method", "method-definition", "App\\Controllers.Home"},
		}},
		{project: "swift55-exercism-audit-selected", language: "swift", files: []string{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", "exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", "exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift"}, declarations: []originalDecl{
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 1, "newScoreBoard", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 5, "addPlayer", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 9, "removePlayer", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 13, "resetScore", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 19, "updateScore", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 25, "orderByPlayers", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 26, "ascendingKeys", "function", "func-definition", "orderByPlayers"},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 32, "orderByScores", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", 33, "descendingValues", "function", "func-definition", "orderByScores"},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 1, "newScoreBoard", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 5, "addPlayer", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 9, "removePlayer", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 13, "resetScore", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 17, "updateScore", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 21, "orderByPlayers", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", 25, "orderByScores", "function", "func-definition", ""},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 6, "RUNALL", "variable", "let", ""},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 8, "HighScoreBoardTests", "struct", "struct", ""},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 10, "testEmptyScores", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 15, "testAddPlayerExplicit", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 23, "testAddPlayerDefault", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 30, "testRemovePlayer", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 40, "testRemoveNonexistentPlayer", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 50, "testResetScore", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 60, "testResetScoreNonexistentPlayer", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 70, "testUpdateScore", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 81, "testUpdateScoreNonexistentPlayer", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 91, "testOrderByPlayers", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", 106, "testOrderByScores", "method", "func-definition", "HighScoreBoardTests"},
			{"exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift", 1, "totalBirdCount", "function", "func-definition", ""},
			{"exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift", 5, "birdsInWeek", "function", "func-definition", ""},
			{"exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift", 9, "fixBirdCountLog", "function", "func-definition", ""},
		}},
		{project: "swift55-swift-compiler-audit-selected", language: "swift", files: []string{"stdlib/public/core/CommandLine.swift", "test/Interpreter/process_arguments.swift", "validation-test/StdlibUnittest/CommandLine.swift", "stdlib/public/core/ByteOrder.swift"}, declarations: []originalDecl{
			{"stdlib/public/core/CommandLine.swift", 18, "_swift_stdlib_getUnsafeArgvArgc", "function", "func-declaration", ""},
			{"stdlib/public/core/CommandLine.swift", 23, "CommandLine", "enum", "enum", ""},
			{"stdlib/public/core/CommandLine.swift", 29, "_argc", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 37, "_unsafeArgv", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 42, "argc", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 69, "unsafeArgv", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 77, "_arguments", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 109, "arguments", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 131, "executablePathCString", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 139, "executablePathCString", "property", "var", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 128, "FPCodeUnit", "alias", "typealias", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 136, "FPCodeUnit", "alias", "typealias", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 146, "_withExecutablePath", "method", "func-declaration", "CommandLine"},
			{"stdlib/public/core/CommandLine.swift", 150, "_executablePathCString", "property", "let", "CommandLine"},
			{"stdlib/public/core/ByteOrder.swift", 16, "ByteOrder", "enum", "enum", ""},
			{"stdlib/public/core/ByteOrder.swift", 19, "bigEndian", "field", "case", "ByteOrder"},
			{"stdlib/public/core/ByteOrder.swift", 23, "littleEndian", "field", "case", "ByteOrder"},
			{"stdlib/public/core/ByteOrder.swift", 27, "native", "property", "var", "ByteOrder"},
			{"validation-test/StdlibUnittest/CommandLine.swift", 13, "CommandLineArguments", "variable", "var", ""},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			project := filepath.Join(root, tc.project)
			paths := make([]string, 0, len(tc.files))
			originals := make(map[string][]string, len(tc.files))
			fullPaths := make(map[string]string, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(project, filepath.FromSlash(rel))
				requireAdditionalOriginalFile(t, path)
				bytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(bytes) > 1<<20 {
					t.Fatalf("original file above 1 MiB: %s", rel)
				}
				originals[rel] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
				paths = append(paths, path)
				fullPaths[rel] = path
			}
			expected := map[string]originalDecl{}
			for _, d := range tc.declarations {
				lines, present := originals[d.file]
				if !present || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
					t.Fatalf("invalid source oracle at %s:%d %s", d.file, d.line, d.name)
				}
				key := d.file + ":" + strconv.Itoa(d.line)
				if _, exists := expected[key]; exists {
					t.Fatalf("duplicate independently inventoried location %s", key)
				}
				expected[key] = d
			}
			h := NewHandler([]string{project})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Presentation: "detailed", MaxFiles: len(paths), MaxSymbols: 128,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(paths) {
				t.Fatalf("incomplete original outline: err=%v failed=%+v out=%+v", err, failed, out)
			}
			actual := map[string]bool{}
			for _, symbol := range out.Symbols {
				rel, err := filepath.Rel(project, symbol.Path)
				if err != nil {
					t.Errorf("unrelatable path %q: %v", symbol.Path, err)
					continue
				}
				rel = filepath.ToSlash(rel)
				if path, exists := fullPaths[rel]; !exists || filepath.Clean(path) != filepath.Clean(symbol.Path) {
					t.Errorf("symbol outside selected original files: %+v", symbol)
					continue
				}
				key := rel + ":" + strconv.Itoa(symbol.NameRange.Start.Line)
				if actual[key] {
					t.Errorf("duplicate symbol position: %s", key)
				}
				actual[key] = true
				d, exists := expected[key]
				if !exists {
					t.Errorf("invented declaration %s: %s %s", key, symbol.Kind, symbol.Name)
					continue
				}
				if symbol.Name != d.name || string(symbol.Kind) != d.kind ||
					symbol.NativeKind != d.native || symbol.ParentQualifiedName != d.parent {
					t.Errorf("misidentified original %s: got %s %s native=%q owner=%q, expected %+v",
						key, symbol.Kind, symbol.Name, symbol.NativeKind, symbol.ParentQualifiedName, d)
				}
			}
			for key, d := range expected {
				if !actual[key] {
					t.Errorf("missing original declaration at %s: %s", key, d.name)
				}
			}
			if len(out.Symbols) != len(tc.declarations) {
				t.Errorf("expected %d original supported declarations, returned %d", len(tc.declarations), len(out.Symbols))
			}
			t.Logf("selected original files=%d independently inventoried supported declarations=%d indexed=%d complete=%v",
				len(tc.files), len(tc.declarations), len(out.Symbols), out.CoverageComplete)
		})
	}
}
