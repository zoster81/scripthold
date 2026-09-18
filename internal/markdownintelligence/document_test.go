package markdownintelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSnapshotQueryUsesMarkspliceStructureAndSnapshotBoundTargets(t *testing.T) {
	source := []byte("Project\n=======\n\nParagraph *with* text.\n\n## Usage\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}

	wantFingerprintBytes := sha256.Sum256(source)
	wantFingerprint := hex.EncodeToString(wantFingerprintBytes[:])
	if snapshot.SourceFingerprint() != wantFingerprint {
		t.Fatalf("source fingerprint = %q, want %q", snapshot.SourceFingerprint(), wantFingerprint)
	}

	headings, err := snapshot.QueryNodes([]string{"heading"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(headings) != 2 {
		t.Fatalf("heading count = %d, want 2: %+v", len(headings), headings)
	}
	if headings[0].Kind != "heading" || headings[0].Attributes["text"] != "Project" || headings[0].Attributes["level"] != 1 || headings[0].Attributes["style"] != "setext" {
		t.Fatalf("first heading = %+v", headings[0])
	}
	if headings[1].Attributes["text"] != "Usage" || headings[1].Attributes["level"] != 2 || headings[1].Attributes["style"] != "atx" {
		t.Fatalf("second heading = %+v", headings[1])
	}
	for _, heading := range headings {
		if len(heading.TargetID) != 64 {
			t.Fatalf("target ID length = %d, want 64: %q", len(heading.TargetID), heading.TargetID)
		}
		if _, err := hex.DecodeString(heading.TargetID); err != nil {
			t.Fatalf("target ID is not lowercase hexadecimal: %q: %v", heading.TargetID, err)
		}
	}

	same, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sameHeadings, err := same.QueryNodes([]string{"heading"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sameHeadings[0].TargetID != headings[0].TargetID {
		t.Fatalf("same snapshot target changed: %q != %q", sameHeadings[0].TargetID, headings[0].TargetID)
	}

	changed, err := Parse([]byte("Project!\n========\n\nParagraph *with* text.\n\n## Usage\n"))
	if err != nil {
		t.Fatal(err)
	}
	changedHeadings, err := changed.QueryNodes([]string{"heading"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if changedHeadings[0].TargetID == headings[0].TargetID {
		t.Fatal("target ID survived a different source snapshot")
	}
}

func TestSnapshotQueryIsBoundedAndKindValidated(t *testing.T) {
	snapshot, err := Parse([]byte("# A\n\nText\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		kinds []string
		limit int
	}{
		{name: "zero limit", limit: 0},
		{name: "negative limit", limit: -1},
		{name: "unknown kind", kinds: []string{"made-up"}, limit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := snapshot.QueryNodes(test.kinds, test.limit); err == nil {
				t.Fatal("expected query rejection")
			}
		})
	}
}

func TestSnapshotTargetIndexesAreLazyAndReusable(t *testing.T) {
	snapshot, err := Parse([]byte("# Alpha\n\nBody.\n\n## Child\n"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.nodeTargetIndex != nil || snapshot.sectionTargetIndex != nil {
		t.Fatal("target indexes initialized eagerly")
	}
	nodes, err := snapshot.QueryNodes([]string{"heading"}, 4)
	if err != nil || len(nodes) != 2 {
		t.Fatalf("nodes=%+v err=%v", nodes, err)
	}
	if _, ok, err := snapshot.Target(nodes[0].TargetID); err != nil || !ok {
		t.Fatalf("first target lookup ok=%t err=%v", ok, err)
	}
	if len(snapshot.nodeTargetIndex) == 0 {
		t.Fatal("node target index was not initialized")
	}
	snapshot.nodeTargetIndex["sentinel"] = resolvedNodeTarget{}
	if _, ok, err := snapshot.Target(nodes[1].TargetID); err != nil || !ok {
		t.Fatalf("second target lookup ok=%t err=%v", ok, err)
	}
	if _, ok := snapshot.nodeTargetIndex["sentinel"]; !ok {
		t.Fatal("node target index was rebuilt instead of reused")
	}
	sections, truncated, err := snapshot.QuerySections(nil, nil, 4)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%t err=%v", sections, truncated, err)
	}
	if _, err := snapshot.sectionHeadingID(sections[0].TargetID); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.sectionTargetIndex) == 0 {
		t.Fatal("section target index was not initialized")
	}
}

func TestSnapshotTargetAndSourceRangeUseOnlyPublicStructuralIdentity(t *testing.T) {
	source := []byte("# Alpha\r\n\r\nBody.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := snapshot.QueryNodes([]string{"heading"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %+v", nodes)
	}

	resolved, ok, err := snapshot.Target(nodes[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || resolved.TargetID != nodes[0].TargetID || resolved.Kind != "heading" {
		t.Fatalf("resolved = %+v ok=%t", resolved, ok)
	}
	content, ok := snapshot.Source(resolved.Range)
	if !ok || string(content) != "Alpha" {
		t.Fatalf("source = %q ok=%t", content, ok)
	}
	if _, ok, err := snapshot.Target("not-a-target"); err != nil || ok {
		t.Fatalf("invalid target lookup ok=%t err=%v", ok, err)
	}
}
