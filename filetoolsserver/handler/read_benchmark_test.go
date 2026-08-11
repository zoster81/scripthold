package handler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkR22Phase10ReadTextFileBoundedOutputAcrossEncodingClasses(b *testing.B) {
	for _, testCase := range phase10EncodingClasses {
		testCase := testCase
		encodedLine, ok := phase9TryEncode(testCase.encoding, testCase.text+"\n", false)
		if !ok {
			b.Fatalf("encode %s benchmark line", testCase.encoding)
		}
		for _, size := range []int{1 << 20, 16 << 20, 64 << 20} {
			b.Run(testCase.name+"/"+fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
				b.StopTimer()
				lineCount := max(1, size/len(encodedLine))
				data := bytes.Repeat(encodedLine, lineCount)
				dir := b.TempDir()
				path := filepath.Join(dir, "large.data")
				if err := os.WriteFile(path, data, 0o600); err != nil {
					b.Fatal(err)
				}
				handler := NewHandler([]string{dir})
				offset := lineCount
				limit := 1
				input := ReadTextFileInput{Path: path, Encoding: testCase.encoding, Offset: &offset, Limit: &limit}
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.StartTimer()

				for range b.N {
					result, output, err := handler.HandleReadTextFile(context.Background(), nil, input)
					if err != nil || result.IsError {
						b.Fatalf("read failed: result=%v err=%v", result, err)
					}
					if output.Content != testCase.text {
						b.Fatalf("unexpected output %q, want %q", output.Content, testCase.text)
					}
				}
			})
		}
	}
}

func BenchmarkHandleReadTextFileBoundedOutput(b *testing.B) {
	line := []byte("alpha beta gamma delta epsilon\n")
	for _, size := range []int{1 << 20, 16 << 20, 64 << 20} {
		b.Run(fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
			b.StopTimer()
			lineCount := size / len(line)
			data := bytes.Repeat(line, lineCount)
			dir := b.TempDir()
			path := filepath.Join(dir, "large.txt")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				b.Fatal(err)
			}
			handler := NewHandler([]string{dir})
			offset := lineCount
			limit := 1
			input := ReadTextFileInput{Path: path, Offset: &offset, Limit: &limit}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.StartTimer()

			for range b.N {
				result, output, err := handler.HandleReadTextFile(context.Background(), nil, input)
				if err != nil || result.IsError {
					b.Fatalf("read failed: result=%v err=%v", result, err)
				}
				if output.Content != "alpha beta gamma delta epsilon" {
					b.Fatalf("unexpected output %q", output.Content)
				}
			}
		})
	}
}
