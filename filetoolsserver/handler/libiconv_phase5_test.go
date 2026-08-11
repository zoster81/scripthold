package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestPhase5PinnedSingleByteFixturesRoundTripExplicitly(t *testing.T) {
	fixtures := []struct {
		file     string
		encoding string
	}{
		{file: "cp737.fixture", encoding: "ibm737"},
		{file: "georgian-academy.fixture", encoding: "georgian-academy"},
		{file: "georgian-ps.fixture", encoding: "georgian-ps"},
		{file: "mac-central-europe.fixture", encoding: "mac-central-europe"},
		{file: "viscii.fixture", encoding: "viscii"},
	}

	absoluteCorpus, err := filepath.Abs(filepath.Join("testdata", "internet-corpus"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{absoluteCorpus})

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.encoding, func(t *testing.T) {
			path := filepath.Join(absoluteCorpus, fixture.file)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
				Path: path, Encoding: fixture.encoding,
			})
			if err != nil || result.IsError {
				t.Fatalf("read result=%+v output=%+v err=%v", result, output, err)
			}
			registered, ok := fileEncoding.Get(fixture.encoding)
			if !ok || registered == nil {
				t.Fatalf("encoding %q is not registered", fixture.encoding)
			}
			roundTripped, err := registered.NewEncoder().Bytes([]byte(output.Content))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTripped, original) {
				t.Fatalf("fixture round-trip changed bytes: got %x want %x", roundTripped, original)
			}
		})
	}
}

func TestPhase5SingleByteConversionRoundTrip(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	path := filepath.Join(root, "greek.data")
	const content = "ΑΒΓ Ελληνικά\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "ibm737", BOM: "never",
	})
	if err != nil || result.IsError || output.TargetEncoding != "ibm737" {
		t.Fatalf("UTF-8 -> IBM737 result=%+v output=%+v err=%v", result, output, err)
	}
	result, readOutput, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path: path, Encoding: "ibm737",
	})
	if err != nil || result.IsError || readOutput.Content != content {
		t.Fatalf("IBM737 read result=%+v output=%+v err=%v", result, readOutput, err)
	}
	result, output, err = h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "ibm737", To: "utf-8", BOM: "never",
	})
	if err != nil || result.IsError || output.TargetEncoding != "utf-8" {
		t.Fatalf("IBM737 -> UTF-8 result=%+v output=%+v err=%v", result, output, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != content {
		t.Fatalf("conversion round-trip = %q, want %q", actual, content)
	}
}

func TestPhase5UndefinedSingleByteFailsClosedWithoutMutation(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	path := filepath.Join(root, "invalid.data")
	original := []byte{'a', '\n', 0x9B} // 0x9B is undefined in pinned CP856.
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path: path, Encoding: "ibm856",
	})
	if err != nil || !readResult.IsError || readResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("malformed read result=%+v err=%v", readResult, err)
	}
	changeResult, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path: path, Encoding: "ibm856", Style: LineEndingCRLF,
	})
	if err != nil || !changeResult.IsError || changeResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("malformed line-ending result=%+v err=%v", changeResult, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("malformed source was mutated: got %x want %x", actual, original)
	}
}
