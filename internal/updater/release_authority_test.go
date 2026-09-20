package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSelectReleaseAuthority(t *testing.T) {
	payload := []byte(`{
		"id":42,
		"tag_name":"v3.3.0",
		"draft":false,
		"prerelease":false,
		"immutable":true,
		"assets":[
			{"id":101,"name":"scripthold_windows_amd64.exe","state":"uploaded","size":4,"digest":"sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"},
			{"id":102,"name":"checksums.txt","state":"uploaded","size":80,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{"id":103,"name":"scripthold_windows_amd64.zip","state":"uploaded","size":100,"digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
		]
	}`)

	selected, err := selectReleaseAuthority(payload, "3.2.1", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ReleaseID != 42 || selected.Tag != "v3.3.0" || selected.Version != "3.3.0" {
		t.Fatalf("unexpected release selection: %#v", selected)
	}
	if selected.Binary.ID != 101 || selected.Binary.Name != "scripthold_windows_amd64.exe" {
		t.Fatalf("unexpected binary asset: %#v", selected.Binary)
	}
	if selected.Checksums.ID != 102 || selected.Checksums.Name != "checksums.txt" {
		t.Fatalf("unexpected checksums asset: %#v", selected.Checksums)
	}
}

func TestSelectReleaseAuthorityRejectsUnsafeMetadata(t *testing.T) {
	base := `{"id":42,"tag_name":"v3.3.0","draft":false,"prerelease":false,"immutable":true,"assets":[{"id":101,"name":"scripthold_linux_amd64","state":"uploaded","size":4,"digest":"sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"},{"id":102,"name":"checksums.txt","state":"uploaded","size":80,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	tests := []struct {
		name    string
		payload string
		current string
	}{
		{name: "draft", payload: strings.Replace(base, `"draft":false`, `"draft":true`, 1), current: "3.2.1"},
		{name: "prerelease", payload: strings.Replace(base, `"prerelease":false`, `"prerelease":true`, 1), current: "3.2.1"},
		{name: "mutable", payload: strings.Replace(base, `"immutable":true`, `"immutable":false`, 1), current: "3.2.1"},
		{name: "prerelease tag", payload: strings.Replace(base, "v3.3.0", "v3.3.0-rc.1", 1), current: "3.2.1"},
		{name: "double v tag", payload: strings.Replace(base, "v3.3.0", "vv3.3.0", 1), current: "3.2.1"},
		{name: "not newer", payload: base, current: "3.3.0"},
		{name: "bad digest", payload: strings.Replace(base, "sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589", "sha256:ABC", 1), current: "3.2.1"},
		{name: "not uploaded", payload: strings.Replace(base, `"state":"uploaded"`, `"state":"new"`, 1), current: "3.2.1"},
		{name: "zero size", payload: strings.Replace(base, `"size":4`, `"size":0`, 1), current: "3.2.1"},
		{name: "duplicate binary", payload: strings.Replace(base, `]}`, `,{"id":104,"name":"scripthold_linux_amd64","state":"uploaded","size":4,"digest":"sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}]}`, 1), current: "3.2.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := selectReleaseAuthority([]byte(test.payload), test.current, "linux", "amd64"); err == nil {
				t.Fatal("expected metadata rejection")
			}
		})
	}
}

func TestSelectReleaseAuthorityRejectsOversizedMetadata(t *testing.T) {
	payload := []byte(strings.Repeat("x", maxReleaseMetadataBytes+1))
	if _, err := selectReleaseAuthority(payload, "3.2.1", "linux", "amd64"); err == nil {
		t.Fatal("oversized release metadata must fail closed")
	}
}

func TestExpectedBinaryAssetName(t *testing.T) {
	tests := map[string]string{
		"windows/amd64": "scripthold_windows_amd64.exe",
		"windows/arm64": "scripthold_windows_arm64.exe",
		"linux/amd64":   "scripthold_linux_amd64",
		"linux/arm64":   "scripthold_linux_arm64",
		"darwin/amd64":  "scripthold_darwin_amd64",
		"darwin/arm64":  "scripthold_darwin_arm64",
	}
	for platform, want := range tests {
		parts := strings.Split(platform, "/")
		got, err := expectedBinaryAssetName(parts[0], parts[1])
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", platform, got, err, want)
		}
	}
	if _, err := expectedBinaryAssetName("plan9", "amd64"); err == nil {
		t.Fatal("unsupported platform must fail closed")
	}
}

