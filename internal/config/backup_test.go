package config

import "testing"

func TestLoadFromEnvironmentBackupDefaultsDisabled(t *testing.T) {
	cfg := LoadFromEnvironment(func(string) string { return "" })
	if cfg.Backup.Enabled() {
		t.Fatal("backup store was enabled without an explicit directory")
	}
	if cfg.Backup.DefaultPolicy != BackupPolicyDisabled {
		t.Fatalf("backup default policy = %q, want %q", cfg.Backup.DefaultPolicy, BackupPolicyDisabled)
	}
	if cfg.Backup.Limits.MaxTotalBytes != DefaultBackupMaxTotalBytes ||
		cfg.Backup.Limits.MaxObjectBytes != DefaultBackupMaxObjectBytes ||
		cfg.Backup.Limits.MaxManifests != DefaultBackupMaxManifests ||
		cfg.Backup.Limits.MaxVersionsPerTarget != DefaultBackupMaxVersionsPerTarget ||
		cfg.Backup.Limits.MaxPinned != DefaultBackupMaxPinned ||
		cfg.Backup.Limits.RetentionDays != DefaultBackupRetentionDays ||
		cfg.Backup.Limits.PlanTTLSeconds != DefaultBackupPlanTTLSeconds {
		t.Fatalf("unexpected backup defaults: %#v", cfg.Backup)
	}
}

func TestLoadFromEnvironmentBackupOverrides(t *testing.T) {
	values := map[string]string{
		EnvBackupStoreDir:             "/var/lib/mcp-backups",
		EnvBackupDefaultPolicy:        BackupPolicyRequired,
		EnvBackupMaxTotalBytes:        "2000",
		EnvBackupMaxObjectBytes:       "300",
		EnvBackupMaxManifests:         "40",
		EnvBackupMaxVersionsPerTarget: "5",
		EnvBackupMaxPinned:            "6",
		EnvBackupRetentionDays:        "7",
		EnvBackupPlanTTLSeconds:       "8",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if !cfg.Backup.Enabled() || cfg.Backup.StoreDir != values[EnvBackupStoreDir] || cfg.Backup.DefaultPolicy != BackupPolicyRequired {
		t.Fatalf("backup store configuration = %#v", cfg.Backup)
	}
	if cfg.Backup.Limits.MaxTotalBytes != 2000 || cfg.Backup.Limits.MaxObjectBytes != 300 ||
		cfg.Backup.Limits.MaxManifests != 40 || cfg.Backup.Limits.MaxVersionsPerTarget != 5 ||
		cfg.Backup.Limits.MaxPinned != 6 || cfg.Backup.Limits.RetentionDays != 7 ||
		cfg.Backup.Limits.PlanTTLSeconds != 8 {
		t.Fatalf("unexpected backup overrides: %#v", cfg.Backup.Limits)
	}
}

func TestLoadFromEnvironmentBackupDefaultPolicyRejectsInvalidValue(t *testing.T) {
	cfg := LoadFromEnvironment(func(name string) string {
		if name == EnvBackupDefaultPolicy {
			return "optional"
		}
		return ""
	})
	if cfg.Backup.DefaultPolicy != BackupPolicyDisabled {
		t.Fatalf("invalid backup default policy produced %q", cfg.Backup.DefaultPolicy)
	}
}

func TestLoadFromEnvironmentBackupHardMaximums(t *testing.T) {
	values := map[string]string{
		EnvBackupMaxTotalBytes:        "1099511627777",
		EnvBackupMaxObjectBytes:       "1073741825",
		EnvBackupMaxManifests:         "1000001",
		EnvBackupMaxVersionsPerTarget: "10001",
		EnvBackupMaxPinned:            "100001",
		EnvBackupRetentionDays:        "3651",
		EnvBackupPlanTTLSeconds:       "86401",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if cfg.Backup.Limits.MaxTotalBytes != DefaultBackupMaxTotalBytes ||
		cfg.Backup.Limits.MaxObjectBytes != DefaultBackupMaxObjectBytes ||
		cfg.Backup.Limits.MaxManifests != DefaultBackupMaxManifests ||
		cfg.Backup.Limits.MaxVersionsPerTarget != DefaultBackupMaxVersionsPerTarget ||
		cfg.Backup.Limits.MaxPinned != DefaultBackupMaxPinned ||
		cfg.Backup.Limits.RetentionDays != DefaultBackupRetentionDays ||
		cfg.Backup.Limits.PlanTTLSeconds != DefaultBackupPlanTTLSeconds {
		t.Fatalf("out-of-range backup limits did not fall back: %#v", cfg.Backup.Limits)
	}
}

func TestLoadFromEnvironmentBackupRejectsNonPositiveAndMalformedValues(t *testing.T) {
	values := map[string]string{
		EnvBackupMaxTotalBytes:        "0",
		EnvBackupMaxObjectBytes:       "-1",
		EnvBackupMaxManifests:         "not-a-number",
		EnvBackupMaxVersionsPerTarget: "0",
		EnvBackupMaxPinned:            "-2",
		EnvBackupRetentionDays:        "0",
		EnvBackupPlanTTLSeconds:       "-3",
	}
	cfg := LoadFromEnvironment(func(name string) string { return values[name] })
	if cfg.Backup.Limits.MaxTotalBytes != DefaultBackupMaxTotalBytes ||
		cfg.Backup.Limits.MaxObjectBytes != DefaultBackupMaxObjectBytes ||
		cfg.Backup.Limits.MaxManifests != DefaultBackupMaxManifests ||
		cfg.Backup.Limits.MaxVersionsPerTarget != DefaultBackupMaxVersionsPerTarget ||
		cfg.Backup.Limits.MaxPinned != DefaultBackupMaxPinned ||
		cfg.Backup.Limits.RetentionDays != DefaultBackupRetentionDays ||
		cfg.Backup.Limits.PlanTTLSeconds != DefaultBackupPlanTTLSeconds {
		t.Fatalf("invalid backup limits did not fall back: %#v", cfg.Backup.Limits)
	}
}
