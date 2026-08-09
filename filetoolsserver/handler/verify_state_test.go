package handler

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	internalexecution "github.com/zoster81/scripthold/internal/execution"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestVerifyStateJSONTextAndFingerprint(t *testing.T) {
	root := t.TempDir()
	jsonPath := filepath.Join(root, "valid.json")
	if err := os.WriteFile(jsonPath, append([]byte{0xef, 0xbb, 0xbf}, []byte("{\"ok\":true}\r\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(root, "legacy.data")
	if err := os.WriteFile(textPath, encodeUTF16LEWithBOM(t, "alpha\r\nbeta \t\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fingerprintPath := filepath.Join(root, "fingerprint.bin")
	if err := os.WriteFile(fingerprintPath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{
		{Type: VerifyCheckJSON, JSON: &JSONVerificationCheck{Path: jsonPath, Encoding: "utf-8"}},
		{Type: VerifyCheckText, Text: &TextVerificationCheck{
			Path: textPath, Encoding: "utf-16-le", BOM: "utf-16-le", LineEndings: "crlf", TrailingWhitespace: "present",
		}},
		{Type: VerifyCheckFingerprint, Fingerprint: &FingerprintVerificationCheck{
			Paths: []string{fingerprintPath}, ExpectedFingerprint: fingerprintRegularFileForTest(t, fingerprintPath),
		}},
	}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
	}
	if !output.Passed || output.CheckCount != 3 || output.PassedCount != 3 || output.FailedCount != 0 || output.ErrorCount != 0 {
		t.Fatalf("unexpected output: %+v", output)
	}
	if got := output.Results[0]; !got.Passed || got.Encoding != "utf-8" || !got.HasBOM || got.BOMType != "utf-8" {
		t.Fatalf("JSON result=%+v", got)
	}
	if got := output.Results[1]; !got.Passed || got.Encoding != "utf-16-le" || got.LineEndingStyle != "crlf" || got.TrailingWhitespaceCount != 1 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Line != 2 {
		t.Fatalf("text result=%+v", got)
	}
	if got := output.Results[2]; !got.Passed || got.ActualFingerprint == "" || got.ActualFingerprint != got.ExpectedFingerprint {
		t.Fatalf("fingerprint result=%+v", got)
	}
}

func TestVerifyStateConditionFailuresAreStructuredResults(t *testing.T) {
	root := t.TempDir()
	jsonPath := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(jsonPath, []byte("{\"broken\":"), 0o644); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(root, "mixed.txt")
	if err := os.WriteFile(textPath, []byte("alpha\r\nbeta \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fingerprintPath := filepath.Join(root, "fingerprint.txt")
	if err := os.WriteFile(fingerprintPath, []byte("actual"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{
		{Type: VerifyCheckJSON, JSON: &JSONVerificationCheck{Path: jsonPath, Encoding: "utf-8"}},
		{Type: VerifyCheckText, Text: &TextVerificationCheck{Path: textPath, Encoding: "utf-8", LineEndings: "lf", TrailingWhitespace: "none"}},
		{Type: VerifyCheckFingerprint, Fingerprint: &FingerprintVerificationCheck{Paths: []string{fingerprintPath}, ExpectedFingerprint: strings.Repeat("a", 64)}},
	}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Passed || output.FailedCount != 3 || output.ErrorCount != 0 {
		t.Fatalf("unexpected output: %+v", output)
	}
	for index, item := range output.Results {
		if item.Passed || item.ErrorCode != "" || item.Message == "" {
			t.Fatalf("result[%d]=%+v", index, item)
		}
	}
	if len(output.Results[0].Diagnostics) == 0 || output.Results[0].Diagnostics[0].Kind != "jsonSyntax" {
		t.Fatalf("JSON diagnostics=%+v", output.Results[0].Diagnostics)
	}
}

func TestVerifyStateUnsupportedEncodingIsPerCheckOperationalError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{
		{Type: VerifyCheckText, Text: &TextVerificationCheck{Path: path, Encoding: "unsupported-encoding"}},
		{Type: VerifyCheckJSON, JSON: &JSONVerificationCheck{Path: path, Encoding: "utf-8"}},
	}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
	}
	if output.ErrorCount != 1 || output.FailedCount != 1 || output.Results[0].ErrorCode != ErrCodeEncoding || output.Results[1].ErrorCode != "" {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestVerifyStateInputRejectsUnknownFieldsAndInvalidUnions(t *testing.T) {
	unknownInputs := []string{
		`{"checks":[],"unknown":true}`,
		`{"checks":[{"type":"json","json":{"path":"x","unknown":true}}]}`,
		`{"checks":[{"type":"json","json":{"path":"x"},"text":{"path":"x"}}]}`,
	}
	for _, raw := range unknownInputs[:2] {
		var input VerifyStateInput
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("expected strict JSON rejection for %s", raw)
		}
	}

	h := NewHandler([]string{t.TempDir()})
	var input VerifyStateInput
	if err := json.Unmarshal([]byte(unknownInputs[2]), &input); err != nil {
		t.Fatal(err)
	}
	result, _, err := h.HandleVerifyState(context.Background(), nil, input)
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid union result=%+v err=%v", result, err)
	}
}

func TestVerifyStateGitDiffCheckUsesLiteralPathsAndClassifiesResults(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	runGitTestCommand(t, git, root, "init")
	pathName := "--no-index ; literal [file].txt"
	path := filepath.Join(root, pathName)
	if err := os.WriteFile(path, []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, git, root, "add", "--", pathName)
	if err := os.WriteFile(path, []byte("dirty \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type:    VerifyCheckGitDiff,
		GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root, Paths: []string{pathName}},
	}}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Passed || output.FailedCount != 1 || output.Results[0].Passed || output.Results[0].ExitCode != 2 && output.Results[0].ExitCode != 1 {
		t.Fatalf("unexpected git output: %+v", output)
	}
	if !strings.Contains(output.Results[0].Stdout+output.Results[0].Stderr, "trailing whitespace") {
		t.Fatalf("git diagnostics=%+v", output.Results[0])
	}
}

func TestVerifyStateGitRejectsNonRepositoryAndEscapingPaths(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available")
	}
	root := t.TempDir()
	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckGitDiff, GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root},
	}}})
	if err != nil || result.IsError || output.ErrorCount != 1 || output.Results[0].ErrorCode != ErrCodeOperationFailed {
		t.Fatalf("non-repository result=%+v output=%+v err=%v", result, output, err)
	}

	runGitTestCommand(t, git, root, "init")
	for _, path := range []string{"../escape.txt", filepath.Join("..", "escape.txt")} {
		result, output, err = h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
			Type: VerifyCheckGitDiff, GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root, Paths: []string{path}},
		}}})
		if err != nil || result.IsError || output.ErrorCount != 1 || output.Results[0].ErrorCode != ErrCodeInvalidInput {
			t.Fatalf("escape %q result=%+v output=%+v err=%v", path, result, output, err)
		}
	}
}

