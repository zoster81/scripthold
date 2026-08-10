package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/taskstore"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestV2PublicJSONTagsUseCamelCase(t *testing.T) {
	shapes := []any{
		ReadTextFileInput{}, ReadTextFileOutput{}, WriteWholeFileInput{}, WriteWholeFileOutput{},
		ListDirectoryInput{}, ListDirectoryOutput{}, ListEncodingsInput{}, ListEncodingsOutput{},
		DetectEncodingInput{}, DetectEncodingOutput{}, ListAllowedDirectoriesInput{}, ListAllowedDirectoriesOutput{},
		GetFileInfoInput{}, GetFileInfoOutput{}, CreateDirectoryInput{}, CreateDirectoryOutput{},
		MoveFileInput{}, MoveFileOutput{}, SearchFilesInput{}, SearchFilesOutput{},
		FingerprintPathsInput{}, FingerprintPathsOutput{}, FingerprintEntry{},
		EditFileInput{}, EditFileOutput{}, EditOperation{},
		PatchPackageInput{}, PatchPackageManifest{}, PatchPackageTarget{}, PatchPackageTargetResult{}, PatchPackageOutput{},
		VerifyStateInput{}, VerificationCheck{}, JSONVerificationCheck{}, TextVerificationCheck{}, GitDiffVerificationCheck{},
		FingerprintVerificationCheck{}, VerificationDiagnostic{}, VerifyStateResult{}, VerifyStateOutput{},
		ReadMultipleFilesInput{}, ReadMultipleFilesOutput{}, FileReadResult{},
		TreeInput{}, TreeOutput{}, DeleteFileInput{}, DeleteFileOutput{}, CopyFileInput{}, CopyFileOutput{},
		ConvertEncodingInput{}, ConvertEncodingOutput{}, GrepInput{}, GrepOutput{}, GrepMatch{},
		DetectLineEndingsInput{}, DetectLineEndingsOutput{}, ChangeLineEndingsInput{}, ChangeLineEndingsOutput{},
		ManageBomInput{}, ManageBomOutput{}, CheckUpdateInput{}, CheckUpdateOutput{},
		TaskRunInput{}, TaskListInput{}, TaskGetInput{}, TaskLogsInput{}, TaskCancelInput{},
		taskstore.SubmitResult{}, taskstore.Task{}, taskstore.TaskEvent{}, taskstore.Result{}, taskstore.ListResult{}, taskstore.LogsResult{}, taskstore.LogChunk{},
	}

	for _, shape := range shapes {
		typeOf := reflect.TypeOf(shape)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			if strings.Contains(name, "_") {
				t.Errorf("%s.%s uses non-camelCase JSON field %q", typeOf.Name(), field.Name, name)
			}
		}
	}

	encoded, err := json.Marshal(DetectEncodingOutput{HasBOM: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"hasBOM":true`) {
		t.Fatalf("DetectEncodingOutput JSON = %s, want hasBOM", encoded)
	}
	encoded, err = json.Marshal(ManageBomOutput{HasBOM: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"hasBOM":true`) {
		t.Fatalf("ManageBomOutput JSON = %s, want hasBOM", encoded)
	}
}

func TestV2ConfiguredRequestLimits(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "data.txt")
	if err := os.WriteFile(path, []byte("match\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits: config.Limits{
			MaxFileBytes:         config.DefaultMaxFileBytes,
			MaxDecodedCharacters: 4,
			MaxLineBytes:         config.DefaultMaxLineBytes,
			MaxBatchFiles:        1,
			MaxMatches:           1,
			MaxOutputBytes:       config.DefaultMaxOutputBytes,
		},
	}))

	maxCharacters := 5
	readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path: path, Encoding: "utf-8", MaxCharacters: &maxCharacters,
	})
	if err != nil || readResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("read limit result=%+v err=%v", readResult, err)
	}

	batchResult, _, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: []string{path, path}})
	if err != nil || batchResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("batch limit result=%+v err=%v", batchResult, err)
	}

	grepResult, _, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "match", Paths: []string{path}, MaxMatches: 2})
	if err != nil || grepResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("grep limit result=%+v err=%v", grepResult, err)
	}
}

func TestV2AmbiguousAndUTF32Policies(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	binaryPath := filepath.Join(tempDir, "binary.data")
	if err := os.WriteFile(binaryPath, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}, 0644); err != nil {
		t.Fatal(err)
	}
	result, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: binaryPath})
	if err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeEncodingAmbiguous {
		t.Fatalf("ambiguous read result=%+v err=%v", result, err)
	}

	utf32Path := filepath.Join(tempDir, "utf32.data")
	if err := os.WriteFile(utf32Path, []byte{0xff, 0xfe, 0x00, 0x00, 'A', 0, 0, 0}, 0644); err != nil {
		t.Fatal(err)
	}
	detectResult, detected, err := h.HandleDetectEncoding(context.Background(), nil, DetectEncodingInput{Path: utf32Path})
	if err != nil || detectResult.IsError || detected.Encoding != "utf-32-le" || detected.BOMType != "utf-32-le" {
		t.Fatalf("UTF-32 detect result=%+v output=%+v err=%v", detectResult, detected, err)
	}
	readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: utf32Path})
	if err != nil || readResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("UTF-32 read result=%+v err=%v", readResult, err)
	}
}

func TestV2SingleToolErrorsExposeStableCodeInMeta(t *testing.T) {
	result := errorResultFromError(operation.New(operation.KindLimit, "output limit exceeded"))
	if result == nil || !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if got, want := result.Meta[ErrorCodeMetaKey], ErrCodeLimit; got != want {
		t.Fatalf("_meta.%s = %#v, want %q", ErrorCodeMetaKey, got, want)
	}

	generic := errorResult("operation failed")
	if got, want := generic.Meta[ErrorCodeMetaKey], ErrCodeOperationFailed; got != want {
		t.Fatalf("generic _meta.%s = %#v, want %q", ErrorCodeMetaKey, got, want)
	}
}
