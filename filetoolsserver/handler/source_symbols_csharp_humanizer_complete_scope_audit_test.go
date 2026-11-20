package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Compare the full selected original Humanizer C# files with an independently
// annotated inventory of namespaces, types, and class members. Constructor
// parameters, enum values, local functions, and expressions are not treated
// as independently indexed class members in this scope.
func TestCSharpHumanizerOriginalCompleteDeclarationInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original Humanizer source corpus not configured")
	}
	project := filepath.Join(root, "csharp-humanizer", "src", "Humanizer", "Bytes")
	type declaration struct {
		file               string
		line               int
		name, kind, parent string
	}
	independentlyInventoried := []declaration{
		{"ByteRate.cs", 3, "Humanizer", "namespace", ""},
		{"ByteRate.cs", 11, "ByteRate", "class", "Humanizer"},
		{"ByteRate.cs", 19, "Size", "property", "Humanizer.ByteRate"},
		{"ByteRate.cs", 24, "Interval", "property", "Humanizer.ByteRate"},
		{"ByteRate.cs", 173, "BytesPerSecond", "property", "Humanizer.ByteRate"},
		{"ByteRate.cs", 30, "Humanize", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 39, "Humanize", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 73, "HumanizeWithUnitSystem", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 95, "ScaleForExplicitUnitSystem", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 126, "ToString", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 133, "CompareTo", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 150, "Equals", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 157, "Equals", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 161, "GetHashCode", "method", "Humanizer.ByteRate"},
		{"ByteRate.cs", 165, "CompareTo", "method", "Humanizer.ByteRate"},
		{"ByteSizeExtensions.cs", 2, "Humanizer", "namespace", ""},
		{"ByteSizeExtensions.cs", 7, "ByteSizeExtensions", "class", "Humanizer"},
		{"ByteSizeExtensions.cs", 12, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 18, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 24, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 30, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 36, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 42, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 48, "Bits", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 54, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 60, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 66, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 72, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 78, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 84, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 90, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 96, "Bytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 102, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 108, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 114, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 120, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 126, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 132, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 138, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 144, "Kilobytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 150, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 156, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 162, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 168, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 174, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 180, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 186, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 192, "Megabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 198, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 204, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 210, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 216, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 222, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 228, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 234, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 240, "Gigabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 246, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 252, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 258, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 264, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 270, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 276, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 282, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 288, "Terabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 294, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 300, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 306, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 312, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 318, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 324, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 330, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 336, "Petabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 342, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 348, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 354, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 360, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 366, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 372, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 378, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 384, "Exabytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 390, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 396, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 402, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 408, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 414, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 420, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 426, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 432, "Pebibytes", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 439, "Humanize", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 446, "Humanize", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 454, "Humanize", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 474, "HumanizeWithUnitSystem", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 494, "HumanizeComposite", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 576, "HumanizeCompositeWithUnitSystem", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 709, "Per", "method", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 683, "DecimalCompositeUnits", "field", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 693, "BinaryCompositeUnits", "field", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeExtensions.cs", 702, "CompositeUnit", "record", "Humanizer.ByteSizeExtensions"},
		{"ByteSizeUnitSystem.cs", 1, "Humanizer", "namespace", ""},
		{"ByteSizeUnitSystem.cs", 12, "ByteSizeUnitSystem", "enum", "Humanizer"},
	}
	files := []string{"ByteRate.cs", "ByteSizeExtensions.cs", "ByteSizeUnitSystem.cs"}
	originals := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(project, file)
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes) > 1<<20 {
			t.Fatalf("original exceeds 1 MiB: %s", file)
		}
		originals[file] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]declaration{}
	for _, d := range independentlyInventoried {
		lines, ok := originals[d.file]
		if !ok || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("invalid independent original declaration %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, exists := expected[key]; exists {
			t.Fatalf("duplicated independently annotated declaration: %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "csharp", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 256,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("incomplete original-file outline: err=%v failed=%+v result=%+v", err, failed, out)
	}
	actual := map[string]bool{}
	for _, s := range out.Symbols {
		file := filepath.Base(s.Path)
		key := file + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if filepath.Clean(s.Path) != filepath.Join(project, file) {
			t.Errorf("unexpected source file: %+v", s)
		}
		if actual[key] {
			t.Errorf("extra same-line declaration or duplicate at %s: %s %s", key, s.Kind, s.Name)
		}
		actual[key] = true
		d, exists := expected[key]
		if !exists {
			t.Errorf("invented declaration at %s: %s %s", key, s.Kind, s.Name)
			continue
		}
		if s.Name != d.name || string(s.Kind) != d.kind || s.NativeKind != d.kind || s.ParentQualifiedName != d.parent {
			t.Errorf("wrong declaration at %s: got %s %s native=%q owner=%q; expected %+v",
				key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
		}
	}
	for key, d := range expected {
		if !actual[key] {
			t.Errorf("missing declaration at %s: %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(independentlyInventoried) {
		t.Errorf("expected %d independently inventoried declarations, returned %d", len(independentlyInventoried), len(out.Symbols))
	}
	t.Logf("full selected files=%d original declaration inventory=%d indexed=%d complete=%v",
		len(files), len(independentlyInventoried), len(out.Symbols), out.CoverageComplete)
}
