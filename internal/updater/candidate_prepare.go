package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	candidatePreparationHTTPTimeout = 2 * time.Minute
	candidateVersionSmokeTimeout    = 5 * time.Second
	maxVersionSmokeOutputBytes      = 4 << 10
)

type candidatePreparationDeps struct {
	client            *http.Client
	apiBase           string
	validateBuildInfo func(path, goos, goarch, revision string) error
	smokeVersion      func(context.Context, string, string) error
}

// PreparedCandidate is a verified raw release binary staged outside the installed executable.
// Call Cleanup when the candidate is no longer needed.
type PreparedCandidate struct {
	ReleaseID int64
	AssetID   int64
	Tag       string
	Version   string
	Commit    string
	Size      int64
	SHA256    string

	path string
}

// Path returns the staged candidate path.
func (candidate *PreparedCandidate) Path() string {
	if candidate == nil {
		return ""
	}
	return candidate.path
}

// Cleanup removes the staged candidate. It is safe to call repeatedly.
func (candidate *PreparedCandidate) Cleanup() error {
	if candidate == nil || candidate.path == "" {
		return nil
	}
	err := os.Remove(candidate.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	candidate.path = ""
	return nil
}

// PrepareCandidate downloads and verifies the next official release candidate without
// modifying the installed executable or persistent installation state.
func PrepareCandidate(ctx context.Context, currentVersion, stagingDirectory string) (*PreparedCandidate, error) {
	client := &http.Client{Timeout: candidatePreparationHTTPTimeout}
	return prepareCandidateWith(ctx, currentVersion, runtime.GOOS, runtime.GOARCH, stagingDirectory, candidatePreparationDeps{
		client:            client,
		apiBase:           officialGitHubRepoAPI,
		validateBuildInfo: validateCandidateBuildInfoFile,
		smokeVersion:      runVersionSmoke,
	})
}

func prepareCandidateWith(
	ctx context.Context,
	currentVersion, goos, goarch, stagingDirectory string,
	deps candidatePreparationDeps,
) (_ *PreparedCandidate, err error) {
	if stagingDirectory == "" {
		return nil, errors.New("candidate staging directory is required")
	}
	stagingInfo, err := os.Stat(stagingDirectory)
	if err != nil {
		return nil, fmt.Errorf("inspect candidate staging directory: %w", err)
	}
	if !stagingInfo.IsDir() {
		return nil, errors.New("candidate staging path is not a directory")
	}
	if deps.client == nil {
		return nil, errors.New("candidate preparation HTTP client is required")
	}
	if deps.apiBase == "" {
		return nil, errors.New("candidate preparation API base is required")
	}
	if deps.validateBuildInfo == nil {
		deps.validateBuildInfo = validateCandidateBuildInfoFile
	}
	if deps.smokeVersion == nil {
		deps.smokeVersion = runVersionSmoke
	}

	latestPayload, err := fetchReleasePayload(ctx, deps.client, strings.TrimSuffix(deps.apiBase, "/")+"/releases/latest")
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	selected, err := selectReleaseAuthority(latestPayload, currentVersion, goos, goarch)
	if err != nil {
		return nil, fmt.Errorf("select release: %w", err)
	}

	commit, err := resolveTagCommitFromBase(ctx, deps.client, deps.apiBase, selected.Tag)
	if err != nil {
		return nil, fmt.Errorf("resolve release commit: %w", err)
	}

	checksumsEndpoint := assetEndpoint(deps.apiBase, selected.Checksums.ID)
	var checksums bytes.Buffer
	if err := downloadAssetFrom(ctx, deps.client, checksumsEndpoint, selected.Checksums, &checksums, nil); err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	if err := validateChecksums(checksums.Bytes(), selected.Binary); err != nil {
		return nil, err
	}

	candidateFile, err := os.CreateTemp(stagingDirectory, candidateTempPattern(goos))
	if err != nil {
		return nil, fmt.Errorf("create candidate staging file: %w", err)
	}
	candidatePath := candidateFile.Name()
	keepCandidate := false
	defer func() {
		if !keepCandidate {
			_ = candidateFile.Close()
			_ = os.Remove(candidatePath)
		}
	}()

	if err := candidateFile.Chmod(0o700); err != nil {
		return nil, fmt.Errorf("set candidate staging permissions: %w", err)
	}
	binaryEndpoint := assetEndpoint(deps.apiBase, selected.Binary.ID)
	if err := downloadAssetFrom(ctx, deps.client, binaryEndpoint, selected.Binary, candidateFile, nil); err != nil {
		return nil, fmt.Errorf("download candidate: %w", err)
	}
	if err := candidateFile.Sync(); err != nil {
		return nil, fmt.Errorf("sync candidate staging file: %w", err)
	}
	if err := candidateFile.Close(); err != nil {
		return nil, fmt.Errorf("close candidate staging file: %w", err)
	}

	refetchedPayload, err := fetchReleasePayload(
		ctx,
		deps.client,
		strings.TrimSuffix(deps.apiBase, "/")+"/releases/"+strconv.FormatInt(selected.ReleaseID, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("re-fetch selected release: %w", err)
	}
	refetched, err := selectReleaseAuthority(refetchedPayload, currentVersion, goos, goarch)
	if err != nil {
		return nil, fmt.Errorf("revalidate selected release: %w", err)
	}
	if refetched != selected {
		return nil, errors.New("selected release metadata changed during candidate preparation")
	}
	refetchedCommit, err := resolveTagCommitFromBase(ctx, deps.client, deps.apiBase, selected.Tag)
	if err != nil {
		return nil, fmt.Errorf("revalidate release commit: %w", err)
	}
	if refetchedCommit != commit {
		return nil, errors.New("selected release tag changed during candidate preparation")
	}
	if err := verifyCandidateFile(candidatePath, selected.Binary); err != nil {
		return nil, fmt.Errorf("revalidate staged candidate bytes: %w", err)
	}

	if err := deps.validateBuildInfo(candidatePath, goos, goarch, commit); err != nil {
		return nil, fmt.Errorf("validate candidate build identity: %w", err)
	}
	if err := deps.smokeVersion(ctx, candidatePath, selected.Version); err != nil {
		return nil, fmt.Errorf("candidate version smoke failed: %w", err)
	}
	if err := verifyCandidateFile(candidatePath, selected.Binary); err != nil {
		return nil, fmt.Errorf("candidate bytes changed during version smoke: %w", err)
	}

	keepCandidate = true
	return &PreparedCandidate{
		ReleaseID: selected.ReleaseID,
		AssetID:   selected.Binary.ID,
		Tag:       selected.Tag,
		Version:   selected.Version,
		Commit:    commit,
		Size:      selected.Binary.Size,
		SHA256:    strings.TrimPrefix(selected.Binary.Digest, "sha256:"),
		path:      candidatePath,
	}, nil
}

func candidateTempPattern(goos string) string {
	if goos == "windows" {
		return ".scripthold-candidate-*.exe"
	}
	return ".scripthold-candidate-*"
}

func verifyCandidateFile(path string, asset releaseAsset) error {
	expected, err := parseGitHubSHA256(asset.Digest)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("candidate is not a regular file")
	}
	if info.Size() != asset.Size {
		return fmt.Errorf("candidate size is %d, expected %d", info.Size(), asset.Size)
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return err
	}
	if written != asset.Size {
		return fmt.Errorf("candidate read size is %d, expected %d", written, asset.Size)
	}
	var actual [sha256.Size]byte
	copy(actual[:], hasher.Sum(nil))
	if actual != expected {
		return errors.New("candidate SHA-256 no longer matches the GitHub release digest")
	}
	return nil
}

func assetEndpoint(apiBase string, assetID int64) string {
	return strings.TrimSuffix(apiBase, "/") + "/releases/assets/" + strconv.FormatInt(assetID, 10)
}

func fetchReleasePayload(ctx context.Context, baseClient *http.Client, endpoint string) ([]byte, error) {
	client := *baseClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "scripthold-self-updater")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub release API returned status %d", response.StatusCode)
	}
	if encoding := strings.TrimSpace(strings.ToLower(response.Header.Get("Content-Encoding"))); encoding != "" && encoding != "identity" {
		return nil, fmt.Errorf("GitHub release API content encoding %q is not allowed", encoding)
	}
	if response.ContentLength > maxReleaseMetadataBytes {
		return nil, errors.New("GitHub release metadata exceeds its size limit")
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > maxReleaseMetadataBytes {
		return nil, errors.New("GitHub release metadata size is outside the allowed range")
	}
	return payload, nil
}

