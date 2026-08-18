package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestMutationSurface_AllSupportedEncodingsPreserveContentAndExactPreparedBytes(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	ctx := context.Background()
	registry := fileEncoding.ListEncodings()
	if len(registry) != 168 {
		t.Fatalf("registered encodings=%d, want 168", len(registry))
	}

	for _, item := range registry {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			representative := representativeTextForEncoding(t, item.Name)
			content := "r23-token\n" + representative + "\nedit-old\nbeta-old\n"
			path := filepath.Join(root, item.Name+".r23")

			writeResult, _, err := h.HandleWriteWholeFile(ctx, nil, WriteWholeFileInput{
				Path: path, Content: content, Encoding: item.Name, BOM: "auto",
			})
			if err != nil || writeResult.IsError {
				t.Fatalf("write_whole_file result=%+v err=%v", writeResult, err)
			}
			assertMutationDecodedFile(t, h, ctx, item.Name, path, content)

			initialBytes := mustReadMutationBytes(t, path)
			noOpResult, noOpPreview, err := h.HandleEditFilePreview(ctx, nil, EditFilePreviewInput{
				Action: "preview", Path: path, Encoding: item.Name,
				Edits: []EditOperation{{OldText: "edit-old", NewText: "edit-old"}},
			})
			if err != nil || noOpResult.IsError || noOpPreview.Changed || noOpPreview.TargetFingerprint != noOpPreview.ResultFingerprint {
				t.Fatalf("edit no-op preview result=%+v output=%+v err=%v", noOpResult, noOpPreview, err)
			}
			assertMutationBytesEqual(t, path, initialBytes, "edit no-op preview")
			noOpApplyResult, noOpApplied, err := h.HandleEditFileApply(ctx, nil, PreviewApplyInput{PreviewID: noOpPreview.PreviewID})
			if err != nil || noOpApplyResult.IsError || noOpApplied.Applied || noOpApplied.Changed {
				t.Fatalf("edit no-op apply result=%+v output=%+v err=%v", noOpApplyResult, noOpApplied, err)
			}
			assertMutationBytesEqual(t, path, initialBytes, "edit no-op apply")

			editBefore := mustReadMutationBytes(t, path)
			editResult, editPreview, err := h.HandleEditFilePreview(ctx, nil, EditFilePreviewInput{
				Action: "preview", Path: path, Encoding: item.Name,
				Edits: []EditOperation{{OldText: "edit-old", NewText: "edit-new"}},
			})
			if err != nil || editResult.IsError || !editPreview.Changed || editPreview.Encoding != item.Name || len(editPreview.ResultFingerprint) != 64 {
				t.Fatalf("edit preview result=%+v output=%+v err=%v", editResult, editPreview, err)
			}
			assertMutationBytesEqual(t, path, editBefore, "edit preview")
			editApplyResult, editApplied, err := h.HandleEditFileApply(ctx, nil, PreviewApplyInput{PreviewID: editPreview.PreviewID})
			if err != nil || editApplyResult.IsError || !editApplied.Applied || !editApplied.Changed {
				t.Fatalf("edit apply result=%+v output=%+v err=%v", editApplyResult, editApplied, err)
			}
			assertMutationFingerprint(t, path, editPreview.ResultFingerprint, "edit apply")
			content = strings.Replace(content, "edit-old", "edit-new", 1)
			assertMutationDecodedFile(t, h, ctx, item.Name, path, content)

			manifest := PatchPackageManifest{
				FormatVersion:        PatchPackageFormatV1,
				FingerprintAlgorithm: "sha256",
				FingerprintMode:      "content-v1",
				Targets: []PatchPackageTarget{{
					Path:                path,
					ExpectedFingerprint: fingerprintRegularFileForTest(t, path),
					Encoding:            item.Name,
					Edits:               []EditOperation{{OldText: "beta-old", NewText: "beta-new"}},
				}},
			}
			patchBefore := mustReadMutationBytes(t, path)
			patchPreviewResult, patchPreview, err := h.HandlePatchPackageRead(ctx, nil, PatchPackageReadInput{Action: patchPackageActionDryRun, Manifest: manifest})
			if err != nil || patchPreviewResult.IsError || patchPreview.ChangedCount != 1 || len(patchPreview.Results) != 1 || patchPreview.Results[0].Encoding != item.Name {
				t.Fatalf("patch preview result=%+v output=%+v err=%v", patchPreviewResult, patchPreview, err)
			}
			assertMutationBytesEqual(t, path, patchBefore, "patch preview")
			patchApplyResult, patchApplied, err := h.HandlePatchPackageApply(ctx, nil, PreviewApplyInput{PreviewID: patchPreview.PreviewID})
			if err != nil || patchApplyResult.IsError || patchApplied.CommittedCount != 1 {
				t.Fatalf("patch apply result=%+v output=%+v err=%v", patchApplyResult, patchApplied, err)
			}
			assertMutationFingerprint(t, path, patchPreview.Results[0].ResultFingerprint, "patch apply")
			content = strings.Replace(content, "beta-old", "beta-new", 1)
			assertMutationDecodedFile(t, h, ctx, item.Name, path, content)

			lineResult, lineOutput, err := h.HandleChangeLineEndings(ctx, nil, ChangeLineEndingsInput{Path: path, Style: LineEndingCRLF, Encoding: item.Name})
			if err != nil || lineResult.IsError || lineOutput.NewStyle != LineEndingCRLF || lineOutput.LinesChanged != 4 {
				t.Fatalf("change_line_endings result=%+v output=%+v err=%v", lineResult, lineOutput, err)
			}
			content = ConvertLineEndings(content, LineEndingCRLF)
			assertMutationDecodedFile(t, h, ctx, item.Name, path, content)

			convertBefore := mustReadMutationBytes(t, path)
			convertPreviewResult, convertPreview, err := h.HandleConvertEncodingPreview(ctx, nil, ConvertEncodingPreviewInput{
				Path: path, From: item.Name, To: "utf-8", BOM: "auto", DryRun: true,
			})
			if err != nil || convertPreviewResult.IsError || len(convertPreview.PreviewID) != 64 || len(convertPreview.ResultFingerprint) != 64 {
				t.Fatalf("convert preview result=%+v output=%+v err=%v", convertPreviewResult, convertPreview, err)
			}
			assertMutationBytesEqual(t, path, convertBefore, "convert preview")
			convertApplyResult, converted, err := h.HandleConvertEncodingApply(ctx, nil, PreviewApplyInput{PreviewID: convertPreview.PreviewID})
			if err != nil || convertApplyResult.IsError || converted.Applied != convertPreview.Changed {
				t.Fatalf("convert apply result=%+v output=%+v err=%v", convertApplyResult, converted, err)
			}
			assertMutationFingerprint(t, path, convertPreview.ResultFingerprint, "convert apply")
			if actual := mustReadMutationBytes(t, path); !bytes.Equal(actual, []byte(content)) {
				t.Fatalf("UTF-8 conversion bytes differ: got %x want %x", actual, []byte(content))
			}

			backBefore := mustReadMutationBytes(t, path)
			backPreviewResult, backPreview, err := h.HandleConvertEncodingPreview(ctx, nil, ConvertEncodingPreviewInput{
				Path: path, From: "utf-8", To: item.Name, BOM: "auto", DryRun: true,
			})
			if err != nil || backPreviewResult.IsError || len(backPreview.PreviewID) != 64 || len(backPreview.ResultFingerprint) != 64 {
				t.Fatalf("convert-back preview result=%+v output=%+v err=%v", backPreviewResult, backPreview, err)
			}
			assertMutationBytesEqual(t, path, backBefore, "convert-back preview")
			backApplyResult, backApplied, err := h.HandleConvertEncodingApply(ctx, nil, PreviewApplyInput{PreviewID: backPreview.PreviewID})
			if err != nil || backApplyResult.IsError || backApplied.Applied != backPreview.Changed {
				t.Fatalf("convert-back apply result=%+v output=%+v err=%v", backApplyResult, backApplied, err)
			}
			assertMutationFingerprint(t, path, backPreview.ResultFingerprint, "convert-back apply")
			assertMutationDecodedFile(t, h, ctx, item.Name, path, content)

			if item.HasBOM {
				exerciseMutationBOMCapability(t, h, ctx, item.Name, path, content)
			}

			if unrepresentable, ok := encodingMatrixUnrepresentableRune(item.Name); ok {
				unsupportedPath := filepath.Join(root, item.Name+".r23.unsupported")
				original := []byte("r23-unrepresentable " + string(unrepresentable))
				if err := os.WriteFile(unsupportedPath, original, 0644); err != nil {
					t.Fatal(err)
				}
				result, output, err := h.HandleConvertEncodingPreview(ctx, nil, ConvertEncodingPreviewInput{
					Path: unsupportedPath, From: "utf-8", To: item.Name, DryRun: true,
				})
				if err != nil || !result.IsError || output.PreviewID != "" {
					t.Fatalf("unrepresentable preview result=%+v output=%+v err=%v", result, output, err)
				}
				assertMutationBytesEqual(t, unsupportedPath, original, "unrepresentable conversion preview")
			}
		})
	}
}

