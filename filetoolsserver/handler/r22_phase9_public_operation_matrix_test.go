package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestR22Phase9PublicOperationMatrix_AllSupportedEncodings(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	ctx := context.Background()

	listResult, listed, err := h.HandleListEncodings(ctx, nil, ListEncodingsInput{})
	if err != nil || listResult.IsError {
		t.Fatalf("list_encodings result=%+v err=%v", listResult, err)
	}
	registry := fileEncoding.ListEncodings()
	if len(listed.Encodings) != len(registry) || len(registry) != 168 {
		t.Fatalf("list_encodings count=%d registry=%d, want 168", len(listed.Encodings), len(registry))
	}
	for index := range registry {
		if listed.Encodings[index].Name != registry[index].Name ||
			listed.Encodings[index].Readable != registry[index].Readable ||
			listed.Encodings[index].Writable != registry[index].Writable ||
			listed.Encodings[index].AutoDetectable != registry[index].AutoDetectable ||
			listed.Encodings[index].ExplicitOnly != registry[index].ExplicitOnly {
			t.Fatalf("list_encodings[%d]=%+v registry=%+v", index, listed.Encodings[index], registry[index])
		}
	}

	for _, item := range registry {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			representative := representativeTextForEncoding(t, item.Name)
			content := "phase9-token\n" + representative + "\nedit-old\nbeta-old\n"
			path := filepath.Join(root, item.Name+".phase9")

			writeResult, writeOutput, err := h.HandleWriteWholeFile(ctx, nil, WriteWholeFileInput{
				Path: path, Content: content, Encoding: item.Name, BOM: "auto",
			})
			if err != nil || writeResult.IsError {
				t.Fatalf("write_whole_file result=%+v output=%+v err=%v", writeResult, writeOutput, err)
			}
			if writeOutput.Encoding != item.Name {
				t.Fatalf("write encoding=%q, want %q", writeOutput.Encoding, item.Name)
			}
			assertPhase9DecodedFile(t, item.Name, path, content)

			readResult, readOutput, err := h.HandleReadTextFile(ctx, nil, ReadTextFileInput{Path: path, Encoding: item.Name})
			if err != nil || readResult.IsError || readOutput.Content != content {
				t.Fatalf("read_text_file result=%+v output=%+v err=%v", readResult, readOutput, err)
			}
			if readOutput.DetectedEncoding != "" || readOutput.EncodingConfidence != 0 {
				t.Fatalf("explicit read unexpectedly reported detection metadata: %+v", readOutput)
			}

			batchResult, batchOutput, err := h.HandleReadMultipleFiles(ctx, nil, ReadMultipleFilesInput{Paths: []string{path}, Encoding: item.Name})
			if err != nil || batchResult.IsError || batchOutput.SuccessCount != 1 || batchOutput.ErrorCount != 0 ||
				len(batchOutput.Results) != 1 || batchOutput.Results[0].Content != content {
				t.Fatalf("read_multiple_files result=%+v output=%+v err=%v", batchResult, batchOutput, err)
			}

			grepResult, grepOutput, err := h.HandleGrep(ctx, nil, GrepInput{
				Pattern: "phase9-token", Paths: []string{path}, Encoding: item.Name, MaxMatches: 10,
			})
			if err != nil || grepResult.IsError || len(grepOutput.Matches) != 1 || grepOutput.TotalMatches != 1 ||
				grepOutput.FilesSearched != 1 || grepOutput.FilesScanned != 1 || grepOutput.FilesSkipped != 0 || !grepOutput.CoverageComplete ||
				grepOutput.Matches[0].Encoding != item.Name {
				t.Fatalf("grep_text_files result=%+v output=%+v err=%v", grepResult, grepOutput, err)
			}

			lineResult, lineOutput, err := h.HandleDetectLineEndings(ctx, nil, DetectLineEndingsInput{Path: path, Encoding: item.Name})
			if err != nil || lineResult.IsError || lineOutput.Style != LineEndingLF {
				t.Fatalf("detect_line_endings result=%+v output=%+v err=%v", lineResult, lineOutput, err)
			}

			beforeNoOp, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			noOpPreviewResult, noOpPreview, err := h.HandleEditFile(ctx, nil, EditFileInput{
				Action: "preview", Path: path, Encoding: item.Name,
				Edits: []EditOperation{{OldText: "edit-old", NewText: "edit-old"}},
			})
			if err != nil || noOpPreviewResult.IsError || noOpPreview.Changed || noOpPreview.TargetFingerprint != noOpPreview.ResultFingerprint {
				t.Fatalf("edit no-op preview result=%+v output=%+v err=%v", noOpPreviewResult, noOpPreview, err)
			}
			noOpApplyResult, noOpApplied, err := h.HandleEditFile(ctx, nil, EditFileInput{Action: "apply", PreviewID: noOpPreview.PreviewID})
			if err != nil || noOpApplyResult.IsError || noOpApplied.Applied || noOpApplied.Changed {
				t.Fatalf("edit no-op apply result=%+v output=%+v err=%v", noOpApplyResult, noOpApplied, err)
			}
			afterNoOp, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(afterNoOp) != string(beforeNoOp) {
				t.Fatalf("edit no-op changed %s bytes", item.Name)
			}

			previewResult, preview, err := h.HandleEditFile(ctx, nil, EditFileInput{
				Action: "preview", Path: path, Encoding: item.Name,
				Edits: []EditOperation{{OldText: "edit-old", NewText: "edit-new"}},
			})
			if err != nil || previewResult.IsError || !preview.Changed || preview.Encoding != item.Name {
				t.Fatalf("edit preview result=%+v output=%+v err=%v", previewResult, preview, err)
			}
			assertPhase9DecodedFile(t, item.Name, path, content)
			applyResult, applied, err := h.HandleEditFile(ctx, nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !applied.Applied || !applied.Changed {
				t.Fatalf("edit apply result=%+v output=%+v err=%v", applyResult, applied, err)
			}
			content = strings.Replace(content, "edit-old", "edit-new", 1)
			assertPhase9DecodedFile(t, item.Name, path, content)

			manifest := PatchPackageManifest{
				FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
				Targets: []PatchPackageTarget{{
					Path: path, ExpectedFingerprint: fingerprintRegularFileForTest(t, path), Encoding: item.Name,
					Edits: []EditOperation{{OldText: "beta-old", NewText: "beta-new"}},
				}},
			}
			patchDryResult, patchDry, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
			if err != nil || patchDryResult.IsError || patchDry.ChangedCount != 1 || len(patchDry.Results) != 1 || patchDry.Results[0].Encoding != item.Name {
				t.Fatalf("patch dryRun result=%+v output=%+v err=%v", patchDryResult, patchDry, err)
			}
			patchApplyResult, patchApplied, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: patchDry.PreviewID})
			if err != nil || patchApplyResult.IsError || patchApplied.CommittedCount != 1 {
				t.Fatalf("patch apply result=%+v output=%+v err=%v", patchApplyResult, patchApplied, err)
			}
			content = strings.Replace(content, "beta-old", "beta-new", 1)
			assertPhase9DecodedFile(t, item.Name, path, content)
			verifiedManifest := manifest
			verifiedManifest.Targets = append([]PatchPackageTarget(nil), manifest.Targets...)
			verifiedManifest.Targets[0].ExpectedResultFingerprint = patchDry.Results[0].ResultFingerprint
			patchVerifyResult, patchVerified, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionVerify, Manifest: verifiedManifest})
			if err != nil || patchVerifyResult.IsError || !patchVerified.Verified || patchVerified.MismatchCount != 0 {
				t.Fatalf("patch verify result=%+v output=%+v err=%v", patchVerifyResult, patchVerified, err)
			}

			changeResult, changeOutput, err := h.HandleChangeLineEndings(ctx, nil, ChangeLineEndingsInput{Path: path, Style: LineEndingCRLF, Encoding: item.Name})
			if err != nil || changeResult.IsError || changeOutput.NewStyle != LineEndingCRLF || changeOutput.LinesChanged != 4 {
				t.Fatalf("change_line_endings result=%+v output=%+v err=%v", changeResult, changeOutput, err)
			}
			content = ConvertLineEndings(content, LineEndingCRLF)
			assertPhase9DecodedFile(t, item.Name, path, content)

			checks := []VerificationCheck{
				{Type: VerifyCheckText, Text: &TextVerificationCheck{Path: path, Encoding: item.Name, LineEndings: LineEndingCRLF, TrailingWhitespace: "none"}},
			}
			jsonPath := filepath.Join(root, item.Name+".phase9.json")
			if jsonBytes, ok := phase9TryEncode(item.Name, "{\"phase9\":true}\r\n", item.HasBOM); ok {
				if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
					t.Fatal(err)
				}
				checks = append(checks, VerificationCheck{Type: VerifyCheckJSON, JSON: &JSONVerificationCheck{Path: jsonPath, Encoding: item.Name}})
			}
			verifyResult, verified, err := h.HandleVerifyState(ctx, nil, VerifyStateInput{Checks: checks})
			if err != nil || verifyResult.IsError || !verified.Passed || verified.PassedCount != len(checks) || verified.ErrorCount != 0 {
				t.Fatalf("verify_state result=%+v output=%+v err=%v", verifyResult, verified, err)
			}

			convertDryResult, convertDry, err := h.HandleConvertEncoding(ctx, nil, ConvertEncodingInput{
				Path: path, From: item.Name, To: "utf-8", DryRun: true,
			})
			if err != nil || convertDryResult.IsError || convertDry.TargetEncoding != "utf-8" {
				t.Fatalf("convert dryRun result=%+v output=%+v err=%v", convertDryResult, convertDry, err)
			}
			convertResult, converted, err := h.HandleConvertEncoding(ctx, nil, ConvertEncodingInput{Path: path, From: item.Name, To: "utf-8"})
			if err != nil || convertResult.IsError || converted.TargetEncoding != "utf-8" {
				t.Fatalf("convert to UTF-8 result=%+v output=%+v err=%v", convertResult, converted, err)
			}
			utf8Bytes, err := os.ReadFile(path)
			if err != nil || string(utf8Bytes) != content {
				t.Fatalf("UTF-8 conversion bytes=%q err=%v, want %q", utf8Bytes, err, content)
			}
			backResult, back, err := h.HandleConvertEncoding(ctx, nil, ConvertEncodingInput{Path: path, From: "utf-8", To: item.Name})
			if err != nil || backResult.IsError || back.TargetEncoding != item.Name {
				t.Fatalf("convert back result=%+v output=%+v err=%v", backResult, back, err)
			}
			assertPhase9DecodedFile(t, item.Name, path, content)

			if unrepresentable, ok := phase9UnrepresentableRune(item.Name); ok {
				unsupportedPath := filepath.Join(root, item.Name+".phase9.unsupported")
				original := []byte("phase9 " + string(unrepresentable))
				if err := os.WriteFile(unsupportedPath, original, 0644); err != nil {
					t.Fatal(err)
				}
				batchConvertResult, batchConvert, err := h.HandleConvertEncoding(ctx, nil, ConvertEncodingInput{
					Paths: []string{unsupportedPath}, From: "utf-8", To: item.Name, DryRun: true,
				})
				if err != nil || batchConvertResult.IsError || batchConvert.ErrorCount != 1 || len(batchConvert.Results) != 1 ||
					batchConvert.Results[0].UnsupportedCount == 0 || batchConvert.Results[0].ErrorCode != ErrCodeEncoding ||
					batchConvert.Results[0].EncodingErrorCode != EncodingErrorUnrepresentable {
					t.Fatalf("unsupported conversion result=%+v output=%+v err=%v", batchConvertResult, batchConvert, err)
				}
				if actual, err := os.ReadFile(unsupportedPath); err != nil || string(actual) != string(original) {
					t.Fatalf("unsupported dryRun mutated bytes=%q err=%v", actual, err)
				}
			}

			if item.HasBOM {
				phase9ExerciseBOMOperations(t, h, path, item.Name, content)
				phase9ExerciseBOMDetection(t, h, root, item.Name, content)
			}
		})
	}
}

