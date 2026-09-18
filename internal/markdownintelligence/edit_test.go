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

func TestPrepareInsertSectionBeforeAndAfterUseSectionTargets(t *testing.T) {
	source := []byte("# Root\r\n\r\n## Alpha\r\n\r\nAlpha body.\r\n\r\n### Alpha Child\r\n\r\nChild body.\r\n\r\n## Beta\r\n\r\nBeta body.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	before, err := snapshot.PrepareInsertSectionBefore(sections[0].TargetID, []byte("## Before Alpha\r\n\r\nBody.\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := snapshot.PrepareInsertSectionAfter(sections[0].TargetID, []byte("## After Alpha\r\n\r\n### Inserted Child\r\n\r\nBody.\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	combined, err := snapshot.ComposeChanges(before, after)
	if err != nil {
		t.Fatal(err)
	}
	result, err := combined.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Root\r\n\r\n## Before Alpha\r\n\r\nBody.\r\n## Alpha\r\n\r\nAlpha body.\r\n\r\n### Alpha Child\r\n\r\nChild body.\r\n\r\n## After Alpha\r\n\r\n### Inserted Child\r\n\r\nBody.\r\n## Beta\r\n\r\nBeta body.\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := combined.Apply(bytes.Replace(source, []byte("Beta body."), []byte("External."), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareInsertSectionBefore(sections[0].HeadingTargetID, []byte("## Invalid\r\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("heading target error=%v, want ErrInvalidTargetKind", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareInsertSectionAfter(paragraphs[0].TargetID, []byte("## Invalid\r\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareInsertSectionBefore(sections[0].TargetID, []byte("Paragraph only.\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("invalid fragment error=%v, want ErrInvalidReplacement", err)
	}
	conflictingBefore, err := snapshot.PrepareInsertSectionBefore(sections[1].TargetID, []byte("## Conflict\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.ComposeChanges(after, conflictingBefore); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("adjacent insertion conflict error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareInsertSectionPreservesLF(t *testing.T) {
	source := []byte("# Root\n\n## Alpha\n\nAlpha.\n\n## Beta\n\nBeta.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	prepared, err := snapshot.PrepareInsertSectionAfter(sections[0].TargetID, []byte("## Inserted\n\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Root\n\n## Alpha\n\nAlpha.\n\n## Inserted\n\nBody.\n## Beta\n\nBeta.\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if bytes.Contains(result, []byte("\r\n")) {
		t.Fatalf("LF insertion introduced CRLF: %q", result)
	}
}

func TestPrepareAppendSectionChildUsesSectionParentAndRequiresNextLevel(t *testing.T) {
	source := []byte("# Root\r\n\r\n## Parent\r\n\r\nBody.\r\n\r\n### Existing\r\n\r\nExisting.\r\n\r\n## Sibling\r\n\r\nSibling.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sections, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(sections) != 2 {
		t.Fatalf("sections=%+v truncated=%v err=%v", sections, truncated, err)
	}
	prepared, err := snapshot.PrepareAppendSectionChild(sections[0].TargetID, []byte("### Added\r\n\r\nAdded body.\r\n\r\n#### Grandchild\r\n\r\nGrandchild body.\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Root\r\n\r\n## Parent\r\n\r\nBody.\r\n\r\n### Existing\r\n\r\nExisting.\r\n\r\n### Added\r\n\r\nAdded body.\r\n\r\n#### Grandchild\r\n\r\nGrandchild body.\r\n## Sibling\r\n\r\nSibling.\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("Sibling."), []byte("External."), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	siblingAfter, err := snapshot.PrepareInsertSectionAfter(sections[0].TargetID, []byte("## Peer\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.ComposeChanges(prepared, siblingAfter); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("child/after conflict error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareAppendSectionChild(sections[0].HeadingTargetID, []byte("### Invalid\r\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("heading target error=%v, want ErrInvalidTargetKind", err)
	}
	for _, fragment := range [][]byte{[]byte("## Wrong sibling\r\n"), []byte("#### Too deep\r\n"), []byte("Paragraph only.\r\n")} {
		if _, err := snapshot.PrepareAppendSectionChild(sections[0].TargetID, fragment); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("fragment %q error=%v, want ErrInvalidReplacement", fragment, err)
		}
	}

	h6Source := []byte("###### Leaf\n")
	h6Snapshot, err := Parse(h6Source)
	if err != nil {
		t.Fatal(err)
	}
	h6Sections, truncated, err := h6Snapshot.QuerySections([]int{6}, nil, 8)
	if err != nil || truncated || len(h6Sections) != 1 {
		t.Fatalf("h6 sections=%+v truncated=%v err=%v", h6Sections, truncated, err)
	}
	if _, err := h6Snapshot.PrepareAppendSectionChild(h6Sections[0].TargetID, []byte("###### Impossible\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("h6 child error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareMoveSectionBeforeAndAfterUseSectionTargets(t *testing.T) {
	source := []byte("# One\r\n\r\n## Move\r\n\r\nMove body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n# Two\r\n\r\n## Anchor\r\n\r\nAnchor body.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	level2, truncated, err := snapshot.QuerySections([]int{2}, nil, 8)
	if err != nil || truncated || len(level2) != 2 {
		t.Fatalf("level2=%+v truncated=%v err=%v", level2, truncated, err)
	}
	level1, truncated, err := snapshot.QuerySections([]int{1}, nil, 8)
	if err != nil || truncated || len(level1) != 2 {
		t.Fatalf("level1=%+v truncated=%v err=%v", level1, truncated, err)
	}
	prepared, err := snapshot.PrepareMoveSectionAfter(level2[0].TargetID, level2[1].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "# One\r\n\r\n# Two\r\n\r\n## Anchor\r\n\r\nAnchor body.\r\n## Move\r\n\r\nMove body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("Anchor body."), []byte("External."), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareMoveSectionBefore(level2[0].HeadingTargetID, level2[1].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("source heading target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareMoveSectionBefore(level2[0].TargetID, level2[1].HeadingTargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("anchor heading target error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareMoveSectionBefore(level2[0].TargetID, level1[1].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("different-level move error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareMoveSectionBefore(level2[0].TargetID, level2[0].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("self move error=%v, want ErrInvalidReplacement", err)
	}

	noopSource := []byte("# A\nA.\n# B\nB.\n")
	noopSnapshot, err := Parse(noopSource)
	if err != nil {
		t.Fatal(err)
	}
	noopSections, truncated, err := noopSnapshot.QuerySections([]int{1}, nil, 8)
	if err != nil || truncated || len(noopSections) != 2 {
		t.Fatalf("noop sections=%+v truncated=%v err=%v", noopSections, truncated, err)
	}
	noop, err := noopSnapshot.PrepareMoveSectionBefore(noopSections[0].TargetID, noopSections[1].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	noopResult, err := noop.Apply(noopSource)
	if err != nil || !bytes.Equal(noopResult, noopSource) {
		t.Fatalf("noop result=%q err=%v want original", noopResult, err)
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

func TestPrepareReplaceListItemPreservesMarkerAndChildren(t *testing.T) {
	source := []byte("1. parent **old**\r\n   - child\r\n2. tail\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	prepared, err := snapshot.PrepareReplaceListItem(items[0].TargetID, []byte("parent **new**"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "1. parent **new**\r\n   - child\r\n2. tail\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	for _, replacement := range [][]byte{nil, []byte("line one\nline two"), []byte("---")} {
		if _, err := snapshot.PrepareReplaceListItem(items[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("replacement %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	wrongKindSnapshot, err := Parse([]byte("# Heading\n"))
	if err != nil {
		t.Fatal(err)
	}
	headings, err := wrongKindSnapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	if _, err := wrongKindSnapshot.PrepareReplaceListItem(headings[0].TargetID, []byte("wrong")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("wrong-kind error=%v, want ErrInvalidTargetKind", err)
	}
	taskSnapshot, err := Parse([]byte("- [ ] task\n- plain\n"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskSnapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	if _, err := taskSnapshot.PrepareReplaceListItem(tasks[0].TargetID, []byte("wrong")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("task target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceListItemSubtreePreservesSemanticParent(t *testing.T) {
	source := []byte("- outer\r\n  - target\r\n    - old child\r\n  - tail\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 4 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	prepared, err := snapshot.PrepareReplaceListItemSubtree(items[1].TargetID, []byte("  - replaced π\r\n    + new child\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "- outer\r\n  - replaced π\r\n    + new child\r\n  - tail\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareReplaceListItemSubtree(items[1].TargetID, []byte("  * changed marker\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("different-marker error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareReplaceListItemSubtree(items[1].TargetID, []byte("  - one\r\n  - two\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("multiple-root error=%v, want ErrInvalidReplacement", err)
	}
	taskSnapshot, err := Parse([]byte("- [ ] task\n"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskSnapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	if _, err := taskSnapshot.PrepareReplaceListItemSubtree(tasks[0].TargetID, []byte("- replacement\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("task target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveListItemRemovesCompleteSubtreeAndIsSnapshotBound(t *testing.T) {
	source := []byte("- root\r\n  - parent π\r\n    - child\r\n  - sibling\r\n- tail\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 5 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	prepared, err := snapshot.PrepareRemoveListItem(items[1].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "- root\r\n  - sibling\r\n- tail\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	wrongKindSnapshot, err := Parse([]byte("# Heading\n"))
	if err != nil {
		t.Fatal(err)
	}
	headings, err := wrongKindSnapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 1 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	if _, err := wrongKindSnapshot.PrepareRemoveListItem(headings[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("wrong-kind error=%v, want ErrInvalidTargetKind", err)
	}

	taskSource := []byte("- [ ] remove\r\n- [x] keep\r\n")
	taskSnapshot, err := Parse(taskSource)
	if err != nil {
		t.Fatal(err)
	}
	taskItems, err := taskSnapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(taskItems) != 2 {
		t.Fatalf("task list items=%+v err=%v", taskItems, err)
	}
	tasks, err := taskSnapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	taskRemoval, err := taskSnapshot.PrepareRemoveListItem(taskItems[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	taskResult, err := taskRemoval.Apply(taskSource)
	if err != nil || string(taskResult) != "- [x] keep\r\n" {
		t.Fatalf("task result=%q err=%v", taskResult, err)
	}
	if _, err := taskSnapshot.PrepareRemoveListItem(tasks[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("task target error=%v, want ErrInvalidTargetKind", err)
	}

	incomplete, err := Parse([]byte("- parent\n  - complex\n\n    second paragraph\n- tail\n"))
	if err != nil {
		t.Fatal(err)
	}
	incompleteItems, err := incomplete.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(incompleteItems) < 1 {
		t.Fatalf("incomplete items=%+v err=%v", incompleteItems, err)
	}
	if _, err := incomplete.PrepareRemoveListItem(incompleteItems[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("incomplete subtree error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareInsertListItemSiblingPreservesSubtreeAndSnapshot(t *testing.T) {
	source := []byte("1. parent\r\n   - anchor\r\n     - child\r\n   - tail\r\n2. end\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 5 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	after, err := snapshot.PrepareInsertListItemAfter(items[1].TargetID, []byte("   - inserted π\r\n     - grandchild\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := after.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "1. parent\r\n   - anchor\r\n     - child\r\n   - inserted π\r\n     - grandchild\r\n   - tail\r\n2. end\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := after.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	before, err := snapshot.PrepareInsertListItemBefore(items[3].TargetID, []byte("   - before tail\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	beforeResult, err := before.Apply(source)
	if err != nil || !bytes.Contains(beforeResult, []byte("   - before tail\r\n   - tail\r\n")) {
		t.Fatalf("before result=%q err=%v", beforeResult, err)
	}
	for _, fragment := range [][]byte{nil, []byte("- wrong indent\r\n"), []byte("   - one\r\n   - two\r\n")} {
		if _, err := snapshot.PrepareInsertListItemBefore(items[3].TargetID, fragment); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("fragment %q error=%v, want ErrInvalidReplacement", fragment, err)
		}
	}
	unsafe, err := Parse([]byte("- alpha\n- beta"))
	if err != nil {
		t.Fatal(err)
	}
	unsafeItems, err := unsafe.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(unsafeItems) != 2 {
		t.Fatalf("unsafe items=%+v err=%v", unsafeItems, err)
	}
	if _, err := unsafe.PrepareInsertListItemAfter(unsafeItems[1].TargetID, []byte("- inserted\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("unsafe EOF error=%v, want ErrInvalidReplacement", err)
	}
	taskSnapshot, err := Parse([]byte("- [ ] task\n- plain\n"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskSnapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	if _, err := taskSnapshot.PrepareInsertListItemBefore(tasks[0].TargetID, []byte("- inserted\n")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("task target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareAppendListItemChildAppendsValidatedSubtree(t *testing.T) {
	source := []byte("- parent\r\n  - existing\r\n- tail\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	prepared, err := snapshot.PrepareAppendListItemChild(items[0].TargetID, []byte("  - child π\r\n    1. grandchild\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "- parent\r\n  - existing\r\n  - child π\r\n    1. grandchild\r\n- tail\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareAppendListItemChild(items[0].TargetID, []byte("- wrong level\r\n")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("wrong-level error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareMoveListItemBeforeAndAfterMovesCompleteSubtree(t *testing.T) {
	source := []byte("1. first\r\n   - move π\r\n     - child\r\n2. second\r\n   - anchor\r\n3. tail\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 6 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	prepared, err := snapshot.PrepareMoveListItemAfter(items[1].TargetID, items[4].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "1. first\r\n2. second\r\n   - anchor\r\n   - move π\r\n     - child\r\n3. tail\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("tail"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareMoveListItemBefore(items[1].TargetID, items[2].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("ancestor/descendant error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareMoveListItemAfter(items[2].TargetID, items[1].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("descendant/ancestor error=%v, want ErrInvalidReplacement", err)
	}
	noopSource := []byte("- alpha\n- beta\n")
	noopSnapshot, err := Parse(noopSource)
	if err != nil {
		t.Fatal(err)
	}
	noopItems, err := noopSnapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(noopItems) != 2 {
		t.Fatalf("noop items=%+v err=%v", noopItems, err)
	}
	noop, err := noopSnapshot.PrepareMoveListItemBefore(noopItems[0].TargetID, noopItems[1].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	noopResult, err := noop.Apply(noopSource)
	if err != nil || !bytes.Equal(noopResult, noopSource) {
		t.Fatalf("noop result=%q err=%v", noopResult, err)
	}
	taskSnapshot, err := Parse([]byte("- [ ] task\n- plain\n"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := taskSnapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	taskItems, err := taskSnapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(taskItems) != 2 {
		t.Fatalf("task items=%+v err=%v", taskItems, err)
	}
	if _, err := taskSnapshot.PrepareMoveListItemBefore(tasks[0].TargetID, taskItems[1].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("task target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareSetTaskCheckedUsesTaskTargetAndPreservesNoOpStyle(t *testing.T) {
	source := []byte("* [X] keep uppercase\r\n- [ ] nested\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := snapshot.QueryNodes([]string{"task"}, 8)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	items, err := snapshot.QueryNodes([]string{"list_item"}, 8)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	noop, err := snapshot.PrepareSetTaskChecked(tasks[0].TargetID, true)
	if err != nil {
		t.Fatal(err)
	}
	noopResult, err := noop.Apply(source)
	if err != nil || !bytes.Equal(noopResult, source) {
		t.Fatalf("noop result=%q err=%v", noopResult, err)
	}
	prepared, err := snapshot.PrepareSetTaskChecked(tasks[1].TargetID, true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "* [X] keep uppercase\r\n- [x] nested\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("nested"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	if _, err := snapshot.PrepareSetTaskChecked(items[1].TargetID, true); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("list-item target error=%v, want ErrInvalidTargetKind", err)
	}
	unchecked, err := snapshot.PrepareSetTaskChecked(tasks[0].TargetID, false)
	if err != nil {
		t.Fatal(err)
	}
	uncheckedResult, err := unchecked.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(uncheckedResult), "* [ ] keep uppercase\r\n- [ ] nested\r\n"; got != want {
		t.Fatalf("unchecked result=%q want=%q", got, want)
	}
}

func TestPrepareReplaceCodeSpanPreservesFenceAndRejectsUnsafeReplacement(t *testing.T) {
	source := []byte("before ``old`code`` after\r\n\nparagraph\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	spans, err := snapshot.QueryNodes([]string{"code_span"}, 8)
	if err != nil || len(spans) != 1 {
		t.Fatalf("spans=%+v err=%v", spans, err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 2 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	prepared, err := snapshot.PrepareReplaceCodeSpan(spans[0].TargetID, []byte("new`code"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "before ``new`code`` after\r\n\nparagraph\r\n"
	if string(result) != want {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply(bytes.Replace(source, []byte("paragraph"), []byte("external"), 1)); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	for _, replacement := range [][]byte{nil, []byte("line one\nline two"), []byte("``")} {
		if _, err := snapshot.PrepareReplaceCodeSpan(spans[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("unsafe replacement %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	if _, err := snapshot.PrepareReplaceCodeSpan(paragraphs[1].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceSimpleInlinePreservesAuthoredDelimiters(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		kind        string
		replacement []byte
		want        string
		prepare     func(*Snapshot, string, []byte) (PreparedChange, error)
	}{
		{name: "strikethrough", source: []byte("prefix ~~caffè 東京~~ suffix\n"), kind: "strikethrough", replacement: []byte("nuovo 東京"), want: "prefix ~~nuovo 東京~~ suffix\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceStrikethrough(id, b)
		}},
		{name: "emphasis", source: []byte("before _old_ after\r\n"), kind: "emphasis", replacement: []byte("new"), want: "before _new_ after\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) { return s.PrepareReplaceEmphasis(id, b) }},
		{name: "strong", source: []byte("before **old** after\r\n"), kind: "strong", replacement: []byte("new"), want: "before **new** after\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) { return s.PrepareReplaceStrong(id, b) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			nodes, err := snapshot.QueryNodes([]string{tt.kind}, 8)
			if err != nil || len(nodes) != 1 {
				t.Fatalf("nodes=%+v err=%v", nodes, err)
			}
			prepared, err := tt.prepare(snapshot, nodes[0].TargetID, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			result, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != tt.want {
				t.Fatalf("result=%q want=%q", result, tt.want)
			}
		})
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
