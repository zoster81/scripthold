package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestR22Phase10ConcurrentBatchAndGrepRemainDeterministicAcrossEncodingClasses(t *testing.T) {
	for _, testCase := range phase10EncodingClasses {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			h := NewHandler([]string{root})
			const fileCount = 12
			paths := make([]string, 0, fileCount)
			wantByPath := make(map[string]string, fileCount)
			for index := 0; index < fileCount; index++ {
				path := filepath.Join(root, fmt.Sprintf("%02d.data", index))
				content := fmt.Sprintf("phase10-%02d %s\n", index, testCase.text)
				encoded, ok := phase9TryEncode(testCase.encoding, content, false)
				if !ok {
					t.Fatalf("encode %s concurrent fixture", testCase.encoding)
				}
				if err := os.WriteFile(path, encoded, 0o644); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
				wantByPath[path] = content
			}

			reversed := append([]string(nil), paths...)
			slices.Reverse(reversed)
			_, batch, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{
				Paths: reversed, Encoding: testCase.encoding,
			})
			if err != nil || batch.SuccessCount != fileCount || batch.ErrorCount != 0 || len(batch.Results) != fileCount {
				t.Fatalf("batch output=%+v err=%v", batch, err)
			}
			for index, result := range batch.Results {
				if result.Path != reversed[index] || result.Content != wantByPath[reversed[index]] {
					t.Fatalf("batch[%d]=%+v want path=%q content=%q", index, result, reversed[index], wantByPath[reversed[index]])
				}
			}

			grepResult, grep, err := h.HandleGrep(context.Background(), nil, GrepInput{
				Pattern: "phase10-", Paths: []string{root}, Encoding: testCase.encoding, MaxMatches: fileCount + 1,
			})
			if err != nil || grepResult.IsError || grep.TotalMatches != fileCount || len(grep.Matches) != fileCount ||
				grep.FilesSearched != fileCount || grep.FilesScanned != fileCount || grep.FilesSkipped != 0 || !grep.CoverageComplete {
				t.Fatalf("grep result=%+v output=%+v err=%v", grepResult, grep, err)
			}
			for index, match := range grep.Matches {
				if match.Path != paths[index] || match.Line != 1 || match.Encoding != testCase.encoding {
					t.Fatalf("grep[%d]=%+v want path=%q encoding=%q", index, match, paths[index], testCase.encoding)
				}
			}
		})
	}
}
