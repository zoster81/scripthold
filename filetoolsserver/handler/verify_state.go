package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	internalexecution "github.com/zoster81/scripthold/internal/execution"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

const (
	maxVerificationDiagnostics        = 1000
	defaultVerificationTimeoutSeconds = 30
	maxVerificationTimeoutSeconds     = 60
)

type verificationGitRequest struct {
	Executable       string
	ExecutableInfo   os.FileInfo
	RepositoryRoot   string
	RequestedRoot    string
	Paths            []string
	TimeoutSeconds   int
	OutputLimitBytes int
	InitialInfo      os.FileInfo
}

// HandleVerifyState runs an ordered batch of bounded read-only checks.
func (h *Handler) HandleVerifyState(ctx context.Context, _ *mcp.CallToolRequest, input VerifyStateInput) (*mcp.CallToolResult, VerifyStateOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.validateVerifyStateInput(input); err != nil {
		return errorResultFromError(err), VerifyStateOutput{}, nil
	}

	output := VerifyStateOutput{
		Passed:     true,
		CheckCount: len(input.Checks),
		Results:    make([]VerifyStateResult, len(input.Checks)),
	}
	perCheckBudget := clampBudgetToInt(h.maxOutputBytes() / int64(len(input.Checks)))
	if perCheckBudget <= 0 {
		perCheckBudget = 1
	}
	retainedBytes := int64(128)
	for index, check := range input.Checks {
		if err := ctx.Err(); err != nil {
			cancelled := operation.Wrap(operation.KindCancelled, "verify_state", "", err)
			return errorResultFromError(cancelled), VerifyStateOutput{}, nil
		}
		result := h.runVerificationCheck(ctx, index, check, perCheckBudget)
		output.Results[index] = result
		encodedResult, encodeErr := json.Marshal(result)
		if encodeErr != nil {
			wrapped := operation.Wrap(operation.KindFilesystem, "encode_verify_state_result", "", encodeErr)
			return errorResultFromError(wrapped), VerifyStateOutput{}, nil
		}
		retainedBytes += int64(len(encodedResult)) + 16
		if retainedBytes > h.maxOutputBytes() {
			limitErr := operation.New(operation.KindLimit, fmt.Sprintf("verification output exceeds limit %d bytes", h.maxOutputBytes()))
			return errorResultFromError(limitErr), VerifyStateOutput{}, nil
		}
		switch {
		case result.ErrorCode != "":
			output.ErrorCount++
			output.Passed = false
		case result.Passed:
			output.PassedCount++
		default:
			output.FailedCount++
			output.Passed = false
		}
		if err := ctx.Err(); err != nil {
			cancelled := operation.Wrap(operation.KindCancelled, "verify_state", "", err)
			return errorResultFromError(cancelled), VerifyStateOutput{}, nil
		}
	}

	text := fmt.Sprintf("Verification completed: %d passed, %d failed, %d operational errors.", output.PassedCount, output.FailedCount, output.ErrorCount)
	encoded, err := json.Marshal(output)
	if err != nil {
		wrapped := operation.Wrap(operation.KindFilesystem, "encode_verify_state_output", "", err)
		return errorResultFromError(wrapped), VerifyStateOutput{}, nil
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		limitErr := operation.New(operation.KindLimit, fmt.Sprintf("verification output exceeds limit %d bytes", h.maxOutputBytes()))
		return errorResultFromError(limitErr), VerifyStateOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) validateVerifyStateInput(input VerifyStateInput) error {
	if len(input.Checks) == 0 {
		return operation.New(operation.KindInvalidInput, "at least one verification check is required")
	}
	if len(input.Checks) > h.maxBatchFiles() {
		return operation.New(operation.KindLimit, fmt.Sprintf("verification check count %d exceeds limit %d", len(input.Checks), h.maxBatchFiles()))
	}
	for index, check := range input.Checks {
		objectCount := 0
		for _, present := range []bool{check.JSON != nil, check.Text != nil, check.GitDiff != nil, check.Fingerprint != nil} {
			if present {
				objectCount++
			}
		}
		if objectCount != 1 {
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d] must contain exactly one type-specific object", index))
		}
		switch check.Type {
		case VerifyCheckJSON:
			if check.JSON == nil {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].type=json requires json", index))
			}
			if strings.TrimSpace(check.JSON.Path) == "" {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].json.path is required", index))
			}
		case VerifyCheckText:
			if check.Text == nil {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].type=text requires text", index))
			}
			if strings.TrimSpace(check.Text.Path) == "" {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].text.path is required", index))
			}
			if !verificationValueAllowed(check.Text.BOM, "any", "none", "present", "utf-8", "utf-16-le", "utf-16-be", "utf-32-le", "utf-32-be") {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].text.bom must be any, none, present, utf-8, utf-16-le, utf-16-be, utf-32-le, or utf-32-be", index))
			}
			if !verificationValueAllowed(check.Text.LineEndings, "any", LineEndingLF, LineEndingCRLF, LineEndingMixed, LineEndingNone) {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].text.lineEndings must be any, lf, crlf, mixed, or none", index))
			}
			if !verificationValueAllowed(check.Text.TrailingWhitespace, "any", "none", "present") {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].text.trailingWhitespace must be any, none, or present", index))
			}
		case VerifyCheckGitDiff:
			if check.GitDiff == nil {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].type=gitDiff requires gitDiff", index))
			}
			if strings.TrimSpace(check.GitDiff.RepositoryRoot) == "" {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].gitDiff.repositoryRoot is required", index))
			}
			if len(check.GitDiff.Paths) > h.maxBatchFiles() {
				return operation.New(operation.KindLimit, fmt.Sprintf("checks[%d].gitDiff path count exceeds limit %d", index, h.maxBatchFiles()))
			}
			if check.GitDiff.TimeoutSeconds < 0 || check.GitDiff.TimeoutSeconds > maxVerificationTimeoutSeconds {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].gitDiff.timeoutSeconds must be between 1 and %d when provided", index, maxVerificationTimeoutSeconds))
			}
		case VerifyCheckFingerprint:
			if check.Fingerprint == nil {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].type=fingerprint requires fingerprint", index))
			}
			if len(check.Fingerprint.Paths) == 0 {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].fingerprint.paths must not be empty", index))
			}
			if len(check.Fingerprint.Paths) > h.maxBatchFiles() {
				return operation.New(operation.KindLimit, fmt.Sprintf("checks[%d].fingerprint path count exceeds limit %d", index, h.maxBatchFiles()))
			}
			if _, err := normalizePatchPackageFingerprint(check.Fingerprint.ExpectedFingerprint); err != nil {
				return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].fingerprint.expectedFingerprint %v", index, err))
			}
		default:
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("checks[%d].type must be json, text, gitDiff, or fingerprint", index))
		}
	}
	return nil
}

