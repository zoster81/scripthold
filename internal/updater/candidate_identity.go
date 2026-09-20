package updater

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
)

const (
	officialGitHubRepoAPI        = "https://api.github.com/repos/zoster81/scripthold"
	officialModulePath           = "github.com/zoster81/scripthold"
	maxGitReferenceMetadataBytes = 64 << 10
)

type gitObjectReference struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type gitReferenceMetadata struct {
	Ref    string             `json:"ref"`
	Object gitObjectReference `json:"object"`
}

type gitTagMetadata struct {
	Tag    string             `json:"tag"`
	Object gitObjectReference `json:"object"`
}

func resolveOfficialTagCommit(ctx context.Context, client *http.Client, tag string) (string, error) {
	return resolveTagCommitFromBase(ctx, client, officialGitHubRepoAPI, tag)
}

func resolveTagCommitFromBase(ctx context.Context, baseClient *http.Client, baseURL, tag string) (string, error) {
	parsed, ok := parseSemanticVersion(tag)
	if !ok || len(parsed.prerelease) != 0 {
		return "", fmt.Errorf("release tag %q is not a stable semantic version", tag)
	}
	if baseClient == nil {
		return "", errors.New("HTTP client is required")
	}

	refEndpoint := strings.TrimSuffix(baseURL, "/") + "/git/ref/tags/" + url.PathEscape(tag)
	var reference gitReferenceMetadata
	if err := fetchBoundedGitHubJSON(ctx, baseClient, refEndpoint, &reference); err != nil {
		return "", fmt.Errorf("resolve release tag reference: %w", err)
	}
	if reference.Ref != "refs/tags/"+tag {
		return "", errors.New("GitHub tag reference does not match the selected release tag")
	}
	if err := validateGitObjectSHA(reference.Object.SHA); err != nil {
		return "", fmt.Errorf("release tag object SHA is invalid: %w", err)
	}

	switch reference.Object.Type {
	case "commit":
		return reference.Object.SHA, nil
	case "tag":
		tagEndpoint := strings.TrimSuffix(baseURL, "/") + "/git/tags/" + reference.Object.SHA
		var annotated gitTagMetadata
		if err := fetchBoundedGitHubJSON(ctx, baseClient, tagEndpoint, &annotated); err != nil {
			return "", fmt.Errorf("resolve annotated release tag: %w", err)
		}
		if annotated.Tag != tag {
			return "", errors.New("annotated Git tag does not match the selected release tag")
		}
		if annotated.Object.Type != "commit" {
			return "", fmt.Errorf("annotated release tag points to unsupported object type %q", annotated.Object.Type)
		}
		if err := validateGitObjectSHA(annotated.Object.SHA); err != nil {
			return "", fmt.Errorf("annotated release commit SHA is invalid: %w", err)
		}
		return annotated.Object.SHA, nil
	default:
		return "", fmt.Errorf("release tag points to unsupported object type %q", reference.Object.Type)
	}
}

func fetchBoundedGitHubJSON(ctx context.Context, baseClient *http.Client, endpoint string, destination any) error {
	client := *baseClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "scripthold-self-updater")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", response.StatusCode)
	}
	if encoding := strings.TrimSpace(strings.ToLower(response.Header.Get("Content-Encoding"))); encoding != "" && encoding != "identity" {
		return fmt.Errorf("GitHub API content encoding %q is not allowed", encoding)
	}
	if response.ContentLength > maxGitReferenceMetadataBytes {
		return errors.New("GitHub API response exceeds its size limit")
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxGitReferenceMetadataBytes+1))
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > maxGitReferenceMetadataBytes {
		return errors.New("GitHub API response size is outside the allowed range")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("GitHub API response contains multiple JSON values")
		}
		return err
	}
	return nil
}

func validateGitObjectSHA(value string) error {
	if (len(value) != 40 && len(value) != 64) || value != strings.ToLower(value) {
		return errors.New("Git object SHA must be 40 or 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || (len(decoded) != 20 && len(decoded) != 32) {
		return errors.New("Git object SHA contains invalid hexadecimal")
	}
	return nil
}

func validateCandidateBuildInfo(info *debug.BuildInfo, expectedOS, expectedArch, expectedRevision string) error {
	if info == nil {
		return errors.New("candidate Go build information is unavailable")
	}
	if info.Main.Path != officialModulePath || info.Main.Replace != nil {
		return fmt.Errorf("candidate main module is not %s", officialModulePath)
	}
	if err := validateGitObjectSHA(expectedRevision); err != nil {
		return fmt.Errorf("expected release revision is invalid: %w", err)
	}

	goos, err := uniqueBuildSetting(info, "GOOS", true)
	if err != nil {
		return err
	}
	if goos != expectedOS {
		return fmt.Errorf("candidate GOOS is %q, expected %q", goos, expectedOS)
	}
	goarch, err := uniqueBuildSetting(info, "GOARCH", true)
	if err != nil {
		return err
	}
	if goarch != expectedArch {
		return fmt.Errorf("candidate GOARCH is %q, expected %q", goarch, expectedArch)
	}
	revision, err := uniqueBuildSetting(info, "vcs.revision", true)
	if err != nil {
		return err
	}
	if revision != expectedRevision {
		return errors.New("candidate VCS revision does not match the selected release tag")
	}
	vcs, err := uniqueBuildSetting(info, "vcs", true)
	if err != nil {
		return err
	}
	if vcs != "git" {
		return fmt.Errorf("candidate VCS is %q, expected git", vcs)
	}
	modified, err := uniqueBuildSetting(info, "vcs.modified", false)
	if err != nil {
		return err
	}
	if modified != "" && modified != "false" {
		return errors.New("candidate build reports modified VCS state")
	}
	return nil
}

func validateCandidateBuildInfoFile(path, expectedOS, expectedArch, expectedRevision string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read candidate Go build information: %w", err)
	}
	return validateCandidateBuildInfo(info, expectedOS, expectedArch, expectedRevision)
}

func uniqueBuildSetting(info *debug.BuildInfo, key string, required bool) (string, error) {
	value := ""
	found := false
	for _, setting := range info.Settings {
		if setting.Key != key {
			continue
		}
		if found {
			return "", fmt.Errorf("candidate build contains duplicate %s settings", key)
		}
		found = true
		value = setting.Value
	}
	if required && (!found || value == "") {
		return "", fmt.Errorf("candidate build setting %s is missing", key)
	}
	return value, nil
}
