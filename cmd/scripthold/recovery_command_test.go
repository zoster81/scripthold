package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBackupRecoveryCommandsStrictly(t *testing.T) {
	root := canonicalBackupTestTempDir(t)
	store := filepath.Join(root, "source")
	output := filepath.Join(root, "plan.json")
	plan := filepath.Join(root, "reviewed-plan.json")
	destination := filepath.Join(root, "recovered")
	report := filepath.Join(root, "report.json")

	got, matched, err := parseBackupRecoveryCommand([]string{
		"backup-store", "recover-plan", "--store", store, "--output=" + output,
		"--max-manifests", "17", "--max-objects=19", "--max-bytes", "23", "--pretty",
	})
	if err != nil || !matched {
		t.Fatalf("plan matched=%v got=%#v err=%v", matched, got, err)
	}
	if got.kind != backupRecoveryPlanCommand || got.store != store || got.output != output ||
		got.maxManifests != 17 || got.maxObjects != 19 || got.maxBytes != 23 || !got.pretty {
		t.Fatalf("plan options=%#v", got)
	}

	got, matched, err = parseBackupRecoveryCommand([]string{
		"backup-store", "recover-apply", "--store=" + store, "--plan", plan,
		"--destination=" + destination, "--report", report, "--pretty",
	})
	if err != nil || !matched {
		t.Fatalf("apply matched=%v got=%#v err=%v", matched, got, err)
	}
	if got.kind != backupRecoveryApplyCommand || got.store != store || got.plan != plan ||
		got.destination != destination || got.report != report || !got.pretty {
		t.Fatalf("apply options=%#v", got)
	}

	invalid := [][]string{
		{"backup-store", "recover-plan", "--store", store},
		{"backup-store", "recover-plan", "--store", store, "--output", "relative.json"},
		{"backup-store", "recover-plan", "--store", store, "--output", output, "--max-manifests=0"},
		{"backup-store", "recover-plan", "--store", store, "--output", output, "--max-objects=-1"},
		{"backup-store", "recover-plan", "--store", store, "--output", output, "--max-bytes=0"},
		{"backup-store", "recover-plan", "--store", store, "--output", output, "--pretty", "--pretty"},
		{"backup-store", "recover-plan", "--store", store, "--store", store, "--output", output},
		{"backup-store", "recover-plan", "--store", store, "--output", output, "--unknown"},
		{"backup-store", "recover-apply", "--store", store, "--plan", plan, "--destination", destination},
		{"backup-store", "recover-apply", "--store", store, "--plan", "relative.json", "--destination", destination, "--report", report},
		{"backup-store", "recover-apply", "--store", store, "--plan", plan, "--destination", "relative", "--report", report},
		{"backup-store", "recover-apply", "--store", store, "--plan", plan, "--destination", destination, "--report", report, "--max-bytes=1"},
		{"backup-store", "recover-apply", "--store", store, "--plan", plan, "--destination", destination, "--report", report, "--report", report},
	}
	for _, args := range invalid {
		if _, matched, err := parseBackupRecoveryCommand(args); !matched || err == nil {
			t.Fatalf("invalid args=%v matched=%v err=%v", args, matched, err)
		}
	}
	for _, args := range [][]string{nil, {"backup-store"}, {"backup-store", "diagnose"}, {"backup-store", "other"}} {
		if _, matched, err := parseBackupRecoveryCommand(args); matched || err != nil {
			t.Fatalf("non-recovery args=%v matched=%v err=%v", args, matched, err)
		}
	}
}

func FuzzParseBackupRecoveryCommand(f *testing.F) {
	for _, seed := range []string{
		"backup-store\x00recover-plan\x00--store=C:\\source\x00--output=C:\\plan.json",
		"backup-store\x00recover-apply\x00--store=C:\\source\x00--plan=C:\\plan.json\x00--destination=C:\\destination\x00--report=C:\\report.json",
		"backup-store\x00recover-plan\x00--unknown",
		"backup-store\x00recover-apply\x00--pretty\x00--pretty",
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
		options, matched, err := parseBackupRecoveryCommand(args)
		expectedMatch := len(args) >= 2 && args[0] == "backup-store" &&
			(args[1] == string(backupRecoveryPlanCommand) || args[1] == string(backupRecoveryApplyCommand))
		if matched != expectedMatch {
			t.Fatalf("matched=%v want=%v args=%q", matched, expectedMatch, args)
		}
		if err == nil && matched {
			if options.store == "" || !filepath.IsAbs(options.store) {
				t.Fatalf("accepted recovery command without absolute store: %#v args=%q", options, args)
			}
			switch options.kind {
			case backupRecoveryPlanCommand:
				if options.output == "" || !filepath.IsAbs(options.output) {
					t.Fatalf("accepted recovery plan command without absolute output: %#v args=%q", options, args)
				}
			case backupRecoveryApplyCommand:
				if options.plan == "" || options.destination == "" || options.report == "" ||
					!filepath.IsAbs(options.plan) || !filepath.IsAbs(options.destination) || !filepath.IsAbs(options.report) {
					t.Fatalf("accepted recovery apply command without absolute paths: %#v args=%q", options, args)
				}
			default:
				t.Fatalf("accepted unknown recovery command kind: %#v", options)
			}
		}
	})
}
