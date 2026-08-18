package sourceintelligence

import (
	"context"
	"testing"
)

func TestDataHardwareDocumentAliasesResolveCanonically(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	if descriptor, ok := registry.Resolve("hcl"); !ok || descriptor.ID != "terraform" {
		t.Fatalf("HCL alias = %+v ok=%v, want terraform", descriptor, ok)
	}
	if descriptor, ok := registry.Resolve("protobuf"); !ok || descriptor.ID != "proto" {
		t.Fatalf("protobuf alias = %+v ok=%v, want proto", descriptor, ok)
	}
}

func TestDistinctiveDetection(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		text string
		want string
	}{
		{name: "graphql", path: "fixture", text: "schema { query: Query }\ntype Query { ping: String }\n", want: "graphql"},
		{name: "proto", path: "fixture", text: "syntax = \"proto3\";\npackage demo;\nmessage Item { string id = 1; }\n", want: "proto"},
		{name: "terraform", path: "fixture", text: "terraform { required_version = \">= 1.6\" }\nresource \"aws_s3_bucket\" \"x\" {}\n", want: "terraform"},
		{name: "vhdl", path: "fixture", text: "entity Counter is\nend Counter;\n", want: "vhdl"},
		{name: "plsql-over-sql-extension", path: "package.sql", text: "CREATE OR REPLACE PACKAGE demo AS\n  PROCEDURE run;\nEND demo;\n/\n", want: "plsql"},
		{name: "openapi-over-yaml-extension", path: "openapi.yaml", text: "openapi: 3.1.0\ninfo:\n  title: Demo\n  version: 1.0.0\npaths: {}\n", want: "openapi"},
		{name: "ansible-over-yaml-extension", path: "playbook.yml", text: "- name: Configure web\n  hosts: web\n  tasks:\n    - name: Ping\n      ansible.builtin.ping:\n", want: "ansible-yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != DetectionProbable || result.Language != tc.want {
				t.Fatalf("detection = %+v, want probable %s", result, tc.want)
			}
		})
	}
}

func TestGenericYAMLAndSQLRemainGenericWithoutDistinctiveEvidence(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		text string
		want string
	}{
		{path: "config.yaml", text: "service:\n  name: api\n  port: 8080\n", want: "yaml"},
		{path: "schema.sql", text: "CREATE TABLE users(id INTEGER);\n", want: "sql"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != tc.want {
			t.Fatalf("%s detection = %+v, want probable %s", tc.path, result, tc.want)
		}
	}
}
