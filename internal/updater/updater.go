// Package updater checks for new releases on GitHub and notifies users.
package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// UpdateCheckURL is the GitHub API endpoint for the latest fork release.
	UpdateCheckURL = "https://api.github.com/repos/zoster81/scripthold/releases/latest"

	// RepoURL is the custom fork repository URL.
	RepoURL = "https://github.com/zoster81/scripthold"

	// ReleaseURL is the human-facing latest release page for this fork.
	ReleaseURL = RepoURL + "/releases/latest"

	// CheckInterval is the minimum time between API calls (respects GitHub rate limits).
	CheckInterval = 30 * time.Minute

	httpTimeout            = 10 * time.Second
	maxUpdateResponseBytes = 1024 * 1024
	maxCacheBytes          = 4 * 1024
)

// cache stores the last check result to avoid excessive API calls.
type cache struct {
	Source        string    `json:"source"`
	LastCheck     time.Time `json:"lastCheck"`
	LatestVersion string    `json:"latestVersion"`
}

// Check checks for updates and returns a notification message if available.
// Returns empty string if: no update, disabled via MCP_NO_UPDATE_CHECK=1, dev version, or error.
// If force is true, the cache is bypassed and a fresh check is performed.
func Check(ctx context.Context, currentVersion string, force bool) string {
	if os.Getenv("MCP_NO_UPDATE_CHECK") == "1" || currentVersion == "dev" || currentVersion == "" {
		return ""
	}

	cacheFile := getCacheFile()
	latestVersion := ""
	cacheHit := false
	if !force {
		if cached := readCache(cacheFile); cacheIsFresh(cached, time.Now()) {
			latestVersion = cached.LatestVersion
			cacheHit = true
		}
	}

	if !cacheHit {
		var err error
		latestVersion, err = fetchLatestVersion(ctx)
		if err != nil {
			// An empty fresh entry suppresses repeated offline requests until the interval elapses.
			_ = writeCache(cacheFile, "")
			return ""
		}
		_ = writeCache(cacheFile, latestVersion)
	}

	if isNewerVersion(latestVersion, currentVersion) {
		return updateMessage(currentVersion, latestVersion)
	}
	return ""
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: httpTimeout}
	return fetchLatestVersionFrom(ctx, client, UpdateCheckURL)
}

func fetchLatestVersionFrom(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "scripthold-update-checker")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxUpdateResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(payload) > maxUpdateResponseBytes {
		return "", errors.New("release response exceeds its size limit")
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&release); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("release response contains multiple JSON values")
		}
		return "", err
	}
	version := strings.TrimPrefix(release.TagName, "v")
	if _, ok := parseSemanticVersion(version); !ok {
		return "", fmt.Errorf("invalid release tag %q", release.TagName)
	}
	return version, nil
}

func getCacheFile() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "scripthold", "update-check.json")
	}
	return ""
}

func readCache(path string) *cache {
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCacheBytes+1))
	if err != nil || len(data) > maxCacheBytes {
		return nil
	}
	var cached cache
	if json.Unmarshal(data, &cached) != nil {
		return nil
	}
	return &cached
}

// CachedLatestVersion returns the latest version from the cache file, if available.
func CachedLatestVersion() string {
	if cached := readCache(getCacheFile()); cacheMatchesSource(cached) {
		if cached.LatestVersion != "" {
			if _, ok := parseSemanticVersion(cached.LatestVersion); !ok {
				return ""
			}
		}
		return cached.LatestVersion
	}
	return ""
}

func cacheMatchesSource(cached *cache) bool {
	return cached != nil && cached.Source == UpdateCheckURL
}

func cacheIsFresh(cached *cache, now time.Time) bool {
	if !cacheMatchesSource(cached) {
		return false
	}
	age := now.Sub(cached.LastCheck)
	if age < 0 || age >= CheckInterval {
		return false
	}
	if cached.LatestVersion == "" {
		return true
	}
	_, ok := parseSemanticVersion(cached.LatestVersion)
	return ok
}

func updateMessage(currentVersion, latestVersion string) string {
	return fmt.Sprintf(
		"scripthold fork update available: %s → %s\n"+
			"Stop the tunnel or MCP client using the binary before replacing it.\n"+
			"Release: %s",
		currentVersion, latestVersion, ReleaseURL)
}

func writeCache(path, version string) (err error) {
	if path == "" {
		return nil
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cache{Source: UpdateCheckURL, LastCheck: time.Now().UTC(), LatestVersion: version})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".update-check-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, temporary.Close())
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	return os.Rename(temporaryPath, path)
}

// isNewerVersion compares semantic versions, including prerelease precedence.
func isNewerVersion(latest, current string) bool {
	left, latestOK := parseSemanticVersion(latest)
	right, currentOK := parseSemanticVersion(current)
	if !latestOK || !currentOK {
		return false
	}
	for index := 0; index < 3; index++ {
		if compared := compareNumericIdentifier(left.core[index], right.core[index]); compared != 0 {
			return compared > 0
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease) > 0
}

type semanticVersion struct {
	core       [3]string
	prerelease []string
}

func parseSemanticVersion(value string) (semanticVersion, bool) {
	value = strings.TrimPrefix(value, "v")
	if value == "" || strings.Count(value, "+") > 1 {
		return semanticVersion{}, false
	}
	withoutBuild, build, hasBuild := strings.Cut(value, "+")
	if hasBuild && !validIdentifiers(build, false) {
		return semanticVersion{}, false
	}
	coreText, prerelease, hasPrerelease := strings.Cut(withoutBuild, "-")
	parts := strings.Split(coreText, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	var parsed semanticVersion
	for index, part := range parts {
		if !validNumericIdentifier(part, false) {
			return semanticVersion{}, false
		}
		parsed.core[index] = part
	}
	if hasPrerelease {
		if !validIdentifiers(prerelease, true) {
			return semanticVersion{}, false
		}
		parsed.prerelease = strings.Split(prerelease, ".")
	}
	return parsed, true
}

func validIdentifiers(value string, enforceNumericLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		numeric := true
		for _, character := range identifier {
			if character < '0' || character > '9' {
				numeric = false
			}
			if !((character >= '0' && character <= '9') || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '-') {
				return false
			}
		}
		if enforceNumericLeadingZero && numeric && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func validNumericIdentifier(value string, allowLeadingZero bool) bool {
	if value == "" || (!allowLeadingZero && len(value) > 1 && value[0] == '0') {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareNumericIdentifier(left, right string) int {
	if len(left) != len(right) {
		if len(left) < len(right) {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		switch {
		case len(left) == 0 && len(right) == 0:
			return 0
		case len(left) == 0:
			return 1
		default:
			return -1
		}
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		leftNumeric := validNumericIdentifier(left[index], true)
		rightNumeric := validNumericIdentifier(right[index], true)
		if leftNumeric && rightNumeric {
			if compared := compareNumericIdentifier(left[index], right[index]); compared != 0 {
				return compared
			}
			continue
		}
		if leftNumeric != rightNumeric {
			if leftNumeric {
				return -1
			}
			return 1
		}
		if compared := strings.Compare(left[index], right[index]); compared != 0 {
			return compared
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}
