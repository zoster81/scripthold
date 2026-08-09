package config

import "testing"

func TestTaskConfigurationDefaultsAndBounds(t *testing.T) {
	values := map[string]string{
		EnvTaskStoreDir:             "/private/tasks",
		EnvTaskMaxConcurrency:       "4",
		EnvTaskMaxQueued:            "80",
		EnvTaskMaxLogBytesPerStream: "1048576",
		EnvTaskMaxRuntimeSeconds:    "0",
		EnvTaskRetentionDays:        "9",
		EnvTaskMaxTerminal:          "500",
		EnvTaskMaxTotalBytes:        "67108864",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if !cfg.Tasks.Enabled() || cfg.Tasks.StoreDir != "/private/tasks" {
		t.Fatalf("task store was not enabled: %#v", cfg.Tasks)
	}
	if cfg.Tasks.MaxConcurrency != 4 || cfg.Tasks.MaxQueued != 80 || cfg.Tasks.MaxRuntimeSeconds != 0 {
		t.Fatalf("unexpected task limits: %#v", cfg.Tasks)
	}
	if cfg.Tasks.MaxLogBytesPerStream != 1048576 || cfg.Tasks.RetentionDays != 9 || cfg.Tasks.MaxTerminal != 500 || cfg.Tasks.MaxTotalBytes != 67108864 {
		t.Fatalf("unexpected task retention/log limits: %#v", cfg.Tasks)
	}

	values[EnvTaskMaxConcurrency] = "999"
	values[EnvTaskMaxRuntimeSeconds] = "-1"
	cfg = LoadFromEnvironment(func(name string) string { return values[name] })
	if cfg.Tasks.MaxConcurrency != DefaultTaskMaxConcurrency || cfg.Tasks.MaxRuntimeSeconds != DefaultTaskMaxRuntimeSeconds {
		t.Fatalf("invalid values did not fall back: %#v", cfg.Tasks)
	}
}