func runVersionSmoke(parent context.Context, path, expectedVersion string) error {
	actual, err := readVersionSmoke(parent, path)
	if err != nil {
		return err
	}
	if actual != expectedVersion {
		return fmt.Errorf("version command returned %q, expected %q", actual, expectedVersion)
	}
	return nil
}

func readVersionSmoke(parent context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, candidateVersionSmokeTimeout)
	defer cancel()

	stdout := &boundedCommandBuffer{limit: maxVersionSmokeOutputBytes}
	stderr := &boundedCommandBuffer{limit: maxVersionSmokeOutputBytes}
	command := exec.CommandContext(ctx, path, "--version")
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("version command timed out or was cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("version command failed: %w", err)
	}
	return parseVersionSmokeOutput(stdout.Bytes(), stderr.Bytes())
}

func parseVersionSmokeOutput(stdout, stderr []byte) (string, error) {
	if len(stderr) != 0 {
		return "", errors.New("version command wrote to stderr")
	}
	actual := string(stdout)
	switch {
	case strings.HasSuffix(actual, "\r\n"):
		actual = strings.TrimSuffix(actual, "\r\n")
	case strings.HasSuffix(actual, "\n"):
		actual = strings.TrimSuffix(actual, "\n")
	}
	if strings.ContainsAny(actual, "\r\n") {
		return "", errors.New("version command returned multiple lines")
	}
	return actual, nil
}

func validateVersionSmokeOutput(stdout, stderr []byte, expectedVersion string) error {
	actual, err := parseVersionSmokeOutput(stdout, stderr)
	if err != nil {
		return err
	}
	if actual != expectedVersion {
		return fmt.Errorf("version command returned %q, expected %q", actual, expectedVersion)
	}
	return nil
}

type boundedCommandBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (buffer *boundedCommandBuffer) Write(data []byte) (int, error) {
	if buffer.limit < 0 || len(data) > buffer.limit-buffer.buffer.Len() {
		return 0, errors.New("command output exceeds its size limit")
	}
	return buffer.buffer.Write(data)
}

func (buffer *boundedCommandBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}
