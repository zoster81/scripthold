package markdownintelligence

import (
	"bytes"
	"errors"
	"strings"
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

func TestPrepareRemoveSectionUsesSectionTargetAndRemovesSubtree(t *testing.T) {
	source := []byte("# Root\r\n\r\nIntro.\r\n\r\n## Remove\r\n\r\nRemove body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	prepared, err := snapshot.PrepareRemoveSection(sections[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "## Remove") || strings.Contains(string(result), "### Child") || strings.Contains(string(result), "Remove body.") || strings.Contains(string(result), "Child body.") {
		t.Fatalf("removed section subtree remains in result=%q", result)
	}
	if !strings.Contains(string(result), "# Root\r\n") || !strings.Contains(string(result), "## Keep\r\n\r\nKeep body.\r\n") {
		t.Fatalf("unrelated section content changed unexpectedly: %q", result)
	}
	updated, err := Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	remaining, truncated, err := updated.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(remaining) != 1 {
		t.Fatalf("remaining sections=%+v truncated=%v err=%v result=%q", remaining, truncated, err, result)
	}
	stale := bytes.Replace(source, []byte("Intro."), []byte("External."), 1)
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareRemoveSection(sections[0].HeadingTargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("heading target error=%v, want ErrInvalidTargetKind", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRemoveSection(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareRemoveSection("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, marksplice.ErrNodeNotFound) {
		t.Fatalf("unknown target error=%v, want ErrNodeNotFound", err)
	}
}

func TestPrepareReplaceSectionBodyPreservesHeadingAndChildHierarchy(t *testing.T) {
	source := []byte("# Root\r\n\r\n## Target\r\n\r\nOld body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	prepared, err := snapshot.PrepareReplaceSectionBody(sections[0].TargetID, []byte("New **body**.\r\n\r\nSecond paragraph.\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "Old body.") || !strings.Contains(string(result), "New **body**.") || !strings.Contains(string(result), "Second paragraph.") {
		t.Fatalf("section body replacement result=%q", result)
	}
	if !strings.Contains(string(result), "## Target\r\n") || !strings.Contains(string(result), "### Child\r\n\r\nChild body.\r\n") || !strings.Contains(string(result), "## Keep\r\n\r\nKeep body.\r\n") {
		t.Fatalf("section hierarchy or unrelated content changed unexpectedly: %q", result)
	}
	updated, err := Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	updatedSections, truncated, err := updated.QuerySections(nil, nil, 8)
	if err != nil || truncated || len(updatedSections) != 4 {
		t.Fatalf("updated sections=%+v truncated=%v err=%v", updatedSections, truncated, err)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("Keep body."), []byte("External."), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareReplaceSectionBody(sections[0].HeadingTargetID, []byte("New body.\r\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("heading target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareReplaceSectionBody(sections[0].TargetID, []byte("### New child\r\n\r\nBody.\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("hierarchy-changing body error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareReplaceSectionReplacesCompleteSubtreeAndPreservesSibling(t *testing.T) {
	source := []byte("# Root\r\n\r\n## Target\r\n\r\nOld body.\r\n\r\n### Old Child\r\n\r\nOld child body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	replacement := []byte("## Replaced\r\n\r\nNew body.\r\n\r\n### New Child\r\n\r\nNew child body.\r\n")
	prepared, err := snapshot.PrepareReplaceSection(sections[0].TargetID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)
	if strings.Contains(got, "## Target") || strings.Contains(got, "### Old Child") || strings.Contains(got, "Old body.") || strings.Contains(got, "Old child body.") {
		t.Fatalf("old section subtree remains in result=%q", result)
	}
	if !strings.Contains(got, "## Replaced\r\n\r\nNew body.\r\n\r\n### New Child\r\n\r\nNew child body.\r\n") {
		t.Fatalf("replacement subtree missing or altered: %q", result)
	}
	if !strings.Contains(got, "## Keep\r\n\r\nKeep body.\r\n") {
		t.Fatalf("unrelated sibling changed unexpectedly: %q", result)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("Keep body."), []byte("External."), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareReplaceSection(sections[0].HeadingTargetID, replacement); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("heading target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareReplaceSection(sections[0].TargetID, []byte("Paragraph only.\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("non-section replacement error=%v, want ErrInvalidReplacement", err)
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

func TestPrepareInsertParagraphBeforeAndAfterAreSnapshotBoundAndSourcePreserving(t *testing.T) {
	beforeSource := []byte("first\r\n\r\ntarget [link](dest)\r\n")
	beforeSnapshot, err := Parse(beforeSource)
	if err != nil {
		t.Fatal(err)
	}
	beforeParagraphs, err := beforeSnapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(beforeParagraphs) != 2 {
		t.Fatalf("before paragraphs=%+v err=%v", beforeParagraphs, err)
	}
	before, err := beforeSnapshot.PrepareInsertParagraphBefore(beforeParagraphs[1].TargetID, []byte("new *paragraph*"))
	if err != nil {
		t.Fatal(err)
	}
	beforeResult, err := before.Apply(beforeSource)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(beforeResult), "first\r\n\r\nnew *paragraph*\r\n\r\ntarget [link](dest)\r\n"; got != want {
		t.Fatalf("before result=%q want %q", got, want)
	}
	if _, err := before.Apply([]byte("first\r\n\r\nexternal\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("before stale error=%v, want ErrSourceConflict", err)
	}

	afterSource := []byte("target\n\nafter\n")
	afterSnapshot, err := Parse(afterSource)
	if err != nil {
		t.Fatal(err)
	}
	afterParagraphs, err := afterSnapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(afterParagraphs) != 2 {
		t.Fatalf("after paragraphs=%+v err=%v", afterParagraphs, err)
	}
	after, err := afterSnapshot.PrepareInsertParagraphAfter(afterParagraphs[0].TargetID, []byte("new **paragraph**"))
	if err != nil {
		t.Fatal(err)
	}
	afterResult, err := after.Apply(afterSource)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(afterResult), "target\n\nnew **paragraph**\n\nafter\n"; got != want {
		t.Fatalf("after result=%q want %q", got, want)
	}
}

func TestPrepareInsertParagraphRejectsInvalidContentAndWrongTargetKind(t *testing.T) {
	snapshot, err := Parse([]byte("# Heading\n\nTarget.\n"))
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	for _, replacement := range [][]byte{nil, []byte("# heading"), []byte("one\n\ntwo")} {
		if _, err := snapshot.PrepareInsertParagraphBefore(paragraphs[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("before content %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	if _, err := snapshot.PrepareInsertParagraphAfter(headings[0].TargetID, []byte("Paragraph.")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("after wrong-kind error=%v, want ErrInvalidTargetKind", err)
	}

	noEOL, err := Parse([]byte("Target."))
	if err != nil {
		t.Fatal(err)
	}
	noEOLParagraphs, err := noEOL.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(noEOLParagraphs) != 1 {
		t.Fatalf("no-EOL paragraphs=%+v err=%v", noEOLParagraphs, err)
	}
	if _, err := noEOL.PrepareInsertParagraphAfter(noEOLParagraphs[0].TargetID, []byte("New.")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("after EOF error=%v, want ErrInvalidReplacement", err)
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
