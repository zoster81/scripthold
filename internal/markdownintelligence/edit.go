package markdownintelligence

import (
	"fmt"

	"github.com/zoster81/marksplice"
)

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
