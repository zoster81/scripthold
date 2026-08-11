package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const internetCorpusDir = "testdata/internet-corpus"

var fullGitRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

var approvedCorpusRepositories = map[string]bool{
	"https://github.com/arthenica/libiconv":  true,
	"https://github.com/oe-mirrors/uchardet": true,
}

type internetCorpusManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Description   string                   `json:"description"`
	Licenses      []internetCorpusArtifact `json:"licenses"`
	Fixtures      []internetCorpusFixture  `json:"fixtures"`
}

type internetCorpusArtifact struct {
	File             string `json:"file"`
	SourceProject    string `json:"source_project"`
	SourceRepository string `json:"source_repository"`
	SourceRevision   string `json:"source_revision"`
	SourcePath       string `json:"source_path"`
	SourceURL        string `json:"source_url"`
	SHA256           string `json:"sha256"`
	ByteLength       int    `json:"byte_length"`
}

type internetCorpusFixture struct {
	Encoding         string                `json:"encoding"`
	File             string                `json:"file"`
	Role             string                `json:"role"`
	SourceProject    string                `json:"source_project"`
	SourceRepository string                `json:"source_repository"`
	SourceRevision   string                `json:"source_revision"`
	SourcePath       string                `json:"source_path"`
	SourceURL        string                `json:"source_url"`
	LicenseFiles     []string              `json:"license_files"`
	SHA256           string                `json:"sha256"`
	ByteLength       int                   `json:"byte_length"`
	NonASCIIBytes    int                   `json:"non_ascii_bytes"`
	Oracle           *internetCorpusOracle `json:"oracle,omitempty"`
}

type internetCorpusOracle struct {
	File             string   `json:"file"`
	Encoding         string   `json:"encoding"`
	SourceProject    string   `json:"source_project"`
	SourceRepository string   `json:"source_repository"`
	SourceRevision   string   `json:"source_revision"`
	SourcePath       string   `json:"source_path"`
	SourceURL        string   `json:"source_url"`
	LicenseFiles     []string `json:"license_files"`
	SHA256           string   `json:"sha256"`
	ByteLength       int      `json:"byte_length"`
}

func TestInternetCorpusManifestIntegrity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(internetCorpusDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= 3 && string(data[:3]) == "\xef\xbb\xbf" {
		t.Fatal("corpus manifest must be UTF-8 without BOM")
	}

	var manifest internetCorpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode corpus manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.Description) == "" {
		t.Fatal("manifest description is empty")
	}
	if len(manifest.Licenses) == 0 || len(manifest.Fixtures) == 0 {
		t.Fatal("manifest licenses and fixtures must both be non-empty")
	}

	declaredFiles := map[string]bool{"manifest.json": true}
	licenseRepositories := make(map[string]string)
	previous := ""
	for _, license := range manifest.Licenses {
		if previous != "" && license.File <= previous {
			t.Fatalf("licenses are not strictly sorted by file: %q after %q", license.File, previous)
		}
		previous = license.File
		verifyCorpusArtifact(t, license.File, license.SourceProject, license.SourceRepository, license.SourceRevision, license.SourcePath, license.SourceURL, license.SHA256, license.ByteLength, false, declaredFiles)
		licenseRepositories[license.File] = license.SourceRepository
	}

	previous = ""
	for _, fixture := range manifest.Fixtures {
		if previous != "" && fixture.File <= previous {
			t.Fatalf("fixtures are not strictly sorted by file: %q after %q", fixture.File, previous)
		}
		previous = fixture.File
		if fixture.Encoding == "" {
			t.Fatalf("fixture %q has empty encoding", fixture.File)
		}
		if fixture.Role != "detection" && fixture.Role != "codec" {
			t.Fatalf("fixture %q has unsupported role %q", fixture.File, fixture.Role)
		}
		verifyLicenseReferences(t, fixture.File, fixture.SourceRepository, fixture.LicenseFiles, licenseRepositories)
		verifyCorpusArtifact(t, fixture.File, fixture.SourceProject, fixture.SourceRepository, fixture.SourceRevision, fixture.SourcePath, fixture.SourceURL, fixture.SHA256, fixture.ByteLength, false, declaredFiles)

		content, err := os.ReadFile(filepath.Join(internetCorpusDir, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		nonASCII := 0
		for _, b := range content {
			if b >= 0x80 {
				nonASCII++
			}
		}
		if nonASCII != fixture.NonASCIIBytes {
			t.Fatalf("corpus file %q non-ASCII bytes = %d, want %d", fixture.File, nonASCII, fixture.NonASCIIBytes)
		}

		if fixture.Oracle != nil {
			oracle := fixture.Oracle
			if oracle.Encoding != "utf-8" {
				t.Fatalf("oracle %q encoding = %q, want utf-8", oracle.File, oracle.Encoding)
			}
			verifyLicenseReferences(t, oracle.File, oracle.SourceRepository, oracle.LicenseFiles, licenseRepositories)
			verifyCorpusArtifact(t, oracle.File, oracle.SourceProject, oracle.SourceRepository, oracle.SourceRevision, oracle.SourcePath, oracle.SourceURL, oracle.SHA256, oracle.ByteLength, true, declaredFiles)
		}
	}

	entries, err := os.ReadDir(internetCorpusDir)
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory in flat corpus: %s", entry.Name())
		}
		actual = append(actual, entry.Name())
	}
	sort.Strings(actual)
	for _, name := range actual {
		if !declaredFiles[name] {
			t.Fatalf("corpus contains undeclared file %q", name)
		}
	}
	for name := range declaredFiles {
		if _, err := os.Stat(filepath.Join(internetCorpusDir, name)); err != nil {
			t.Fatalf("declared corpus file %q: %v", name, err)
		}
	}
}

