package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var errNoUpdateAvailable = errors.New("no newer self-update release is available")

const (
	githubReleaseAssetHost  = "release-assets.githubusercontent.com"
	checksumsAssetName      = "checksums.txt"
	maxReleaseMetadataBytes = 1 << 20
	maxSelfUpdateAssetBytes = int64(256 << 20)
	maxChecksumsBytes       = int64(128 << 10)
	maxChecksumEntries      = 512
)

type releaseAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type releaseMetadata struct {
	ID         int64          `json:"id"`
	TagName    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Immutable  bool           `json:"immutable"`
	Assets     []releaseAsset `json:"assets"`
}

type selectedRelease struct {
	ReleaseID int64
	Tag       string
	Version   string
	Binary    releaseAsset
	Checksums releaseAsset
}

func selectReleaseAuthority(payload []byte, currentVersion, goos, goarch string) (selectedRelease, error) {
	if len(payload) == 0 || len(payload) > maxReleaseMetadataBytes {
		return selectedRelease{}, errors.New("release metadata size is outside the allowed range")
	}

	var release releaseMetadata
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&release); err != nil {
		return selectedRelease{}, fmt.Errorf("decode release metadata: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return selectedRelease{}, errors.New("release metadata contains multiple JSON values")
		}
		return selectedRelease{}, fmt.Errorf("decode release metadata trailer: %w", err)
	}
	if release.ID <= 0 {
		return selectedRelease{}, errors.New("release ID is invalid")
	}
	if release.Draft || release.Prerelease || !release.Immutable {
		return selectedRelease{}, errors.New("release is not a stable immutable publication")
	}

	parsed, ok := parseSemanticVersion(release.TagName)
	version := strings.TrimPrefix(release.TagName, "v")
	if !ok || len(parsed.prerelease) != 0 {
		return selectedRelease{}, fmt.Errorf("release tag %q is not a stable semantic version", release.TagName)
	}
	if _, ok := parseSemanticVersion(currentVersion); !ok {
		return selectedRelease{}, fmt.Errorf("current version %q is not semantic", currentVersion)
	}
	if !isNewerVersion(version, currentVersion) {
		return selectedRelease{}, fmt.Errorf("%w: release %q is not newer than %q", errNoUpdateAvailable, version, currentVersion)
	}

	binaryName, err := expectedBinaryAssetName(goos, goarch)
	if err != nil {
		return selectedRelease{}, err
	}
	binary, err := selectUniqueReleaseAsset(release.Assets, binaryName, maxSelfUpdateAssetBytes)
	if err != nil {
		return selectedRelease{}, err
	}
	checksums, err := selectUniqueReleaseAsset(release.Assets, checksumsAssetName, maxChecksumsBytes)
	if err != nil {
		return selectedRelease{}, err
	}

	return selectedRelease{
		ReleaseID: release.ID,
		Tag:       release.TagName,
		Version:   version,
		Binary:    binary,
		Checksums: checksums,
	}, nil
}

func expectedBinaryAssetName(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "scripthold_windows_amd64.exe", nil
	case "windows/arm64":
		return "scripthold_windows_arm64.exe", nil
	case "linux/amd64":
		return "scripthold_linux_amd64", nil
	case "linux/arm64":
		return "scripthold_linux_arm64", nil
	case "darwin/amd64":
		return "scripthold_darwin_amd64", nil
	case "darwin/arm64":
		return "scripthold_darwin_arm64", nil
	default:
		return "", fmt.Errorf("self-update is unsupported on %s/%s", goos, goarch)
	}
}

func selectUniqueReleaseAsset(assets []releaseAsset, name string, maxBytes int64) (releaseAsset, error) {
	var selected releaseAsset
	count := 0
	for _, asset := range assets {
		if asset.Name != name {
			continue
		}
		selected = asset
		count++
	}
	if count != 1 {
		return releaseAsset{}, fmt.Errorf("release must contain exactly one %q asset", name)
	}
	if selected.ID <= 0 {
		return releaseAsset{}, fmt.Errorf("release asset %q has an invalid ID", name)
	}
	if selected.State != "uploaded" {
		return releaseAsset{}, fmt.Errorf("release asset %q is not uploaded", name)
	}
	if selected.Size <= 0 || selected.Size > maxBytes {
		return releaseAsset{}, fmt.Errorf("release asset %q size is outside the allowed range", name)
	}
	if _, err := parseGitHubSHA256(selected.Digest); err != nil {
		return releaseAsset{}, fmt.Errorf("release asset %q digest is invalid: %w", name, err)
	}
	return selected, nil
}