func verificationValueAllowed(value string, allowed ...string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		normalized = "any"
	}
	for _, candidate := range allowed {
		if normalized == candidate {
			return true
		}
	}
	return false
}

func (h *Handler) runVerificationCheck(ctx context.Context, index int, check VerificationCheck, outputBudget int) VerifyStateResult {
	switch check.Type {
	case VerifyCheckJSON:
		return h.verifyJSONCheck(ctx, index, check.JSON)
	case VerifyCheckText:
		return h.verifyTextCheck(ctx, index, check.Text)
	case VerifyCheckGitDiff:
		return h.verifyGitDiffCheck(ctx, index, check.GitDiff, outputBudget)
	case VerifyCheckFingerprint:
		return h.verifyFingerprintCheck(ctx, index, check.Fingerprint)
	default:
		return verificationErrorResult(index, check.Type, "", operation.New(operation.KindInvalidInput, "unsupported verification check"))
	}
}

func (h *Handler) verifyJSONCheck(ctx context.Context, index int, check *JSONVerificationCheck) VerifyStateResult {
	result := VerifyStateResult{Index: index, Type: VerifyCheckJSON, Path: check.Path}
	validated := h.ValidatePath(check.Path)
	if !validated.Ok() {
		return verificationMCPErrorResult(index, VerifyCheckJSON, check.Path, validated.Result)
	}
	stream, err := h.openDecodedTextStream(ctx, validated.Path, check.Encoding)
	if err != nil {
		return verificationErrorResult(index, VerifyCheckJSON, check.Path, err)
	}
	defer stream.Close()
	result.Encoding = stream.Charset
	result.HasBOM = stream.BOM.HasBOM
	result.BOMType = stream.BOM.Type
	if stream.FileSizeBytes > h.maxFileBytes() {
		limitErr := operation.New(operation.KindLimit, fmt.Sprintf("JSON source size %d exceeds limit %d", stream.FileSizeBytes, h.maxFileBytes()))
		return verificationErrorResult(index, VerifyCheckJSON, check.Path, limitErr)
	}
	limit := h.maxFileBytes()
	if limit == int64(^uint64(0)>>1) {
		limit--
	}
	data, err := io.ReadAll(io.LimitReader(stream.Reader, limit+1))
	if err != nil {
		return verificationErrorResult(index, VerifyCheckJSON, check.Path, err)
	}
	if int64(len(data)) > h.maxFileBytes() {
		limitErr := operation.New(operation.KindLimit, fmt.Sprintf("decoded JSON exceeds limit %d bytes", h.maxFileBytes()))
		return verificationErrorResult(index, VerifyCheckJSON, check.Path, limitErr)
	}
	if _, err := stream.Finish(); err != nil {
		return verificationErrorResult(index, VerifyCheckJSON, check.Path, err)
	}
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		result.Passed = false
		result.Message = "JSON is malformed"
		diagnostic := VerificationDiagnostic{Kind: "jsonSyntax", Message: boundedVerificationMessage(err.Error()), Path: check.Path}
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			diagnostic.Line, diagnostic.Column = jsonOffsetLineColumn(data, syntax.Offset)
		}
		result.Diagnostics = []VerificationDiagnostic{diagnostic}
		return result
	}
	result.Passed = true
	result.Message = "JSON is syntactically valid"
	return result
}

