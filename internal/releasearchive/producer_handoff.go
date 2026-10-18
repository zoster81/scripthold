package releasearchive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxGitHubArtifactNumber = 9007199254740991

// StageNormalProducerOriginals creates one private producer-local stage with
// the exact fifteen independently authenticated normal originals. Each output
// is named for the original raw upload-artifact (archive:false) attempt.
// It does not authenticate trusted digests or Sigstore signer identity, run a
// workflow, upload, or authorize publication.
//
// A nonempty stage returned with an error is PARTIAL, not a usable output.
// The caller must never upload or publish it and must handle safe cleanup.
func StageNormalProducerOriginals(tag, distDir, stagingParent string, runID, attempt int64, plan []NormalProducerSource, trusted []ExpectedFile) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("normal original producer staging requires native Linux")
	}
	return stageNormalProducerOriginals(tag, distDir, stagingParent, runID, attempt, plan, trusted)
}

// stageNormalProducerOriginals is exercised on other operating systems;
// Linux owns the public owner-only staging and syncing contract.
func stageNormalProducerOriginals(tag, distDir, stagingParent string, runID, attempt int64, plan []NormalProducerSource, trusted []ExpectedFile) (string, error) {
	if runID <= 0 || runID > maxGitHubArtifactNumber ||
		attempt <= 0 || attempt > maxGitHubArtifactNumber ||
		!filepath.IsAbs(distDir) || !filepath.IsAbs(stagingParent) {
		return "", errors.New("invalid producer identity or trusted directory")
	}
	bound, err := BindNormalProducerSourcePlan(tag, plan, trusted)
	if err != nil {
		return "", err
	}
	physical := make([]ExpectedFile, 0, len(bound))
	for _, original := range bound {
		name := fmt.Sprintf("scripthold-raw-normal-%d-%d-%s", runID, attempt, original.Original.Name)
		if len(name) > 240 || !portableStagingName(name) {
			return "", errors.New("attempt-scoped normal original name is not portable")
		}
		physical = append(physical, ExpectedFile{
			Name: name, Size: original.Original.Size, SHA256: original.Original.SHA256,
		})
	}
	sourceInfo, err := os.Lstat(distDir)
	if err != nil {
		return "", fmt.Errorf("inspect producer dist root: %w", err)
	}
	if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("producer dist root must be a real directory")
	}
	parentInfo, err := os.Stat(stagingParent)
	if err != nil {
		return "", fmt.Errorf("inspect trusted staging parent: %w", err)
	}
	if !parentInfo.IsDir() || os.SameFile(sourceInfo, parentInfo) {
		return "", errors.New("producer source and staging parent must be separate")
	}
	// In particular, the source must not acquire new staging children. Resolve
	// legitimate system path aliases before checking directory containment.
	sourceReal, err := filepath.EvalSymlinks(distDir)
	if err != nil {
		return "", fmt.Errorf("resolve trusted producer dist root: %w", err)
	}
	parentReal, err := filepath.EvalSymlinks(stagingParent)
	if err != nil {
		return "", fmt.Errorf("resolve trusted staging parent: %w", err)
	}
	for _, pair := range [][2]string{{sourceReal, parentReal}, {parentReal, sourceReal}} {
		rel, relErr := filepath.Rel(pair[0], pair[1])
		if relErr != nil {
			return "", fmt.Errorf("compare producer and staging roots: %w", relErr)
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return "", errors.New("producer and staging directory trees must not overlap")
		}
	}
	stage, err := createPrivateRawStagingDirectory(stagingParent)
	if err != nil {
		return "", err
	}
	for i, original := range bound {
		if err := stageVerifiedProducerSourceFile(distDir, original.SourceSubdir,
			original.SourceLeaf, stage, physical[i].Name, original.Original); err != nil {
			return stage, fmt.Errorf("stage original %q: %w", original.Original.Name, err)
		}
	}
	if err := verifyFlatDirectory(stage, physical); err != nil {
		return stage, fmt.Errorf("complete producer original stage failed verification: %w", err)
	}
	return stage, nil
}