func verifyLicenseReferences(t *testing.T, file, sourceRepository string, refs []string, licenses map[string]string) {
	t.Helper()
	if len(refs) == 0 {
		t.Fatalf("corpus file %q has no license references", file)
	}
	for _, ref := range refs {
		licenseRepository, ok := licenses[ref]
		if !ok {
			t.Fatalf("corpus file %q references unknown license %q", file, ref)
		}
		if licenseRepository != sourceRepository {
			t.Fatalf("corpus file %q source %q references license %q from %q", file, sourceRepository, ref, licenseRepository)
		}
	}
}

func verifyCorpusArtifact(t *testing.T, file, project, repository, revision, sourcePath, sourceURL, wantSHA string, wantLength int, requireUTF8 bool, declared map[string]bool) {
	t.Helper()
	validateCorpusRelativePath(t, file)
	if declared[file] {
		t.Fatalf("duplicate corpus file declaration %q", file)
	}
	declared[file] = true
	if project == "" || repository == "" || sourcePath == "" || sourceURL == "" {
		t.Fatalf("corpus file %q has incomplete provenance", file)
	}
	if !approvedCorpusRepositories[repository] {
		t.Fatalf("corpus file %q uses unapproved source repository %q", file, repository)
	}
	if !fullGitRevisionPattern.MatchString(revision) {
		t.Fatalf("corpus file %q source revision %q is not a full Git commit", file, revision)
	}
	rawRepository := strings.Replace(repository, "https://github.com/", "https://raw.githubusercontent.com/", 1)
	expectedSourceURL := rawRepository + "/" + revision + "/" + sourcePath
	if sourceURL != expectedSourceURL {
		t.Fatalf("corpus file %q source URL = %q, want %q", file, sourceURL, expectedSourceURL)
	}

	content, err := os.ReadFile(filepath.Join(internetCorpusDir, file))
	if err != nil {
		t.Fatalf("read corpus file %q: %v", file, err)
	}
	if len(content) != wantLength {
		t.Fatalf("corpus file %q byte length = %d, want %d", file, len(content), wantLength)
	}
	sum := sha256.Sum256(content)
	actualSHA := hex.EncodeToString(sum[:])
	if actualSHA != wantSHA {
		t.Fatalf("corpus file %q SHA-256 = %s, want %s", file, actualSHA, wantSHA)
	}
	if requireUTF8 && !utf8.Valid(content) {
		t.Fatalf("oracle %q is not valid UTF-8", file)
	}
}

func validateCorpusRelativePath(t *testing.T, path string) {
	t.Helper()
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		t.Fatalf("unsafe corpus relative path %q", path)
	}
	if strings.ContainsAny(path, `/\\`) {
		t.Fatalf("corpus file %q must stay in the flat corpus directory", path)
	}
}
