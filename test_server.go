//go:build ignore

// Manual test for all MCP server operations.
// Run with: go run test_server.go

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/encoding"
)

var failed = 0

func check(name string, ok bool) {
	fmt.Printf("%-40s ", name)
	if ok {
		fmt.Println("OK")
	} else {
		fmt.Println("FAIL")
		failed++
	}
}

func main() {
	requestedBaseDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(requestedBaseDir)
	baseDir, err := filepath.EvalSymlinks(requestedBaseDir)
	if err != nil {
		panic(err)
	}
	baseDir = filepath.Clean(baseDir)
	tempDir := filepath.Join(baseDir, "public")
	if err := os.Mkdir(tempDir, 0o700); err != nil {
		panic(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(baseDir, "backup-store"),
		PublicAllowedDirectories: []string{tempDir},
	})
	if err != nil {
		panic(err)
	}
	defer store.Close()

	h := handler.NewHandler([]string{tempDir}, handler.WithBackupStore(store))
	ctx := context.Background()

	fmt.Printf("Server version: %s\n\n", filetoolsserver.Version)

	// Basic tools
	r1, o1, _ := h.HandleListAllowedDirectories(ctx, nil, handler.ListAllowedDirectoriesInput{})
	check("list_allowed_directories", !r1.IsError && len(o1.Directories) == 1)

	r2, o2, _ := h.HandleListEncodings(ctx, nil, handler.ListEncodingsInput{})
	check("list_encodings", !r2.IsError && len(o2.Encodings) > 0)

	// Write/Read UTF-8
	testFile := filepath.Join(tempDir, "test.txt")
	r3, _, _ := h.HandleWriteWholeFile(ctx, nil, handler.WriteWholeFileInput{Path: testFile, Content: "Hello!", Encoding: "utf-8"})
	check("write_whole_file (UTF-8)", !r3.IsError)

	r4, o4, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: testFile})
	check("read_text_file (UTF-8)", !r4.IsError && o4.Content == "Hello!")

	// Write/Read CP1251
	cyrillicFile := filepath.Join(tempDir, "cyrillic.txt")
	r5, _, _ := h.HandleWriteWholeFile(ctx, nil, handler.WriteWholeFileInput{Path: cyrillicFile, Content: "Здравей!", Encoding: "cp1251"})
	check("write_whole_file (CP1251)", !r5.IsError)

	r6, o6, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: cyrillicFile, Encoding: "cp1251"})
	check("read_text_file (CP1251)", !r6.IsError && o6.Content == "Здравей!")

	// Detection and info
	r7, o7, _ := h.HandleDetectEncoding(ctx, nil, handler.DetectEncodingInput{Path: testFile})
	check("detect_encoding", !r7.IsError && o7.Encoding != "")

	r8, o8, _ := h.HandleGetFileInfo(ctx, nil, handler.GetFileInfoInput{Path: testFile})
	check("get_file_info", !r8.IsError && o8.IsFile && o8.Size > 0)

	// Directory operations
	r9, o9, _ := h.HandleListDirectory(ctx, nil, handler.ListDirectoryInput{Path: tempDir})
	check("list_directory", !r9.IsError && len(o9.Files) >= 2)

	os.Mkdir(filepath.Join(tempDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tempDir, "subdir", "nested.txt"), []byte("x"), 0644)

	r10, o10, _ := h.HandleTree(ctx, nil, handler.TreeInput{Path: tempDir})
	check("tree", !r10.IsError && o10.FileCount >= 2 && o10.DirCount >= 1)

	r11, o11, _ := h.HandleFingerprintPaths(ctx, nil, handler.FingerprintPathsInput{Paths: []string{tempDir}, IncludeEntries: true})
	check("fingerprint_paths", !r11.IsError && len(o11.Fingerprint) == 64 && o11.FileCount >= 3 && o11.DirectoryCount >= 2 && len(o11.Entries) > 0)

	rPackageFingerprint, oPackageFingerprint, _ := h.HandleFingerprintPaths(ctx, nil, handler.FingerprintPathsInput{Paths: []string{testFile}})
	rPackage, oPackage, _ := h.HandlePatchPackage(ctx, nil, handler.PatchPackageInput{
		Action: "dryRun",
		Manifest: handler.PatchPackageManifest{
			FormatVersion: handler.PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1", BackupPolicy: "required",
			Targets: []handler.PatchPackageTarget{{
				Path: testFile, ExpectedFingerprint: oPackageFingerprint.Fingerprint,
				Edits: []handler.EditOperation{{OldText: "Hello!", NewText: "Hello package!"}},
			}},
		},
	})
	packageData, _ := os.ReadFile(testFile)
	check("patch_package (required dryRun)", !rPackageFingerprint.IsError && !rPackage.IsError && len(oPackage.PreviewID) == 64 && oPackage.BackupPolicy == "required" && oPackage.BackupCount == 0 && oPackage.TargetCount == 1 && oPackage.ChangedCount == 1 && store.Index().ManifestCount == 0 && string(packageData) == "Hello!")
	rPackageApply, oPackageApply, _ := h.HandlePatchPackage(ctx, nil, handler.PatchPackageInput{Action: "apply", PreviewID: oPackage.PreviewID})
	packageAppliedData, _ := os.ReadFile(testFile)
	check("patch_package (required apply)", !rPackageApply.IsError && oPackageApply.Applied && oPackageApply.BackupCount == 1 && len(oPackageApply.Results[0].BackupID) == 64 && store.Index().ManifestCount == 1 && string(packageAppliedData) == "Hello package!")
	packageManifest := handler.PatchPackageManifest{
		FormatVersion: handler.PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []handler.PatchPackageTarget{{
			Path: testFile, ExpectedFingerprint: oPackageFingerprint.Fingerprint,
			ExpectedResultFingerprint: oPackage.Results[0].ResultFingerprint,
			Edits:                     []handler.EditOperation{{OldText: "Hello!", NewText: "Hello package!"}},
		}},
	}
	rPackageVerify, oPackageVerify, _ := h.HandlePatchPackage(ctx, nil, handler.PatchPackageInput{Action: "verify", Manifest: packageManifest})
	check("patch_package (verify)", !rPackageVerify.IsError && oPackageVerify.Verified)

	verifyJSON := filepath.Join(tempDir, "verify.json")
	os.WriteFile(verifyJSON, []byte("{\"ok\":true}\n"), 0644)
	rVerifyState, oVerifyState, _ := h.HandleVerifyState(ctx, nil, handler.VerifyStateInput{Checks: []handler.VerificationCheck{
		{Type: handler.VerifyCheckJSON, JSON: &handler.JSONVerificationCheck{Path: verifyJSON, Encoding: "utf-8"}},
		{Type: handler.VerifyCheckText, Text: &handler.TextVerificationCheck{Path: testFile, Encoding: "utf-8", BOM: "none", LineEndings: "none", TrailingWhitespace: "none"}},
		{Type: handler.VerifyCheckFingerprint, Fingerprint: &handler.FingerprintVerificationCheck{Paths: []string{testFile}, ExpectedFingerprint: oPackage.Results[0].ResultFingerprint}},
	}})
	check("verify_state", !rVerifyState.IsError && oVerifyState.Passed && oVerifyState.PassedCount == 3)

	rBackupStatus, oBackupStatus, _ := h.HandleBackupStore(ctx, nil, handler.BackupStoreInput{Action: handler.BackupStoreActionStatus})
	check("backup_store (enabled status)", !rBackupStatus.IsError && oBackupStatus.Enabled && oBackupStatus.State == handler.BackupStoreStateReady)

	rPreview, oPreview, _ := h.HandleEditFile(ctx, nil, handler.EditFileInput{
		Action: "preview", Path: testFile, Edits: []handler.EditOperation{{OldText: "Hello package!", NewText: "Hello preview!"}}, BackupPolicy: "required",
	})
	previewData, _ := os.ReadFile(testFile)
	check("edit_file (required preview)", !rPreview.IsError && len(oPreview.PreviewID) == 64 && oPreview.BackupPolicy == "required" && string(previewData) == "Hello package!")
	rApply, oApply, _ := h.HandleEditFile(ctx, nil, handler.EditFileInput{Action: "apply", PreviewID: oPreview.PreviewID})
	appliedData, _ := os.ReadFile(testFile)
	check("edit_file (required apply)", !rApply.IsError && oApply.Applied && len(oApply.BackupID) == 64 && store.Index().ManifestCount == 2 && string(appliedData) == "Hello preview!")

	rRestorePreview, oRestorePreview, _ := h.HandleBackupStore(ctx, nil, handler.BackupStoreInput{
		Action: handler.BackupStoreActionRestorePreview, BackupID: oPackageApply.Results[0].BackupID,
	})
	check("backup_store (restore preview)", !rRestorePreview.IsError && oRestorePreview.Restore != nil && len(oRestorePreview.Restore.PreviewID) == 64 && !oRestorePreview.Restore.Applied && store.Index().ManifestCount == 2)
	rRestoreApply, oRestoreApply, _ := h.HandleBackupStore(ctx, nil, handler.BackupStoreInput{
		Action: handler.BackupStoreActionRestoreApply, PreviewID: oRestorePreview.Restore.PreviewID,
	})
	restoredData, _ := os.ReadFile(testFile)
	check("backup_store (restore apply)", !rRestoreApply.IsError && oRestoreApply.Restore != nil && oRestoreApply.Restore.Applied && len(oRestoreApply.Restore.SafetyBackupID) == 64 && store.Index().ManifestCount == 3 && string(restoredData) == "Hello!")

	rGCDryRun, oGCDryRun, _ := h.HandleBackupStore(ctx, nil, handler.BackupStoreInput{Action: handler.BackupStoreActionGCDryRun})
	check("backup_store (GC dry run)", !rGCDryRun.IsError && oGCDryRun.GC != nil && len(oGCDryRun.GC.PreviewID) == 64 && oGCDryRun.GC.State == handler.BackupStoreGCStatePrepared)
	rGCApply, oGCApply, _ := h.HandleBackupStore(ctx, nil, handler.BackupStoreInput{Action: handler.BackupStoreActionGCApply, PreviewID: oGCDryRun.GC.PreviewID})
	check("backup_store (GC apply)", !rGCApply.IsError && oGCApply.GC != nil && (oGCApply.GC.State == handler.BackupStoreGCStateApplied || oGCApply.GC.State == handler.BackupStoreGCStateNoop))

	// Offset/Limit pagination
	multiFile := filepath.Join(tempDir, "multi.txt")
	os.WriteFile(multiFile, []byte("a\nb\nc\nd"), 0644)
	limit, offset := 2, 3
	r12, o12, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: multiFile, Limit: &limit})
	check("read_text_file (limit=2)", !r12.IsError && o12.Content == "a\nb")

	r13, o13, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: multiFile, Offset: &offset, Limit: &limit})
	check("read_text_file (offset=3, limit=2)", !r13.IsError && o13.Content == "c\nd")

	// Auto-detect uses an unambiguous UTF-8 fixture. CP1251 is intentionally
	// exercised explicitly above because phase-7 conservative detection rejects
	// plausible single-byte confusion instead of forcing a legacy guess.
	r14, o14, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: testFile})
	check("read_text_file (auto-detect)", !r14.IsError && o14.DetectedEncoding == "utf-8" && o14.Content == "Hello!")

	enc, ok := encoding.Get("cp1251")
	check("encoding registry", ok && enc != nil)

	// Security: path validation
	r16, _, _ := h.HandleReadTextFile(ctx, nil, handler.ReadTextFileInput{Path: filepath.Join(tempDir, "..", "..", "etc", "passwd")})
	check("path validation (deny)", r16.IsError)

	fmt.Println()
	if failed > 0 {
		fmt.Printf("FAILED: %d test(s)\n", failed)
		os.Exit(1)
	}
	fmt.Println("All tests passed!")
}
