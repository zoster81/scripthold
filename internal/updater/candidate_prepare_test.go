package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrepareCandidateWithComposesPinnedEvidence(t *testing.T) {
	binary := []byte("verified-candidate")
	binarySum := sha256.Sum256(binary)
	binaryDigest := "sha256:" + hex.EncodeToString(binarySum[:])
	checksums := []byte(hex.EncodeToString(binarySum[:]) + "  scripthold_linux_amd64\n")
	checksumsSum := sha256.Sum256(checksums)
	checksumsDigest := "sha256:" + hex.EncodeToString(checksumsSum[:])
	release := fmt.Sprintf(`{"id":42,"tag_name":"v3.3.0","draft":false,"prerelease":false,"immutable":true,"assets":[{"id":202,"name":"scripthold_linux_amd64","state":"uploaded","size":%d,"digest":"%s"},{"id":201,"name":"checksums.txt","state":"uploaded","size":%d,"digest":"%s"}]}`, len(binary), binaryDigest, len(checksums), checksumsDigest)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases/latest", "/releases/42":
			_, _ = fmt.Fprint(writer, release)
		case "/git/ref/tags/v3.3.0":
			_, _ = fmt.Fprint(writer, `{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}`)
		case "/releases/assets/201":
			_, _ = writer.Write(checksums)
		case "/releases/assets/202":
			_, _ = writer.Write(binary)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var buildValidated bool
	var smoked bool
	prepared, err := prepareCandidateWith(context.Background(), "3.2.1", "linux", "amd64", t.TempDir(), candidatePreparationDeps{
		client:  server.Client(),
		apiBase: server.URL,
		validateBuildInfo: func(path, goos, goarch, revision string) error {
			buildValidated = true
			if goos != "linux" || goarch != "amd64" || revision != "0123456789abcdef0123456789abcdef01234567" {
				return fmt.Errorf("unexpected build evidence")
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if string(data) != string(binary) {
				return fmt.Errorf("candidate bytes changed")
			}
			return nil
		},
		smokeVersion: func(_ context.Context, path, version string) error {
			smoked = true
			if version != "3.3.0" {
				return fmt.Errorf("unexpected version %q", version)
			}
			if _, statErr := os.Stat(path); statErr != nil {
				return statErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Cleanup()

	if !buildValidated || !smoked {
		t.Fatalf("validation calls: build=%v smoke=%v", buildValidated, smoked)
	}
	if prepared.ReleaseID != 42 || prepared.AssetID != 202 || prepared.Version != "3.3.0" ||
		prepared.Tag != "v3.3.0" || prepared.Commit != "0123456789abcdef0123456789abcdef01234567" ||
		prepared.Size != int64(len(binary)) || prepared.SHA256 != strings.TrimPrefix(binaryDigest, "sha256:") {
		t.Fatalf("unexpected prepared evidence: %#v", prepared)
	}
	info, statErr := os.Stat(prepared.Path())
	if statErr != nil {
		t.Fatal(statErr)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("candidate is not executable: mode=%v", info.Mode())
	}
}

func TestPrepareCandidateWithRejectsReleaseDriftAndRemovesCandidate(t *testing.T) {
	binary := []byte("verified-candidate")
	binarySum := sha256.Sum256(binary)
	binaryDigest := "sha256:" + hex.EncodeToString(binarySum[:])
	checksums := []byte(hex.EncodeToString(binarySum[:]) + "  scripthold_linux_amd64\n")
	checksumsSum := sha256.Sum256(checksums)
	checksumsDigest := "sha256:" + hex.EncodeToString(checksumsSum[:])
	release := fmt.Sprintf(`{"id":42,"tag_name":"v3.3.0","draft":false,"prerelease":false,"immutable":true,"assets":[{"id":202,"name":"scripthold_linux_amd64","state":"uploaded","size":%d,"digest":"%s"},{"id":201,"name":"checksums.txt","state":"uploaded","size":%d,"digest":"%s"}]}`, len(binary), binaryDigest, len(checksums), checksumsDigest)
	drifted := strings.Replace(release, `"immutable":true`, `"immutable":false`, 1)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases/latest":
			_, _ = fmt.Fprint(writer, release)
		case "/releases/42":
			_, _ = fmt.Fprint(writer, drifted)
		case "/git/ref/tags/v3.3.0":
			_, _ = fmt.Fprint(writer, `{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}`)
		case "/releases/assets/201":
			_, _ = writer.Write(checksums)
		case "/releases/assets/202":
			_, _ = writer.Write(binary)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	_, err := prepareCandidateWith(context.Background(), "3.2.1", "linux", "amd64", directory, candidatePreparationDeps{
		client:            server.Client(),
		apiBase:           server.URL,
		validateBuildInfo: func(string, string, string, string) error { return nil },
		smokeVersion:      func(context.Context, string, string) error { return nil },
	})
	if err == nil {
		t.Fatal("release drift must fail closed")
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed preparation left artifacts: %v", entries)
	}
}

func TestPrepareCandidateWithDoesNotSmokeBeforeStaticValidation(t *testing.T) {
	binary := []byte("verified-candidate")
	binarySum := sha256.Sum256(binary)
	binaryDigest := "sha256:" + hex.EncodeToString(binarySum[:])
	checksums := []byte(hex.EncodeToString(binarySum[:]) + "  scripthold_linux_amd64\n")
	checksumsSum := sha256.Sum256(checksums)
	checksumsDigest := "sha256:" + hex.EncodeToString(checksumsSum[:])
	release := fmt.Sprintf(`{"id":42,"tag_name":"v3.3.0","draft":false,"prerelease":false,"immutable":true,"assets":[{"id":202,"name":"scripthold_linux_amd64","state":"uploaded","size":%d,"digest":"%s"},{"id":201,"name":"checksums.txt","state":"uploaded","size":%d,"digest":"%s"}]}`, len(binary), binaryDigest, len(checksums), checksumsDigest)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases/latest", "/releases/42":
			_, _ = fmt.Fprint(writer, release)
		case "/git/ref/tags/v3.3.0":
			_, _ = fmt.Fprint(writer, `{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}`)
		case "/releases/assets/201":
			_, _ = writer.Write(checksums)
		case "/releases/assets/202":
			_, _ = writer.Write(binary)
		}
	}))
	defer server.Close()

	var smokeCalls atomic.Int32
	_, err := prepareCandidateWith(context.Background(), "3.2.1", "linux", "amd64", t.TempDir(), candidatePreparationDeps{
		client:  server.Client(),
		apiBase: server.URL,
		validateBuildInfo: func(string, string, string, string) error {
			return fmt.Errorf("static identity rejected")
		},
		smokeVersion: func(context.Context, string, string) error {
			smokeCalls.Add(1)
			return nil
		},
	})
	if err == nil {
		t.Fatal("static validation failure must reject candidate")
	}
	if smokeCalls.Load() != 0 {
		t.Fatal("candidate was executed before static validation succeeded")
	}
}

func TestValidateVersionSmokeOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		stdout string
		stderr string
		valid  bool
	}{
		{name: "lf", stdout: "3.3.0\n", valid: true},
		{name: "crlf", stdout: "3.3.0\r\n", valid: true},
		{name: "no newline", stdout: "3.3.0", valid: true},
		{name: "extra output", stdout: "3.3.0\nextra\n"},
		{name: "leading space", stdout: " 3.3.0\n"},
		{name: "stderr", stdout: "3.3.0\n", stderr: "warning"},
		{name: "wrong version", stdout: "3.3.1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateVersionSmokeOutput([]byte(test.stdout), []byte(test.stderr), "3.3.0")
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
		})
	}
}

func TestPreparedCandidateCleanupIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(path, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	prepared := &PreparedCandidate{path: path}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("candidate still exists: %v", err)
	}
}

func TestVerifyCandidateFileDetectsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate")
	payload := []byte("candidate-bytes")
	sum := sha256.Sum256(payload)
	asset := releaseAsset{
		Size:   int64(len(payload)),
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
	}
	if err := os.WriteFile(path, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := verifyCandidateFile(path, asset); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed-bytes!"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := verifyCandidateFile(path, asset); err == nil {
		t.Fatal("mutated candidate must fail verification")
	}
}

func TestCandidateTempPatternWindowsSuffix(t *testing.T) {
	if got := candidateTempPattern("windows"); !strings.HasSuffix(got, ".exe") {
		t.Fatalf("windows candidate pattern = %q", got)
	}
	if got := candidateTempPattern("linux"); strings.HasSuffix(got, ".exe") {
		t.Fatalf("linux candidate pattern = %q", got)
	}
}

func TestBoundedCommandBufferRejectsExcessOutput(t *testing.T) {
	buffer := &boundedCommandBuffer{limit: 4}
	if _, err := buffer.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte("5")); err == nil {
		t.Fatal("oversized command output must fail closed")
	}
}
