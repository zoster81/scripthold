package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
)

func TestParseBackupDiagnosticCommandIsStrictAndUnambiguous(t *testing.T) {
	options, matched, err := parseBackupDiagnosticCommand([]string{
		"backup-store", "diagnose",
		"--store", `C:\store`,
		"--mode=full",
		"--max-objects", "17",
		"--max-bytes=23",
		"--pretty",
	})
	if err != nil || !matched {
		t.Fatalf("parse diagnostic command matched=%v options=%#v err=%v", matched, options, err)
	}
	if options.store != `C:\store` || options.mode != backupstore.AuditFull || options.maxObjects != 17 || options.maxBytes != 23 || !options.pretty {
		t.Fatalf("diagnostic options=%#v", options)
	}

	for _, args := range [][]string{
		nil,
		{"backup-store"},
		{"backup-store", "unknown"},
		{"--", "backup-store", "diagnose"},
		{filepath.Join(t.TempDir(), "backup-store"), "diagnose"},
	} {
		if _, matched, err := parseBackupDiagnosticCommand(args); err != nil || matched {
			t.Fatalf("non-command args=%v matched=%v err=%v", args, matched, err)
		}
	}

	invalid := []struct {
		name string
		args []string
	}{
		{name: "missing store", args: []string{"backup-store", "diagnose"}},
		{name: "store without value", args: []string{"backup-store", "diagnose", "--store"}},
		{name: "store consumes flag", args: []string{"backup-store", "diagnose", "--store", "--mode=full"}},
		{name: "empty store", args: []string{"backup-store", "diagnose", "--store="}},
		{name: "duplicate store", args: []string{"backup-store", "diagnose", "--store=a", "--store=b"}},
		{name: "invalid mode", args: []string{"backup-store", "diagnose", "--store=a", "--mode=deep"}},
		{name: "duplicate mode", args: []string{"backup-store", "diagnose", "--store=a", "--mode=quick", "--mode=full"}},
		{name: "zero objects", args: []string{"backup-store", "diagnose", "--store=a", "--max-objects=0"}},
		{name: "negative objects", args: []string{"backup-store", "diagnose", "--store=a", "--max-objects=-1"}},
		{name: "overflow objects", args: []string{"backup-store", "diagnose", "--store=a", "--max-objects=999999999999999999999999"}},
		{name: "zero bytes", args: []string{"backup-store", "diagnose", "--store=a", "--max-bytes=0"}},
		{name: "duplicate pretty", args: []string{"backup-store", "diagnose", "--store=a", "--pretty", "--pretty"}},
		{name: "unknown option", args: []string{"backup-store", "diagnose", "--store=a", "--repair"}},
		{name: "positional", args: []string{"backup-store", "diagnose", "--store=a", "extra"}},
		{name: "separator", args: []string{"backup-store", "diagnose", "--store=a", "--"}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, matched, err := parseBackupDiagnosticCommand(tc.args); !matched || err == nil {
				t.Fatalf("invalid args matched=%v err=%v", matched, err)
			}
		})
	}
}

func FuzzParseBackupDiagnosticCommand(f *testing.F) {
	for _, seed := range []string{
		"backup-store\x00diagnose\x00--store=/tmp/store",
		"backup-store\x00diagnose\x00--store=C:\\store\x00--mode=full\x00--pretty",
		"backup-store\x00unknown",
		"--\x00backup-store\x00diagnose",
		"backup-store\x00diagnose\x00--repair",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		args := strings.Split(raw, "\x00")
		if len(args) > 32 {
			t.Skip()
		}
		options, matched, err := parseBackupDiagnosticCommand(args)
		expectedMatch := len(args) >= 2 && args[0] == "backup-store" && args[1] == "diagnose"
		if matched != expectedMatch {
			t.Fatalf("matched=%v want=%v args=%q", matched, expectedMatch, args)
		}
		if err == nil && matched {
			if options.store == "" || (options.mode != backupstore.AuditQuick && options.mode != backupstore.AuditFull) ||
				options.maxObjects < 0 || options.maxBytes < 0 {
				t.Fatalf("accepted invalid options=%#v args=%q", options, args)
			}
		}
	})
}