func jsonOffsetLineColumn(data []byte, offset int64) (line, column int) {
	line, column = 1, 1
	if offset < 1 {
		return line, column
	}
	end := int(offset - 1)
	if end > len(data) {
		end = len(data)
	}
	for index := 0; index < end; {
		if data[index] == '\n' {
			line++
			column = 1
			index++
			continue
		}
		_, width := utf8.DecodeRune(data[index:end])
		if width <= 0 {
			width = 1
		}
		column++
		index += width
	}
	return line, column
}

func (h *Handler) verifyTextCheck(ctx context.Context, index int, check *TextVerificationCheck) VerifyStateResult {
	result := VerifyStateResult{Index: index, Type: VerifyCheckText, Path: check.Path}
	validated := h.ValidatePath(check.Path)
	if !validated.Ok() {
		return verificationMCPErrorResult(index, VerifyCheckText, check.Path, validated.Result)
	}
	stream, err := h.openDecodedTextStream(ctx, validated.Path, check.Encoding)
	if err != nil {
		return verificationErrorResult(index, VerifyCheckText, check.Path, err)
	}
	defer stream.Close()
	result.Encoding = stream.Charset
	result.HasBOM = stream.BOM.HasBOM
	result.BOMType = stream.BOM.Type

	crlfCount := 0
	lfCount := 0
	trailingCount := 0
	totalLines, err := textstream.ScanLines(ctx, stream.Reader, h.maxLineBytes(), func(line textstream.Line) error {
		switch string(line.Ending) {
		case "\r\n":
			crlfCount++
		case "\n":
			lfCount++
		}
		if hasTrailingSpaceOrTab(line.Data) {
			trailingCount++
			if len(result.Diagnostics) < maxVerificationDiagnostics {
				result.Diagnostics = append(result.Diagnostics, VerificationDiagnostic{
					Kind: "trailingWhitespace", Message: "line has trailing spaces or tabs", Line: line.Number,
				})
			} else {
				result.DiagnosticsTruncated = true
			}
		}
		return nil
	})
	if err != nil {
		return verificationErrorResult(index, VerifyCheckText, check.Path, err)
	}
	if _, err := stream.Finish(); err != nil {
		return verificationErrorResult(index, VerifyCheckText, check.Path, err)
	}
	result.TotalLines = totalLines
	result.LineEndingStyle = determineStyle(crlfCount, lfCount)
	result.TrailingWhitespaceCount = trailingCount

	failures := make([]string, 0, 3)
	if !verifyBOMExpectation(check.BOM, result.HasBOM, result.BOMType) {
		failures = append(failures, fmt.Sprintf("BOM expectation %q was not met", normalizedVerificationExpectation(check.BOM)))
	}
	if expectation := normalizedVerificationExpectation(check.LineEndings); expectation != "any" && expectation != result.LineEndingStyle {
		failures = append(failures, fmt.Sprintf("line-ending expectation %q was not met; actual style is %q", expectation, result.LineEndingStyle))
	}
	switch normalizedVerificationExpectation(check.TrailingWhitespace) {
	case "none":
		if trailingCount > 0 {
			failures = append(failures, fmt.Sprintf("found trailing whitespace on %d lines", trailingCount))
		}
	case "present":
		if trailingCount == 0 {
			failures = append(failures, "expected trailing whitespace but none was found")
		}
	}
	result.Passed = len(failures) == 0
	if result.Passed {
		result.Message = "text-format expectations passed"
	} else {
		result.Message = strings.Join(failures, "; ")
	}
	return result
}

