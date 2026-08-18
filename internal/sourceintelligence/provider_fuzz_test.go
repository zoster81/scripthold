package sourceintelligence

import (
	"context"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func FuzzProviderAnalyzersNoPanic(f *testing.F) {
	manifest := loadProviderContractManifest(f)
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		f.Fatal(err)
	}

	analyzers := make([]SourceAnalyzer, 0, len(manifest.Providers))
	indexByLanguage := make(map[string]uint16, len(manifest.Providers))
	for _, contract := range manifest.Providers {
		if contract.Status != "active" {
			continue
		}
		descriptor, ok := registry.Resolve(contract.Language)
		if !ok {
			f.Fatalf("provider contract language %q missing from registry", contract.Language)
		}
		analyzer, ok := AnalyzerFor(descriptor)
		if !ok {
			f.Fatalf("provider contract language %q has no analyzer", contract.Language)
		}
		index := uint16(len(analyzers))
		indexByLanguage[contract.Language] = index
		analyzers = append(analyzers, analyzer)
		f.Add("\n", index)
	}
	if len(analyzers) == 0 {
		f.Fatal("provider contract has no active analyzers")
	}

	for _, seed := range providerRiskSeeds() {
		index, ok := indexByLanguage[seed.language]
		if !ok {
			f.Fatalf("provider fuzz seed references inactive or missing language %q", seed.language)
		}
		f.Add(seed.text, index)
	}

	f.Fuzz(func(t *testing.T, text string, selector uint16) {
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 128))
		if err != nil {
			switch operation.KindOf(err) {
			case operation.KindInvalidInput, operation.KindLimit, operation.KindUnsupported:
				return
			default:
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, operation.KindOf(err))
			}
		}
		assertAnalyzerResultWithinContractBounds(t, result, 128)
		assertAnalyzerResultCoordinates(t, result)
	})
}

type providerFuzzSeed struct {
	language string
	text     string
}

