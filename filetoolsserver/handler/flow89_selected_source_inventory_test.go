package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type flow89ExpectedSymbol struct {
	file, name, parent string
	line               int
}

// These exact supported-declaration inventories were transcribed from complete
// independent source reads, before viewing development analyzer outlines.
// They complement task-navigation tests, not full Flow grammar certification.
func TestFlow89SelectedOriginalSupportedDeclarationInventory(t *testing.T) {
	metro := os.Getenv("SCRIPTHOLD_REAL_FLOW89_METRO_ROOT")
	jest := os.Getenv("SCRIPTHOLD_REAL_FLOW89_JEST_ROOT")
	if metro == "" && jest == "" {
		t.Skip("pinned original Flow files not configured")
	}
	if metro == "" || jest == "" {
		t.Fatal("both independent original projects are required")
	}
	cases := []struct {
		name, root string
		want       []flow89ExpectedSymbol
	}{
		{
			name: "Metro ambient modules and EventEmitter declarations",
			root: metro,
			want: []flow89ExpectedSymbol{
				{"eventemitter3.js.flow", "eventemitter3", "", 4},
				{"eventemitter3.js.flow", "ListenerFn", "eventemitter3", 5},
				{"eventemitter3.js.flow", "EventEmitter", "eventemitter3", 6},
				{"eventemitter3.js.flow", "EventEmitter", "eventemitter3.EventEmitter", 7},
				{"eventemitter3.js.flow", "prefixed", "eventemitter3.EventEmitter", 8},
				{"eventemitter3.js.flow", "eventNames", "eventemitter3.EventEmitter", 9},
				{"eventemitter3.js.flow", "listeners", "eventemitter3.EventEmitter", 10},
				{"eventemitter3.js.flow", "listenerCount", "eventemitter3.EventEmitter", 11},
				{"eventemitter3.js.flow", "on", "eventemitter3.EventEmitter", 12},
				{"eventemitter3.js.flow", "addListener", "eventemitter3.EventEmitter", 13},
				{"eventemitter3.js.flow", "once", "eventemitter3.EventEmitter", 18},
				{"eventemitter3.js.flow", "removeAllListeners", "eventemitter3.EventEmitter", 19},
				{"eventemitter3.js.flow", "removeListener", "eventemitter3.EventEmitter", 20},
				{"eventemitter3.js.flow", "off", "eventemitter3.EventEmitter", 26},
				{"eventemitter3.js.flow", "emit", "eventemitter3.EventEmitter", 32},
				{"eventemitter3.js.flow", "eventemitter3/index", "", 38},
				{"eventemitter3.js.flow", "eventemitter3/index.js", "", 41},
				{"accepts.js", "accepts", "", 12},
			},
		},
		{
			name: "Jest intro component and real application entry",
			root: jest,
			want: []flow89ExpectedSymbol{
				{"Intro.js", "styles", "", 11},
				{"Intro.js", "Intro", "", 30},
				{"Intro.js", "render", "Intro", 31},
				{"index.js", "App", "", 13},
				{"index.js", "render", "App", 14},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapping := make(map[string]bool, len(tc.want))
			pathsByName := map[string]string{}
			for _, entry := range tc.want {
				if _, exists := pathsByName[entry.file]; !exists {
					path := filepath.Join(tc.root, entry.file)
					info, err := os.Lstat(path)
					if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
						t.Fatalf("original file missing, non-regular or oversized: %s info=%+v err=%v", path, info, err)
					}
					if _, err := os.ReadFile(path); err != nil {
						t.Fatal(err)
					}
					pathsByName[entry.file] = path
				}
				key := fmt.Sprintf("%s|%s|%s|%d", entry.file, entry.name, entry.parent, entry.line)
				if _, exists := mapping[key]; exists {
					t.Fatalf("duplicate independently inventoried declaration: %s", key)
				}
				mapping[key] = false
			}
			paths := make([]string, 0, len(pathsByName))
			for _, path := range pathsByName {
				paths = append(paths, path)
			}
			_, outline, err := NewHandler([]string{tc.root}).SourceSymbols(
				context.Background(), nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Language: "flow",
					Paths: paths, MaxFiles: len(paths), MaxSymbols: len(tc.want) + 16,
				})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != len(paths) || len(outline.Symbols) != len(tc.want) {
				t.Fatalf("complete supported declaration inventory mismatched: err=%v parsed=%d actual=%d want=%d complete=%v truncated=%v symbols=%+v",
					err, outline.FilesParsed, len(outline.Symbols), len(tc.want), outline.CoverageComplete, outline.Truncated, outline.Symbols)
			}
			for _, symbol := range outline.Symbols {
				key := fmt.Sprintf("%s|%s|%s|%d",
					filepath.Base(symbol.Path), symbol.Name, symbol.ParentQualifiedName, symbol.NameRange.Start.Line)
				if _, found := mapping[key]; !found {
					t.Errorf("original source did not support returned declaration: %+v", symbol)
					continue
				}
				if mapping[key] {
					t.Errorf("duplicate returned declaration identity: %s", key)
				}
				mapping[key] = true
				if symbol.Name == "EventEmitter" && symbol.NameRange.Start.Line == 6 &&
					string(symbol.Kind) != "class" {
					t.Errorf("ambient type must be a class: %+v", symbol)
				}
				if symbol.Name == "EventEmitter" && symbol.NameRange.Start.Line == 7 &&
					string(symbol.Kind) != "constructor" {
					t.Errorf("declared constructor must remain distinct: %+v", symbol)
				}
			}
			for key, seen := range mapping {
				if !seen {
					t.Errorf("independently inventoried supported declaration omitted: %s", key)
				}
			}
			t.Logf("full selected-file supported-declaration inventory verified: %d symbols in %d files", len(tc.want), len(paths))
		})
	}
}
