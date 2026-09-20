package filesystem

import (
	"errors"
	"fmt"
)

// PrepareExecutableReplacementCandidate qualifies target metadata and prepares
// candidate for a later same-filesystem atomic executable replacement. It does
// not modify target or move candidate.
func PrepareExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	if targetPath == "" || candidatePath == "" {
		return errors.New("executable replacement paths are required")
	}
	if targetIdentity.key == "" || candidateIdentity.key == "" ||
		targetIdentity.isDir || candidateIdentity.isDir {
		return errors.New("executable replacement file identities are required")
	}
	sameVolume, err := targetIdentity.SameVolume(candidateIdentity)
	if err != nil {
		return err
	}
	if !sameVolume {
		return errors.New("executable replacement candidate is not on the target filesystem")
	}
	if err := prepareExecutableReplacementCandidate(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return fmt.Errorf("prepare executable replacement metadata: %w", err)
	}
	targetMatches, err := targetIdentity.Matches(targetPath)
	if err != nil {
		return err
	}
	if !targetMatches {
		return errors.New("executable target identity changed during replacement preparation")
	}
	candidateMatches, err := candidateIdentity.Matches(candidatePath)
	if err != nil {
		return err
	}
	if !candidateMatches {
		return errors.New("executable candidate identity changed during replacement preparation")
	}
	return nil
}