func hasTrailingSpaceOrTab(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	last := data[len(data)-1]
	return last == ' ' || last == '\t'
}

func normalizedVerificationExpectation(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "any"
	}
	return normalized
}

func verifyBOMExpectation(expectation string, hasBOM bool, bomType string) bool {
	switch normalizedVerificationExpectation(expectation) {
	case "any":
		return true
	case "none":
		return !hasBOM
	case "present":
		return hasBOM
	default:
		return hasBOM && bomType == normalizedVerificationExpectation(expectation)
	}
}

func (h *Handler) verifyFingerprintCheck(ctx context.Context, index int, check *FingerprintVerificationCheck) VerifyStateResult {
	expected, _ := normalizePatchPackageFingerprint(check.ExpectedFingerprint)
	result := VerifyStateResult{Index: index, Type: VerifyCheckFingerprint, ExpectedFingerprint: expected}
	validated := make([]string, len(check.Paths))
	for pathIndex, path := range check.Paths {
		if strings.TrimSpace(path) == "" {
			return verificationErrorResult(index, VerifyCheckFingerprint, path, operation.New(operation.KindInvalidInput, fmt.Sprintf("paths[%d] must be non-empty", pathIndex)))
		}
		validation := h.ValidatePath(path)
		if !validation.Ok() {
			return verificationMCPErrorResult(index, VerifyCheckFingerprint, path, validation.Result)
		}
		validated[pathIndex] = validation.Path
	}
	fingerprint, err := filesystem.FingerprintPaths(ctx, validated, filesystem.FingerprintOptions{
		ResolvedAllowedDirs: h.ResolvedAllowedDirs(),
		RespectGitignore:    shouldRespectGitignore(check.RespectGitignore),
		MaxEntries:          h.maxFingerprintEntries(),
	})
	if err != nil {
		return verificationErrorResult(index, VerifyCheckFingerprint, "", err)
	}
	result.ActualFingerprint = fingerprint.Fingerprint
	result.Passed = result.ActualFingerprint == result.ExpectedFingerprint
	if result.Passed {
		result.Message = "fingerprint matches"
	} else {
		result.Message = "fingerprint does not match"
	}
	return result
}

