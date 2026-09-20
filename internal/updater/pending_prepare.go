package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

const (
	knownGoodArtifactName = "known-good"
	candidateArtifactName = "candidate"
	helperArtifactName    = "helper"
)

type pendingPreparationDeps struct {
	installedEvidence  installedEvidenceDeps
	validateCandidate  func(context.Context, *PreparedCandidate, string, string) error
	newTransactionID   func() (string, error)
	beforePendingWrite func() error
}

type publishedArtifact struct {
	path     string
	snapshot filesystem.FileSnapshot
}

// PreparePendingTransaction durably publishes one verified local transaction
// without starting a helper or modifying the installed executable.
func PreparePendingTransaction(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	admission *ProcessAdmission,
	candidate *PreparedCandidate,
) error {
	_, err := preparePendingTransactionWith(ctx, boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH, pendingPreparationDeps{})
	return err
}

func preparePendingTransactionWith(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	admission *ProcessAdmission,
	candidate *PreparedCandidate,
	goos, goarch string,
	deps pendingPreparationDeps,
) (_ installationState, err error) {
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return installationState{}, err
	}
	if err := admission.validateFor(boundary); err != nil {
		return installationState{}, err
	}
	if deps.validateCandidate == nil {
		deps.validateCandidate = validatePreparedCandidateForPending
	}
	if deps.newTransactionID == nil {
		deps.newTransactionID = generateTransactionID
	}
	if err := deps.validateCandidate(ctx, candidate, goos, goarch); err != nil {
		return installationState{}, fmt.Errorf("revalidate prepared candidate: %w", err)
	}

	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		return installationState{}, fmt.Errorf("acquire installation control lock: %w", err)
	}
	defer func() {
		if closeErr := control.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return installationState{}, fmt.Errorf("validate installation control lock: %w", err)
	}
	if err := admission.validateFor(boundary); err != nil {
		return installationState{}, err
	}

	stableState, err := readInstallationStateLocked(boundary, inspection)
	if err != nil {
		return installationState{}, err
	}
	if stableState.Pending != nil {
		return installationState{}, errors.New("an update transaction is already pending")
	}
	observed, err := observeInstalledBinary(ctx, inspection, goos, goarch, deps.installedEvidence)
	if err != nil {
		return installationState{}, err
	}
	if observed != stableState.Current {
		return installationState{}, errors.New("installed executable does not match stable current state")
	}
	if candidate == nil || !isNewerVersion(candidate.Version, stableState.Current.Version) {
		return installationState{}, errors.New("prepared candidate is not newer than current state")
	}

	transactionID, err := deps.newTransactionID()
	if err != nil {
		return installationState{}, fmt.Errorf("generate transaction ID: %w", err)
	}
	pending := installationPendingState{
		TransactionID:    transactionID,
		SourceVersion:    stableState.Current.Version,
		SourceSHA256:     stableState.Current.SHA256,
		CandidateVersion: candidate.Version,
		CandidateSHA256:  candidate.SHA256,
		CandidateBuild: installationBuildIdentity{
			Module:   officialModulePath,
			GOOS:     goos,
			GOARCH:   goarch,
			VCS:      "git",
			Revision: candidate.Commit,
			VCSClean: true,
		},
		ReleaseID: candidate.ReleaseID,
		AssetID:   candidate.AssetID,
		Tag:       candidate.Tag,
		Commit:    candidate.Commit,
	}
	nextState := stableState
	nextState.Pending = &pending
	if err := validateInstallationState(nextState, inspection); err != nil {
		return installationState{}, err
	}

	paths := []string{
		filepath.Join(boundary.Directory, knownGoodArtifactName),
		filepath.Join(boundary.Directory, candidateArtifactName),
		filepath.Join(boundary.Directory, helperArtifactName),
	}
	for _, path := range paths {
		if _, statErr := os.Lstat(path); statErr == nil {
			return installationState{}, fmt.Errorf("pending artifact already exists: %s", filepath.Base(path))
		} else if !os.IsNotExist(statErr) {
			return installationState{}, fmt.Errorf("inspect pending artifact %s: %w", filepath.Base(path), statErr)
		}
	}

	var published []publishedArtifact
	rollback := true
	defer func() {
		if !rollback {
			return
		}
		for index := len(published) - 1; index >= 0; index-- {
			artifact := published[index]
			if validateErr := filesystem.ValidateOwnerOnlyExecutable(artifact.path); validateErr != nil {
				err = errors.Join(err, fmt.Errorf("validate rollback artifact %s: %w", filepath.Base(artifact.path), validateErr))
				continue
			}
			if removeErr := filesystem.RemoveFile(artifact.path, &artifact.snapshot); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback artifact %s: %w", filepath.Base(artifact.path), removeErr))
			}
		}
	}()

	knownGoodPath := paths[0]
	knownGood, err := publishVerifiedArtifact(
		ctx,
		inspection.ExecutablePath,
		inspection.ExecutableIdentity,
		0,
		stableState.Current.SHA256,
		knownGoodPath,
	)
	if err != nil {
		return installationState{}, fmt.Errorf("publish known-good artifact: %w", err)
	}
	published = append(published, knownGood)

	candidateIdentity, err := filesystem.CaptureSingleLinkFileIdentity(candidate.Path())
	if err != nil {
		return installationState{}, fmt.Errorf("capture prepared candidate identity: %w", err)
	}
	candidateArtifact, err := publishVerifiedArtifact(
		ctx,
		candidate.Path(),
		candidateIdentity,
		candidate.Size,
		candidate.SHA256,
		paths[1],
	)
	if err != nil {
		return installationState{}, fmt.Errorf("publish candidate artifact: %w", err)
	}
	published = append(published, candidateArtifact)

	knownGoodIdentity, err := filesystem.CaptureSingleLinkFileIdentity(knownGoodPath)
	if err != nil {
		return installationState{}, fmt.Errorf("capture known-good identity: %w", err)
	}
	helperArtifact, err := publishVerifiedArtifact(
		ctx,
		knownGoodPath,
		knownGoodIdentity,
		knownGood.snapshot.Size,
		stableState.Current.SHA256,
		paths[2],
	)
	if err != nil {
		return installationState{}, fmt.Errorf("publish helper artifact: %w", err)
	}
	published = append(published, helperArtifact)

	if deps.beforePendingWrite != nil {
		if err := deps.beforePendingWrite(); err != nil {
			return installationState{}, err
		}
	}
	observed, err = observeInstalledBinary(ctx, inspection, goos, goarch, deps.installedEvidence)
	if err != nil {
		return installationState{}, err
	}
	if observed != stableState.Current {
		return installationState{}, errors.New("installed executable changed during transaction preparation")
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return installationState{}, fmt.Errorf("revalidate control lock before pending publication: %w", err)
	}
	if err := admission.validateFor(boundary); err != nil {
		return installationState{}, err
	}
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return installationState{}, err
	}

	// From this point onward, retain artifacts on any error because the pending
	// state publication may have crossed its durable commit boundary.
	rollback = false
	if err := persistInstallationStateLocked(boundary, inspection, nextState, false, &stableState); err != nil {
		return installationState{}, fmt.Errorf("publish pending installation state: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return installationState{}, fmt.Errorf("revalidate control lock after pending publication: %w", err)
	}
	return nextState, nil
}

