//go:build !race

package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestPHPHeredocMaskingAvoidsPlainSourceCopyAllocation(t *testing.T) {
	ctx := context.Background()
	plain := strings.Repeat("function plain($value) { return $value + 1; }\n", 2048)

	plainAllocs := testing.AllocsPerRun(20, func() {
		masked, diagnostics, err := maskPHPHeredocs(ctx, plain)
		if err != nil {
			t.Fatal(err)
		}
		if masked != plain {
			t.Fatal("plain PHP source was unexpectedly changed")
		}
		if len(diagnostics) != 0 {
			t.Fatalf("plain PHP diagnostics = %+v", diagnostics)
		}
	})
	if plainAllocs != 0 {
		t.Fatalf("plain PHP heredoc masking allocations = %.0f, want 0", plainAllocs)
	}

	withHeredoc := "<?php\n$value = <<<TXT\nclass Hidden {}\nTXT;\nfunction after() {}\n"
	heredocAllocs := testing.AllocsPerRun(20, func() {
		masked, diagnostics, err := maskPHPHeredocs(ctx, withHeredoc)
		if err != nil {
			t.Fatal(err)
		}
		if masked == withHeredoc {
			t.Fatal("valid PHP heredoc was not masked")
		}
		if len(diagnostics) != 0 {
			t.Fatalf("valid PHP heredoc diagnostics = %+v", diagnostics)
		}
	})
	if heredocAllocs > 2 {
		t.Fatalf("valid PHP heredoc masking allocations = %.0f, want <= 2", heredocAllocs)
	}
}