func assertPhase9DecodedFile(t *testing.T, encodingName, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeLineEndingFixture(t, encodingName, data); got != want {
		t.Fatalf("decoded %s content=%q, want %q", encodingName, got, want)
	}
}

func phase9UnrepresentableRune(encodingName string) (rune, bool) {
	if fileEncoding.IsUTF8(encodingName) {
		return 0, false
	}
	registered, ok := fileEncoding.Get(encodingName)
	if !ok || registered == nil {
		return 0, false
	}
	for _, candidate := range []rune{'🌍', '💩', '\U0010FFFF', '漢', 'Ж', '€', '\uF8FF'} {
		if _, err := registered.NewEncoder().String(string(candidate)); err != nil {
			return candidate, true
		}
	}
	return 0, false
}

func phase9ExerciseBOMOperations(t *testing.T, h *Handler, path, encodingName, content string) {
	t.Helper()
	ctx := context.Background()

	_, detected, err := h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "detect"})
	if err != nil {
		t.Fatal(err)
	}
	if detected.HasBOM {
		stripResult, stripped, err := h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "strip"})
		if err != nil || stripResult.IsError || !stripped.Changed {
			t.Fatalf("initial BOM strip result=%+v output=%+v err=%v", stripResult, stripped, err)
		}
	}
	addResult, added, err := h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "add", Encoding: encodingName})
	if err != nil || addResult.IsError || !added.Changed || !added.HasBOM || added.BOMType != encodingName {
		t.Fatalf("BOM add result=%+v output=%+v err=%v", addResult, added, err)
	}
	_, detected, err = h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "detect"})
	if err != nil || !detected.HasBOM || detected.BOMType != encodingName {
		t.Fatalf("BOM detect output=%+v err=%v", detected, err)
	}
	stripResult, stripped, err := h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "strip"})
	if err != nil || stripResult.IsError || !stripped.Changed || stripped.HasBOM {
		t.Fatalf("final BOM strip result=%+v output=%+v err=%v", stripResult, stripped, err)
	}
	assertPhase9DecodedFile(t, encodingName, path, content)
}