func validatePreparedCandidateForPending(ctx context.Context, candidate *PreparedCandidate, goos, goarch string) error {
	if candidate == nil || candidate.Path() == "" {
		return errors.New("prepared candidate is unavailable")
	}
	if candidate.ReleaseID <= 0 || candidate.AssetID <= 0 || candidate.Size <= 0 || candidate.Size > maxSelfUpdateAssetBytes {
		return errors.New("prepared candidate release evidence is invalid")
	}
	if candidate.Tag != "v"+candidate.Version {
		return errors.New("prepared candidate tag does not match version")
	}
	version, ok := parseSemanticVersion(candidate.Version)
	if !ok || len(version.prerelease) != 0 {
		return errors.New("prepared candidate version is not stable semantic version")
	}
	if err := validateGitObjectSHA(candidate.Commit); err != nil {
		return fmt.Errorf("prepared candidate commit is invalid: %w", err)
	}
	if _, err := parseGitHubSHA256("sha256:" + candidate.SHA256); err != nil {
		return fmt.Errorf("prepared candidate digest is invalid: %w", err)
	}
	name, err := expectedBinaryAssetName(goos, goarch)
	if err != nil {
		return err
	}
	asset := releaseAsset{
		ID:     candidate.AssetID,
		Name:   name,
		State:  "uploaded",
		Size:   candidate.Size,
		Digest: "sha256:" + candidate.SHA256,
	}
	if err := verifyCandidateFile(candidate.Path(), asset); err != nil {
		return err
	}
	if err := validateCandidateBuildInfoFile(candidate.Path(), goos, goarch, candidate.Commit); err != nil {
		return err
	}
	if err := runVersionSmoke(ctx, candidate.Path(), candidate.Version); err != nil {
		return err
	}
	return verifyCandidateFile(candidate.Path(), asset)
}

func generateTransactionID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func publishVerifiedArtifact(
	ctx context.Context,
	sourcePath string,
	sourceIdentity filesystem.ObjectIdentity,
	expectedSize int64,
	expectedSHA256, destination string,
) (publishedArtifact, error) {
	file, err := filesystem.OpenVerifiedSingleLinkFile(sourcePath, sourceIdentity)
	if err != nil {
		return publishedArtifact{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return publishedArtifact{}, err
	}
	size := info.Size()
	if size <= 0 || size > maxSelfUpdateAssetBytes {
		_ = file.Close()
		return publishedArtifact{}, errors.New("artifact source size is outside the allowed range")
	}
	if expectedSize > 0 && size != expectedSize {
		_ = file.Close()
		return publishedArtifact{}, fmt.Errorf("artifact source size is %d, expected %d", size, expectedSize)
	}
	digest, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(digest) != 32 {
		_ = file.Close()
		return publishedArtifact{}, errors.New("artifact expected SHA-256 is invalid")
	}

	snapshot, writeErr := filesystem.WriteOwnerOnlyExecutableNoReplace(ctx, destination, file, size)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return publishedArtifact{}, errors.Join(writeErr, closeErr)
	}
	if !snapshot.MatchesContentDigest(size, digest) {
		removeErr := filesystem.RemoveFile(destination, &snapshot)
		return publishedArtifact{}, errors.Join(errors.New("published artifact digest mismatch"), removeErr)
	}
	return publishedArtifact{path: destination, snapshot: snapshot}, nil
}