func providerRiskSeeds() []providerFuzzSeed {
	return []providerFuzzSeed{
		{"c", "#include <stdio.h>\nstruct Point { int x; };\nint work(int x) { return x; }\n"},
		{"cpp", "namespace Demo { template <class T> class Box { public: T get() const; }; }\n"},
		{"java", "package demo; public class Box<T> { public T get() { return null; } }\n"},
		{"kotlin", "package demo\ndata class Box<T>(val value: T) { fun get(): T = value }\n"},
		{"javascript", "export const value = () => 1;\nclass Box { run() {} }\nconst re = /class Fake {}/g;\n"},
		{"typescript", "interface Box<T> { value: T; }\nconst map = <T,>(v:T):T => v;\n"},
		{"rust", "macro_rules! x { () => { struct Fake; } }\npub struct Box<T> { pub value: T }\n"},
		{"php", "<?php\nclass Box { public function get() {} }\n"},
		{"ruby", "class Box\n  def get\n  end\nend\n"},
		{"swift", "struct Box { func get() {} }\n"},
		{"pascal", "program Demo;\ntype\n  TBox = class end;\n"},
		{"delphi", "unit Demo;\ninterface\ntype\n  TBox<T> = class end;\nimplementation\nend.\n"},
		{"vb6", "Attribute VB_Name = \"M\"\nPublic Sub Run()\nEnd Sub\n"},
		{"vbscript", "Class Box\nPublic Sub Run()\nEnd Sub\nEnd Class\n"},
		{"fsharp", "namespace Demo\nlet run x = x\n"},
		{"cpp-cli", "public ref class Box {};\n"},
		{"jscript-net", "package Demo { function run() {} }\n"},
		{"cil", ".assembly Demo {}\n.class public Box {}\n"},
		{"powershell", "function Get-Value {}\n"},
		{"classic-asp", "<%@ Language=\"JScript\" %><% function run() {} %>"},
		{"razor", "@functions { public void Run() {} }\n"},
		{"xaml", `<Window x:Class="Demo.View" />`},
		{"mql4", "input int Period = 14;\nint OnInit() { return 0; }\n"},
		{"mql5", "input double Lots = 0.1;\nvoid OnTick() {}\n"},
		{"objective-c", "@interface Service : NSObject\n- (void)run;\n@end\n"},
		{"dart", "import 'dart:async';\nclass Service {}\n"},
		{"d", "module demo;\nclass Service {}\n"},
		{"zig", "const std = @import(\"std\");\npub fn main() void {}\n"},
		{"nim", "proc run*() = discard\n"},
		{"solidity", "pragma solidity ^0.8.20;\ncontract Service {}\n"},
		{"apex", "trigger AccountTrigger on Account (before insert) {}\n"},
		{"al", "codeunit 50100 Worker { procedure Run() begin end; }\n"},
		{"arduino", "#include <Arduino.h>\nvoid setup() {}\n"},
		{"perl", "my $data = <<'EOF';\nsub Fake {}\nEOF\nsub Real {}\n"},
		{"lua", "function run() end\n"},
		{"luau", "export type User = { name: string }\n"},
		{"elixir", "defmodule Demo do\n  def run(), do: :ok\nend\n"},
		{"erlang", "-module(demo).\nrun() -> ok.\n"},
		{"gleam", "pub fn run() { Nil }\n"},
		{"groovy", "def pattern = /class Fake {}/\nclass Worker { def run() {} }\n"},
		{"shell", "run() { :; }\n"},
		{"bash", "function run() { :; }\n"},
		{"tcl", "proc run {} {}\n"},
		{"autohotkey", "class Worker { Run() { return 1 } }\n"},
		{"fortran", "module Demo\ncontains\nsubroutine run()\nend subroutine run\nend module Demo\n"},
		{"cobol", "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. DEMO.\n       PROCEDURE DIVISION.\n"},
		{"ada", "package Demo is\n  procedure Run;\nend Demo;\n"},
		{"matlab", "classdef Worker\nmethods\nfunction run(obj)\nend\nend\nend\n"},
		{"octave", "function run()\nendfunction\n"},
		{"julia", "module Demo\nmutable struct Worker\n value::Int\nend\nend\n"},
		{"r", "run <- function(x) x\n"},
		{"haskell", "module Demo where\ndata Worker = Worker Int\nrun :: Worker -> Int\n"},
		{"ocaml", "module Demo = struct\nlet run x = x\nend\n"},
		{"common-lisp", "(defpackage :demo)\n(in-package :demo)\n(defun run (x) x)\n"},
		{"clojure", "(ns demo.core)\n(defn run [x] x)\n"},
		{"emacs-lisp", "(defcustom phase9-value 1 \"marker\")\n(defun run (x) x)\n"},
		{"sql", "CREATE TABLE users(id INTEGER);\n"},
		{"plsql", "CREATE OR REPLACE PACKAGE demo AS\nPROCEDURE run;\nEND demo;\n/\n"},
		{"graphql", "schema { query: Query }\ntype Query { ping: String }\n"},
		{"terraform", "resource \"demo\" \"main\" {}\n"},
		{"nix", "let\n answer = 42;\nin {}\n"},
		{"proto", "syntax = \"proto3\";\nmessage Item { string id = 1; }\n"},
		{"vhdl", "entity Counter is\nend Counter;\n"},
		{"verilog", "module counter; wire ready; endmodule\n"},
		{"systemverilog", "interface bus_if; logic ready; endinterface\n"},
		{"assembly", "main:\n nop\n"},
		{"html", "<main id=\"hero\"></main>\n"},
		{"xml", "<item id=\"root\"/>\n"},
		{"css", ".card { display: block; }\n"},
		{"scss", "$gap: 1rem;\n.card { color: red; }\n"},
		{"sass", "$gap: 1rem\n.card\n color: red\n"},
		{"less", "@gap: 1rem;\n.card { margin: @gap; }\n"},
		{"json", "{\"service\":{\"name\":\"api\"}}\n"},
		{"yaml", "service:\n  name: api\n"},
		{"toml", "[server]\nhost = \"localhost\"\n"},
		{"markdown", "# Project\n\n## Usage\n"},
		{"openapi", "openapi: 3.1.0\npaths:\n  /users:\n    get:\n      operationId: listUsers\n"},
		{"ansible-yaml", "- name: Play\n  hosts: all\n  tasks:\n    - name: Ping\n      debug:\n"},
		{"vue", `<main id="hero"></main><script lang="ts">function load() {}</script><style>.card {}</style>`},
		{"svelte", `<main id="hero"></main><script>function load() {}</script>`},
		{"astro", "---\nfunction load() {}\n---\n<main id=\"hero\"></main>\n"},
		{"php-html", `<main id="hero"></main><?php function load() {} ?>`},
		{"jsp", `<main id="hero"></main><%! class Helper {} %>`},
		{"jinja", `<main id="hero"></main>{% macro render() %}{% endmacro %}`},
		{"twig", `<main id="hero"></main>{% block content %}{% endblock %}`},
		{"blade", `<main id="hero"></main>@section('content') @php function load() {} @endphp`},
		{"ejs", `<main id="hero"></main><% function load() {} %>`},
		{"scala", "package demo\nclass Service:\n  def run(value: Int): Int = value\n"},
		{"flow", "/* @flow */\nexport opaque type ID = string;\nexport class Service { run(value: ID): ID { return value; } }\n"},
	}
}
