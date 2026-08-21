package config

import "testing"

func TestLoad_DefaultReliabilityPolicy(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	if cfg.Reliability.Enabled() || cfg.Reliability.StoreDir != "" ||
		cfg.Reliability.MaxSynchronousSeconds != DefaultCallMaxSyncSeconds ||
		cfg.Reliability.DeferredSyncWaitSeconds != DefaultDeferredSyncWaitSeconds ||
		cfg.Reliability.DeferredMaxRuntimeSeconds != DefaultDeferredMaxRuntimeSeconds ||
		cfg.Reliability.DeferredMaxConcurrency != DefaultDeferredMaxConcurrency ||
		cfg.Reliability.DeferredMaxQueued != DefaultDeferredMaxQueued ||
		cfg.Reliability.DeferredRetentionSeconds != DefaultDeferredRetentionSeconds ||
		cfg.Reliability.DeferredMaxTotalBytes != DefaultDeferredMaxTotalBytes ||
		cfg.Reliability.MaxInlineResponseBytes != DefaultMaxInlineResponseBytes ||
		cfg.Reliability.ResponseChunkBytes != DefaultResponseChunkBytes {
		t.Fatalf("unexpected reliability defaults: %#v", cfg.Reliability)
	}
}

func TestLoad_ReliabilityPolicyUsesBoundedEnvironment(t *testing.T) {
	values := map[string]string{
		EnvDeferredStoreDir:          "private-deferred",
		EnvCallMaxSyncSeconds:        "40",
		EnvDeferredSyncWaitSeconds:   "10",
		EnvDeferredMaxRuntimeSeconds: "240",
		EnvDeferredMaxConcurrency:    "3",
		EnvDeferredMaxQueued:         "12",
		EnvDeferredRetentionSeconds:  "1800",
		EnvDeferredMaxTotalBytes:     "268435456",
		EnvMaxInlineResponseBytes:    "2097152",
		EnvResponseChunkBytes:        "524288",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if !cfg.Reliability.Enabled() || cfg.Reliability.StoreDir != "private-deferred" ||
		cfg.Reliability.MaxSynchronousSeconds != 40 ||
		cfg.Reliability.DeferredSyncWaitSeconds != 10 ||
		cfg.Reliability.DeferredMaxRuntimeSeconds != 240 ||
		cfg.Reliability.DeferredMaxConcurrency != 3 ||
		cfg.Reliability.DeferredMaxQueued != 12 ||
		cfg.Reliability.DeferredRetentionSeconds != 1800 ||
		cfg.Reliability.DeferredMaxTotalBytes != 268435456 ||
		cfg.Reliability.MaxInlineResponseBytes != 2097152 ||
		cfg.Reliability.ResponseChunkBytes != 524288 {
		t.Fatalf("unexpected reliability policy: %#v", cfg.Reliability)
	}
}

func TestLoad_ReliabilityPolicyRejectsUnsafeValues(t *testing.T) {
	values := map[string]string{
		EnvCallMaxSyncSeconds:        "120",
		EnvDeferredSyncWaitSeconds:   "90",
		EnvDeferredMaxRuntimeSeconds: "999999",
		EnvDeferredMaxConcurrency:    "1000",
		EnvDeferredMaxQueued:         "99999",
		EnvDeferredRetentionSeconds:  "99999999",
		EnvDeferredMaxTotalBytes:     "99999999999999",
		EnvMaxInlineResponseBytes:    "16777216",
		EnvResponseChunkBytes:        "8388608",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if cfg.Reliability.MaxSynchronousSeconds != DefaultCallMaxSyncSeconds ||
		cfg.Reliability.DeferredSyncWaitSeconds != DefaultDeferredSyncWaitSeconds ||
		cfg.Reliability.DeferredMaxRuntimeSeconds != DefaultDeferredMaxRuntimeSeconds ||
		cfg.Reliability.DeferredMaxConcurrency != DefaultDeferredMaxConcurrency ||
		cfg.Reliability.DeferredMaxQueued != DefaultDeferredMaxQueued ||
		cfg.Reliability.DeferredRetentionSeconds != DefaultDeferredRetentionSeconds ||
		cfg.Reliability.DeferredMaxTotalBytes != DefaultDeferredMaxTotalBytes ||
		cfg.Reliability.MaxInlineResponseBytes != DefaultMaxInlineResponseBytes ||
		cfg.Reliability.ResponseChunkBytes != DefaultResponseChunkBytes {
		t.Fatalf("unsafe reliability values did not fail closed: %#v", cfg.Reliability)
	}
}
