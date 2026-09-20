package filesystem

import (
	"errors"
	"fmt"
	"os"
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
	if err := validateExecutableReplacementInputs(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return err
	}
	if err := prepareExecutableReplacementCandidate(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return fmt.Errorf("prepare executable replacement metadata: %w", err)
	}
	if err := verifyExecutableReplacementIdentities(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return err
	}
	return nil
}

// CommitExecutableReplacementCandidate atomically consumes candidate to replace
// target after preparation. It returns the new target identity.
func CommitExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) (ObjectIdentity, error) {
	if err := validateExecutableReplacementInputs(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return ObjectIdentity{}, err
	}
	if err := verifyExecutableReplacementIdentities(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return ObjectIdentity{}, err
	}
	if err := commitExecutableReplacementCandidate(targetPath, targetIdentity, candidatePath, candidateIdentity); err != nil {
		return ObjectIdentity{}, fmt.Errorf("commit executable replacement: %w", err)
	}
	if _, err := os.Lstat(candidatePath); err == nil {
		return ObjectIdentity{}, errors.New("executable replacement candidate was not consumed")
	} else if !os.IsNotExist(err) {
		return ObjectIdentity{}, fmt.Errorf("inspect consumed executable candidate: %w", err)
	}
	installedIdentity, err := CaptureSingleLinkFileIdentity(targetPath)
	if err != nil {
		return ObjectIdentity{}, fmt.Errorf("capture committed executable identity: %w", err)
	}
	if installedIdentity.StableKey() != candidateIdentity.StableKey() ||
		installedIdentity.VolumeKey() != candidateIdentity.VolumeKey() {
		return ObjectIdentity{}, errors.New("committed executable identity does not match consumed candidate")
	}
	return installedIdentity, nil
}

func validateExecutableReplacementInputs(
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
	return nil
}

func verifyExecutableReplacementIdentities(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	targetMatches, err := targetIdentity.Matches(targetPath)
	if err != nil {
		return err
	}
	if !targetMatches {
		return errors.New("executable target identity changed during replacement operation")
	}
	candidateMatches, err := candidateIdentity.Matches(candidatePath)
	if err != nil {
		return err
	}
	if !candidateMatches {
		return errors.New("executable candidate identity changed during replacement operation")
	}
	return nil
}
