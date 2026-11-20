package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSSKeyframesOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		keyframes         []struct {
			name string
			line int
		}
		selector string
	}{
		{
			name: "animate-css", env: "SCRIPTHOLD_REAL_CSS_KEYFRAMES_ANIMATE",
			marker: "@keyframes hinge {",
			keyframes: []struct {
				name string
				line int
			}{{"hinge", 1}},
			selector: ".hinge",
		},
		{
			name: "hover-css", env: "SCRIPTHOLD_REAL_CSS_KEYFRAMES_HOVER",
			marker: "@keyframes circle {",
			keyframes: []struct {
				name string
				line int
			}{{"circle", 478}, {"initials", 491}},
			selector: ".ild-ident",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("two independently authored complete CSS original files not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 15000 {
				t.Fatalf("complete original CSS source unavailable/nonregular/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("authored upstream CSS source marker drift")
			}
			lines := strings.Split(source, "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "css",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.OutlineFiles) != 1 ||
				out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("authored CSS original MCP outline failed: %v %+v %+v", err, failure, out)
			}
			all := collectCSSOriginalOutline(out.OutlineFiles[0].Symbols)
			for _, k := range tc.keyframes {
				if k.line > len(lines) || k.line < 1 {
					t.Fatalf("authored CSS keyframe position outside source: %+v", k)
				}
				column := strings.Index(lines[k.line-1], "@keyframes") + 1
				if column == 0 || !strings.Contains(lines[k.line-1], k.name) {
					t.Fatalf("expected keyframe header %q not in original line %d", k.name, k.line)
				}
				found := false
				for _, node := range all {
					if node.Kind == "section" && node.Name == k.name &&
						node.Line == k.line && node.Column == column {
						found = true
					}
				}
				if !found {
					t.Errorf("CSS original animation %q declaration header at %d:%d missing from MCP outline", k.name, k.line, column)
				}
			}
			hasSelector := false
			for _, node := range all {
				if node.Kind == "selector" {
					switch node.Name {
					case "from", "to", "0%", "20%", "40%", "50%", "60%", "75%", "80%", "100%":
						t.Errorf("CSS keyframe stop incorrectly emitted as selector: %+v", node)
					}
					if node.Name == tc.selector {
						hasSelector = true
					}
				}
			}
			if !hasSelector {
				t.Errorf("neighboring original CSS selector %q missing from outline", tc.selector)
			}
			failure, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "css",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("authored CSS original MCP digest failed: %v %+v %+v", err, failure, digest)
			}
			sections := 0
			for _, count := range digest.Digests[0].DeclarationCounts {
				if count.Kind == "section" {
					sections += count.Count
				}
			}
			if sections < len(tc.keyframes) {
				t.Fatalf("original CSS keyframes missing from MCP digest: %d < %d", sections, len(tc.keyframes))
			}
		})
	}
}

func collectCSSOriginalOutline(nodes []*SourceOutlineNode) []*SourceOutlineNode {
	all := make([]*SourceOutlineNode, 0, len(nodes))
	for _, node := range nodes {
		all = append(all, node)
		all = append(all, collectCSSOriginalOutline(node.Children)...)
	}
	return all
}