func TestVerifyStateGitRejectsEscapingSymlinkAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available without privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckGitDiff, GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root, Paths: []string{"escape/file.txt"}},
	}}})
	if err != nil || result.IsError || output.ErrorCount != 1 || output.Results[0].ErrorCode != ErrCodeSymlinkEscape {
		t.Fatalf("symlink result=%+v output=%+v err=%v", result, output, err)
	}
}

func TestVerifyStateGitOperationalFailuresAndCancellation(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	h.verifyGitExecutable = func() (string, error) {
		return "", operation.New(operation.KindNotFound, "git executable is unavailable")
	}
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckGitDiff, GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root},
	}}})
	if err != nil || result.IsError || output.ErrorCount != 1 || output.Results[0].ErrorCode != ErrCodeNotFound {
		t.Fatalf("missing git result=%+v output=%+v err=%v", result, output, err)
	}

	h = NewHandler([]string{root})
	h.verifyGitRun = func(context.Context, verificationGitRequest) (internalexecution.Result, error) {
		return internalexecution.Result{ExitCode: -1, TimedOut: true}, nil
	}
	result, output, err = h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckGitDiff, GitDiff: &GitDiffVerificationCheck{RepositoryRoot: root, TimeoutSeconds: 1},
	}}})
	if err != nil || result.IsError || output.ErrorCount != 1 || output.Results[0].ErrorCode != ErrCodeCancelled || !output.Results[0].TimedOut {
		t.Fatalf("timeout result=%+v output=%+v err=%v", result, output, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, _, err = h.HandleVerifyState(ctx, nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckJSON, JSON: &JSONVerificationCheck{Path: filepath.Join(root, "x.json")},
	}}})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancelled result=%+v err=%v", result, err)
	}
}

