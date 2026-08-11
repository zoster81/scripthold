package handler

import (
	"context"
	"testing"
)

func TestHandleListEncodings(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	input := ListEncodingsInput{}

	result, output, err := h.HandleListEncodings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	// Check encodings list
	if len(output.Encodings) == 0 {
		t.Fatal("expected encodings list, got empty")
	}

	// Check the encoding registry remains populated; exact 24-encoding coverage is verified by the real-fixture integration tests.
	if len(output.Encodings) < 15 {
		t.Errorf("expected at least 15 encodings, got %d", len(output.Encodings))
	}

	// Helper to check if encoding exists
	hasEncoding := func(name string) bool {
		for _, enc := range output.Encodings {
			if enc.Name == name {
				return true
			}
			for _, alias := range enc.Aliases {
				if alias == name {
					return true
				}
			}
		}
		return false
	}

	// Check for UTF-8
	if !hasEncoding("utf-8") {
		t.Error("expected utf-8 in encodings list")
	}

	// Check for Windows-1251 (Cyrillic)
	if !hasEncoding("windows-1251") && !hasEncoding("cp1251") {
		t.Error("expected windows-1251/cp1251 in encodings list")
	}

	// Check for Windows-1252 (Western European)
	if !hasEncoding("windows-1252") && !hasEncoding("cp1252") {
		t.Error("expected windows-1252/cp1252 in encodings list")
	}

	// Check for Windows-1250 (Central European)
	if !hasEncoding("windows-1250") && !hasEncoding("cp1250") {
		t.Error("expected windows-1250/cp1250 in encodings list")
	}

	// Check encoding structure and explicit capability metadata.
	for _, enc := range output.Encodings {
		if enc.Name == "" {
			t.Error("encoding name should not be empty")
		}
		if enc.DisplayName == "" {
			t.Error("encoding display name should not be empty")
		}
		if enc.Description == "" {
			t.Error("encoding description should not be empty")
		}
		if !enc.Readable || !enc.Writable {
			t.Errorf("supported encoding %q lacks read/write capability", enc.Name)
		}
		if enc.AutoDetectable == enc.ExplicitOnly {
			t.Errorf("encoding %q must be exactly auto-detectable or explicit-only", enc.Name)
		}
	}
}
