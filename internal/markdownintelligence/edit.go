package markdownintelligence

import (
	"bytes"
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
// snapshot and delegates the source-preserving mutation to Marksplice. It fails
// closed when the rename would change an anchor that a resolved local fragment
// relationship currently targets.
func (s *Snapshot) PrepareRenameHeading(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRenameHeading(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	relationships := s.document.LinkRelationships()
	if !hasResolvedLocalFragmentRelationship(relationships) {
		return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
	}
	candidate, err := change.Apply(s.source)
	if err != nil {
		return PreparedChange{}, err
	}
	if bytes.Equal(candidate, s.source) {
		return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
	}
	heading, ok := s.document.Heading(node.ID())
	if !ok {
		return PreparedChange{}, marksplice.ErrInvalidTargetKind
	}
	candidateDocument, err := marksplice.Parse(candidate)
	if err != nil {
		return PreparedChange{}, err
	}
	if headingRenameInvalidatesResolvedFragment(s.document, candidateDocument, heading.Range(), relationships) {
		return PreparedChange{}, fmt.Errorf("%w: heading rename would invalidate a resolved local fragment relationship", marksplice.ErrInvalidReplacement)
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

func hasResolvedLocalFragmentRelationship(relationships []marksplice.LinkRelationship) bool {
	for _, relationship := range relationships {
		if relationship.FragmentStatus() == marksplice.LinkFragmentResolved {
			return true
		}
	}
	return false
}

func headingRenameInvalidatesResolvedFragment(before, after *marksplice.Document, renamedRange marksplice.Range, relationships []marksplice.LinkRelationship) bool {
	for _, relationship := range relationships {
		if relationship.FragmentStatus() != marksplice.LinkFragmentResolved {
			continue
		}
		originalTarget, ok := relationship.FragmentTarget()
		if !ok {
			return true
		}
		candidateTarget, ok := after.ResolveFragment(relationship.Destination())
		if !ok || candidateTarget.Kind() != originalTarget.Kind() || candidateTarget.Value() != originalTarget.Value() {
			return true
		}
		if originalTarget.Kind() == marksplice.FragmentTargetHTMLAnchor {
			anchor, ok := before.HTMLAnchor(originalTarget.NodeID())
			if !ok || rangesOverlap(anchor.Range(), renamedRange) {
				return true
			}
		}
	}
	return headingRenameChangesReferencedAnchor(before, after, relationships)
}

func rangesOverlap(left, right marksplice.Range) bool {
	return left.Start < right.End && right.Start < left.End
}

func headingRenameChangesReferencedAnchor(before, after *marksplice.Document, relationships []marksplice.LinkRelationship) bool {
	beforeAnchors := before.HeadingAnchors()
	afterAnchors := after.HeadingAnchors()
	if len(beforeAnchors) != len(afterAnchors) {
		return true
	}
	changed := make(map[marksplice.NodeID]struct{})
	for index, beforeAnchor := range beforeAnchors {
		if beforeAnchor.Value() != afterAnchors[index].Value() {
			changed[beforeAnchor.HeadingID()] = struct{}{}
		}
	}
	if len(changed) == 0 {
		return false
	}
	for _, relationship := range relationships {
		if relationship.FragmentStatus() != marksplice.LinkFragmentResolved {
			continue
		}
		target, ok := relationship.FragmentTarget()
		if !ok || target.Kind() != marksplice.FragmentTargetHeading {
			continue
		}
		if _, ok := changed[target.NodeID()]; ok {
			return true
		}
	}
	return false
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

// PrepareReplaceCodeSpan resolves the opaque Scripthold code-span target against
// this exact snapshot and delegates source-preserving content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceCodeSpan(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceCodeSpan(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceStrikethrough resolves the opaque Scripthold strikethrough target
// against this exact snapshot and delegates source-preserving content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceStrikethrough(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceStrikethrough(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceEmphasis resolves the opaque Scripthold emphasis target against
// this exact snapshot and delegates source-preserving content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceEmphasis(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceEmphasis(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceStrong resolves the opaque Scripthold strong target against this
// exact snapshot and delegates source-preserving content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceStrong(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceStrong(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceFencedCode resolves the opaque Scripthold fenced-code target
// against this exact snapshot and delegates body replacement to Marksplice.
func (s *Snapshot) PrepareReplaceFencedCode(targetID string, replacement []byte) (PreparedChange, error) {
	id, err := s.fencedBlockID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceFencedCode(id, replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareSetFencedBlockInfo resolves the opaque Scripthold fenced-code target
// against this exact snapshot and delegates info-string mutation to Marksplice.
func (s *Snapshot) PrepareSetFencedBlockInfo(targetID string, info []byte) (PreparedChange, error) {
	id, err := s.fencedBlockID(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareSetFencedBlockInfo(id, info)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceInlineLinkDestination resolves the opaque Scripthold inline-link
// target against this exact snapshot and delegates destination replacement to Marksplice.
func (s *Snapshot) PrepareReplaceInlineLinkDestination(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceInlineLinkDestination(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceInlineLinkLabel resolves the opaque Scripthold inline-link target
// against this exact snapshot and delegates label replacement to Marksplice.
func (s *Snapshot) PrepareReplaceInlineLinkLabel(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceInlineLinkLabel(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceImageDestination resolves the opaque Scripthold image target
// against this exact snapshot and delegates destination replacement to Marksplice.
func (s *Snapshot) PrepareReplaceImageDestination(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceImageDestination(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceImageAlt resolves the opaque Scripthold image target against this
// exact snapshot and delegates alt-text replacement to Marksplice.
func (s *Snapshot) PrepareReplaceImageAlt(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceImageAlt(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRenameReferenceDefinition resolves the opaque Scripthold
// reference-definition target against this exact snapshot and delegates coordinated
// definition/occurrence renaming to Marksplice.
func (s *Snapshot) PrepareRenameReferenceDefinition(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRenameReferenceDefinition(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveReferenceDefinition resolves the opaque Scripthold
// reference-definition target against this exact snapshot and delegates guarded
// definition removal to Marksplice.
func (s *Snapshot) PrepareRemoveReferenceDefinition(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveReferenceDefinition(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceReferenceDefinitionDestination resolves the opaque Scripthold
// reference-definition target against this exact snapshot and delegates destination
// replacement to Marksplice.
func (s *Snapshot) PrepareReplaceReferenceDefinitionDestination(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceReferenceDefinitionDestination(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceReferenceDefinitionTitle resolves the opaque Scripthold
// reference-definition target and delegates existing-title replacement to Marksplice.
func (s *Snapshot) PrepareReplaceReferenceDefinitionTitle(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceReferenceDefinitionTitle(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareAddReferenceDefinitionTitle resolves the opaque Scripthold
// reference-definition target and delegates absent-title insertion to Marksplice.
func (s *Snapshot) PrepareAddReferenceDefinitionTitle(targetID string, title []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareAddReferenceDefinitionTitle(node.ID(), title)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveReferenceDefinitionTitle resolves the opaque Scripthold
// reference-definition target and delegates owned-title removal to Marksplice.
func (s *Snapshot) PrepareRemoveReferenceDefinitionTitle(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveReferenceDefinitionTitle(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceFrontMatterValue resolves the opaque Scripthold front-matter
// field target against this exact snapshot and delegates value replacement to Marksplice.
func (s *Snapshot) PrepareReplaceFrontMatterValue(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceFrontMatterValue(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRenameFrontMatterField resolves the opaque Scripthold front-matter
// field target against this exact snapshot and delegates key renaming to Marksplice.
func (s *Snapshot) PrepareRenameFrontMatterField(targetID string, key []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRenameFrontMatterField(node.ID(), key)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveFrontMatterField resolves the opaque Scripthold front-matter
// field target against this exact snapshot and delegates physical-line removal to Marksplice.
func (s *Snapshot) PrepareRemoveFrontMatterField(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveFrontMatterField(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRenameFootnoteDefinition resolves the opaque Scripthold footnote-definition
// target against this exact snapshot and delegates coordinated definition/reference renaming to Marksplice.
func (s *Snapshot) PrepareRenameFootnoteDefinition(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRenameFootnote(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceFootnoteDefinitionBody resolves the opaque Scripthold footnote-definition
// target against this exact snapshot and delegates logical multiline body replacement to Marksplice.
func (s *Snapshot) PrepareReplaceFootnoteDefinitionBody(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceFootnoteDefinitionBodyMultiline(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveFootnoteDefinition resolves the opaque Scripthold footnote-definition
// target against this exact snapshot and delegates complete-container removal to Marksplice.
func (s *Snapshot) PrepareRemoveFootnoteDefinition(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveFootnoteDefinition(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareSyncTOC resolves the opaque Scripthold heading target against this
// exact snapshot and delegates managed-TOC synchronization to Marksplice.
func (s *Snapshot) PrepareSyncTOC(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareSyncTOC(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareSetAlertKind resolves the opaque Scripthold blockquote target
// against this exact snapshot and delegates GitHub-alert kind mutation to Marksplice.
func (s *Snapshot) PrepareSetAlertKind(targetID string, kind marksplice.AlertKind) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareSetAlertKind(node.ID(), kind)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceAlertBody resolves the opaque Scripthold blockquote target
// against this exact snapshot and delegates GitHub-alert body replacement to Marksplice.
func (s *Snapshot) PrepareReplaceAlertBody(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceAlertBody(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceBlockquoteContent resolves the opaque Scripthold blockquote
// target against this exact snapshot and delegates content replacement to Marksplice.
func (s *Snapshot) PrepareReplaceBlockquoteContent(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceBlockquoteContent(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveBlockquote resolves the opaque Scripthold blockquote target
// against this exact snapshot and delegates complete-container removal to Marksplice.
func (s *Snapshot) PrepareRemoveBlockquote(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveBlockquote(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveThematicBreak resolves the opaque Scripthold thematic-break
// target against this exact snapshot and delegates exact-line removal to Marksplice.
func (s *Snapshot) PrepareRemoveThematicBreak(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveThematicBreak(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceMathExpression resolves the opaque Scripthold math-expression target
// against this exact snapshot and delegates payload replacement to Marksplice.
func (s *Snapshot) PrepareReplaceMathExpression(targetID string, replacement []byte) (PreparedChange, error) {
	resolved, ok, err := s.resolveNodeTarget(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	var id marksplice.NodeID
	if ok {
		id = resolved.node.ID()
	} else {
		id, err = s.fencedBlockID(targetID)
		if err != nil {
			return PreparedChange{}, err
		}
		expression, exists := s.document.MathExpression(id)
		if !exists || expression.Style() != marksplice.MathExpressionFencedBlock {
			return PreparedChange{}, marksplice.ErrInvalidTargetKind
		}
	}
	change, err := s.document.PrepareReplaceMathExpression(id, replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceHTMLComment resolves the opaque Scripthold HTML comment target
// against this exact snapshot and delegates payload replacement to Marksplice.
func (s *Snapshot) PrepareReplaceHTMLComment(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceHTMLComment(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceHTMLAnchor resolves the opaque Scripthold HTML anchor target
// against this exact snapshot and delegates id/name value replacement to Marksplice.
func (s *Snapshot) PrepareReplaceHTMLAnchor(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceHTMLAnchor(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceAutoLink resolves the opaque Scripthold autolink target against
// this exact snapshot and delegates value replacement to Marksplice.
func (s *Snapshot) PrepareReplaceAutoLink(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceAutoLink(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceInlineLinkTitle resolves the opaque Scripthold inline-link
// target against this exact snapshot and delegates existing-title replacement to Marksplice.
func (s *Snapshot) PrepareReplaceInlineLinkTitle(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceInlineLinkTitle(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareAddInlineLinkTitle resolves the opaque Scripthold inline-link target
// against this exact snapshot and delegates absent-title insertion to Marksplice.
func (s *Snapshot) PrepareAddInlineLinkTitle(targetID string, title []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareAddInlineLinkTitle(node.ID(), title)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveInlineLinkTitle resolves the opaque Scripthold inline-link target
// against this exact snapshot and delegates owned-title removal to Marksplice.
func (s *Snapshot) PrepareRemoveInlineLinkTitle(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveInlineLinkTitle(node.ID())
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareReplaceImageTitle resolves the opaque Scripthold image target against
// this exact snapshot and delegates existing-title replacement to Marksplice.
func (s *Snapshot) PrepareReplaceImageTitle(targetID string, replacement []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareReplaceImageTitle(node.ID(), replacement)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareAddImageTitle resolves the opaque Scripthold image target against this
// exact snapshot and delegates absent-title insertion to Marksplice.
func (s *Snapshot) PrepareAddImageTitle(targetID string, title []byte) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareAddImageTitle(node.ID(), title)
	if err != nil {
		return PreparedChange{}, err
	}
	return PreparedChange{change: change, sourceFingerprint: s.fingerprint}, nil
}

// PrepareRemoveImageTitle resolves the opaque Scripthold image target against
// this exact snapshot and delegates owned-title removal to Marksplice.
func (s *Snapshot) PrepareRemoveImageTitle(targetID string) (PreparedChange, error) {
	node, err := s.targetNode(targetID)
	if err != nil {
		return PreparedChange{}, err
	}
	change, err := s.document.PrepareRemoveImageTitle(node.ID())
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

func (s *Snapshot) fencedBlockID(fencedBlockTargetID string) (marksplice.NodeID, error) {
	var zero marksplice.NodeID
	if s == nil || s.document == nil {
		return zero, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if !validTargetID(fencedBlockTargetID) {
		return zero, fmt.Errorf("%w: markdown fenced-block target is invalid", marksplice.ErrNodeNotFound)
	}
	s.fencedBlockTargetIndexOnce.Do(func() {
		blocks := s.document.FencedBlocks()
		if len(blocks) > maxTargetScanNodes {
			s.fencedBlockTargetIndexErr = fmt.Errorf("%w: markdown fenced-block target scan exceeds %d blocks", marksplice.ErrInvalidQuery, maxTargetScanNodes)
			return
		}
		index := make(map[string]marksplice.NodeID, len(blocks))
		for _, block := range blocks {
			key := targetID(s.fingerprint, "fenced_block", block.Range())
			if _, exists := index[key]; !exists {
				index[key] = block.ID()
			}
		}
		s.fencedBlockTargetIndex = index
	})
	if s.fencedBlockTargetIndexErr != nil {
		return zero, s.fencedBlockTargetIndexErr
	}
	if id, ok := s.fencedBlockTargetIndex[fencedBlockTargetID]; ok {
		return id, nil
	}
	resolved, ok, err := s.resolveNodeTarget(fencedBlockTargetID)
	if err != nil {
		return zero, err
	}
	if ok {
		if resolved.node.Kind() != marksplice.KindFencedCode {
			return zero, marksplice.ErrInvalidTargetKind
		}
		if _, exists := s.document.FencedBlock(resolved.node.ID()); !exists {
			return zero, marksplice.ErrInvalidTargetKind
		}
		return resolved.node.ID(), nil
	}
	return zero, fmt.Errorf("%w: markdown fenced-block target was not found", marksplice.ErrNodeNotFound)
}

func (s *Snapshot) sectionHeadingID(sectionTargetID string) (marksplice.NodeID, error) {
	var zero marksplice.NodeID
	if s == nil || s.document == nil {
		return zero, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if !validTargetID(sectionTargetID) {
		return zero, fmt.Errorf("%w: markdown section target is invalid", marksplice.ErrNodeNotFound)
	}
	s.sectionTargetIndexOnce.Do(func() {
		sections, err := s.document.QuerySections(marksplice.SectionQuery{Limit: maxTargetScanNodes + 1})
		if err != nil {
			s.sectionTargetIndexErr = err
			return
		}
		if len(sections) > maxTargetScanNodes {
			s.sectionTargetIndexErr = fmt.Errorf("%w: markdown section target scan exceeds %d sections", marksplice.ErrInvalidQuery, maxTargetScanNodes)
			return
		}
		index := make(map[string]marksplice.NodeID, len(sections))
		for _, section := range sections {
			key := targetID(s.fingerprint, "section", section.Range())
			if _, exists := index[key]; !exists {
				index[key] = section.HeadingID()
			}
		}
		s.sectionTargetIndex = index
	})
	if s.sectionTargetIndexErr != nil {
		return zero, s.sectionTargetIndexErr
	}
	if headingID, ok := s.sectionTargetIndex[sectionTargetID]; ok {
		return headingID, nil
	}
	if _, ok, err := s.resolveNodeTarget(sectionTargetID); err != nil {
		return zero, err
	} else if ok {
		return zero, marksplice.ErrInvalidTargetKind
	}
	return zero, fmt.Errorf("%w: markdown section target was not found", marksplice.ErrNodeNotFound)
}

func (s *Snapshot) targetNode(targetID string) (marksplice.Node, error) {
	if !validTargetID(targetID) {
		return marksplice.Node{}, fmt.Errorf("%w: markdown target is invalid", marksplice.ErrNodeNotFound)
	}
	resolved, ok, err := s.resolveNodeTarget(targetID)
	if err != nil {
		return marksplice.Node{}, err
	}
	if !ok {
		return marksplice.Node{}, fmt.Errorf("%w: markdown target was not found", marksplice.ErrNodeNotFound)
	}
	return resolved.node, nil
}
