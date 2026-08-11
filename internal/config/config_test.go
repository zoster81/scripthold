package config

import "testing"

func TestLoad_DefaultEncoding(t *testing.T) {
	t.Setenv(EnvDefaultEncoding, "")
	cfg := Load()
	if DefaultEncoding != "utf-8" || cfg.DefaultEncoding != "utf-8" {
		t.Fatalf("default encoding = %q, want utf-8", cfg.DefaultEncoding)
	}
}

func TestLoad_CustomEncoding(t *testing.T) {
	t.Setenv(EnvDefaultEncoding, "cp1251")
	cfg := Load()
	if cfg.DefaultEncoding != "windows-1251" {
		t.Fatalf("encoding = %q, want canonical windows-1251", cfg.DefaultEncoding)
	}
}

func TestLoad_UTF32Encoding(t *testing.T) {
	t.Setenv(EnvDefaultEncoding, "utf32le")
	cfg := Load()
	if cfg.DefaultEncoding != "utf-32-le" {
		t.Fatalf("encoding = %q, want canonical utf-32-le", cfg.DefaultEncoding)
	}
}

func TestLoad_InvalidEncodingFallsBack(t *testing.T) {
	t.Setenv(EnvDefaultEncoding, "invalid-encoding-xyz")
	cfg := Load()
	if cfg.DefaultEncoding != DefaultEncoding {
		t.Fatalf("encoding = %q, want fallback %q", cfg.DefaultEncoding, DefaultEncoding)
	}
}

func TestLoad_InvalidLegacyThresholdFallsBack(t *testing.T) {
	clearLimitEnvironment(t)
	t.Setenv(EnvMemoryThreshold, "not-a-number")
	cfg := Load()
	if cfg.Limits.MaxFileBytes != DefaultMaxFileBytes || cfg.Limits.MaxOutputBytes != DefaultMaxOutputBytes {
		t.Fatalf("invalid legacy threshold changed defaults: %#v", cfg.Limits)
	}
}

func TestLoad_NegativeLegacyThresholdFallsBack(t *testing.T) {
	clearLimitEnvironment(t)
	t.Setenv(EnvMemoryThreshold, "-1000")
	cfg := Load()
	if cfg.Limits.MaxFileBytes != DefaultMaxFileBytes || cfg.Limits.MaxOutputBytes != DefaultMaxOutputBytes {
		t.Fatalf("negative legacy threshold changed defaults: %#v", cfg.Limits)
	}
}
