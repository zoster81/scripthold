package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceSymbolsRoutesDataHardwareAndDocumentLanguages(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"schema.sql":   "CREATE SCHEMA app;\nCREATE TABLE app.users (id INTEGER);\n",
		"package.pkb":  "CREATE OR REPLACE PACKAGE demo AS\n  PROCEDURE run(value NUMBER);\nEND demo;\n/\n",
		"api.graphql":  "schema { query: Query }\ntype Query { ping: String }\nquery Ping { ping }\n",
		"main.tf":      "terraform { required_version = \">= 1.6\" }\nresource \"demo_resource\" \"main\" {}\n",
		"default.nix":  "let\n  answer = 42;\nin { inherit answer; }\n",
		"model.proto":  "syntax = \"proto3\";\npackage demo.v1;\nmessage User { string id = 1; }\n",
		"counter.vhd":  "entity Counter is\nend Counter;\narchitecture rtl of Counter is\n  signal count : integer;\nbegin\nend rtl;\n",
		"counter.v":    "module counter;\n  wire ready;\nendmodule\n",
		"counter.sv":   "interface bus_if; logic ready; endinterface\nmodule top; logic active; endmodule\n",
		"start.asm":    ".text\nmain:\n  nop\n",
		"index.html":   "<!doctype html><main id=\"hero\"></main>\n",
		"data.xml":     "<?xml version=\"1.0\"?><item id=\"root-item\"/>\n",
		"site.css":     ".card { display: block; }\n",
		"site.scss":    "$gap: 1rem;\n.card { color: red; }\n",
		"site.sass":    "$gap: 1rem\n.card\n  color: red\n",
		"site.less":    "@gap: 1rem;\n.card { margin: @gap; }\n",
		"config.json":  "{\"service\":{\"name\":\"api\"}}\n",
		"config.yaml":  "service:\n  name: api\n",
		"config.toml":  "[server]\nhost = \"localhost\"\n",
		"README.md":    "# Project\n\n## Usage\n",
		"openapi.yaml": "openapi: 3.1.0\ninfo:\n  title: Demo\n  version: 1.0.0\npaths:\n  /users:\n    get:\n      operationId: listUsers\n",
		"playbook.yml": "- name: Configure web\n  hosts: web\n  tasks:\n    - name: Ping\n      ansible.builtin.ping:\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{filepath.Dir(root)})
	toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 1024,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("outline err=%v toolErr=%+v", err, toolErr)
	}
	if result.FilesConsidered != len(files) || result.FilesParsed != len(files)-1 || result.FilesSkipped != 1 || result.CoverageComplete {
		t.Fatalf("outline summary=%+v", result)
	}

	wantLanguages := map[string]bool{
		"sql": false, "plsql": false, "graphql": false, "terraform": false, "nix": false, "proto": false,
		"vhdl": false, "verilog": false, "systemverilog": false, "assembly": false,
		"html": false, "xml": false, "css": false, "scss": false, "sass": false, "less": false,
		"json": false, "yaml": false, "toml": false, "openapi": false, "ansible-yaml": false,
	}
	markdownSkipped := false
	for _, file := range result.Files {
		if filepath.Base(file.Path) == "README.md" {
			if file.Status != "skipped" || file.ErrorCode != ErrCodeUnsupported || file.Detection.Language != "" {
				t.Fatalf("Markdown must remain outside Source Intelligence: %+v", file)
			}
			markdownSkipped = true
			continue
		}
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("file routing=%+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	if !markdownSkipped {
		t.Fatalf("Markdown fixture was not reported as skipped: %+v", result.Files)
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed language %s: %+v", language, result.Files)
		}
	}

	for _, name := range []string{
		"app", "app.users", "demo", "demo.run", "schema", "Query", "Query.ping", "Ping",
		"demo_resource.main", "answer", "demo.v1", "demo.v1.User", "demo.v1.User.id",
		"Counter", "Counter.rtl", "Counter.rtl.count", "counter", "counter.ready", "bus_if", "bus_if.ready", "top", "top.active", "main",
		"hero", "root-item", ".card", "gap", "service", "service.name", "server", "server.host", "listUsers", "Configure web", "Configure web.Ping",
	} {
		found := false
		for _, symbol := range result.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 10 symbol %s; symbols=%+v", name, result.Symbols)
		}
	}
}
