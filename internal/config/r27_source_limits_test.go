package config

import (
	"reflect"
	"strconv"
	"testing"
)

type r27SourceLimitExpectation struct {
	field        string
	environment  string
	defaultValue int64
	hardMaximum  int64
}

var r27SourceLimitContract = []r27SourceLimitExpectation{
	{field: "MaxResults", environment: "MCP_SOURCE_MAX_RESULTS", defaultValue: 10_000, hardMaximum: 100_000},
	{field: "MaxGraphNodes", environment: "MCP_SOURCE_MAX_GRAPH_NODES", defaultValue: 5_000, hardMaximum: 50_000},
	{field: "MaxGraphEdges", environment: "MCP_SOURCE_MAX_GRAPH_EDGES", defaultValue: 20_000, hardMaximum: 200_000},
	{field: "MaxGraphDepth", environment: "MCP_SOURCE_MAX_GRAPH_DEPTH", defaultValue: 8, hardMaximum: 64},
	{field: "MaxContextBytes", environment: "MCP_SOURCE_MAX_CONTEXT_BYTES", defaultValue: 1024 * 1024, hardMaximum: 8 * 1024 * 1024},
	{field: "MaxContextItems", environment: "MCP_SOURCE_MAX_CONTEXT_ITEMS", defaultValue: 256, hardMaximum: 4_096},
	{field: "MaxIndexProjects", environment: "MCP_SOURCE_MAX_INDEX_PROJECTS", defaultValue: 4, hardMaximum: 16},
	{field: "MaxIndexGenerations", environment: "MCP_SOURCE_MAX_INDEX_GENERATIONS", defaultValue: 2, hardMaximum: 4},
}

func TestR27SourceLimitsDefaults(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	for _, expectation := range r27SourceLimitContract {
		if got := r27SourceLimitValue(t, cfg, expectation.field); got != expectation.defaultValue {
			t.Errorf("%s default = %d, want %d", expectation.field, got, expectation.defaultValue)
		}
	}
}

func TestR27SourceLimitsEnvironmentOverrides(t *testing.T) {
	for _, expectation := range r27SourceLimitContract {
		t.Run(expectation.field, func(t *testing.T) {
			override := expectation.defaultValue + 1
			cfg := LoadFromEnvironment(func(name string) string {
				if name == expectation.environment {
					return strconv.FormatInt(override, 10)
				}
				return ""
			})
			if got := r27SourceLimitValue(t, cfg, expectation.field); got != override {
				t.Fatalf("%s override = %d, want %d", expectation.field, got, override)
			}
		})
	}
}

func TestR27SourceLimitsRejectValuesAboveHardMaximum(t *testing.T) {
	for _, expectation := range r27SourceLimitContract {
		t.Run(expectation.field, func(t *testing.T) {
			cfg := LoadFromEnvironment(func(name string) string {
				if name == expectation.environment {
					return strconv.FormatInt(expectation.hardMaximum+1, 10)
				}
				return ""
			})
			if got := r27SourceLimitValue(t, cfg, expectation.field); got != expectation.defaultValue {
				t.Fatalf("%s over-hard-max = %d, want fallback %d", expectation.field, got, expectation.defaultValue)
			}
		})
	}
}

func r27SourceLimitValue(t *testing.T, cfg *Config, field string) int64 {
	t.Helper()
	value := reflect.ValueOf(cfg).Elem().FieldByName("Source").FieldByName(field)
	if !value.IsValid() {
		t.Fatalf("R27 source limit %s is missing", field)
	}
	return value.Int()
}
