package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase8SourceSymbolsRoutesDynamicBEAMAndScriptingLanguages(t *testing.T) {
	autoRoot := filepath.Join(canonicalHandlerTestDir(t), "auto")
	if err := os.MkdirAll(autoRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	autoFiles := map[string]string{
		"worker.pl":    "use strict;\npackage PerlBox;\nsub run { 1 }\n",
		"main.lua":     "local function LuaBox() return true end\n",
		"main.luau":    "--!strict\nexport type LuauBox = { name: string }\n",
		"worker.ex":    "defmodule ElixirBox do\n  def run(), do: :ok\nend\n",
		"worker.erl":   "-module(erlang_box).\nrun() -> ok.\n",
		"main.gleam":   "pub type GleamBox { GleamBox(name: String) }\n",
		"build.groovy": "def GroovyBox() {}\n",
		"build.sh":     "#!/bin/sh\nBashBox() { :; }\n",
		"main.tcl":     "namespace eval TclBox { proc run {} {} }\n",
		"main.ahk":     "#Requires AutoHotkey v2.0\nclass AHKBox { Run() { return 1 } }\n",
	}
	for name, content := range autoFiles {
		if err := os.WriteFile(filepath.Join(autoRoot, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{filepath.Dir(autoRoot)})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{autoRoot}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 512,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("auto outline err=%v toolErr=%+v", err, toolErr)
	}
	if outline.FilesConsidered != len(autoFiles) || outline.FilesParsed != len(autoFiles) || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 8 auto outline summary = %+v", outline)
	}
	wantLanguages := map[string]bool{
		"perl": false, "lua": false, "luau": false, "elixir": false, "erlang": false,
		"gleam": false, "groovy": false, "bash": false, "tcl": false, "autohotkey": false,
	}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 8 auto file routing = %+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed Phase 8 language %s: %+v", language, outline.Files)
		}
	}
	for _, name := range []string{"PerlBox", "PerlBox.run", "LuaBox", "LuauBox", "ElixirBox", "ElixirBox.run", "erlang_box", "erlang_box.run", "GleamBox", "GroovyBox", "BashBox", "TclBox", "TclBox.run", "AHKBox", "AHKBox.Run"} {
		found := false
		for _, symbol := range outline.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 8 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	explicitRoot := filepath.Join(filepath.Dir(autoRoot), "explicit")
	if err := os.MkdirAll(explicitRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(explicitRoot, "script.inc")
	if err := os.WriteFile(path, []byte("source \"ignored-in-posix.sh\"\nShellBox() { :; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Language: "shell", Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 64,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("explicit shell err=%v toolErr=%+v", err, toolErr)
	}
	if result.FilesParsed != 1 || result.FilesSkipped != 0 || !result.CoverageComplete || len(result.Files) != 1 || result.Files[0].Detection.Language != "shell" {
		t.Fatalf("explicit shell result=%+v", result)
	}
	found := false
	for _, symbol := range result.Symbols {
		if symbol.QualifiedName == "ShellBox" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("explicit shell missing ShellBox; symbols=%+v", result.Symbols)
	}
}