func TestValidateGitHubAssetRedirect(t *testing.T) {
	valid, _ := url.Parse("https://release-assets.githubusercontent.com/path?token=opaque")
	if err := validateGitHubAssetRedirect(valid); err != nil {
		t.Fatalf("valid redirect rejected: %v", err)
	}
	for _, raw := range []string{
		"http://release-assets.githubusercontent.com/path",
		"https://user@release-assets.githubusercontent.com/path",
		"https://release-assets.githubusercontent.com:444/path",
		"https://objects.githubusercontent.com/path",
		"https://release-assets.githubusercontent.com.evil.test/path",
	} {
		location, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateGitHubAssetRedirect(location); err == nil {
			t.Fatalf("unsafe redirect accepted: %s", raw)
		}
	}
}

func TestDownloadAssetDirectAndDigestValidation(t *testing.T) {
	payload := []byte("abcd")
	sum := sha256.Sum256(payload)
	asset := releaseAsset{ID: 7, Name: "scripthold_linux_amd64", State: "uploaded", Size: int64(len(payload)), Digest: "sha256:" + hex.EncodeToString(sum[:])}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("Accept = %q", request.Header.Get("Accept"))
		}
		if request.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("Accept-Encoding = %q", request.Header.Get("Accept-Encoding"))
		}
		writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	var destination bytes.Buffer
	if err := downloadAssetFrom(context.Background(), server.Client(), server.URL, asset, &destination, func(*url.URL) error {
		return fmt.Errorf("unexpected redirect")
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(destination.Bytes(), payload) {
		t.Fatalf("download = %q, want %q", destination.Bytes(), payload)
	}

	bad := asset
	bad.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	destination.Reset()
	if err := downloadAssetFrom(context.Background(), server.Client(), server.URL, bad, &destination, func(*url.URL) error { return nil }); err == nil {
		t.Fatal("digest mismatch must fail")
	}
}

func TestDownloadAssetRejectsSecondRedirectAndWrongLength(t *testing.T) {
	asset := releaseAsset{ID: 7, Name: "candidate", State: "uploaded", Size: 4, Digest: "sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}

	second := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://example.invalid/again", http.StatusFound)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, second.URL, http.StatusFound)
	}))
	defer first.Close()

	err := downloadAssetFrom(context.Background(), first.Client(), first.URL, asset, &bytes.Buffer{}, func(location *url.URL) error {
		if location.String() != second.URL {
			return fmt.Errorf("unexpected redirect %s", location)
		}
		return nil
	})
	if err == nil {
		t.Fatal("second redirect must fail")
	}

	wrongLength := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Length", "3")
		_, _ = writer.Write([]byte("abc"))
	}))
	defer wrongLength.Close()
	if err := downloadAssetFrom(context.Background(), wrongLength.Client(), wrongLength.URL, asset, &bytes.Buffer{}, func(*url.URL) error { return nil }); err == nil {
		t.Fatal("wrong content length must fail")
	}
}

func TestChecksumsConsistency(t *testing.T) {
	asset := releaseAsset{Name: "scripthold_linux_amd64", Digest: "sha256:88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}
	valid := []byte("88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589  scripthold_linux_amd64\n")
	if err := validateChecksums(valid, asset); err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{
		[]byte("bad  scripthold_linux_amd64\n"),
		[]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  scripthold_linux_amd64\n"),
		[]byte("88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589  dir/scripthold_linux_amd64\n"),
		[]byte("88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589  scripthold_linux_amd64\n88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589  scripthold_linux_amd64\n"),
	} {
		if err := validateChecksums(payload, asset); err == nil {
			t.Fatalf("invalid checksums accepted: %q", payload)
		}
	}
}
