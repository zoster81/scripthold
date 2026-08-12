package config

import "testing"

func TestFilesystemPackageLimitsDefaults(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	limits := cfg.Limits
	if limits.MaxFilesystemPackageOperations != DefaultMaxFilesystemPackageOperations ||
		limits.MaxFilesystemPackageBytes != DefaultMaxFilesystemPackageBytes ||
		limits.MaxFilesystemRecursiveEntries != DefaultMaxFilesystemRecursiveEntries ||
		limits.MaxFilesystemRecursiveDepth != DefaultMaxFilesystemRecursiveDepth ||
		limits.MaxFilesystemAggregateBytes != DefaultMaxFilesystemAggregateBytes ||
		limits.MaxFilesystemStagingBytes != DefaultMaxFilesystemStagingBytes ||
		limits.MaxFilesystemPackagePreviews != DefaultMaxFilesystemPackagePreviews ||
		limits.MaxFilesystemPackagePreviewBytes != DefaultMaxFilesystemPackagePreviewBytes ||
		limits.FilesystemPackagePreviewTTLSeconds != DefaultFilesystemPackagePreviewTTLSeconds {
		t.Fatalf("unexpected filesystem package limits: %#v", limits)
	}
}

func TestFilesystemPackageLimitsAreBoundedByHardCeilings(t *testing.T) {
	values := map[string]string{
		EnvMaxFilesystemPackageOperations:     "32",
		EnvMaxFilesystemPackageBytes:          "1048576",
		EnvMaxFilesystemRecursiveEntries:      "2048",
		EnvMaxFilesystemRecursiveDepth:        "64",
		EnvMaxFilesystemAggregateBytes:        "2097152",
		EnvMaxFilesystemStagingBytes:          "3145728",
		EnvMaxFilesystemPackagePreviews:       "8",
		EnvMaxFilesystemPackagePreviewBytes:   "4194304",
		EnvFilesystemPackagePreviewTTLSeconds: "120",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	limits := cfg.Limits
	if limits.MaxFilesystemPackageOperations != 32 || limits.MaxFilesystemPackageBytes != 1048576 ||
		limits.MaxFilesystemRecursiveEntries != 2048 || limits.MaxFilesystemRecursiveDepth != 64 ||
		limits.MaxFilesystemAggregateBytes != 2097152 || limits.MaxFilesystemStagingBytes != 3145728 ||
		limits.MaxFilesystemPackagePreviews != 8 || limits.MaxFilesystemPackagePreviewBytes != 4194304 ||
		limits.FilesystemPackagePreviewTTLSeconds != 120 {
		t.Fatalf("filesystem package environment overrides were not applied: %#v", limits)
	}

	overflow := map[string]string{
		EnvMaxFilesystemPackageOperations:     "4097",
		EnvMaxFilesystemPackageBytes:          "67108865",
		EnvMaxFilesystemRecursiveEntries:      "1000001",
		EnvMaxFilesystemRecursiveDepth:        "1025",
		EnvMaxFilesystemAggregateBytes:        "1099511627777",
		EnvMaxFilesystemStagingBytes:          "1099511627777",
		EnvMaxFilesystemPackagePreviews:       "1025",
		EnvMaxFilesystemPackagePreviewBytes:   "1073741825",
		EnvFilesystemPackagePreviewTTLSeconds: "86401",
	}
	cfg = LoadFromEnvironment(func(name string) string { return overflow[name] })
	limits = cfg.Limits
	if limits.MaxFilesystemPackageOperations != DefaultMaxFilesystemPackageOperations ||
		limits.MaxFilesystemPackageBytes != DefaultMaxFilesystemPackageBytes ||
		limits.MaxFilesystemRecursiveEntries != DefaultMaxFilesystemRecursiveEntries ||
		limits.MaxFilesystemRecursiveDepth != DefaultMaxFilesystemRecursiveDepth ||
		limits.MaxFilesystemAggregateBytes != DefaultMaxFilesystemAggregateBytes ||
		limits.MaxFilesystemStagingBytes != DefaultMaxFilesystemStagingBytes ||
		limits.MaxFilesystemPackagePreviews != DefaultMaxFilesystemPackagePreviews ||
		limits.MaxFilesystemPackagePreviewBytes != DefaultMaxFilesystemPackagePreviewBytes ||
		limits.FilesystemPackagePreviewTTLSeconds != DefaultFilesystemPackagePreviewTTLSeconds {
		t.Fatalf("hard ceilings did not fail closed to defaults: %#v", limits)
	}
}
