package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source-first sample from Svelte Bits' copy-code-button.svelte.
// This exercises development MCP handlers, not an installed server or runtime.
func TestSvelte5DestructuredPropsHandlerNavigation(t *testing.T) {
	const source = `<script lang="ts">
 import type { HTMLButtonAttributes } from "svelte/elements";
 let {
   class: className,
   copyCode,
   copied = false,
   ...rest
 }: {
   copyCode: () => void;
   copied?: boolean;
   typeOnly?: string;
 } & HTMLButtonAttributes = $props();
</script>
<button onclick={copyCode}></button>
`
	root := t.TempDir()
	path := filepath.Join(root, "copy-code-button.svelte")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	h, ctx := NewHandler([]string{root}), context.Background()
	_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{path}, Language: "svelte", MaxFiles: 1,
	})
	if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
		t.Fatalf("Svelte5 handler digest failed: err=%v %+v", err, digest)
	}
	_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Language: "svelte",
		Presentation: "detailed", MaxFiles: 1, MaxSymbols: 80,
	})
	if err != nil || outline.Truncated || !outline.CoverageComplete {
		t.Fatalf("Svelte5 handler outline incomplete: err=%v %+v", err, outline)
	}
	names := map[string]int{}
	id := ""
	for _, symbol := range outline.Symbols {
		names[symbol.Name] = symbol.NameRange.Start.Line
		if symbol.Name == "copyCode" && symbol.NameRange.Start.Line == 5 {
			id = symbol.ID
		}
	}
	for name, line := range map[string]int{"className": 4, "copyCode": 5, "copied": 6, "rest": 7} {
		if names[name] != line {
			t.Errorf("missing runtime Svelte5 prop binding %s:%d: %+v", name, line, names)
		}
	}
	for _, name := range []string{"class", "typeOnly", "onclick", "HTMLButtonAttributes"} {
		if _, falsePositive := names[name]; falsePositive {
			t.Errorf("invented runtime Svelte5 variable from property/type/template: %s", name)
		}
	}
	if id == "" {
		t.Fatal("source-safe selector for copyCode missing")
	}
	_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "svelte",
		Query: "copyCode", Match: "exact", MaxFiles: 1, MaxSymbols: 30,
		FileDetails: "relevant",
	})
	if err != nil || len(found.Files) != 1 {
		t.Fatalf("Svelte5 find failed: err=%v %+v", err, found)
	}
	present := false
	for _, s := range found.Symbols {
		if s.ID == id && s.NameRange.Start.Line == 5 {
			present = true
		}
	}
	f := found.Files[0]
	if !present || f.SourceFingerprint != digest.Digests[0].SourceFingerprint || f.Encoding == "" {
		t.Fatalf("source selector inconsistent: %+v", found)
	}
	_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: id, Language: "svelte",
		Encoding: f.Encoding, SourceFingerprint: f.SourceFingerprint, MaxBytes: 8192,
	})
	if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "copyCode,") {
		t.Fatalf("source-exact Svelte5 prop excerpt unavailable: err=%v %+v", err, shown.Show)
	}
	_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Paths: []string{path},
		Language: "svelte", Query: "copyCode", Match: "exact",
		MaxFiles: 1, MaxResults: 30,
	})
	if err != nil || query.Search == nil || query.Coverage.ResultsTruncated {
		t.Fatalf("Svelte5 structural search failed: err=%v %+v", err, query)
	}
	linked := false
	for _, hit := range query.Search.Matches {
		if hit.SymbolID == id && hit.SourceFingerprint == f.SourceFingerprint {
			linked = true
		}
	}
	if !linked {
		t.Fatalf("structural search did not link Svelte5 runtime binding: %+v", query.Search)
	}
}