func TestRunCommandBackupStoreDiagnoseHealthyAndMaintenanceReports(t *testing.T) {
	root := filepath.Join(canonicalBackupTestTempDir(t), "backup-store")
	store, err := backupstore.Open(backupstore.Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestedEnvironment := make(map[string]int)
	getenv := func(name string) string {
		requestedEnvironment[name]++
		if name == config.EnvBackupStoreDir {
			return filepath.Join(t.TempDir(), "ambient-store-must-not-be-used")
		}
		return ""
	}
	code := runCommand(context.Background(), []string{"backup-store", "diagnose", "--store", root}, &stdout, &stderr, getenv)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("healthy diagnosis code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if requestedEnvironment[config.EnvBackupStoreDir] != 0 {
		t.Fatalf("diagnostic command read %s", config.EnvBackupStoreDir)
	}
	var healthy backupstore.DiagnosticReport
	if err := json.Unmarshal(stdout.Bytes(), &healthy); err != nil {
		t.Fatalf("decode healthy report: %v\n%s", err, stdout.String())
	}
	if !healthy.SafeForNormalOpen || len(healthy.Issues) != 0 {
		t.Fatalf("healthy report=%#v", healthy)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("healthy report exposed store path: %s", stdout.String())
	}

	if err := os.Remove(filepath.Join(root, "index", "index-v1.json")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = runCommand(context.Background(), []string{"backup-store", "diagnose", "--store=" + root, "--pretty"}, &stdout, &stderr, func(string) string { return "" })
	if code != 2 || stderr.Len() != 0 {
		t.Fatalf("maintenance diagnosis code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\n  \"formatVersion\"")) {
		t.Fatalf("pretty JSON not emitted: %q", stdout.String())
	}
	var maintenance backupstore.DiagnosticReport
	if err := json.Unmarshal(stdout.Bytes(), &maintenance); err != nil {
		t.Fatal(err)
	}
	if !maintenance.SafeForNormalOpen || len(maintenance.Issues) == 0 {
		t.Fatalf("maintenance report=%#v", maintenance)
	}
}

func TestRunCommandBackupStoreDiagnoseErrorsArePathFreeAndNonMutating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-store")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{"backup-store", "diagnose", "--store", root}, &stdout, &stderr, func(string) string { return "" })
	if code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("missing diagnosis code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), root) {
		t.Fatalf("diagnostic stderr exposed store path: %q", stderr.String())
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("diagnostic command created missing store: %v", err)
	}

	activeRoot := filepath.Join(canonicalBackupTestTempDir(t), "active-store")
	active, err := backupstore.Open(backupstore.Options{Directory: activeRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	stdout.Reset()
	stderr.Reset()
	code = runCommand(context.Background(), []string{"backup-store", "diagnose", "--store", activeRoot}, &stdout, &stderr, func(string) string { return "" })
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "already in use") {
		t.Fatalf("active diagnosis code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), activeRoot) {
		t.Fatalf("active diagnostic stderr exposed store path: %q", stderr.String())
	}
}

func TestRunCommandBackupStoreDiagnoseHonorsOutputLimit(t *testing.T) {
	root := filepath.Join(canonicalBackupTestTempDir(t), "backup-store")
	store, err := backupstore.Open(backupstore.Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{"backup-store", "diagnose", "--store", root}, &stdout, &stderr, func(name string) string {
		if name == config.EnvMaxOutputBytes {
			return "1"
		}
		return ""
	})
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "output") {
		t.Fatalf("output-limited diagnosis code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func canonicalBackupTestTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve temporary directory: %v", err)
	}
	return filepath.Clean(resolved)
}
