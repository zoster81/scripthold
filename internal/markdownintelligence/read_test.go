package markdownintelligence

import (
	"errors"
	"strings"
	"testing"

	"github.com/zoster81/marksplice"
)

func TestSnapshotInspectProjectsMarkspliceStructureWithinLimit(t *testing.T) {
	source := []byte("---\ntitle: Demo\n---\n\n# Project\n\nSee [child](#child).\n\n## Child\n\n```go\nfmt.Println(\"x\")\n```\n\n> [!NOTE]\n> Important.\n\n[^note]: Footnote text.\n\nUse it [^note].\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}

	inspected, err := snapshot.Inspect(32)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Truncated {
		t.Fatal("unexpected truncation")
	}
	if len(inspected.Nodes) == 0 || len(inspected.Sections) != 2 {
		t.Fatalf("inspect nodes=%d sections=%d", len(inspected.Nodes), len(inspected.Sections))
	}
	if len(inspected.Relationships) == 0 || inspected.Relationships[0].Destination != "#child" {
		t.Fatalf("relationships=%+v", inspected.Relationships)
	}
	if len(inspected.HeadingAnchors) != 2 || len(inspected.FencedBlocks) != 1 || len(inspected.Alerts) != 1 || len(inspected.FootnoteReferences) != 1 {
		t.Fatalf("anchors=%+v fenced=%+v alerts=%+v footnotes=%+v", inspected.HeadingAnchors, inspected.FencedBlocks, inspected.Alerts, inspected.FootnoteReferences)
	}
	if inspected.FrontMatter == nil || inspected.FrontMatter.Format == "" {
		t.Fatalf("front matter=%+v", inspected.FrontMatter)
	}

	limited, err := snapshot.Inspect(1)
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || len(limited.Nodes) > 1 || len(limited.Sections) > 1 || len(limited.Relationships) > 1 {
		t.Fatalf("bounded inspect=%+v", limited)
	}
}

func TestSnapshotSectionsFragmentsAndRelationshipsRemainMarkspliceBound(t *testing.T) {
	snapshot, err := Parse([]byte("# Project\n\n[child](#child)\n\n## Child\n\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}

	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(sections) != 1 || sections[0].Level != 2 || sections[0].HeadingTargetID == "" {
		t.Fatalf("sections=%+v truncated=%v", sections, truncated)
	}

	resolved, ok := snapshot.ResolveFragment("#child")
	if !ok || resolved.TargetID != sections[0].HeadingTargetID || resolved.Value != "child" {
		t.Fatalf("resolved=%+v ok=%v section=%+v", resolved, ok, sections[0])
	}
	if !snapshot.ValidateFragment("child") || snapshot.ValidateFragment("missing") {
		t.Fatal("fragment validation did not follow Marksplice resolution")
	}

	relationships, truncated, err := snapshot.Relationships(1)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(relationships) != 1 || relationships[0].FragmentTargetID != resolved.TargetID {
		t.Fatalf("relationships=%+v truncated=%v", relationships, truncated)
	}
}

func TestSnapshotGenerationIsExplicitAndInvalidKindsFailClosed(t *testing.T) {
	snapshot, err := Parse([]byte("# Project\n\n## Child\n\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}

	toc, err := snapshot.Generate("toc")
	if err != nil {
		t.Fatal(err)
	}
	if len(toc) == 0 || !strings.Contains(string(toc), "Project") || !strings.Contains(string(toc), "Child") {
		t.Fatalf("toc=%q", toc)
	}

	canonical, err := snapshot.Generate("canonical_markdown")
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) == 0 || !strings.Contains(string(canonical), "Project") {
		t.Fatalf("canonical=%q", canonical)
	}

	if _, err := snapshot.Generate("html"); !errors.Is(err, marksplice.ErrInvalidQuery) {
		t.Fatalf("invalid generation error=%v", err)
	}
}
