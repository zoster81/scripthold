package markdownintelligence

import (
	"errors"
	"testing"

	"github.com/zoster81/marksplice"
)

func TestPrepareRenameHeadingIsSnapshotBoundAndSourcePreserving(t *testing.T) {
	source := []byte("# Old\r\n\r\nBody.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}

	prepared, err := snapshot.PrepareRenameHeading(headings[0].TargetID, []byte("New"))
	if err != nil {
		t.Fatal(err)
	}
	if prepared.SourceFingerprint() != snapshot.SourceFingerprint() {
		t.Fatalf("prepared fingerprint=%q want %q", prepared.SourceFingerprint(), snapshot.SourceFingerprint())
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result), "# New\r\n\r\nBody.\r\n"; got != want {
		t.Fatalf("result=%q want %q", got, want)
	}

	changed := []byte("# Old\r\n\r\nExternal.\r\n")
	if _, err := prepared.Apply(changed); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("changed-source error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareRenameHeadingRejectsUnknownTarget(t *testing.T) {
	snapshot, err := Parse([]byte("# Old\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = snapshot.PrepareRenameHeading("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []byte("New"))
	if !errors.Is(err, marksplice.ErrNodeNotFound) {
		t.Fatalf("error=%v, want ErrNodeNotFound", err)
	}
}

func TestPrepareSetHeadingLevelIsSnapshotBoundAndSourcePreserving(t *testing.T) {
	source := []byte("# Title\r\n\r\nBody.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	prepared, err := snapshot.PrepareSetHeadingLevel(headings[0].TargetID, 3)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result), "### Title\r\n\r\nBody.\n"; got != want {
		t.Fatalf("result=%q want %q", got, want)
	}
	if _, err := prepared.Apply([]byte("# External\r\n\r\nBody.\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareSetHeadingLevelRejectsOutOfRangeLevel(t *testing.T) {
	snapshot, err := Parse([]byte("# Title\n"))
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	for _, level := range []int{0, 7} {
		if _, err := snapshot.PrepareSetHeadingLevel(headings[0].TargetID, level); err == nil {
			t.Fatalf("level %d unexpectedly accepted", level)
		}
	}
}

func TestPrepareReplaceParagraphIsSnapshotBoundAndSourcePreserving(t *testing.T) {
	source := []byte("Old paragraph.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	prepared, err := snapshot.PrepareReplaceParagraph(paragraphs[0].TargetID, []byte("New paragraph."))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result), "New paragraph.\r\n"; got != want {
		t.Fatalf("result=%q want %q", got, want)
	}
	if _, err := prepared.Apply([]byte("External paragraph.\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareReplaceParagraphAcceptsInlineMarkdownAndRejectsMultipleParagraphs(t *testing.T) {
	source := []byte("Old paragraph.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	prepared, err := snapshot.PrepareReplaceParagraph(paragraphs[0].TargetID, []byte("New **bold** paragraph."))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := updated.QueryNodes([]string{"strong"}, 8)
	if err != nil || len(strong) != 1 {
		t.Fatalf("strong=%+v err=%v result=%q", strong, err, result)
	}
	if _, err := snapshot.PrepareReplaceParagraph(paragraphs[0].TargetID, []byte("First.\n\nSecond.")); err == nil {
		t.Fatal("multiple paragraphs unexpectedly accepted as one paragraph replacement")
	}
}

func TestPrepareReplaceParagraphRejectsWrongTargetKind(t *testing.T) {
	snapshot, err := Parse([]byte("# Heading\n"))
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	if _, err := snapshot.PrepareReplaceParagraph(headings[0].TargetID, []byte("Paragraph.")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveParagraphIsSnapshotBoundAndRejectsWrongTargetKind(t *testing.T) {
	source := []byte("# Title\r\n\r\nRemove me.\r\n\r\nKeep **this**.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 2 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	prepared, err := snapshot.PrepareRemoveParagraph(paragraphs[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := updated.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("remaining paragraphs=%+v err=%v result=%q", remaining, err, result)
	}
	if _, err := prepared.Apply([]byte("# Title\r\n\r\nExternal.\r\n\r\nKeep **this**.\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	if _, err := snapshot.PrepareRemoveParagraph(headings[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("wrong-kind error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestComposeChangesCombinesIndependentPreparedEditsAndRejectsOverlap(t *testing.T) {
	source := []byte("# One\n\n## Two\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 2 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	first, err := snapshot.PrepareRenameHeading(headings[0].TargetID, []byte("First"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := snapshot.PrepareRenameHeading(headings[1].TargetID, []byte("Second"))
	if err != nil {
		t.Fatal(err)
	}
	combined, err := snapshot.ComposeChanges(first, second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := combined.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result), "# First\n\n## Second\n"; got != want {
		t.Fatalf("result=%q want %q", got, want)
	}
	if _, err := combined.Apply([]byte("# One\n\n## External\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.ComposeChanges(first, first); err == nil {
		t.Fatal("overlapping prepared changes unexpectedly composed")
	}
}