func TestVerifyStateLimitsDiagnosticsAndOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "trailing.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x \n", maxVerificationDiagnostics+20)), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckText, Text: &TextVerificationCheck{Path: path, Encoding: "utf-8", TrailingWhitespace: "none"},
	}}})
	if err != nil || result.IsError || !output.Results[0].DiagnosticsTruncated || len(output.Results[0].Diagnostics) != maxVerificationDiagnostics {
		t.Fatalf("diagnostic limit result=%+v output=%+v err=%v", result, output, err)
	}

	limited := NewHandler([]string{root}, WithConfig(&config.Config{DefaultEncoding: "utf-8", Limits: config.Limits{
		MaxFileBytes: config.DefaultMaxFileBytes, MaxDecodedCharacters: config.DefaultMaxDecodedCharacters,
		MaxLineBytes: config.DefaultMaxLineBytes, MaxBatchFiles: config.DefaultMaxBatchFiles,
		MaxOutputBytes: 1,
	}}))
	result, _, err = limited.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckText, Text: &TextVerificationCheck{Path: path, Encoding: "utf-8"},
	}}})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output limit result=%+v err=%v", result, err)
	}
}

func runGitTestCommand(t *testing.T, git, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command(git, args...)
	cmd.Dir = cwd
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestVerificationEnvironmentDoesNotInheritCredentialsOrExecutionFlags(t *testing.T) {
	t.Setenv("SECRET_TEST_TOKEN", "secret")
	t.Setenv("MCP_ENABLE_EXECUTION", "1")
	t.Setenv("LANG", "it_IT.UTF-8")
	t.Setenv("LC_ALL", "it_IT.UTF-8")
	env := verificationEnvironment()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "SECRET_TEST_TOKEN=") || strings.Contains(joined, "MCP_ENABLE_EXECUTION=") {
		t.Fatalf("unsafe inherited environment: %s", joined)
	}
	if strings.Contains(joined, "LANG=it_IT") || strings.Contains(joined, "LC_ALL=it_IT") {
		t.Fatalf("ambient locale survived filtering: %s", joined)
	}
	for _, required := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_NOSYSTEM=1", "LANG=C", "LC_ALL=C"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q in environment: %s", required, joined)
		}
	}
}

func TestVerifyStateFingerprintUsesSharedPrimitive(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	shared, err := filesystem.FingerprintPaths(context.Background(), []string{path}, filesystem.FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}), RespectGitignore: true, MaxEntries: config.DefaultMaxFingerprintEntries,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, output, _ := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
		Type: VerifyCheckFingerprint, Fingerprint: &FingerprintVerificationCheck{Paths: []string{path}, ExpectedFingerprint: shared.Fingerprint},
	}}})
	if output.Results[0].ActualFingerprint != shared.Fingerprint {
		t.Fatalf("actual=%s shared=%s", output.Results[0].ActualFingerprint, shared.Fingerprint)
	}
}
