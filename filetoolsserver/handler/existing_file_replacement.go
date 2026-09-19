package handler

import "github.com/zoster81/scripthold/internal/filesystem"

// preparedExistingFileReplacement is a non-owning view of one approved
// replacement of an existing regular file. The preview that created the view
// retains ownership of resultData and the file-identity slot.
type preparedExistingFileReplacement struct {
	requestedPath     string
	resolvedPath      string
	resultData        []byte
	targetFingerprint string
	resultFingerprint string
	identitySlot      **filesystem.FileIdentity
	changed           bool
	forceWritable     bool
}

func preparedPatchPackageReplacement(target *preparedPatchPackageTarget) preparedExistingFileReplacement {
	return preparedExistingFileReplacement{
		requestedPath:     target.requestedPath,
		resolvedPath:      target.resolvedPath,
		resultData:        target.prepared.data,
		targetFingerprint: target.prepared.targetFingerprint,
		resultFingerprint: target.prepared.resultFingerprint,
		identitySlot:      &target.prepared.identityFile,
		changed:           target.prepared.changed,
		forceWritable:     target.prepared.forceWritable,
	}
}

func (replacement preparedExistingFileReplacement) identity() *filesystem.FileIdentity {
	if replacement.identitySlot == nil {
		return nil
	}
	return *replacement.identitySlot
}

func (replacement preparedExistingFileReplacement) closeIdentity() error {
	identity := replacement.identity()
	if identity == nil {
		return nil
	}
	if err := identity.Close(); err != nil {
		return err
	}
	*replacement.identitySlot = nil
	return nil
}
