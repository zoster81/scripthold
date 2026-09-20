package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveTagCommitFromBase(t *testing.T) {
	t.Run("lightweight", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/git/ref/tags/v3.3.0" {
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
			_, _ = fmt.Fprint(writer, `{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}`)
		}))
		defer server.Close()

		got, err := resolveTagCommitFromBase(context.Background(), server.Client(), server.URL, "v3.3.0")
		if err != nil || got != "0123456789abcdef0123456789abcdef01234567" {
			t.Fatalf("resolve = %q, %v", got, err)
		}
	})

	t.Run("annotated", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/git/ref/tags/v3.3.0":
				_, _ = fmt.Fprint(writer, `{"ref":"refs/tags/v3.3.0","object":{"type":"tag","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
			case "/git/tags/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa":
				_, _ = fmt.Fprint(writer, `{"tag":"v3.3.0","object":{"type":"commit","sha":"89abcdef0123456789abcdef0123456789abcdef"}}`)
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
		}))
		defer server.Close()

		got, err := resolveTagCommitFromBase(context.Background(), server.Client(), server.URL, "v3.3.0")
		if err != nil || got != "89abcdef0123456789abcdef0123456789abcdef" {
			t.Fatalf("resolve = %q, %v", got, err)
		}
	})
}

func TestResolveTagCommitRejectsUnsafeEvidence(t *testing.T) {
	tests := []string{
		`{"ref":"refs/tags/other","object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}`,
		`{"ref":"refs/tags/v3.3.0","object":{"type":"tree","sha":"0123456789abcdef0123456789abcdef01234567"}}`,
		`{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"ABC"}}`,
	}
	for _, body := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(writer, body)
		}))
		_, err := resolveTagCommitFromBase(context.Background(), server.Client(), server.URL, "v3.3.0")
		server.Close()
		if err == nil {
			t.Fatalf("unsafe tag evidence accepted: %s", body)
		}
	}
}

func TestValidateCandidateBuildInfo(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/zoster81/scripthold"},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: "linux"},
			{Key: "GOARCH", Value: "amd64"},
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	if err := validateCandidateBuildInfo(info, "linux", "amd64", "0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*debug.BuildInfo)
	}{
		{name: "wrong module", mutate: func(value *debug.BuildInfo) { value.Main.Path = "example.com/other" }},
		{name: "dirty", mutate: func(value *debug.BuildInfo) { setBuildSetting(value, "vcs.modified", "true") }},
		{name: "wrong os", mutate: func(value *debug.BuildInfo) { setBuildSetting(value, "GOOS", "darwin") }},
		{name: "wrong revision", mutate: func(value *debug.BuildInfo) { setBuildSetting(value, "vcs.revision", strings.Repeat("a", 40)) }},
		{name: "wrong vcs", mutate: func(value *debug.BuildInfo) { setBuildSetting(value, "vcs", "other") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			copyInfo := *info
			copyInfo.Settings = append([]debug.BuildSetting(nil), info.Settings...)
			test.mutate(&copyInfo)
			if err := validateCandidateBuildInfo(&copyInfo, "linux", "amd64", "0123456789abcdef0123456789abcdef01234567"); err == nil {
				t.Fatal("expected build-info rejection")
			}
		})
	}
}

func setBuildSetting(info *debug.BuildInfo, key, value string) {
	for index := range info.Settings {
		if info.Settings[index].Key == key {
			info.Settings[index].Value = value
			return
		}
	}
	info.Settings = append(info.Settings, debug.BuildSetting{Key: key, Value: value})
}
