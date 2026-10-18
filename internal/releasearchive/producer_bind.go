package releasearchive

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// NormalProducerSource identifies one normal release original and its
// producer-local source *beneath an independently trusted dist root*. These
// paths must not cross jobs as release authority.
type NormalProducerSource struct {
	Original     ExpectedFile `json:"original"`
	SourceSubdir string       `json:"sourceSubdir"`
	SourceLeaf   string       `json:"sourceLeaf"`
}

type producerSourceLocation struct {
	name, subdir, leaf string
}

var normalProducerTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// canonicalProducerSources is an independent, fixed file-layout contract for
// the pinned GoReleaser normal outputs plus two original signed sidecars.
// A new producer layout must be explicitly reviewed rather than inferred from
// arbitrary paths found in artifacts.json.
func canonicalProducerSources(tag string) ([]producerSourceLocation, error) {
	if len(tag) > 100 || !normalProducerTagPattern.MatchString(tag) {
		return nil, errors.New("invalid normal release tag")
	}
	sources := make([]producerSourceLocation, 0, 15)
	for _, osName := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			stem := "scripthold_" + osName + "_" + arch
			variant := "v8.0"
			if arch == "amd64" {
				variant = "v1"
			}
			binaryLeaf, binaryExtension, archiveExtension := "scripthold", "", ".tar.gz"
			if osName == "windows" {
				binaryLeaf, binaryExtension, archiveExtension = "scripthold.exe", ".exe", ".zip"
			}
			sources = append(sources,
				producerSourceLocation{stem + binaryExtension, stem + "_" + variant, binaryLeaf},
				producerSourceLocation{stem + archiveExtension, "", stem + archiveExtension},
			)
		}
	}
	bundle := "scripthold-" + tag + "-provenance.sigstore.json"
	jsonl := "scripthold-" + tag + "-provenance.intoto.jsonl"
	sources = append(sources, producerSourceLocation{"checksums.txt", "", "checksums.txt"},
		producerSourceLocation{bundle, "", bundle}, producerSourceLocation{jsonl, "", jsonl})
	for _, item := range sources {
		if !portableStagingName(item.name) || !portableStagingName(item.leaf) ||
			(item.subdir != "" && !portableStagingName(item.subdir)) {
			return nil, errors.New("normal producer output name exceeds the portable limit")
		}
	}
	return sources, nil
}

// BindNormalProducerSourcePlan verifies the complete fifteen-item name,
// digest, size, and producer-local layout mapping against independent trusted
// ExpectedFile declarations. It is a pure structural check: the caller must
// independently authenticate the trusted inventory and signature authority,
// and later verify every actual source byte via the rooted Linux copy API.
// Return values own their strings/structs; no filesystem or publication occurs.
func BindNormalProducerSourcePlan(tag string, plan []NormalProducerSource, trusted []ExpectedFile) ([]NormalProducerSource, error) {
	locations, err := canonicalProducerSources(tag)
	if err != nil {
		return nil, err
	}
	if len(plan) != len(locations) || len(trusted) != len(locations) {
		return nil, errors.New("normal producer requires exactly fifteen originals")
	}
	byName := make(map[string]NormalProducerSource, len(locations))
	trustedByName := make(map[string]ExpectedFile, len(locations))
	expected := make(map[string]producerSourceLocation, len(locations))
	for _, loc := range locations {
		expected[loc.name] = loc
	}
	seenTrusted := make(map[string]bool, len(locations))
	for _, file := range trusted {
		loc, ok := expected[file.Name]
		if !ok || seenTrusted[strings.ToLower(file.Name)] || !validProducerOriginal(file, loc.name) {
			return nil, errors.New("invalid or duplicate trusted normal producer original")
		}
		seenTrusted[strings.ToLower(file.Name)] = true
		trustedByName[file.Name] = file
	}
	for _, file := range plan {
		loc, ok := expected[file.Original.Name]
		if !ok || !validProducerOriginal(file.Original, loc.name) ||
			file.SourceSubdir != loc.subdir || file.SourceLeaf != loc.leaf ||
			!portableStagingName(file.SourceLeaf) ||
			(file.SourceSubdir != "" && !portableStagingName(file.SourceSubdir)) {
			return nil, fmt.Errorf("invalid normal producer source mapping for %q", file.Original.Name)
		}
		if _, duplicate := byName[file.Original.Name]; duplicate {
			return nil, errors.New("duplicate normal producer original")
		}
		if file.Original != trustedByName[file.Original.Name] {
			return nil, errors.New("normal producer original differs from independently trusted bytes")
		}
		byName[file.Original.Name] = file
	}
	result := make([]NormalProducerSource, 0, len(locations))
	for _, loc := range locations {
		file, ok := byName[loc.name]
		if !ok {
			return nil, errors.New("missing normal producer original")
		}
		result = append(result, file)
	}
	return result, nil
}

func validProducerOriginal(file ExpectedFile, name string) bool {
	if file.Name != name || !portableStagingName(file.Name) ||
		file.Size <= 0 || file.Size > maxAssetBytes || !validDigest(file.SHA256) {
		return false
	}
	if name == "checksums.txt" && file.Size > 16*1024 {
		return false
	}
	if (strings.HasSuffix(name, ".sigstore.json") || strings.HasSuffix(name, ".intoto.jsonl")) &&
		file.Size > 4*1024*1024 {
		return false
	}
	return true
}
