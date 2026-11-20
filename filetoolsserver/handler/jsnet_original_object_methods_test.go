package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shortcut commands are authored as function-valued properties of a
// returned object literal, not as top-level JScript.NET function declarations.
func TestJScriptNetOriginalReturnedObjectMethods(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSNET88_ADMIN")
	if root == "" {
		t.Skip("immutable original JScript.NET admin project not configured")
	}
	names := []string{"package.js", "utils.js", "parameters.js"}
	data := make(map[string]string, len(names))
	for _, name := range names {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatalf("original JScript.NET source not a bounded regular file %q: info=%v err=%v", name, info, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[name] = string(raw)
	}
	// Independent original source oracles: editing the shortcut creation is
	// separate from changing the path generator or the parameter parser.
	lines := strings.Split(data["package.js"], "\n")
	for _, target := range []struct {
		line    int
		excerpt string
	}{
		{35, "Create: function (markdownPath) {"},
		{49, "Delete: function () {"},
	} {
		if target.line <= 0 || target.line > len(lines) || !strings.Contains(lines[target.line-1], target.excerpt) {
			t.Fatalf("original shortcut method oracle changed at line %d", target.line)
		}
	}
	if !strings.Contains(data["package.js"], "IconLink: {") ||
		!strings.Contains(data["package.js"], "shell.CreateShortcut(this.Path)") ||
		!strings.Contains(data["package.js"], "DeleteFile(this.Path)") ||
		!strings.Contains(data["utils.js"], "function GenerateRandomPath(extension)") ||
		!strings.Contains(data["parameters.js"], "var Param = (function") {
		t.Fatal("original shortcut-maintenance boundary changed")
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	targetPath := filepath.Join(root, "package.js")
	_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{targetPath},
		Language: "jscript-net", MaxFiles: 1,
	})
	if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
		len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
		t.Fatalf("original JScript.NET digest unavailable: %v %+v", err, digest)
	}
	_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{targetPath},
		Language: "jscript-net", MaxFiles: 1, MaxSymbols: 128,
	})
	if err != nil || outline.Truncated || outline.FilesParsed != 1 {
		t.Fatalf("original JScript.NET outline unavailable: %v %+v", err, outline)
	}
	ids := make(map[string]bool)
	for _, symbol := range outline.Symbols {
		ids[symbol.ID] = true
	}
	for _, target := range []struct {
		name string
		line int
	}{
		{"Create", 35},
		{"Delete", 49},
	} {
		t.Run(target.name, func(t *testing.T) {
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{targetPath}, Language: "jscript-net",
				Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == target.name && sym.Kind == "method" &&
					sym.NativeKind == "object-function" && sym.ParentQualifiedName == "IconLink" &&
					sym.ParentID != "" && sym.NameRange.Start.Line == target.line {
					id = sym.ID
				}
			}
			if err != nil || id == "" || !ids[id] || len(found.Files) != 1 {
				t.Fatalf("original shortcut object method not found at %d: err=%v symbols=%+v", target.line, err, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("original JScript.NET selector differs from digest: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: targetPath, Language: "jscript-net",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.name+": function") {
				t.Fatalf("original shortcut method source missing: %v %+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{targetPath},
				Language: "jscript-net", Query: target.name, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			linked := false
			if query.Search != nil {
				for _, hit := range query.Search.Matches {
					if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
						hit.Range.Start.Line == target.line {
						linked = true
					}
				}
			}
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
				t.Fatalf("original shortcut method not source-linked: %v %+v", err, query)
			}
		})
	}
	for _, falseName := range []string{"CreateShortcut", "PwshExePath", "Set"} {
		_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{targetPath}, Language: "jscript-net",
			Query: falseName, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
		})
		if err != nil || len(found.Symbols) != 0 {
			t.Fatalf("JScript.NET call/data property invented as method: %s err=%v symbols=%+v", falseName, err, found.Symbols)
		}
	}
}
