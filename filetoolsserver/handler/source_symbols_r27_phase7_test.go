package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase7SourceSymbolsRoutesTradingAndSpecialtyLanguages(t *testing.T) {
	autoRoot := filepath.Join(t.TempDir(), "auto")
	if err := os.MkdirAll(autoRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	autoFiles := map[string]string{
		"expert.mq4":  "input int MQL4Box = 14;\nint OnInit() { return 0; }\n",
		"expert.mq5":  "input double MQL5Box = 0.1;\nvoid OnTick() {}\n",
		"Service.m":   "#import <Foundation/Foundation.h>\n@interface ObjCBox : NSObject\n- (void)run;\n@end\n",
		"Bridge.mm":   "#import <Foundation/Foundation.h>\n@interface ObjCPPBox : NSObject\n@end\nclass CPPBox {};\n",
		"main.dart":   "import 'dart:async';\nclass DartBox { void run() {} }\n",
		"main.d":      "module demo;\nclass DBox {}\n",
		"main.zig":    "const std = @import(\"std\");\npub const ZigBox = struct {};\n",
		"main.nim":    "proc NimBox*() = discard\n",
		"Token.sol":   "pragma solidity ^0.8.20;\ncontract SolidityBox {}\n",
		"Service.cls": "public with sharing class ApexBox {}\n",
		"App.al":      "codeunit 50100 ALBox { procedure Run() begin end; }\n",
		"Sketch.ino":  "#include <Arduino.h>\nclass ArduinoBox {};\nvoid setup() {}\n",
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
		t.Fatalf("Phase 7 auto outline summary = %+v", outline)
	}
	wantLanguages := map[string]bool{
		"mql4": false, "mql5": false, "objective-c": false, "objective-cpp": false,
		"dart": false, "d": false, "zig": false, "nim": false, "solidity": false, "apex": false, "al": false, "arduino": false,
	}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 7 auto file routing = %+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed Phase 7 language %s: %+v", language, outline.Files)
		}
	}
	for _, name := range []string{"MQL4Box", "MQL5Box", "ObjCBox", "ObjCPPBox", "CPPBox", "DartBox", "demo.DBox", "ZigBox", "NimBox", "SolidityBox", "ApexBox", "ALBox", "ArduinoBox"} {
		found := false
		for _, symbol := range outline.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 7 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	explicitRoot := filepath.Join(filepath.Dir(autoRoot), "explicit")
	if err := os.MkdirAll(explicitRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filename, language, text, want string
	}{
		{"shared4.mqh", "mql4", "input int Header4 = 1;\n", "Header4"},
		{"shared5.mqh", "mql5", "input int Header5 = 1;\n", "Header5"},
		{"shared.h", "objective-c", "@protocol ObjCHeader\n@end\n", "ObjCHeader"},
		{"shared.hpp", "objective-cpp", "@interface ObjCPPHeader\n@end\n", "ObjCPPHeader"},
	} {
		path := filepath.Join(explicitRoot, tc.filename)
		if err := os.WriteFile(path, []byte(tc.text), 0o600); err != nil {
			t.Fatal(err)
		}
		toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Paths: []string{path}, Language: tc.language, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 64,
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
