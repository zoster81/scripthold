package config

import "testing"

func TestMarkdownWorkspaceLimitsDefaults(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	limits := cfg.Limits
	if limits.MaxMarkdownWorkspaceDocuments != DefaultMarkdownWorkspaceMaxDocuments ||
		limits.MaxMarkdownWorkspaceRelationships != DefaultMarkdownWorkspaceMaxRelationships {
		t.Fatalf("unexpected Markdown workspace limits: %#v", limits)
	}
}

func TestMarkdownWorkspaceLimitsAreBoundedByHardCeilings(t *testing.T) {
	values := map[string]string{
		EnvMarkdownWorkspaceMaxDocuments:     "2048",
		EnvMarkdownWorkspaceMaxRelationships: "50000",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	limits := cfg.Limits
	if limits.MaxMarkdownWorkspaceDocuments != 2048 ||
		limits.MaxMarkdownWorkspaceRelationships != 50000 {
		t.Fatalf("Markdown workspace environment overrides were not applied: %#v", limits)
	}

	overflow := map[string]string{
		EnvMarkdownWorkspaceMaxDocuments:     "10001",
		EnvMarkdownWorkspaceMaxRelationships: "250001",
	}
	cfg = LoadFromEnvironment(func(name string) string { return overflow[name] })
	limits = cfg.Limits
	if limits.MaxMarkdownWorkspaceDocuments != DefaultMarkdownWorkspaceMaxDocuments ||
		limits.MaxMarkdownWorkspaceRelationships != DefaultMarkdownWorkspaceMaxRelationships {
		t.Fatalf("Markdown workspace hard ceilings did not fail closed to defaults: %#v", limits)
	}
}