func (h *Handler) verifyGitDiffCheck(ctx context.Context, index int, check *GitDiffVerificationCheck, outputBudget int) VerifyStateResult {
	result := VerifyStateResult{Index: index, Type: VerifyCheckGitDiff, Path: check.RepositoryRoot}
	rootValidation := h.ValidatePath(check.RepositoryRoot)
	if !rootValidation.Ok() {
		return verificationMCPErrorResult(index, VerifyCheckGitDiff, check.RepositoryRoot, rootValidation.Result)
	}
	info, err := os.Stat(rootValidation.Path)
	if err != nil {
		return verificationErrorResult(index, VerifyCheckGitDiff, check.RepositoryRoot, operation.WrapFilesystem("inspect_verification_repository", check.RepositoryRoot, err))
	}
	if !info.IsDir() {
		return verificationErrorResult(index, VerifyCheckGitDiff, check.RepositoryRoot, operation.New(operation.KindInvalidInput, "repositoryRoot must be a directory"))
	}
	relativePaths := make([]string, len(check.Paths))
	for pathIndex, requested := range check.Paths {
		relative, pathErr := h.validateVerificationGitPath(check.RepositoryRoot, rootValidation.Path, requested)
		if pathErr != nil {
			return verificationErrorResult(index, VerifyCheckGitDiff, requested, pathErr)
		}
		relativePaths[pathIndex] = relative
	}
	executable, err := h.verifyGitExecutable()
	if err != nil {
		return verificationErrorResult(index, VerifyCheckGitDiff, check.RepositoryRoot, err)
	}
	executableInfo, err := os.Stat(executable)
	if err != nil {
		return verificationErrorResult(index, VerifyCheckGitDiff, executable, operation.WrapFilesystem("inspect_verification_git", executable, err))
	}
	if !executableInfo.Mode().IsRegular() {
		return verificationErrorResult(index, VerifyCheckGitDiff, executable, operation.New(operation.KindInvalidInput, "git executable must be a regular file"))
	}
	timeout := check.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultVerificationTimeoutSeconds
	}
	outputLimit := internalexecution.DefaultOutputLimitBytes
	conservativeBudget := outputBudget / 16
	if conservativeBudget > 0 && conservativeBudget < outputLimit {
		outputLimit = conservativeBudget
	}
	if outputLimit <= 0 {
		outputLimit = 1
	}
	executionResult, err := h.verifyGitRun(ctx, verificationGitRequest{
		Executable: executable, ExecutableInfo: executableInfo, RepositoryRoot: rootValidation.Path, RequestedRoot: check.RepositoryRoot,
		Paths: relativePaths, TimeoutSeconds: timeout, OutputLimitBytes: outputLimit, InitialInfo: info,
	})
	if err != nil {
		return verificationErrorResult(index, VerifyCheckGitDiff, check.RepositoryRoot, err)
	}
	result.ExitCode = executionResult.ExitCode
	result.Stdout = executionResult.Stdout
	result.Stderr = executionResult.Stderr
	result.TimedOut = executionResult.TimedOut
	result.ExecutionCancelled = executionResult.ExecutionCancelled
	result.OutputTruncated = executionResult.OutputTruncated
	result.DurationMillis = executionResult.DurationMillis
	if executionResult.TimedOut || executionResult.ExecutionCancelled {
		result.ErrorCode = ErrCodeCancelled
		if executionResult.TimedOut {
			result.Error = "git diff --check timed out"
		} else {
			result.Error = "git diff --check was cancelled"
		}
		result.Message = result.Error
		return result
	}
	switch executionResult.ExitCode {
	case 0:
		result.Passed = true
		result.Message = "git diff --check passed"
	case 1, 2:
		result.Passed = false
		result.Message = "git diff --check reported whitespace errors"
	default:
		result.ErrorCode = ErrCodeOperationFailed
		result.Error = boundedVerificationMessage(firstNonEmpty(executionResult.Stderr, executionResult.Stdout, fmt.Sprintf("git exited with code %d", executionResult.ExitCode)))
		result.Message = result.Error
	}
	return result
}

func (h *Handler) validateVerificationGitPath(requestedRoot, resolvedRoot, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", operation.New(operation.KindInvalidInput, "git diff path must be non-empty")
	}
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", operation.New(operation.KindInvalidInput, "git diff paths must be relative to repositoryRoot")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", operation.New(operation.KindInvalidInput, "git diff path escapes repositoryRoot")
	}
	joined := filepath.Join(requestedRoot, clean)
	validation := h.ValidatePath(joined)
	if !validation.Ok() {
		return "", validation.Err
	}
	relativeResolved, err := filepath.Rel(resolvedRoot, validation.Path)
	if err != nil || relativeResolved == ".." || strings.HasPrefix(relativeResolved, ".."+string(filepath.Separator)) {
		return "", operation.New(operation.KindAccessDenied, "git diff path resolves outside repositoryRoot")
	}
	return filepath.ToSlash(clean), nil
}