func exerciseMutationBOMCapability(t *testing.T, h *Handler, ctx context.Context, encodingName, path, content string) {
	t.Helper()
	detectResult, detected, err := h.HandleManageBOMRead(ctx, nil, ManageBOMReadInput{Path: path, Action: manageBOMActionDetect})
	if err != nil || detectResult.IsError {
		t.Fatalf("BOM detect result=%+v output=%+v err=%v", detectResult, detected, err)
	}

	if detected.HasBOM {
		stripMutationBOM(t, h, ctx, encodingName, path, content)
		addMutationBOM(t, h, ctx, encodingName, path, content)
		return
	}
	addMutationBOM(t, h, ctx, encodingName, path, content)
	stripMutationBOM(t, h, ctx, encodingName, path, content)
}

func addMutationBOM(t *testing.T, h *Handler, ctx context.Context, encodingName, path, content string) {
	t.Helper()
	before := mustReadMutationBytes(t, path)
	previewResult, preview, err := h.HandleManageBOMRead(ctx, nil, ManageBOMReadInput{Path: path, Action: manageBOMActionAddPreview, Encoding: encodingName})
	if err != nil || previewResult.IsError || !preview.Changed || !preview.HasBOM || len(preview.PreviewID) != 64 {
		t.Fatalf("BOM add preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	assertMutationBytesEqual(t, path, before, "BOM add preview")
	applyResult, applied, err := h.HandleManageBOMApply(ctx, nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied || !applied.HasBOM {
		t.Fatalf("BOM add apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	assertMutationFingerprint(t, path, preview.ResultFingerprint, "BOM add apply")
	assertMutationDecodedFile(t, h, ctx, encodingName, path, content)
}

func stripMutationBOM(t *testing.T, h *Handler, ctx context.Context, encodingName, path, content string) {
	t.Helper()
	before := mustReadMutationBytes(t, path)
	previewResult, preview, err := h.HandleManageBOMRead(ctx, nil, ManageBOMReadInput{Path: path, Action: manageBOMActionStripPreview})
	if err != nil || previewResult.IsError || !preview.Changed || preview.HasBOM || len(preview.PreviewID) != 64 {
		t.Fatalf("BOM strip preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	assertMutationBytesEqual(t, path, before, "BOM strip preview")
	applyResult, applied, err := h.HandleManageBOMApply(ctx, nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied || applied.HasBOM {
		t.Fatalf("BOM strip apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	assertMutationFingerprint(t, path, preview.ResultFingerprint, "BOM strip apply")
	assertMutationDecodedFile(t, h, ctx, encodingName, path, content)
}

func assertMutationDecodedFile(t *testing.T, h *Handler, ctx context.Context, encodingName, path, want string) {
	t.Helper()
	result, output, err := h.HandleReadTextFile(ctx, nil, ReadTextFileInput{Path: path, Encoding: encodingName})
	if err != nil || result.IsError || output.Content != want {
		t.Fatalf("read %s result=%+v output=%+v err=%v, want content %q", encodingName, result, output, err, want)
	}
}

func assertMutationFingerprint(t *testing.T, path, want, operation string) {
	t.Helper()
	if got := fingerprintRegularFileForTest(t, path); got != want {
		t.Fatalf("%s fingerprint=%s, want %s", operation, got, want)
	}
}

func mustReadMutationBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertMutationBytesEqual(t *testing.T, path string, want []byte, operation string) {
	t.Helper()
	if got := mustReadMutationBytes(t, path); !bytes.Equal(got, want) {
		t.Fatalf("%s mutated bytes: got %x want %x", operation, got, want)
	}
}