func parseGitHubSHA256(value string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return digest, errors.New("digest must be sha256 followed by 64 lowercase hexadecimal characters")
	}
	hexValue := strings.TrimPrefix(value, "sha256:")
	if hexValue != strings.ToLower(hexValue) {
		return digest, errors.New("digest must use lowercase hexadecimal")
	}
	decoded, err := hex.DecodeString(hexValue)
	if err != nil || len(decoded) != sha256.Size {
		return digest, errors.New("digest contains invalid hexadecimal")
	}
	copy(digest[:], decoded)
	return digest, nil
}

func validateGitHubAssetRedirect(location *url.URL) error {
	if location == nil {
		return errors.New("asset redirect location is missing")
	}
	if location.Scheme != "https" || location.User != nil || location.Opaque != "" {
		return errors.New("asset redirect URL is not an allowed HTTPS URL")
	}
	if location.Hostname() != githubReleaseAssetHost {
		return fmt.Errorf("asset redirect host %q is not allowed", location.Hostname())
	}
	if port := location.Port(); port != "" && port != "443" {
		return fmt.Errorf("asset redirect port %q is not allowed", port)
	}
	return nil
}

func downloadAssetFrom(
	ctx context.Context,
	baseClient *http.Client,
	endpoint string,
	asset releaseAsset,
	destination io.Writer,
	redirectValidator func(*url.URL) error,
) error {
	if baseClient == nil {
		return errors.New("HTTP client is required")
	}
	if destination == nil {
		return errors.New("download destination is required")
	}
	if asset.Size <= 0 || asset.Size > maxSelfUpdateAssetBytes {
		return errors.New("asset size is outside the allowed range")
	}
	expectedDigest, err := parseGitHubSHA256(asset.Digest)
	if err != nil {
		return err
	}
	if redirectValidator == nil {
		redirectValidator = validateGitHubAssetRedirect
	}

	client := *baseClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := newAssetRequest(ctx, endpoint)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}

	if response.StatusCode == http.StatusFound {
		location, locationErr := response.Location()
		closeErr := response.Body.Close()
		if locationErr != nil {
			return errors.Join(fmt.Errorf("read asset redirect: %w", locationErr), closeErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if err := redirectValidator(location); err != nil {
			return err
		}

		request, err = newAssetRequest(ctx, location.String())
		if err != nil {
			return err
		}
		response, err = client.Do(request)
		if err != nil {
			return err
		}
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download returned status %d", response.StatusCode)
	}
	if encoding := strings.TrimSpace(strings.ToLower(response.Header.Get("Content-Encoding"))); encoding != "" && encoding != "identity" {
		return fmt.Errorf("asset download content encoding %q is not allowed", encoding)
	}
	if response.ContentLength >= 0 && response.ContentLength != asset.Size {
		return fmt.Errorf("asset Content-Length is %d, expected %d", response.ContentLength, asset.Size)
	}

	hasher := sha256.New()
	limited := io.LimitReader(response.Body, asset.Size+1)
	written, err := io.Copy(io.MultiWriter(destination, hasher), limited)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	if written != asset.Size {
		return fmt.Errorf("asset download size is %d, expected %d", written, asset.Size)
	}
	var actual [sha256.Size]byte
	copy(actual[:], hasher.Sum(nil))
	if actual != expectedDigest {
		return errors.New("asset SHA-256 does not match the GitHub release digest")
	}
	return nil
}

func newAssetRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "scripthold-self-updater")
	return request, nil
}

func validateChecksums(payload []byte, asset releaseAsset) error {
	if int64(len(payload)) <= 0 || int64(len(payload)) > maxChecksumsBytes {
		return errors.New("checksums data size is outside the allowed range")
	}
	expected, err := parseGitHubSHA256(asset.Digest)
	if err != nil {
		return fmt.Errorf("selected asset digest is invalid: %w", err)
	}
	expectedHex := hex.EncodeToString(expected[:])

	seen := make(map[string]string)
	for lineNumber, rawLine := range strings.Split(string(payload), "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("checksums line %d is malformed", lineNumber+1)
		}
		hashText := fields[0]
		name := strings.TrimPrefix(fields[1], "*")
		if len(hashText) != sha256.Size*2 || hashText != strings.ToLower(hashText) {
			return fmt.Errorf("checksums line %d has an invalid SHA-256", lineNumber+1)
		}
		if decoded, decodeErr := hex.DecodeString(hashText); decodeErr != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("checksums line %d has an invalid SHA-256", lineNumber+1)
		}
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("checksums line %d has an invalid asset name", lineNumber+1)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("checksums contains duplicate entry for %q", name)
		}
		seen[name] = hashText
		if len(seen) > maxChecksumEntries {
			return errors.New("checksums contains too many entries")
		}
	}

	actual, ok := seen[asset.Name]
	if !ok {
		return fmt.Errorf("checksums does not contain %q", asset.Name)
	}
	if actual != expectedHex {
		return fmt.Errorf("checksums SHA-256 for %q disagrees with the GitHub asset digest", asset.Name)
	}
	return nil
}
