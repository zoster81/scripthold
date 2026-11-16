package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Only a direct literal module name is a source-known Lua require dependency.
// A nested format string or a dynamic suffix is not the loaded module.
func TestLuaRequireDirectLiteralDependencies(t *testing.T) {
	const fixture = "local A = require(\"direct.core\")\n" +
		"local B = require 'direct.bare'\n" +
		"local C = require(\"direct.single\")\n" +
		"local D = require(string.format(\"dynamic.%s\", name))\n" +
		"local E = require(module .. \".suffix\")\n" +
		"local F = manager.require(\"not.a.module.import\")\n" +
		"local G = require(\"dynamic.prefix.\" .. name)\n" +
		"local H = require(lookup(\"nested.only\"))\n" +
		"local I = require(\"direct.after\")\n"
	for _, eol := range []string{"LF", "CRLF"} {
		t.Run("portable-"+eol, func(t *testing.T) {
			source := fixture
			if eol == "CRLF" {
				source = strings.ReplaceAll(source, "\n", "\r\n")
			}
			checkLuaImports(t, source, []string{"direct.core", "direct.bare", "direct.single", "direct.after"})
		})
	}
	for _, tc := range []struct {
		name, env, needle string
		expected          []string
	}{
		{"overseer.nvim", "SCRIPTHOLD_REAL_LUA_OVERSEER", "require(string.format(\"overseer.strategy.%s\", name))", []string{"overseer.util"}},
		{"zenbones.nvim", "SCRIPTHOLD_REAL_LUA_ZENBONES", "require(string.format(\"lualine.themes.%s_%s\", name, ness))", []string{"lush", "zenbones.term"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 100000 || info.Size() < 100 {
				t.Fatalf("original Lua source unavailable/unbounded: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(raw)
			if !strings.Contains(source, tc.needle) {
				t.Fatal("independent original Lua oracle drift")
			}
			checkLuaImports(t, source, tc.expected)
		})
	}
}

func checkLuaImports(t *testing.T, source string, expected []string) {
	t.Helper()
	result, err := (LuaAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.lua", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatalf("unexpected truncated Lua analysis: %+v", result.Analysis)
	}
	got := make([]string, 0, len(result.Dependencies))
	for _, dep := range result.Dependencies {
		if dep.Kind == StructuralDependencyImport {
			got = append(got, dep.Value)
		}
	}
	if len(got) != len(expected) {
		t.Fatalf("Lua import facts=%q; wanted exactly %q", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("Lua import facts=%q; wanted exactly %q", got, expected)
		}
	}
}
