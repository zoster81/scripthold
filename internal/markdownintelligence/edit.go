package markdownintelligence

import (
	"fmt"

	"github.com/zoster81/marksplice"
)

// MaxEditOperations bounds one atomic Markdown edit plan before Marksplice
// composition so caller-controlled planning work remains finite.
const MaxEditOperations = 64

// PreparedChange wraps one Marksplice ChangeSet without exposing Marksplice
// snapshot-local node identity. It remains bound to the exact parsed source.
type PreparedChange struct {
	change            marksplice.ChangeSet
	sourceFingerprint string
}

// SourceFingerprint reports the exact Markdown snapshot for which this change
// was prepared.
func (p PreparedChange) SourceFingerprint() string {
	return p.sourceFingerprint
}

// Apply delegates source-conflict enforcement to Marksplice.
func (p PreparedChange) Apply(source []byte) ([]byte, error) {
	return p.change.Apply(source)
}

// ComposeChanges delegates atomic multi-edit composition to Marksplice. Every
// constituent change must have been prepared from this exact snapshot.
func (s *Snapshot) ComposeChanges(changes ...PreparedChange) (PreparedChange, error) {
	if s == nil || s.document == nil {
		return PreparedChange{}, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if len(changes) == 0 || len(changes) > MaxEditOperations {
		return PreparedChange{}, fmt.Errorf("%w: markdown edit requires 1..%d prepared changes", marksplice.ErrInvalidQuery, MaxEditOperations)
	}
	markspliceChanges := make([]marksplice.ChangeSet, len(changes))
	for index, prepared := range changes {
		if prepared.sourceFingerprint != s.fingerprint {
			return PreparedChange{}, fmt.Errorf("%w: prepared change belongs to a different markdown snapshot", marksplice.ErrSourceConflict)
		}
		markspliceChanges[index] = prepared.change
	}
	combined, err := s.document.ComposeChanges(markspliceChanges...)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: combined, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRenameHeading resolves the opaque Scripthold target against this exact
// snapshot and delegates the source-preserving mutation to Marksplice.
func (s *Snapshot) PrepareRenameHeading(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRenameHeading(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareSetHeadingLevel resolves the opaque Scripthold target against this
// exact snapshot and delegates the source-preserving hierarchy change to Marksplice.
func (s *Snapshot) PrepareSetHeadingLevel(targetID string, level int) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareSetHeadingLevel(node.ID(), level)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareSetTaskChecked resolves the opaque Scripthold task target against this
// exact snapshot and delegates the one-byte checkbox-state mutation to Marksplice.
func (s *Snapshot) PrepareSetTaskChecked(targetID string, checked bool) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareSetTaskChecked(node.ID(), checked)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceSection resolves the opaque Scripthold section target against
// this exact snapshot and delegates complete subtree replacement to Marksplice.
func (s *Snapshot) PrepareReplaceSection(targetID string, replacement []byte) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceSection(headingID, replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertSectionBefore resolves the opaque Scripthold section anchor
// against this exact snapshot and delegates sibling subtree insertion to Marksplice.
func (s *Snapshot) PrepareInsertSectionBefore(targetID string, fragment []byte) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertSectionBefore(headingID, fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertSectionAfter resolves the opaque Scripthold section anchor
// against this exact snapshot and delegates sibling subtree insertion to Marksplice.
func (s *Snapshot) PrepareInsertSectionAfter(targetID string, fragment []byte) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertSectionAfter(headingID, fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareAppendSectionChild resolves the opaque Scripthold parent section
// against this exact snapshot and delegates direct-child subtree insertion to Marksplice.
func (s *Snapshot) PrepareAppendSectionChild(targetID string, fragment []byte) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareAppendSectionChild(headingID, fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareMoveSectionBefore resolves the opaque Scripthold source and anchor sections
// against this exact snapshot and delegates complete subtree movement to Marksplice.
func (s *Snapshot) PrepareMoveSectionBefore(targetID, anchorTargetID string) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	anchorHeadingID, err := s.sectionHeadingID(anchorTargetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareMoveSectionBefore(headingID, anchorHeadingID)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareMoveSectionAfter resolves the opaque Scripthold source and anchor sections
// against this exact snapshot and delegates complete subtree movement to Marksplice.
func (s *Snapshot) PrepareMoveSectionAfter(targetID, anchorTargetID string) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	anchorHeadingID, err := s.sectionHeadingID(anchorTargetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareMoveSectionAfter(headingID, anchorHeadingID)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceSectionBody resolves the opaque Scripthold section target
// against this exact snapshot and delegates direct-body replacement to Marksplice.
func (s *Snapshot) PrepareReplaceSectionBody(targetID string, replacement []byte) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceSectionBody(headingID, replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveSection resolves the opaque Scripthold section target against
// this exact snapshot and delegates complete subtree removal to Marksplice.
func (s *Snapshot) PrepareRemoveSection(targetID string) (PreparedChange, error) {
	headingID, err := s.sectionHeadingID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveSection(headingID)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceListItem resolves the opaque Scripthold list-item target against
// this exact snapshot and delegates source-preserving item-content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceListItem(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceListItem(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceListItemSubtree resolves the opaque Scripthold list-item target
// against this exact snapshot and delegates complete subtree replacement to Marksplice.
func (s *Snapshot) PrepareReplaceListItemSubtree(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceListItemSubtree(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertListItemBefore resolves the opaque Scripthold list-item anchor
// against this exact snapshot and delegates sibling subtree insertion to Marksplice.
func (s *Snapshot) PrepareInsertListItemBefore(targetID string, fragment []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertListItemBefore(node.ID(), fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertListItemAfter resolves the opaque Scripthold list-item anchor
// against this exact snapshot and delegates sibling subtree insertion to Marksplice.
func (s *Snapshot) PrepareInsertListItemAfter(targetID string, fragment []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertListItemAfter(node.ID(), fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareAppendListItemChild resolves the opaque Scripthold parent target against
// this exact snapshot and delegates validated child-subtree insertion to Marksplice.
func (s *Snapshot) PrepareAppendListItemChild(targetID string, fragment []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareAppendListItemChild(node.ID(), fragment)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareMoveListItemBefore resolves source and anchor list-item targets against
// this exact snapshot and delegates complete subtree movement to Marksplice.
func (s *Snapshot) PrepareMoveListItemBefore(targetID, anchorTargetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	anchor, err := s.targetNode(anchorTargetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareMoveListItemBefore(node.ID(), anchor.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareMoveListItemAfter resolves source and anchor list-item targets against
// this exact snapshot and delegates complete subtree movement to Marksplice.
func (s *Snapshot) PrepareMoveListItemAfter(targetID, anchorTargetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	anchor, err := s.targetNode(anchorTargetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareMoveListItemAfter(node.ID(), anchor.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveListItem resolves the opaque Scripthold list-item target against
// this exact snapshot and delegates complete supported subtree removal to Marksplice.
func (s *Snapshot) PrepareRemoveListItem(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveListItem(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceParagraph resolves the opaque Scripthold target against this
// exact snapshot and delegates source-preserving paragraph replacement to Marksplice.
func (s *Snapshot) PrepareReplaceParagraph(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceParagraph(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertParagraphBefore resolves the opaque Scripthold anchor against
// this exact snapshot and delegates source-preserving paragraph insertion to Marksplice.
func (s *Snapshot) PrepareInsertParagraphBefore(targetID string, markdown []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertParagraphBefore(node.ID(), markdown)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareInsertParagraphAfter resolves the opaque Scripthold anchor against
// this exact snapshot and delegates source-preserving paragraph insertion to Marksplice.
func (s *Snapshot) PrepareInsertParagraphAfter(targetID string, markdown []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareInsertParagraphAfter(node.ID(), markdown)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveParagraph resolves the opaque Scripthold target against this
// exact snapshot and delegates source-preserving paragraph removal to Marksplice.
func (s *Snapshot) PrepareRemoveParagraph(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveParagraph(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

func (s *Snapshot) sectionHeadingID(sectionTargetID string) (marksplice.NodeID, error) {
	var zero marksplice.NodeID
	if s == nil || s.document == nil {
		return zero, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if !validTargetID(sectionTargetID) {
		return zero, fmt.Errorf("%w: markdown section target is invalid", marksplice.ErrNodeNotFound)
	}
	sections, err := s.document.QuerySections(marksplice.SectionQuery{Limit: maxTargetScanNodes + 1})
	if err != nil {
		return zero, err
	}
	if len(sections) > maxTargetScanNodes {
		return zero, fmt.Errorf("%w: markdown section target scan exceeds %d sections", marksplice.ErrInvalidQuery, maxTargetScanNodes)
	}
	for _, section := range sections {
		if targetID(s.fingerprint, "section", section.Range()) == sectionTargetID {
			return section.HeadingID(), nil
		}
	}
	if _, ok, err := s.resolveNodeTarget(sectionTargetID); err != nil {
		return zero, err
	} else if ok {
		return zero, marksplice.ErrInvalidTargetKind
	}
	return zero, fmt.Errorf("%w: markdown section target was not found", marksplice.ErrNodeNotFound)
}

func (s *Snapshot) targetNode(targetID string) (marksplice.Node, error) {
	if s == nil || s.document == nil {
		return marksplice.Node{}, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if !validTargetID(targetID) {
		return marksplice.Node{}, fmt.Errorf("%w: markdown target is invalid", marksplice.ErrNodeNotFound)
	}
	matches, err := s.document.QueryNodes(marksplice.NodeQuery{Limit: maxTargetScanNodes + 1})
	if err != nil {
		return marksplice.Node{}, err
	}
	if len(matches) > maxTargetScanNodes {
		return marksplice.Node{}, fmt.Errorf("%w: markdown target scan exceeds %d nodes", marksplice.ErrInvalidQuery, maxTargetScanNodes)
	}
	for _, match := range matches {
		if s.summarize(match).TargetID == targetID {
			return match.Node(), nil
		}
	}
	return marksplice.Node{}, fmt.Errorf("%w: markdown target was not found", marksplice.ErrNodeNotFound)
}
