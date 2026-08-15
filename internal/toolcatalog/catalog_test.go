package toolcatalog

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCatalogIsCompleteAndUnique(t *testing.T) {
	definitions := All()
	if got, want := len(definitions), 36; got != want {
		t.Fatalf("catalog contains %d tools, want %d", got, want)
	}
	if _, exists := Lookup("directory_tree"); exists {
		t.Fatal("deprecated directory_tree tool must not be exposed in 2.0")
	}
	if _, exists := Lookup("write_file"); exists {
		t.Fatal("ambiguous write_file tool name must not be exposed")
	}
	if definition, exists := Lookup("write_whole_file"); !exists {
		t.Fatal("write_whole_file tool must be exposed")
	} else if !strings.Contains(definition.Description, "complete file contents") {
		t.Fatalf("write_whole_file description does not make replacement semantics explicit: %q", definition.Description)
	}

	seen := make(map[string]struct{}, len(definitions))
	for i, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" {
			t.Fatalf("tool %d has an empty name", i)
		}
		if strings.TrimSpace(definition.Title) == "" {
			t.Fatalf("tool %q has an empty title", definition.Name)
		}
		if strings.TrimSpace(definition.Description) == "" {
			t.Fatalf("tool %q has an empty description", definition.Name)
		}
		if _, exists := seen[definition.Name]; exists {
			t.Fatalf("duplicate tool name %q", definition.Name)
		}
		seen[definition.Name] = struct{}{}

		got, ok := Lookup(definition.Name)
		if !ok || !reflect.DeepEqual(got, definition) {
			t.Fatalf("Lookup(%q) = %#v, %v; want %#v, true", definition.Name, got, ok, definition)
		}
	}
}

func TestAllReturnsIndependentCopy(t *testing.T) {
	first := All()
	first[0].Name = "modified"
	second := All()
	if second[0].Name == "modified" {
		t.Fatal("All() returned shared mutable catalog storage")
	}
}

func TestDocumentationCoversEveryCatalogTool(t *testing.T) {
	root := repositoryRoot(t)
	readme := readFile(t, filepath.Join(root, "README.md"))
	tools := readFile(t, filepath.Join(root, "TOOLS.md"))

	for _, definition := range All() {
		if !strings.Contains(readme, "[`"+definition.Name+"`](TOOLS.md#") {
			t.Errorf("README.md does not link tool %q", definition.Name)
		}
		if !strings.Contains(tools, "### "+definition.Name) {
			t.Errorf("TOOLS.md does not contain a section for %q", definition.Name)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
