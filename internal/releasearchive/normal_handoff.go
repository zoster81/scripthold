package releasearchive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RawNormalFile maps one authenticated physical Actions download basename to
// the independently authenticated original release asset and byte digest.
// A caller must establish the exact 15-asset normal allowlist and source job
// identity before passing this local-only mapping; no host paths cross jobs.
type RawNormalFile struct {
	DownloadName string       `json:"downloadName"`
	Original     ExpectedFile `json:"original"`
}

// StageRawNormalHandoff stages 15 original files only after their download
// plan matches a separately authenticated release inventory. Its caller must
// independently authenticate both original inventory and GitHub job/artifact
// identities; this function only verifies original bytes and local staging.
// On failure after creation the returned nonempty path is partial evidence,
// NEVER a usable stage. Cleanup belongs to the trusted same-run caller.
func StageRawNormalHandoff(sourceDir, trustedParent string, files []RawNormalFile, trusted []ExpectedFile) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("normal raw staging requires a Linux runner")
	}
	bound, err := BindRawNormalHandoffPlan(files, trusted)
	if err != nil {
		return "", err
	}
	return stageRawNormalHandoff(sourceDir, trustedParent, bound)
}

// validateRawNormalEntries is the common allocation-bounded, read-only
// authority preflight for JSON decoding and for the physical source stage.
// It does not authenticate GitHub jobs or allowlist asset names by itself.
func validateRawNormalEntries(files []RawNormalFile) ([]ExpectedFile, []ExpectedFile, error) {
	if len(files) != 15 {
		return nil, nil, errors.New("normal raw staging requires exactly 15 originals")
	}
	physical := make([]ExpectedFile, 0, len(files))
	canonical := make([]ExpectedFile, 0, len(files))
	sourceNames := make(map[string]bool, len(files))
	originalNames := make(map[string]bool, len(files))
	for _, entry := range files {
		original := entry.Original
		if !portableStagingName(entry.DownloadName) ||
			!portableStagingName(original.Name) ||
			original.Size <= 0 || original.Size > maxAssetBytes ||
			!validDigest(original.SHA256) {
			return nil, nil, errors.New("invalid normal raw staging entry")
		}
		sourceKey := strings.ToLower(entry.DownloadName)
		originalKey := strings.ToLower(original.Name)
		if sourceNames[sourceKey] || originalNames[originalKey] {
			return nil, nil, errors.New("duplicate normal raw staging name")
		}
		sourceNames[sourceKey], originalNames[originalKey] = true, true
		physical = append(physical, ExpectedFile{
			Name: entry.DownloadName, Size: original.Size, SHA256: original.SHA256,
		})
		canonical = append(canonical, original)
	}
	return physical, canonical, nil
}

// The core can be tested on Windows, while its public operation remains
// restricted to Linux where owner-only destination policy is enforceable.
func stageRawNormalHandoff(sourceDir, trustedParent string, files []RawNormalFile) (string, error) {
	if !filepath.IsAbs(sourceDir) || !filepath.IsAbs(trustedParent) {
		return "", errors.New("normal staging directories must use trusted absolute paths")
	}
	physical, canonical, err := validateRawNormalEntries(files)
	if err != nil {
		return "", err
	}

	// A stage created inside the downloaded flat inventory would invalidate
	// that inventory after preflight. Check directory object identity, not
	// pathname spelling, so aliases to the same trusted directory also fail.
	sourceInfo, err := os.Stat(sourceDir)
	if err != nil {
		return "", fmt.Errorf("inspect normal download root: %w", err)
	}
	parentInfo, err := os.Stat(trustedParent)
	if err != nil {
		return "", fmt.Errorf("inspect trusted staging parent: %w", err)
	}
	if !sourceInfo.IsDir() || !parentInfo.IsDir() ||
		os.SameFile(sourceInfo, parentInfo) {
		return "", errors.New("normal downloads and staging parent must be distinct directories")
	}

	// All source originals and the complete flat inventory must pass before
	// any new staging destination is created. Copy still rechecks each source.
	if err := verifyFlatDirectory(sourceDir, physical); err != nil {
		return "", fmt.Errorf("verify all normal raw downloads: %w", err)
	}
	stage, err := createPrivateRawStagingDirectory(trustedParent)
	if err != nil {
		return "", err
	}
	for _, entry := range files {
		if err := stageVerifiedSourceFile(sourceDir, entry.DownloadName, stage,
			entry.Original.Name, entry.Original); err != nil {
			return stage, fmt.Errorf("stage normal original %q: %w", entry.Original.Name, err)
		}
	}
	if err := verifyFlatDirectory(stage, canonical); err != nil {
		return stage, fmt.Errorf("verify complete private normal handoff: %w", err)
	}
	return stage, nil
}
