package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Independently inventoried declarations from two complete original attrs files:
// definitions, nested/conditional functions, module exports and class fields.
// A field assignment is only a source declaration; this audit does not certify
// attribute decorators' generated runtime behavior or their value semantics.
func TestPythonAttrsOriginalCompleteDeclarationInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original attrs source corpus not configured")
	}
	project := filepath.Join(root, "python-attrs", "src", "attr")
	type declaration struct {
		file               string
		line               int
		name, kind, parent string
	}
	independentlyInventoried := []declaration{
		{"validators.py", 43, "set_disabled", "function", ""},
		{"validators.py", 61, "get_disabled", "function", ""},
		{"validators.py", 74, "disabled", "function", ""},
		{"validators.py", 114, "instance_of", "function", ""},
		{"validators.py", 153, "matches_re", "function", ""},
		{"validators.py", 215, "optional", "function", ""},
		{"validators.py", 260, "in_", "function", ""},
		{"validators.py", 317, "is_callable", "function", ""},
		{"validators.py", 362, "deep_iterable", "function", ""},
		{"validators.py", 411, "deep_mapping", "function", ""},
		{"validators.py", 479, "lt", "function", ""},
		{"validators.py", 494, "le", "function", ""},
		{"validators.py", 509, "ge", "function", ""},
		{"validators.py", 524, "gt", "function", ""},
		{"validators.py", 539, "ne", "function", ""},
		{"validators.py", 570, "max_len", "function", ""},
		{"validators.py", 599, "min_len", "function", ""},
		{"validators.py", 633, "_subclass_of", "function", ""},
		{"validators.py", 687, "not_", "function", ""},
		{"validators.py", 743, "or_", "function", ""},
		{"validators.py", 94, "_InstanceOfValidator", "class", ""},
		{"validators.py", 97, "__call__", "method", "_InstanceOfValidator"},
		{"validators.py", 110, "__repr__", "method", "_InstanceOfValidator"},
		{"validators.py", 132, "_MatchesReValidator", "class", ""},
		{"validators.py", 136, "__call__", "method", "_MatchesReValidator"},
		{"validators.py", 149, "__repr__", "method", "_MatchesReValidator"},
		{"validators.py", 202, "_OptionalValidator", "class", ""},
		{"validators.py", 205, "__call__", "method", "_OptionalValidator"},
		{"validators.py", 211, "__repr__", "method", "_OptionalValidator"},
		{"validators.py", 237, "_InValidator", "class", ""},
		{"validators.py", 241, "__call__", "method", "_InValidator"},
		{"validators.py", 256, "__repr__", "method", "_InValidator"},
		{"validators.py", 296, "_IsCallableValidator", "class", ""},
		{"validators.py", 297, "__call__", "method", "_IsCallableValidator"},
		{"validators.py", 313, "__repr__", "method", "_IsCallableValidator"},
		{"validators.py", 334, "_DeepIterable", "class", ""},
		{"validators.py", 340, "__call__", "method", "_DeepIterable"},
		{"validators.py", 350, "__repr__", "method", "_DeepIterable"},
		{"validators.py", 389, "_DeepMapping", "class", ""},
		{"validators.py", 394, "__call__", "method", "_DeepMapping"},
		{"validators.py", 407, "__repr__", "method", "_DeepMapping"},
		{"validators.py", 462, "_NumberValidator", "class", ""},
		{"validators.py", 467, "__call__", "method", "_NumberValidator"},
		{"validators.py", 475, "__repr__", "method", "_NumberValidator"},
		{"validators.py", 555, "_MaxLengthValidator", "class", ""},
		{"validators.py", 558, "__call__", "method", "_MaxLengthValidator"},
		{"validators.py", 566, "__repr__", "method", "_MaxLengthValidator"},
		{"validators.py", 584, "_MinLengthValidator", "class", ""},
		{"validators.py", 587, "__call__", "method", "_MinLengthValidator"},
		{"validators.py", 595, "__repr__", "method", "_MinLengthValidator"},
		{"validators.py", 613, "_SubclassOfValidator", "class", ""},
		{"validators.py", 616, "__call__", "method", "_SubclassOfValidator"},
		{"validators.py", 629, "__repr__", "method", "_SubclassOfValidator"},
		{"validators.py", 651, "_NotValidator", "class", ""},
		{"validators.py", 666, "__call__", "method", "_NotValidator"},
		{"validators.py", 683, "__repr__", "method", "_NotValidator"},
		{"validators.py", 724, "_OrValidator", "class", ""},
		{"validators.py", 727, "__call__", "method", "_OrValidator"},
		{"validators.py", 739, "__repr__", "method", "_OrValidator"},
		{"converters.py", 19, "optional", "function", ""},
		{"converters.py", 36, "optional_converter", "function", "optional"},
		{"converters.py", 43, "optional_converter", "function", "optional"},
		{"converters.py", 64, "default_if_none", "function", ""},
		{"converters.py", 106, "default_if_none_converter", "function", "default_if_none"},
		{"converters.py", 114, "default_if_none_converter", "function", "default_if_none"},
		{"converters.py", 123, "to_bool", "function", ""},
		{"validators.py", 19, "__all__", "variable", ""},
		{"converters.py", 11, "__all__", "variable", ""},
		{"validators.py", 95, "type", "field", "_InstanceOfValidator"},
		{"validators.py", 133, "pattern", "field", "_MatchesReValidator"},
		{"validators.py", 134, "match_func", "field", "_MatchesReValidator"},
		{"validators.py", 203, "validator", "field", "_OptionalValidator"},
		{"validators.py", 238, "options", "field", "_InValidator"},
		{"validators.py", 239, "_original_options", "field", "_InValidator"},
		{"validators.py", 335, "member_validator", "field", "_DeepIterable"},
		{"validators.py", 336, "iterable_validator", "field", "_DeepIterable"},
		{"validators.py", 390, "key_validator", "field", "_DeepMapping"},
		{"validators.py", 391, "value_validator", "field", "_DeepMapping"},
		{"validators.py", 392, "mapping_validator", "field", "_DeepMapping"},
		{"validators.py", 463, "bound", "field", "_NumberValidator"},
		{"validators.py", 464, "compare_op", "field", "_NumberValidator"},
		{"validators.py", 465, "compare_func", "field", "_NumberValidator"},
		{"validators.py", 556, "max_length", "field", "_MaxLengthValidator"},
		{"validators.py", 585, "min_length", "field", "_MinLengthValidator"},
		{"validators.py", 614, "type", "field", "_SubclassOfValidator"},
		{"validators.py", 652, "validator", "field", "_NotValidator"},
		{"validators.py", 653, "msg", "field", "_NotValidator"},
		{"validators.py", 659, "exc_types", "field", "_NotValidator"},
		{"validators.py", 725, "validators", "field", "_OrValidator"},
	}
	files := []string{"validators.py", "converters.py"}
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
			t.Fatalf("original file exceeds 1 MiB: %s", file)
		}
		originals[file] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]declaration{}
	for _, d := range independentlyInventoried {
		lines, ok := originals[d.file]
		if !ok || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("invalid original declaration %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, exists := expected[key]; exists {
			t.Fatalf("duplicate independent declaration at %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "python", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 128,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("original outline incomplete: err=%v failed=%+v result=%+v", err, failed, out)
	}
	actual := map[string]bool{}
	for _, s := range out.Symbols {
		file := filepath.Base(s.Path)
		key := file + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if filepath.Clean(s.Path) != filepath.Join(project, file) {
			t.Errorf("unexpected source file: %+v", s)
		}
		if actual[key] {
			t.Errorf("duplicate original source position: %s", key)
		}
		actual[key] = true
		d, ok := expected[key]
		if !ok {
			t.Errorf("invented Python declaration at %s: %s %s", key, s.Kind, s.Name)
			continue
		}
		if s.Name != d.name || string(s.Kind) != d.kind || s.NativeKind != d.kind || s.ParentQualifiedName != d.parent {
			t.Errorf("incorrect original at %s: %s %s native=%q parent=%q, expected %+v",
				key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
		}
	}
	for key, d := range expected {
		if !actual[key] {
			t.Errorf("missing original Python declaration at %s: %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(independentlyInventoried) {
		t.Errorf("expected %d original declarations but got %d", len(independentlyInventoried), len(out.Symbols))
	}
	t.Logf("full selected original Python files=%d independent definitions=%d indexed=%d complete=%v",
		len(files), len(independentlyInventoried), len(out.Symbols), out.CoverageComplete)
}
