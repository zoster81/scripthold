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

func TestPrepareReplaceDirectLinkFamilyPreservesAuthoredSyntax(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		kind        string
		replacement []byte
		want        string
		prepare     func(*Snapshot, string, []byte) (PreparedChange, error)
	}{
		{name: "inline link destination", source: []byte("before [label](<old/path> \"title\") after\r\n"), kind: "inline_link", replacement: []byte("new/path"), want: "before [label](<new/path> \"title\") after\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceInlineLinkDestination(id, b)
		}},
		{name: "inline link label", source: []byte("before [old](path) after\n"), kind: "inline_link", replacement: []byte("new"), want: "before [new](path) after\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceInlineLinkLabel(id, b)
		}},
		{name: "image destination", source: []byte("before ![alt](<old path> 'title') after\n"), kind: "image", replacement: []byte("new path"), want: "before ![alt](<new path> 'title') after\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceImageDestination(id, b)
		}},
		{name: "image alt", source: []byte("before ![old](path) after\n"), kind: "image", replacement: []byte("new"), want: "before ![new](path) after\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceImageAlt(id, b)
		}},
		{name: "autolink", source: []byte("before <https://old.example/path> after\n"), kind: "autolink", replacement: []byte("https://new.example/path"), want: "before <https://new.example/path> after\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceAutoLink(id, b)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			nodes, err := snapshot.QueryNodes([]string{tt.kind}, 4)
			if err != nil || len(nodes) != 1 {
				t.Fatalf("nodes=%+v err=%v", nodes, err)
			}
			prepared, err := tt.prepare(snapshot, nodes[0].TargetID, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			got, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("result=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestPrepareFencedCodeMutationsPreserveAuthoredContainer(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		replacement []byte
		want        string
		prepare     func(*Snapshot, string, []byte) (PreparedChange, error)
	}{
		{name: "replace body", source: []byte("```` go extra\nline one\nline two\n  `````  \n"), replacement: []byte("new one\nnew two"), want: "```` go extra\nnew one\nnew two\n  `````  \n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceFencedCode(id, b)
		}},
		{name: "populate empty body", source: []byte("```math\n```\n"), replacement: []byte("x + y"), want: "```math\nx + y\n```\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceFencedCode(id, b)
		}},
		{name: "replace info", source: []byte("  ~~~~  go old  \nbody\n ~~~~~~   \n"), replacement: []byte("typescript module"), want: "  ~~~~  typescript module  \nbody\n ~~~~~~   \n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareSetFencedBlockInfo(id, b)
		}},
		{name: "clear info CRLF", source: []byte("```  go extra  \r\nbody\r\n```\r\n"), replacement: []byte(""), want: "```    \r\nbody\r\n```\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareSetFencedBlockInfo(id, b)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			inspect, err := snapshot.Inspect(4)
			if err != nil || len(inspect.FencedBlocks) != 1 {
				t.Fatalf("inspect=%+v err=%v", inspect, err)
			}
			prepared, err := tt.prepare(snapshot, inspect.FencedBlocks[0].TargetID, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			got, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("result=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestPrepareFencedCodeMutationsAcceptQueryNodeTargetsWhenAvailable(t *testing.T) {
	source := []byte("```go\nold body\n```\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := snapshot.QueryNodes([]string{"fenced_code"}, 4)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes=%+v err=%v", nodes, err)
	}

	prepared, err := snapshot.PrepareReplaceFencedCode(nodes[0].TargetID, []byte("new body"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\nnew body\n```\n"; string(got) != want {
		t.Fatalf("result=%q want=%q", got, want)
	}
}

func TestPrepareFencedCodeMutationsPreserveMarkspliceRejections(t *testing.T) {
	source := []byte("```go\nbody\n```\n\nparagraph\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := snapshot.Inspect(4)
	if err != nil || len(inspect.FencedBlocks) != 1 {
		t.Fatalf("inspect=%+v err=%v", inspect, err)
	}
	targetID := inspect.FencedBlocks[0].TargetID
	if _, err := snapshot.PrepareReplaceFencedCode(targetID, nil); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("empty body error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareSetFencedBlockInfo(targetID, []byte("bad\ninfo")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("multiline info error=%v, want ErrInvalidReplacement", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 4)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceFencedCode(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRenameReferenceDefinitionUpdatesBoundOccurrences(t *testing.T) {
	source := []byte("[one]: <dest> \"Title\"\r\n\r\n[visible][one] [one][] [one] ![alt][one]\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	prepared, err := snapshot.PrepareRenameReferenceDefinition(definitions[0].TargetID, []byte("renamed"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "[renamed]: <dest> \"Title\"\r\n\r\n[visible][renamed] [one][renamed] [one][renamed] ![alt][renamed]\r\n"
	if string(got) != want {
		t.Fatalf("result=%q want=%q", got, want)
	}
}

func TestPrepareRenameReferenceDefinitionPreservesMarksplicePreconditions(t *testing.T) {
	source := []byte("[one]: <dest-one>\n[two]: <dest-two>\n\n[visible][one]\n\nparagraph\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
	if err != nil || len(definitions) != 2 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	var oneTarget string
	for _, definition := range definitions {
		if definition.Attributes["label"] == "one" {
			oneTarget = definition.TargetID
			break
		}
	}
	if oneTarget == "" {
		t.Fatal("reference definition one not found")
	}
	for _, replacement := range [][]byte{nil, []byte("bad]label"), []byte("bad\nlabel"), []byte("TWO")} {
		if _, err := snapshot.PrepareRenameReferenceDefinition(oneTarget, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("rename %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	noOp, err := snapshot.PrepareRenameReferenceDefinition(oneTarget, []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := noOp.Apply(source); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("no-op result=%q err=%v", got, err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 4)
	if err != nil || len(paragraphs) != 2 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRenameReferenceDefinition(paragraphs[0].TargetID, []byte("renamed")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveReferenceDefinitionRemovesOnlyUnusedDefinition(t *testing.T) {
	source := []byte("before\r\n\r\n  [unused]: <target> \"Title\"   \r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	prepared, err := snapshot.PrepareRemoveReferenceDefinition(definitions[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := "before\r\n\r\n\r\nafter\r\n"
	if string(got) != want {
		t.Fatalf("result=%q want=%q", got, want)
	}
}

func TestPrepareRemoveReferenceDefinitionPreservesMarksplicePreconditions(t *testing.T) {
	for _, source := range []string{
		"[docs]: <target>\n\n[full][docs]\n",
		"[docs]: <target>\n\n[docs][]\n",
		"[docs]: <target>\n\n[docs]\n",
		"[docs]: <target>\n\n![docs]\n",
	} {
		snapshot, err := Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		definitions, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
		if err != nil || len(definitions) != 1 {
			t.Fatalf("definitions=%+v err=%v", definitions, err)
		}
		if _, err := snapshot.PrepareRemoveReferenceDefinition(definitions[0].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("used definition error=%v, want ErrInvalidReplacement for %q", err, source)
		}
	}

	source := []byte("[unused]: <target>\n\nparagraph\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 4)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRemoveReferenceDefinition(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReferenceDefinitionPartsPreserveAuthoredSyntax(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		replacement []byte
		want        string
		prepare     func(*Snapshot, string, []byte) (PreparedChange, error)
	}{
		{name: "replace destination", source: []byte("  [docs]: <old/path>\t'Old title'   \r\n"), replacement: []byte("new/path"), want: "  [docs]: <new/path>\t'Old title'   \r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceReferenceDefinitionDestination(id, b)
		}},
		{name: "replace title", source: []byte("  [docs]: <old/path>\t'Old title'   \r\n"), replacement: []byte("New title"), want: "  [docs]: <old/path>\t'New title'   \r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceReferenceDefinitionTitle(id, b)
		}},
		{name: "add title", source: []byte("[docs]: <target>\n"), replacement: []byte("Title"), want: "[docs]: <target> \"Title\"\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareAddReferenceDefinitionTitle(id, b)
		}},
		{name: "remove title", source: []byte("[docs]: <target> \"Title\"\n"), want: "[docs]: <target>\n", prepare: func(s *Snapshot, id string, _ []byte) (PreparedChange, error) {
			return s.PrepareRemoveReferenceDefinitionTitle(id)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			nodes, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
			if err != nil || len(nodes) != 1 {
				t.Fatalf("nodes=%+v err=%v", nodes, err)
			}
			prepared, err := tt.prepare(snapshot, nodes[0].TargetID, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			got, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("result=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestPrepareReferenceDefinitionPartsPreserveMarkspliceRejections(t *testing.T) {
	source := []byte("[docs]: <target>\n\nparagraph\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"reference_definition"}, 4)
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	id := definitions[0].TargetID
	if _, err := snapshot.PrepareReplaceReferenceDefinitionDestination(id, nil); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("empty destination error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareReplaceReferenceDefinitionTitle(id, []byte("new")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("replace absent title error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareRemoveReferenceDefinitionTitle(id); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("remove absent title error=%v, want ErrInvalidReplacement", err)
	}

	withTitle, err := Parse([]byte("[docs]: <target> \"Old\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	withTitleDefinitions, err := withTitle.QueryNodes([]string{"reference_definition"}, 4)
	if err != nil || len(withTitleDefinitions) != 1 {
		t.Fatalf("with-title definitions=%+v err=%v", withTitleDefinitions, err)
	}
	if _, err := withTitle.PrepareAddReferenceDefinitionTitle(withTitleDefinitions[0].TargetID, []byte("second")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("add existing title error=%v, want ErrInvalidReplacement", err)
	}

	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 4)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceReferenceDefinitionDestination(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareDirectTitleLifecyclePreservesAuthoredSyntax(t *testing.T) {
	tests := []struct {
		name    string
		source  []byte
		kind    string
		text    []byte
		want    string
		prepare func(*Snapshot, string, []byte) (PreparedChange, error)
	}{
		{name: "replace inline link title", source: []byte("[label](dest   \"old title\")\n"), kind: "inline_link", text: []byte("new"), want: "[label](dest   \"new\")\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceInlineLinkTitle(id, b)
		}},
		{name: "add inline link title", source: []byte("[label](<dest path>   )\n"), kind: "inline_link", text: []byte("new title"), want: "[label](<dest path>    \"new title\")\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareAddInlineLinkTitle(id, b)
		}},
		{name: "remove inline link title", source: []byte("[label](dest   'old')\n"), kind: "inline_link", want: "[label](dest   )\n", prepare: func(s *Snapshot, id string, _ []byte) (PreparedChange, error) {
			return s.PrepareRemoveInlineLinkTitle(id)
		}},
		{name: "replace image title", source: []byte("![alt](dest  (old title))\r\n"), kind: "image", text: []byte("a longer title"), want: "![alt](dest  (a longer title))\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareReplaceImageTitle(id, b)
		}},
		{name: "add image title", source: []byte("![alt](image.png)\r\n"), kind: "image", text: []byte("caption p"), want: "![alt](image.png \"caption p\")\r\n", prepare: func(s *Snapshot, id string, b []byte) (PreparedChange, error) {
			return s.PrepareAddImageTitle(id, b)
		}},
		{name: "remove image title", source: []byte("![alt](dest 'old')\n"), kind: "image", want: "![alt](dest )\n", prepare: func(s *Snapshot, id string, _ []byte) (PreparedChange, error) {
			return s.PrepareRemoveImageTitle(id)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			nodes, err := snapshot.QueryNodes([]string{tt.kind}, 4)
			if err != nil || len(nodes) != 1 {
				t.Fatalf("nodes=%+v err=%v", nodes, err)
			}
			prepared, err := tt.prepare(snapshot, nodes[0].TargetID, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			got, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("result=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestPrepareDirectTitleLifecyclePreservesMarkspliceStatePreconditions(t *testing.T) {
	withoutTitle := []byte("[label](dest)\n")
	snapshot, err := Parse(withoutTitle)
	if err != nil {
		t.Fatal(err)
	}
	links, err := snapshot.QueryNodes([]string{"inline_link"}, 4)
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%+v err=%v", links, err)
	}
	if _, err := snapshot.PrepareReplaceInlineLinkTitle(links[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("replace absent title error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareRemoveInlineLinkTitle(links[0].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("remove absent title error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareAddInlineLinkTitle(links[0].TargetID, []byte("bad\ntitle")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("multiline add title error=%v, want ErrInvalidReplacement", err)
	}

	withTitle := []byte("![alt](dest 'old')\n")
	imageSnapshot, err := Parse(withTitle)
	if err != nil {
		t.Fatal(err)
	}
	images, err := imageSnapshot.QueryNodes([]string{"image"}, 4)
	if err != nil || len(images) != 1 {
		t.Fatalf("images=%+v err=%v", images, err)
	}
	if _, err := imageSnapshot.PrepareAddImageTitle(images[0].TargetID, []byte("second")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("add existing title error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareReplaceDirectLinkFamilyPreservesMarkspliceRejections(t *testing.T) {
	source := []byte("[label](path)\n\n![alt](path)\n\n<https://example.test>\n\nparagraph\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	links, err := snapshot.QueryNodes([]string{"inline_link"}, 4)
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%+v err=%v", links, err)
	}
	images, err := snapshot.QueryNodes([]string{"image"}, 4)
	if err != nil || len(images) != 1 {
		t.Fatalf("images=%+v err=%v", images, err)
	}
	autoLinks, err := snapshot.QueryNodes([]string{"autolink"}, 4)
	if err != nil || len(autoLinks) != 1 {
		t.Fatalf("autolinks=%+v err=%v", autoLinks, err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}

	if _, err := snapshot.PrepareReplaceInlineLinkDestination(links[0].TargetID, []byte("bad)tail")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("unsafe inline-link destination error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareReplaceImageAlt(images[0].TargetID, nil); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("empty image alt error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareReplaceAutoLink(autoLinks[0].TargetID, []byte("not-a-link")); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("unsafe autolink error=%v, want ErrInvalidReplacement", err)
	}
	if _, err := snapshot.PrepareReplaceInlineLinkLabel(paragraphs[len(paragraphs)-1].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
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

func TestPrepareReplaceFrontMatterValuePreservesYAMLCRLFAndSourceBinding(t *testing.T) {
	source := []byte("---\r\ntitle: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 2 {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	var titleTarget string
	for _, field := range fields {
		if field.Attributes["key"] == "title" {
			titleTarget = field.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", fields)
	}

	prepared, err := snapshot.PrepareReplaceFrontMatterValue(titleTarget, []byte("New"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("---\r\ntitle: \"New\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply([]byte("---\r\ntitle: \"Old\"\r\nkeep: changed\r\n---\r\n\r\nBody.\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareReplaceFrontMatterValuePreservesTOMLStyleAndNoOp(t *testing.T) {
	source := []byte("+++\ntitle   =   'Old'   # keep this comment\ncount = 1\n+++\n\nBody.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 1 || fields[0].Attributes["key"] != "title" {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	titleTarget := fields[0].TargetID

	prepared, err := snapshot.PrepareReplaceFrontMatterValue(titleTarget, []byte("New"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("+++\ntitle   =   'New'   # keep this comment\ncount = 1\n+++\n\nBody.\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}

	noOp, err := snapshot.PrepareReplaceFrontMatterValue(titleTarget, []byte("Old"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := noOp.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatalf("no-op result=%q want original=%q", unchanged, source)
	}
}

func TestPrepareReplaceFrontMatterValuePreservesMarkspliceRejections(t *testing.T) {
	source := []byte("---\ntitle: Old\n---\n\nParagraph.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 1 {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	targetID := fields[0].TargetID

	for _, replacement := range [][]byte{nil, []byte("line one\nline two")} {
		if _, err := snapshot.PrepareReplaceFrontMatterValue(targetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("replacement=%q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}

	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceFrontMatterValue(paragraphs[0].TargetID, []byte("New")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceFrontMatterValueDoesNotTargetDuplicateKeys(t *testing.T) {
	source := []byte("---\ntitle: one\ntitle: two\nkeep: yes\n---\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0].Attributes["key"] != "keep" {
		t.Fatalf("duplicate-key fields unexpectedly targetable: %+v", fields)
	}
}

func TestPrepareRenameFrontMatterFieldPreservesYAMLCRLFAndSourceBinding(t *testing.T) {
	source := []byte("---\r\ntitle: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 2 {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	var titleTarget string
	for _, field := range fields {
		if field.Attributes["key"] == "title" {
			titleTarget = field.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", fields)
	}

	prepared, err := snapshot.PrepareRenameFrontMatterField(titleTarget, []byte("name"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("---\r\nname: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply([]byte("---\r\ntitle: \"Old\"\r\nkeep: changed\r\n---\r\n\r\nBody.\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareRenameFrontMatterFieldPreservesTOMLStyleAndNoOp(t *testing.T) {
	source := []byte("+++\ntitle   =   'Old'   # keep this comment\ncount = 1\n+++\n\nBody.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 1 || fields[0].Attributes["key"] != "title" {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	titleTarget := fields[0].TargetID

	prepared, err := snapshot.PrepareRenameFrontMatterField(titleTarget, []byte("name"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("+++\nname   =   'Old'   # keep this comment\ncount = 1\n+++\n\nBody.\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}

	noOp, err := snapshot.PrepareRenameFrontMatterField(titleTarget, []byte("title"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := noOp.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatalf("no-op result=%q want original=%q", unchanged, source)
	}
}

func TestPrepareRenameFrontMatterFieldPreservesMarksplicePreconditions(t *testing.T) {
	source := []byte("---\ntitle: Old\nauthor: Ada\n---\n\nParagraph.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 2 {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	var titleTarget string
	for _, field := range fields {
		if field.Attributes["key"] == "title" {
			titleTarget = field.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", fields)
	}
	for _, key := range [][]byte{nil, []byte("bad key"), []byte("bad\nkey"), []byte("author")} {
		if _, err := snapshot.PrepareRenameFrontMatterField(titleTarget, key); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("rename key %q error=%v, want ErrInvalidReplacement", key, err)
		}
	}

	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRenameFrontMatterField(paragraphs[0].TargetID, []byte("name")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveFrontMatterFieldPreservesYAMLCRLFAndSourceBinding(t *testing.T) {
	source := []byte("---\r\ntitle: \"Old\"\r\nauthor: \"Ada\"\r\n---\r\n\r\nBody.\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 2 {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}
	var titleTarget string
	for _, field := range fields {
		if field.Attributes["key"] == "title" {
			titleTarget = field.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", fields)
	}

	prepared, err := snapshot.PrepareRemoveFrontMatterField(titleTarget)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("---\r\nauthor: \"Ada\"\r\n---\r\n\r\nBody.\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply([]byte("---\r\ntitle: \"Old\"\r\nauthor: \"Grace\"\r\n---\r\n\r\nBody.\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareRemoveFrontMatterFieldPreservesTOMLEnvelopeAndUnrelatedSource(t *testing.T) {
	source := []byte("+++\ntitle   =   'Old'   # remove whole line\ncount = 1\n+++\n\nBody.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := snapshot.QueryNodes([]string{"front_matter_field"}, 8)
	if err != nil || len(fields) != 1 || fields[0].Attributes["key"] != "title" {
		t.Fatalf("fields=%+v err=%v", fields, err)
	}

	prepared, err := snapshot.PrepareRemoveFrontMatterField(fields[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("+++\ncount = 1\n+++\n\nBody.\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
}

func TestPrepareRemoveFrontMatterFieldPreservesMarkspliceTargetValidation(t *testing.T) {
	source := []byte("---\ntitle: Old\n---\n\nParagraph.\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRemoveFrontMatterField(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRenameFootnoteDefinitionPreservesBoundReferencesAndMarkspliceSemantics(t *testing.T) {
	source := []byte("Use[^One] and `[^One]` plus [^ghost].\r\n\r\n[^One]: Alpha\r\n[^two]: Beta\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"footnote_definition"}, 8)
	if err != nil || len(definitions) != 2 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}

	prepared, err := snapshot.PrepareRenameFootnoteDefinition(definitions[0].TargetID, []byte("renamed"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("Use[^renamed] and `[^One]` plus [^ghost].\r\n\r\n[^renamed]: Alpha\r\n[^two]: Beta\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}

	noOp, err := snapshot.PrepareRenameFootnoteDefinition(definitions[0].TargetID, []byte("One"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := noOp.Apply(source); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("no-op result=%q err=%v", got, err)
	}
	for _, replacement := range [][]byte{nil, []byte("two"), []byte("bad]label"), []byte("[bad")} {
		if _, err := snapshot.PrepareRenameFootnoteDefinition(definitions[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("rename %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	caseVariant, err := snapshot.PrepareRenameFootnoteDefinition(definitions[0].TargetID, []byte("Two"))
	if err != nil {
		t.Fatalf("case-distinct label unexpectedly rejected: %v", err)
	}
	if got, err := caseVariant.Apply(source); err != nil || !bytes.Contains(got, []byte("[^Two]: Alpha")) {
		t.Fatalf("case-distinct result=%q err=%v", got, err)
	}

	renameSecond, err := snapshot.PrepareRenameFootnoteDefinition(definitions[1].TargetID, []byte("dos"))
	if err != nil {
		t.Fatal(err)
	}
	composed, err := snapshot.ComposeChanges(prepared, renameSecond)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := composed.Apply(source); err != nil || !bytes.Equal(got, []byte("Use[^renamed] and `[^One]` plus [^ghost].\r\n\r\n[^renamed]: Alpha\r\n[^dos]: Beta\r\n")) {
		t.Fatalf("composed result=%q err=%v", got, err)
	}

	stale := append([]byte(nil), source...)
	stale[0] = 'u'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}

	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRenameFootnoteDefinition(paragraphs[0].TargetID, []byte("renamed")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceFootnoteDefinitionBodyPreservesLayoutAndMarkspliceSemantics(t *testing.T) {
	source := []byte("Use[^n] and [^m].\r\n\r\n[^n]: first\r\n\r\n    second\r\n[^m]: keep\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"footnote_definition"}, 8)
	if err != nil || len(definitions) != 2 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}

	prepared, err := snapshot.PrepareReplaceFootnoteDefinitionBody(definitions[0].TargetID, []byte("alpha\n\nbeta"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("Use[^n] and [^m].\r\n\r\n[^n]: alpha\r\n\r\n    beta\r\n[^m]: keep\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}

	noOp, err := snapshot.PrepareReplaceFootnoteDefinitionBody(definitions[0].TargetID, []byte("first\n\nsecond"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := noOp.Apply(source); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("no-op result=%q err=%v", got, err)
	}
	for _, replacement := range [][]byte{nil, []byte(""), []byte("bad\r\nbody")} {
		if _, err := snapshot.PrepareReplaceFootnoteDefinitionBody(definitions[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("replace %q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}

	renameSecond, err := snapshot.PrepareRenameFootnoteDefinition(definitions[1].TargetID, []byte("other"))
	if err != nil {
		t.Fatal(err)
	}
	composed, err := snapshot.ComposeChanges(prepared, renameSecond)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := composed.Apply(source); err != nil || !bytes.Equal(got, []byte("Use[^n] and [^other].\r\n\r\n[^n]: alpha\r\n\r\n    beta\r\n[^other]: keep\r\n")) {
		t.Fatalf("composed result=%q err=%v", got, err)
	}

	stale := append([]byte(nil), source...)
	stale[0] = 'u'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceFootnoteDefinitionBody(paragraphs[0].TargetID, []byte("replacement")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveFootnoteDefinitionPreservesExternalOccurrenceBytesAndSourceBinding(t *testing.T) {
	source := []byte("See[^n] and [^m]\r\n\r\n[^n]: remove\r\n[^m]: keep\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.QueryNodes([]string{"footnote_definition"}, 8)
	if err != nil || len(definitions) != 2 {
		t.Fatalf("definitions=%+v err=%v", definitions, err)
	}
	prepared, err := snapshot.PrepareRemoveFootnoteDefinition(definitions[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("See[^n] and [^m]\r\n\r\n[^m]: keep\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	candidate, err := Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := candidate.QueryNodes([]string{"footnote_definition"}, 8); err != nil || len(got) != 1 {
		t.Fatalf("remaining definitions=%+v err=%v", got, err)
	}
	stale := append([]byte(nil), source...)
	stale[0] = 's'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}

	paragraphSource := []byte("Paragraph.\n\n[^n]: body\n")
	paragraphSnapshot, err := Parse(paragraphSource)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := paragraphSnapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := paragraphSnapshot.PrepareRemoveFootnoteDefinition(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareSyncTOCPreservesManagedSectionAndSourceBinding(t *testing.T) {
	source := []byte("# Root\r\n\r\n## Contents\r\n\r\n- [Root](#old-root)\r\n- [Child](#child)\r\n\r\n## Child\r\nbody\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	headings, err := snapshot.QueryNodes([]string{"heading"}, 8)
	if err != nil || len(headings) != 3 {
		t.Fatalf("headings=%+v err=%v", headings, err)
	}
	prepared, err := snapshot.PrepareSyncTOC(headings[1].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("# Root\r\n\r\n## Contents\r\n\r\n- [Root](#root)\r\n  - [Contents](#contents)\r\n  - [Child](#child)\r\n\r\n## Child\r\nbody\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	stale := append([]byte(nil), source...)
	stale[0] = 'X'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareSyncTOC(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareSetAlertKindPreservesShapeAndSourceBinding(t *testing.T) {
	source := []byte("before\r\n\r\n> [!NOTE]\r\n> body\r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareSetAlertKind(blockquotes[0].TargetID, marksplice.AlertKindWarning)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n> [!WARNING]\r\n> body\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	noOp, err := snapshot.PrepareSetAlertKind(blockquotes[0].TargetID, marksplice.AlertKindNote)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := noOp.Apply(source); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("no-op result=%q err=%v", got, err)
	}
	if _, err := snapshot.PrepareSetAlertKind(blockquotes[0].TargetID, marksplice.AlertKindUnknown); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("unknown kind error=%v, want ErrInvalidReplacement", err)
	}
	stale := append([]byte(nil), source...)
	stale[0] = 'B'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareSetAlertKind(paragraphs[0].TargetID, marksplice.AlertKindTip); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceAlertBodyPreservesMarkerShapeAndSourceBinding(t *testing.T) {
	source := []byte("before\r\n\r\n> [!NOTE]\r\n> old\r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareReplaceAlertBody(blockquotes[0].TargetID, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n> [!NOTE]\r\n> new\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	stale := append([]byte(nil), source...)
	stale[0] = 'B'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}

	noOp, err := snapshot.PrepareReplaceAlertBody(blockquotes[0].TargetID, []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	noOpResult, err := noOp.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(noOpResult, source) {
		t.Fatalf("no-op result=%q want original", noOpResult)
	}
}

func TestPrepareReplaceAlertBodyPreservesUniformMultilineShape(t *testing.T) {
	source := []byte("before\r\n\r\n  > [!WARNING]\r\n  > one\r\n  > two\r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareReplaceAlertBody(blockquotes[0].TargetID, []byte("alpha\nbeta"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n  > [!WARNING]\r\n  > alpha\r\n  > beta\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
}

func TestPrepareReplaceAlertBodyPreservesMarkspliceRejections(t *testing.T) {
	tests := []struct {
		name    string
		source  []byte
		wantErr error
	}{
		{name: "ordinary blockquote", source: []byte("> ordinary\n"), wantErr: marksplice.ErrInvalidTargetKind},
		{name: "marker only", source: []byte("> [!NOTE]\n"), wantErr: marksplice.ErrInvalidTargetKind},
		{name: "lazy continuation", source: []byte("> [!NOTE]\n> body\nafter\n"), wantErr: marksplice.ErrInvalidReplacement},
		{name: "mixed prefix", source: []byte("> [!NOTE]\n> body one\n>body two\n"), wantErr: marksplice.ErrInvalidReplacement},
		{name: "mixed EOL", source: []byte("> [!NOTE]\r\n> one\r\n> two\n"), wantErr: marksplice.ErrInvalidReplacement},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
			if err != nil || len(blockquotes) != 1 {
				t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
			}
			if _, err := snapshot.PrepareReplaceAlertBody(blockquotes[0].TargetID, []byte("new")); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
		})
	}

	snapshot, err := Parse([]byte("Paragraph.\n\n> [!TIP]\n> body\n"))
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	if _, err := snapshot.PrepareReplaceAlertBody(blockquotes[0].TargetID, nil); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("empty replacement error=%v, want ErrInvalidReplacement", err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceAlertBody(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceBlockquoteContentPreservesSourceShapeAndBinding(t *testing.T) {
	source := []byte("before\r\n\r\n> old\r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareReplaceBlockquoteContent(blockquotes[0].TargetID, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n> new\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	stale := append([]byte(nil), source...)
	stale[0] = 'B'
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}

	noOp, err := snapshot.PrepareReplaceBlockquoteContent(blockquotes[0].TargetID, []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	noOpResult, err := noOp.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(noOpResult, source) {
		t.Fatalf("no-op result=%q want original", noOpResult)
	}
	if _, err := snapshot.PrepareReplaceBlockquoteContent(blockquotes[0].TargetID, nil); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("empty replacement error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareReplaceBlockquoteContentPreservesUniformMultilineShape(t *testing.T) {
	source := []byte("before\r\n\r\n> one\r\n> two\r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareReplaceBlockquoteContent(blockquotes[0].TargetID, []byte("alpha\nbeta"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n> alpha\r\n> beta\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
}

func TestPrepareReplaceBlockquoteContentPreservesMarkspliceRejections(t *testing.T) {
	tests := []struct {
		name    string
		source  []byte
		wantErr error
	}{
		{name: "alert", source: []byte("> [!NOTE]\n> body\n"), wantErr: marksplice.ErrInvalidReplacement},
		{name: "lazy continuation", source: []byte("> quoted\nafter\n"), wantErr: marksplice.ErrInvalidReplacement},
		{name: "mixed EOL", source: []byte("> one\r\n> two\n"), wantErr: marksplice.ErrInvalidReplacement},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
			if err != nil || len(blockquotes) != 1 {
				t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
			}
			if _, err := snapshot.PrepareReplaceBlockquoteContent(blockquotes[0].TargetID, []byte("new")); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
		})
	}

	snapshot, err := Parse([]byte("Paragraph.\n\n> quote\n"))
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceBlockquoteContent(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveBlockquotePreservesContainerOwnershipAndSourceBinding(t *testing.T) {
	tests := []struct {
		name   string
		source []byte
		want   []byte
	}{
		{
			name:   "ordinary CRLF",
			source: []byte("before\r\n\r\n> quoted\r\n> second\r\n\r\nafter\r\n"),
			want:   []byte("before\r\n\r\n\r\nafter\r\n"),
		},
		{
			name:   "GitHub alert CRLF",
			source: []byte("before\r\n\r\n> [!NOTE]\r\n> Important.\r\n\r\nafter\r\n"),
			want:   []byte("before\r\n\r\n\r\nafter\r\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
			if err != nil || len(blockquotes) != 1 {
				t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
			}

			prepared, err := snapshot.PrepareRemoveBlockquote(blockquotes[0].TargetID)
			if err != nil {
				t.Fatal(err)
			}
			result, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(result, tt.want) {
				t.Fatalf("result=%q want=%q", result, tt.want)
			}

			stale := append([]byte(nil), tt.source...)
			stale[0] = 'B'
			if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
				t.Fatalf("stale error=%v, want ErrSourceConflict", err)
			}
		})
	}

	snapshot, err := Parse([]byte("Paragraph.\n\n> quote\n"))
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRemoveBlockquote(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareRemoveBlockquoteRemovesMarkspliceOwnedLazyContinuation(t *testing.T) {
	source := []byte("before\n> quoted\nafter\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	blockquotes, err := snapshot.QueryNodes([]string{"blockquote"}, 8)
	if err != nil || len(blockquotes) != 1 {
		t.Fatalf("blockquotes=%+v err=%v", blockquotes, err)
	}
	prepared, err := snapshot.PrepareRemoveBlockquote(blockquotes[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
}

func TestPrepareRemoveThematicBreakPreservesCRLFAndSourceBinding(t *testing.T) {
	source := []byte("before\r\n\r\n  * * *  \r\n\r\nafter\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	breaks, err := snapshot.QueryNodes([]string{"thematic_break"}, 8)
	if err != nil || len(breaks) != 1 {
		t.Fatalf("breaks=%+v err=%v", breaks, err)
	}

	prepared, err := snapshot.PrepareRemoveThematicBreak(breaks[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before\r\n\r\n\r\nafter\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	stale := []byte("before\r\n\r\n  * * *  \r\n\r\nchanged\r\n")
	if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareRemoveThematicBreakFailsClosedOnParagraphJoin(t *testing.T) {
	source := []byte("before\n***\nafter\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	breaks, err := snapshot.QueryNodes([]string{"thematic_break"}, 8)
	if err != nil || len(breaks) != 1 {
		t.Fatalf("breaks=%+v err=%v", breaks, err)
	}
	if _, err := snapshot.PrepareRemoveThematicBreak(breaks[0].TargetID); !errors.Is(err, marksplice.ErrInvalidReplacement) {
		t.Fatalf("join-hazard error=%v, want ErrInvalidReplacement", err)
	}
}

func TestPrepareRemoveThematicBreakPreservesMarkspliceTargetValidation(t *testing.T) {
	source := []byte("Paragraph.\n\n---\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareRemoveThematicBreak(paragraphs[0].TargetID); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceMathExpressionPreservesReviewedStylesAndSourceBinding(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		style       string
		payload     []byte
		replacement []byte
		want        []byte
	}{
		{name: "inline dollar CRLF", source: []byte("before $x + y$ after\r\n"), style: "inline_dollar", payload: []byte("x + y"), replacement: []byte("a + b"), want: []byte("before $a + b$ after\r\n")},
		{name: "inline backtick CRLF", source: []byte("before $`x + y`$ after\r\n"), style: "inline_backtick", payload: []byte("x + y"), replacement: []byte("a + b"), want: []byte("before $`a + b`$ after\r\n")},
		{name: "block dollar CRLF", source: []byte("$$x + y$$\r\n"), style: "block_dollar", payload: []byte("x + y"), replacement: []byte("a + b"), want: []byte("$$a + b$$\r\n")},
		{name: "fenced math multiline", source: []byte("```math\nx + y\n```\n"), style: "fenced_block", payload: []byte("x + y"), replacement: []byte("a + b\nc + d"), want: []byte("```math\na + b\nc + d\n```\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			var targetID string
			if tt.style == "fenced_block" {
				inspect, inspectErr := snapshot.Inspect(4)
				if inspectErr != nil || len(inspect.FencedBlocks) != 1 || inspect.FencedBlocks[0].Language != "math" {
					t.Fatalf("fenced blocks=%+v err=%v", inspect.FencedBlocks, inspectErr)
				}
				targetID = inspect.FencedBlocks[0].TargetID
			} else {
				nodes, queryErr := snapshot.QueryNodes([]string{"math_expression"}, 4)
				if queryErr != nil || len(nodes) != 1 {
					direct := snapshot.document.MathExpressions()
					styles := make([]string, 0, len(direct))
					for _, expression := range direct {
						styles = append(styles, mathExpressionStyleName(expression.Style()))
					}
					t.Fatalf("math expressions=%+v directStyles=%v err=%v", nodes, styles, queryErr)
				}
				if got := nodes[0].Attributes["style"]; got != tt.style {
					t.Fatalf("style=%#v want=%q", got, tt.style)
				}
				targetID = nodes[0].TargetID
			}

			prepared, err := snapshot.PrepareReplaceMathExpression(targetID, tt.replacement)
			if err != nil {
				t.Fatal(err)
			}
			got, err := prepared.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("result=%q want=%q", got, tt.want)
			}

			noOp, err := snapshot.PrepareReplaceMathExpression(targetID, tt.payload)
			if err != nil {
				t.Fatal(err)
			}
			unchanged, err := noOp.Apply(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(unchanged, tt.source) {
				t.Fatalf("no-op result=%q want original=%q", unchanged, tt.source)
			}

			stale := append(append([]byte(nil), tt.source...), []byte("changed")...)
			if _, err := prepared.Apply(stale); !errors.Is(err, marksplice.ErrSourceConflict) {
				t.Fatalf("stale error=%v, want ErrSourceConflict", err)
			}
		})
	}
}

func TestPrepareReplaceMathExpressionPreservesMarkspliceRejections(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		style       string
		replacement []byte
	}{
		{name: "inline dollar delimiter", source: []byte("$x$\n"), style: "inline_dollar", replacement: []byte("bad$split")},
		{name: "inline backtick delimiter", source: []byte("$`x`$\n"), style: "inline_backtick", replacement: []byte("bad`split")},
		{name: "block dollar multiline", source: []byte("$$x$$\n"), style: "block_dollar", replacement: []byte("line one\nline two")},
		{name: "fenced math empty", source: []byte("```math\nx\n```\n"), style: "fenced_block", replacement: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			var targetID string
			if tt.style == "fenced_block" {
				inspect, inspectErr := snapshot.Inspect(4)
				if inspectErr != nil || len(inspect.FencedBlocks) != 1 {
					t.Fatalf("fenced blocks=%+v err=%v", inspect.FencedBlocks, inspectErr)
				}
				targetID = inspect.FencedBlocks[0].TargetID
			} else {
				nodes, queryErr := snapshot.QueryNodes([]string{"math_expression"}, 4)
				if queryErr != nil || len(nodes) != 1 || nodes[0].Attributes["style"] != tt.style {
					t.Fatalf("math expressions=%+v err=%v", nodes, queryErr)
				}
				targetID = nodes[0].TargetID
			}
			if _, err := snapshot.PrepareReplaceMathExpression(targetID, tt.replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
				t.Fatalf("replacement=%q error=%v, want ErrInvalidReplacement", tt.replacement, err)
			}
		})
	}

	snapshot, err := Parse([]byte("Paragraph.\n\n```go\nbody\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 4)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}
	if _, err := snapshot.PrepareReplaceMathExpression(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph target error=%v, want ErrInvalidTargetKind", err)
	}
	inspect, err := snapshot.Inspect(4)
	if err != nil || len(inspect.FencedBlocks) != 1 {
		t.Fatalf("fenced blocks=%+v err=%v", inspect.FencedBlocks, err)
	}
	if _, err := snapshot.PrepareReplaceMathExpression(inspect.FencedBlocks[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("non-math fenced target error=%v, want ErrInvalidTargetKind", err)
	}
}

func TestPrepareReplaceHTMLCommentPreservesWrapperAndSourceBinding(t *testing.T) {
	source := []byte("before <!--  old comment  --> after\r\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := snapshot.QueryNodes([]string{"html_comment"}, 8)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("comments=%+v err=%v", nodes, err)
	}

	prepared, err := snapshot.PrepareReplaceHTMLComment(nodes[0].TargetID, []byte("new comment"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before <!--  new comment  --> after\r\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply([]byte("before <!--  old comment  --> changed\r\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}

	noOp, err := snapshot.PrepareReplaceHTMLComment(nodes[0].TargetID, []byte("old comment"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := noOp.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatalf("no-op result=%q want original=%q", unchanged, source)
	}
}

func TestPrepareReplaceHTMLAnchorPreservesAttributeStyleAndSourceBinding(t *testing.T) {
	source := []byte("before <A class='x' ID=\"old-anchor\">text</A> after\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := snapshot.QueryNodes([]string{"html_anchor"}, 8)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("anchors=%+v err=%v", nodes, err)
	}
	if nodes[0].Attributes["attribute"] != "id" {
		t.Fatalf("anchor attributes=%+v, want id", nodes[0].Attributes)
	}

	prepared, err := snapshot.PrepareReplaceHTMLAnchor(nodes[0].TargetID, []byte("new-anchor"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Apply(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("before <A class='x' ID=\"new-anchor\">text</A> after\n")
	if !bytes.Equal(result, want) {
		t.Fatalf("result=%q want=%q", result, want)
	}
	if _, err := prepared.Apply([]byte("before <A class='x' ID=\"old-anchor\">changed</A> after\n")); !errors.Is(err, marksplice.ErrSourceConflict) {
		t.Fatalf("stale error=%v, want ErrSourceConflict", err)
	}
}

func TestPrepareReplaceRawHTMLPreservesMarkspliceRejections(t *testing.T) {
	source := []byte("paragraph <!-- old --> <a id=\"old-anchor\">x</a>\n")
	snapshot, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	comments, err := snapshot.QueryNodes([]string{"html_comment"}, 8)
	if err != nil || len(comments) != 1 {
		t.Fatalf("comments=%+v err=%v", comments, err)
	}
	anchors, err := snapshot.QueryNodes([]string{"html_anchor"}, 8)
	if err != nil || len(anchors) != 1 {
		t.Fatalf("anchors=%+v err=%v", anchors, err)
	}
	paragraphs, err := snapshot.QueryNodes([]string{"paragraph"}, 8)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("paragraphs=%+v err=%v", paragraphs, err)
	}

	for _, replacement := range [][]byte{nil, []byte("line one\nline two"), []byte("bad --> split")} {
		if _, err := snapshot.PrepareReplaceHTMLComment(comments[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("comment replacement=%q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	for _, replacement := range [][]byte{nil, []byte("line one\nline two"), []byte("bad\"anchor")} {
		if _, err := snapshot.PrepareReplaceHTMLAnchor(anchors[0].TargetID, replacement); !errors.Is(err, marksplice.ErrInvalidReplacement) {
			t.Fatalf("anchor replacement=%q error=%v, want ErrInvalidReplacement", replacement, err)
		}
	}
	if _, err := snapshot.PrepareReplaceHTMLComment(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph comment error=%v, want ErrInvalidTargetKind", err)
	}
	if _, err := snapshot.PrepareReplaceHTMLAnchor(paragraphs[0].TargetID, []byte("new")); !errors.Is(err, marksplice.ErrInvalidTargetKind) {
		t.Fatalf("paragraph anchor error=%v, want ErrInvalidTargetKind", err)
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