func findVerificationGit() (string, error) {
	candidates := []string{"git"}
	if runtime.GOOS == "windows" {
		candidates = []string{"git.exe", "git"}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		return filepath.Clean(absolute), nil
	}
	return "", operation.New(operation.KindNotFound, "git executable is unavailable")
}

func (h *Handler) runVerificationGit(ctx context.Context, request verificationGitRequest) (internalexecution.Result, error) {
	args := []string{
		"--no-pager", "--literal-pathspecs",
		"-c", "core.fsmonitor=false",
		"-c", "core.whitespace=blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol",
		"-c", "diff.external=",
		"-c", "interactive.diffFilter=",
		"diff", "--check", "--no-ext-diff", "--no-textconv", "--",
	}
	args = append(args, request.Paths...)
	plan, err := internalexecution.Prepare(internalexecution.Request{
		Program: request.Executable, Args: args, WorkingDirectory: request.RepositoryRoot,
		TimeoutSeconds: request.TimeoutSeconds, OutputLimitBytes: request.OutputLimitBytes,
		Environment: verificationEnvironment(),
	})
	if err != nil {
		return internalexecution.Result{}, err
	}
	return plan.Run(ctx, func() error {
		executableInfo, err := os.Stat(request.Executable)
		if err != nil {
			return operation.WrapFilesystem("revalidate_verification_git", request.Executable, err)
		}
		if !executableInfo.Mode().IsRegular() || !os.SameFile(request.ExecutableInfo, executableInfo) {
			return operation.New(operation.KindConflict, "git executable identity changed before verification")
		}
		validation := h.ValidatePath(request.RequestedRoot)
		if !validation.Ok() {
			return validation.Err
		}
		if validation.Path != request.RepositoryRoot {
			return operation.New(operation.KindConflict, "repositoryRoot changed before git verification")
		}
		current, err := os.Stat(validation.Path)
		if err != nil {
			return operation.WrapFilesystem("revalidate_verification_repository", validation.Path, err)
		}
		if !current.IsDir() || !os.SameFile(request.InitialInfo, current) {
			return operation.New(operation.KindConflict, "repositoryRoot identity changed before git verification")
		}
		return nil
	})
}

func verificationEnvironment() []string {
	allowed := []string{"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "TMPDIR"}
	environment := make([]string, 0, len(allowed)+10)
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok && value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	environment = append(environment,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_EXTERNAL_DIFF=",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_PAGER=cat",
		"PAGER=cat",
		"LANG=C",
		"LC_ALL=C",
	)
	sort.Strings(environment)
	return environment
}

func verificationMCPErrorResult(index int, checkType, path string, failure *mcp.CallToolResult) VerifyStateResult {
	code := ErrCodeOperationFailed
	message := "verification operation failed"
	if failure != nil {
		if value, ok := failure.Meta[ErrorCodeMetaKey].(string); ok && value != "" {
			code = value
		}
		if len(failure.Content) > 0 {
			if text, ok := failure.Content[0].(*mcp.TextContent); ok && text.Text != "" {
				message = text.Text
			}
		}
	}
	return VerifyStateResult{Index: index, Type: checkType, Path: path, ErrorCode: code, Error: boundedVerificationMessage(message), Message: boundedVerificationMessage(message)}
}

func verificationErrorResult(index int, checkType, path string, err error) VerifyStateResult {
	mapping := mapOperationError(err, path)
	message := boundedVerificationMessage(mapping.Message)
	return VerifyStateResult{Index: index, Type: checkType, Path: path, ErrorCode: mapping.BatchCode, Error: message, Message: message}
}

func boundedVerificationMessage(message string) string {
	const limit = 1024
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) <= limit {
		return message
	}
	end := limit
	for end > 0 && !utf8.ValidString(message[:end]) {
		end--
	}
	return message[:end]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "verification failed"
}
