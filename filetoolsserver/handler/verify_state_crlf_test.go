package handler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyStateGitDiffTreatsCRAsLineTerminatorNotTrailingWhitespace(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	runGitTestCommand(t, git, root, "init")
	runGitTestCommand(t, git, root, "config", "core.autocrlf", "false")

	baseline := map[string][]byte{
		"lf-clean.txt":    []byte("base\n"),
		"crlf-clean.txt":  []byte("base\r\n"),
		"crlf-space.txt":  []byte("base\r\n"),
		"crlf-tab.txt":    []byte("base\r\n"),
		"mixed-clean.txt": []byte("base\r\n"),
		"noeof-clean.txt": []byte("base\r\n"),
	}
	for name, payload := range baseline {
		if err := os.WriteFile(filepath.Join(root, name), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitTestCommand(t, git, root, "add", "--", ".")

	modified := map[string][]byte{
		"lf-clean.txt":    []byte("base\nclean\n"),
		"crlf-clean.txt":  []byte("base\r\nclean\r\n"),
		"crlf-space.txt":  []byte("base\r\ndirty \r\n"),
		"crlf-tab.txt":    []byte("base\r\ndirty\t\r\n"),
		"mixed-clean.txt": []byte("base\r\nlf\ncrlf\r\n"),
		"noeof-clean.txt": []byte("base\r\nclean"),
	}
	for name, payload := range modified {
		if err := os.WriteFile(filepath.Join(root, name), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{root})
	tests := []struct {
		name string
		pass bool
	}{
		{name: "lf-clean.txt", pass: true},
		{name: "crlf-clean.txt", pass: true},
		{name: "crlf-space.txt", pass: false},
		{name: "crlf-tab.txt", pass: false},
		{name: "mixed-clean.txt", pass: true},
		{name: "noeof-clean.txt", pass: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
				Type:    VerifyCheckGitDiff,
				GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root, Paths: []string{tt.name}},
			}}})
			if err != nil || result.IsError || output.ErrorCount != 0 || len(output.Results) != 1 {
				t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
			}
			got := output.Results[0]
			if got.Passed != tt.pass {
				t.Fatalf("passed=%v, want %v; stdout=%q stderr=%q", got.Passed, tt.pass, got.Stdout, got.Stderr)
			}
			if !tt.pass && !strings.Contains(got.Stdout+got.Stderr, "trailing whitespace") {
				t.Fatalf("expected trailing-whitespace diagnostic: %+v", got)
			}
		})
	}
}
