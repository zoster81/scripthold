package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"2.0.0", "1.0.0", true},
		{"1.1.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "2.0.0", false},
		{"v1.1.0", "1.0.0", true},
		{"1.1.0-beta", "1.0.0", true},
		{"2.1.0", "2.1.0-internal-9de5ebc", true},
		{"2.1.0-internal.10", "2.1.0-internal.2", true},
		{"2.1.0+build.2", "2.1.0+build.1", false},
		{"2.1.0-beta", "2.1.0", false},
		{"invalid", "1.0.0", false},
	}
	for _, test := range tests {
		if got := isNewerVersion(test.latest, test.current); got != test.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", test.latest, test.current, got, test.want)
		}
	}
}

func TestParseSemanticVersion(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"1.2.3", true},
		{"v1.2.3", true},
		{"1.2.3-beta.1+build.7", true},
		{"1.2", false},
		{"1", false},
		{"01.2.3", false},
		{"1.2.3-beta.01", false},
		{"1.2.3+", false},
		{"", false},
	}
	for _, test := range tests {
		_, valid := parseSemanticVersion(test.input)
		if valid != test.valid {
			t.Errorf("parseSemanticVersion(%q) valid = %v, want %v", test.input, valid, test.valid)
		}
	}
}

func TestFetchLatestVersionFrom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "scripthold-update-checker" {
			t.Errorf("unexpected User-Agent %q", request.Header.Get("User-Agent"))
		}
		_, _ = fmt.Fprint(writer, `{"tag_name":"v2.1.0"}`)
	}))
	defer server.Close()

	version, err := fetchLatestVersionFrom(context.Background(), server.Client(), server.URL)
	if err != nil || version != "2.1.0" {
		t.Fatalf("fetchLatestVersionFrom() = %q, %v", version, err)
	}
}

func TestFetchLatestVersionRejectsInvalidOrOversizedResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid tag", body: `{"tag_name":"release"}`},
		{name: "multiple values", body: `{"tag_name":"v2.1.0"} {}`},
		{name: "oversized", body: `{"tag_name":"v2.1.0","body":"` + strings.Repeat("x", maxUpdateResponseBytes) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, test.body)
			}))
			defer server.Close()
			if _, err := fetchLatestVersionFrom(context.Background(), server.Client(), server.URL); err == nil {
				t.Fatal("expected response rejection")
			}
		})
	}
}

func TestCacheFreshnessIncludesFailuresAndRejectsFutureOrInvalidEntries(t *testing.T) {
	now := time.Now().UTC()
	if !cacheIsFresh(&cache{Source: UpdateCheckURL, LastCheck: now.Add(-time.Minute)}, now) {
		t.Fatal("a recent cached network failure must suppress another request")
	}
	if cacheIsFresh(&cache{Source: UpdateCheckURL, LastCheck: now.Add(time.Minute), LatestVersion: "2.1.0"}, now) {
		t.Fatal("a future cache timestamp must not remain fresh")
	}
	if cacheIsFresh(&cache{Source: UpdateCheckURL, LastCheck: now, LatestVersion: "invalid"}, now) {
		t.Fatal("an invalid cached version must not remain fresh")
	}
}

func TestCacheRoundTripIsBoundedAndLeavesNoTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "update-check.json")
	if err := writeCache(path, "2.1.0"); err != nil {
		t.Fatal(err)
	}
	cached := readCache(path)
	if cached == nil || cached.LatestVersion != "2.1.0" || cached.Source != UpdateCheckURL {
		t.Fatalf("unexpected cache: %#v", cached)
	}
	if err := writeCache(path, "2.2.0"); err != nil {
		t.Fatalf("replace cache atomically: %v", err)
	}
	if cached := readCache(path); cached == nil || cached.LatestVersion != "2.2.0" {
		t.Fatalf("unexpected replaced cache: %#v", cached)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("unexpected cache directory entries: %v", entries)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxCacheBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if cached := readCache(path); cached != nil {
		t.Fatalf("oversized cache must be rejected: %#v", cached)
	}
}

func TestCheckDisabled(t *testing.T) {
	t.Setenv("MCP_NO_UPDATE_CHECK", "1")
	if msg := Check(context.Background(), "1.0.0", false); msg != "" {
		t.Errorf("Check with disabled should return empty, got %q", msg)
	}
}

func TestCheckDevVersion(t *testing.T) {
	if msg := Check(context.Background(), "dev", false); msg != "" {
		t.Errorf("Check with dev version should return empty, got %q", msg)
	}
	if msg := Check(context.Background(), "", false); msg != "" {
		t.Errorf("Check with empty version should return empty, got %q", msg)
	}
}

func TestForkUpdateSource(t *testing.T) {
	const expectedAPI = "https://api.github.com/repos/zoster81/scripthold/releases/latest"
	const expectedRepo = "https://github.com/zoster81/scripthold"
	if UpdateCheckURL != expectedAPI {
		t.Fatalf("UpdateCheckURL = %q, want %q", UpdateCheckURL, expectedAPI)
	}
	if RepoURL != expectedRepo || ReleaseURL != expectedRepo+"/releases/latest" {
		t.Fatalf("unexpected fork URLs: %q, %q", RepoURL, ReleaseURL)
	}
}

func TestCacheMatchesCurrentSource(t *testing.T) {
	if cacheMatchesSource(nil) {
		t.Fatal("nil cache must not match")
	}
	if cacheMatchesSource(&cache{Source: "https://example.com/releases/latest"}) {
		t.Fatal("cache from another release source must not match")
	}
	if !cacheMatchesSource(&cache{Source: UpdateCheckURL}) {
		t.Fatal("cache from the configured fork must match")
	}
}

func TestUpdateMessageFormat(t *testing.T) {
	msg := updateMessage("1.0.0", "1.1.0")
	for _, expected := range []string{"1.0.0", "1.1.0", ReleaseURL, "tunnel or MCP client"} {
		if !strings.Contains(msg, expected) {
			t.Errorf("message %q does not contain %q", msg, expected)
		}
	}
	if strings.Contains(strings.ToLower(msg), "claude") {
		t.Errorf("message must be client-neutral: %q", msg)
	}
}