func phase9ExerciseBOMDetection(t *testing.T, h *Handler, root, encodingName, content string) {
	t.Helper()
	path := filepath.Join(root, encodingName+".phase9.bom-detect")
	encoded, ok := phase9TryEncode(encodingName, content, true)
	if !ok {
		t.Fatalf("BOM-capable encoding %q could not encode its Phase 9 detection fixture", encodingName)
	}
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandleDetectEncoding(context.Background(), nil, DetectEncodingInput{Path: path, Mode: "chunked"})
	if err != nil || result.IsError || output.Ambiguous || output.Encoding != encodingName || !output.HasBOM || output.BOMType != encodingName {
		t.Fatalf("BOM detect_encoding result=%+v output=%+v err=%v", result, output, err)
	}
}

func TestR22Phase9DetectEncoding_TrustedCorpusThroughPublicHandler(t *testing.T) {
	manifestBytes, err := os.ReadFile(filepath.Join(internetCorpusDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest internetCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	h := NewHandler([]string{root})
	trusted := 0
	for _, fixture := range manifest.Fixtures {
		if fixture.Role != "detection" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(internetCorpusDir, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		expected := fileEncoding.Detect(data)
		if expected.Charset == "" || expected.Confidence < fileEncoding.MinConfidenceThreshold {
			continue
		}
		trusted++
		path := filepath.Join(root, fixture.File)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		result, output, err := h.HandleDetectEncoding(context.Background(), nil, DetectEncodingInput{Path: path, Mode: "chunked"})
		if err != nil || result.IsError || output.Ambiguous || output.Encoding != expected.Charset {
			t.Fatalf("fixture %s public detection result=%+v output=%+v expected=%+v err=%v", fixture.File, result, output, expected, err)
		}
	}
	if trusted != 30 {
		t.Fatalf("trusted detection fixtures=%d, want 30", trusted)
	}
}

func phase9TryEncode(encodingName, content string, withBOM bool) ([]byte, bool) {
	var data []byte
	if fileEncoding.IsUTF8(encodingName) {
		data = []byte(content)
	} else {
		registered, ok := fileEncoding.Get(encodingName)
		if !ok || registered == nil {
			return nil, false
		}
		encoded, err := registered.NewEncoder().Bytes([]byte(content))
		if err != nil {
			return nil, false
		}
		data = encoded
	}
	if withBOM {
		bom := fileEncoding.BOMBytesFor(encodingName)
		if len(bom) == 0 {
			return nil, false
		}
		data = append(append([]byte(nil), bom...), data...)
	}
	return data, true
}
