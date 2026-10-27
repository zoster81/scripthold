package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

func TestLanguageDetectorContextualExtensionCorroboration(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		path         string
		text         string
		wantState    DetectionState
		wantLanguage string
	}{
		{
			name: "Vue single-file component with TypeScript setup",
			path: "Header.vue",
			text: "<script setup lang=\"ts\">\nimport { computed } from 'vue'\n" +
				"const state = computed(() => true)\n</script>\n<template><div>{{state}}</div></template>\n",
			wantState: DetectionProbable, wantLanguage: "vue",
		},
		{
			name: "HTML document with embedded script",
			path: "adc.html",
			text: "<!DOCTYPE html>\n<html><head></head><body>" +
				"<script>\nlet count = 1;\nfunction init() {}\n</script></body></html>",
			wantState: DetectionProbable, wantLanguage: "html",
		},
		{
			name: "Proto2 without syntax directive but with message and package",
			path: "addressbook.proto",
			text: "package tutorial;\noption java_package = \"example.book\";\n" +
				"message Person {\n required string name = 1;\n optional int32 id = 2;\n}\n",
			wantState: DetectionProbable, wantLanguage: "proto",
		},
		{
			name:      "Vue extension disagrees with standalone Go",
			path:      "bad.vue",
			text:      "package main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "HTML extension disagrees with standalone Go",
			path:      "bad.html",
			text:      "package main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "Proto extension disagrees with standalone Go",
			path:      "bad.proto",
			text:      "package main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "Proto fake message inside comment is not corroboration",
			path:      "fake.proto",
			text:      "// package tutorial;\n// message Person {\npackage main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "Vue tag in HTML comment is not corroboration",
			path:      "fake.vue",
			text:      "<!-- <script setup> </script> -->\npackage main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "Existing misleading Python extension stays ambiguous",
			path:      "wrong.py",
			text:      "package main\nfunc main() {}\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "Shared classic BASIC extension stays ambiguous",
			path:      "legacy.bas",
			text:      "Sub Main()\nEnd Sub\n",
			wantState: DetectionAmbiguous,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.wantState || got.Language != tc.wantLanguage {
				t.Fatalf("DetectLanguage(%q)=%+v; want %s %q", tc.path, got, tc.wantState, tc.wantLanguage)
			}
			if tc.wantLanguage != "" && !hasEvidenceForLanguage(got, EvidenceContentMarker, tc.wantLanguage) {
				t.Errorf("missing contextual content corroboration for %q: %+v", tc.wantLanguage, got.Evidence)
			}
		})
	}
}

func TestLanguageDetectorCorpusRoutingOracle(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		env, language string
	}{
		{"SCRIPTHOLD_ORACLE_VUE_FILE", "vue"},
		{"SCRIPTHOLD_ORACLE_HTML_FILE", "html"},
		{"SCRIPTHOLD_ORACLE_PROTO_FILE", "proto"},
	} {
		path := os.Getenv(tc.env)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: path, Text: string(data)})
		if err != nil {
			t.Fatal(err)
		}
		if got.State != DetectionProbable || got.Language != tc.language {
			t.Errorf("corpus %s: %+v, want probable %s", tc.language, got, tc.language)
		}
	}
}

func hasEvidenceForLanguage(result DetectionResult, kind EvidenceKind, language string) bool {
	for _, evidence := range result.Evidence {
		if evidence.Kind == kind && evidence.Language == language {
			return true
		}
	}
	return false
}
