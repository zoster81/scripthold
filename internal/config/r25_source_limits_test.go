package config

import (
	"reflect"
	"strconv"
	"testing"
)

type r25SourceLimitExpectation struct {
	field        string
	environment  string
	defaultValue int64
	hardMaximum  int64
}

var r25SourceLimitContract = []r25SourceLimitExpectation{
	{field: "MaxInputPaths", environment: "MCP_SOURCE_MAX_INPUT_PATHS", defaultValue: 32, hardMaximum: 256},
	{field: "MaxFiles", environment: "MCP_SOURCE_MAX_FILES", defaultValue: 256, hardMaximum: 4_096},
	{field: "MaxAggregateBytes", environment: "MCP_SOURCE_MAX_AGGREGATE_BYTES", defaultValue: 64 * 1024 * 1024, hardMaximum: 512 * 1024 * 1024},
	{field: "MaxFileBytes", environment: "MCP_SOURCE_MAX_FILE_BYTES", defaultValue: 8 * 1024 * 1024, hardMaximum: 64 * 1024 * 1024},
	{field: "MaxSymbols", environment: "MCP_SOURCE_MAX_SYMBOLS", defaultValue: 10_000, hardMaximum: 100_000},
	{field: "MaxSignatureBytes", environment: "MCP_SOURCE_MAX_SIGNATURE_BYTES", defaultValue: 8 * 1024, hardMaximum: 64 * 1024},
	{field: "MaxShowBytes", environment: "MCP_SOURCE_MAX_SHOW_BYTES", defaultValue: 1024 * 1024, hardMaximum: 8 * 1024 * 1024},
	{field: "MaxDiagnostics", environment: "MCP_SOURCE_MAX_DIAGNOSTICS", defaultValue: 256, hardMaximum: 4_096},
	{field: "MaxDetectorProbes", environment: "MCP_SOURCE_MAX_DETECTOR_PROBES", defaultValue: 4, hardMaximum: 16},
	{field: "MaxNesting", environment: "MCP_SOURCE_MAX_NESTING", defaultValue: 256, hardMaximum: 2_048},
	{field: "MaxConcurrency", environment: "MCP_SOURCE_MAX_CONCURRENCY", defaultValue: 4, hardMaximum: 32},
	{field: "MaxRequestSeconds", environment: "MCP_SOURCE_MAX_REQUEST_SECONDS", defaultValue: 30, hardMaximum: 300},
	{field: "MaxOutputBytes", environment: "MCP_SOURCE_MAX_OUTPUT_BYTES", defaultValue: 16 * 1024 * 1024, hardMaximum: 64 * 1024 * 1024},
}

func TestR25SourceLimitsDefaults(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	for _, expectation := range r25SourceLimitContract {
		if got := r25SourceLimitValue(t, cfg, expectation.field); got != expectation.defaultValue {
			t.Errorf("%s default = %d, want %d", expectation.field, got, expectation.defaultValue)
		}
	}
}

func TestR25SourceLimitsEnvironmentOverrides(t *testing.T) {
	for _, expectation := range r25SourceLimitContract {
		t.Run(expectation.field, func(t *testing.T) {
			override := expectation.defaultValue + 1
			if override > expectation.hardMaximum {
				override = expectation.hardMaximum
			}
			cfg := LoadFromEnvironment(func(name string) string {
				if name == expectation.environment {
					return strconv.FormatInt(override, 10)
				}
				return ""
			})
			if got := r25SourceLimitValue(t, cfg, expectation.field); got != override {
				t.Fatalf("%s override = %d, want %d", expectation.field, got, override)
			}
		})
	}
}

func TestR25SourceLimitsRejectValuesAboveHardMaximum(t *testing.T) {
	for _, expectation := range r25SourceLimitContract {
		t.Run(expectation.field, func(t *testing.T) {
			cfg := LoadFromEnvironment(func(name string) string {
				if name == expectation.environment {
					return strconv.FormatInt(expectation.hardMaximum+1, 10)
				}
				return ""
			})
			if got := r25SourceLimitValue(t, cfg, expectation.field); got != expectation.defaultValue {
				t.Fatalf("%s over-hard-max value = %d, want fallback %d", expectation.field, got, expectation.defaultValue)
			}
		})
	}
}

func r25SourceLimitValue(t *testing.T, cfg *Config, field string) int64 {
	t.Helper()
	if cfg == nil {
		t.Fatal("nil configuration")
	}
	configValue := reflect.ValueOf(cfg)
	if configValue.Kind() != reflect.Pointer || configValue.IsNil() {
		t.Fatalf("unexpected configuration value %T", cfg)
	}
	source := configValue.Elem().FieldByName("Source")
	if !source.IsValid() {
		t.Fatal("R25 source limits are not implemented: Config.Source is missing")
	}
	value := source.FieldByName(field)
	if !value.IsValid() {
		t.Fatalf("R25 source limit %s is missing", field)
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	default:
		t.Fatalf("R25 source limit %s has unsupported kind %s", field, value.Kind())
		return 0
	}
}
