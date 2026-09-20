//go:build windows

package filesystem

func prepareExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	target, err := OpenVerifiedSingleLinkFile(targetPath, targetIdentity)
	if err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	candidate, err := OpenVerifiedSingleLinkFile(candidatePath, candidateIdentity)
	if err != nil {
		return err
	}
	if err := candidate.Close(); err != nil {
		return err
	}
	// ReplaceFileW preserves target security metadata during the later commit.
	// The commit primitive will reopen the candidate with write authority and
	// FlushFileBuffers immediately before replacement.
	return ValidateOwnerOnlyExecutable(candidatePath)
}
