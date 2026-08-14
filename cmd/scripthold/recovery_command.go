package main

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zoster81/scripthold/internal/backupstore"
)

type backupRecoveryCommandKind string

const (
	backupRecoveryPlanCommand  backupRecoveryCommandKind = "recover-plan"
	backupRecoveryApplyCommand backupRecoveryCommandKind = "recover-apply"
)

type backupRecoveryCommandOptions struct {
	kind        backupRecoveryCommandKind
	store       string
	output      string
	plan        string
	destination string
	report      string

	maxManifests int
	maxObjects   int
	maxBytes     int64
	pretty       bool
}

func parseBackupRecoveryCommand(args []string) (backupRecoveryCommandOptions, bool, error) {
	if len(args) < 2 || args[0] != "backup-store" {
		return backupRecoveryCommandOptions{}, false, nil
	}
	kind := backupRecoveryCommandKind(args[1])
	if kind != backupRecoveryPlanCommand && kind != backupRecoveryApplyCommand {
		return backupRecoveryCommandOptions{}, false, nil
	}

	options := backupRecoveryCommandOptions{kind: kind}
	seen := make(map[string]bool)
	for index := 2; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--pretty":
			if seen["pretty"] {
				return backupRecoveryCommandOptions{}, true, errors.New("--pretty may be specified only once")
			}
			seen["pretty"] = true
			options.pretty = true
		case argument == "--store":
			value, next, err := diagnosticOptionValue(args, index, "--store")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPathOption(&options.store, seen, "store", "--store", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case strings.HasPrefix(argument, "--store="):
			if err := setRecoveryPathOption(&options.store, seen, "store", "--store", strings.TrimPrefix(argument, "--store=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && argument == "--output":
			value, next, err := diagnosticOptionValue(args, index, "--output")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPathOption(&options.output, seen, "output", "--output", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && strings.HasPrefix(argument, "--output="):
			if err := setRecoveryPathOption(&options.output, seen, "output", "--output", strings.TrimPrefix(argument, "--output=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && argument == "--max-manifests":
			value, next, err := diagnosticOptionValue(args, index, "--max-manifests")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPositiveIntOption(&options.maxManifests, seen, "max-manifests", "--max-manifests", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && strings.HasPrefix(argument, "--max-manifests="):
			if err := setRecoveryPositiveIntOption(&options.maxManifests, seen, "max-manifests", "--max-manifests", strings.TrimPrefix(argument, "--max-manifests=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && argument == "--max-objects":
			value, next, err := diagnosticOptionValue(args, index, "--max-objects")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPositiveIntOption(&options.maxObjects, seen, "max-objects", "--max-objects", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && strings.HasPrefix(argument, "--max-objects="):
			if err := setRecoveryPositiveIntOption(&options.maxObjects, seen, "max-objects", "--max-objects", strings.TrimPrefix(argument, "--max-objects=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && argument == "--max-bytes":
			value, next, err := diagnosticOptionValue(args, index, "--max-bytes")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPositiveInt64Option(&options.maxBytes, seen, "max-bytes", "--max-bytes", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryPlanCommand && strings.HasPrefix(argument, "--max-bytes="):
			if err := setRecoveryPositiveInt64Option(&options.maxBytes, seen, "max-bytes", "--max-bytes", strings.TrimPrefix(argument, "--max-bytes=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && argument == "--plan":
			value, next, err := diagnosticOptionValue(args, index, "--plan")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPathOption(&options.plan, seen, "plan", "--plan", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && strings.HasPrefix(argument, "--plan="):
			if err := setRecoveryPathOption(&options.plan, seen, "plan", "--plan", strings.TrimPrefix(argument, "--plan=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && argument == "--destination":
			value, next, err := diagnosticOptionValue(args, index, "--destination")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPathOption(&options.destination, seen, "destination", "--destination", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && strings.HasPrefix(argument, "--destination="):
			if err := setRecoveryPathOption(&options.destination, seen, "destination", "--destination", strings.TrimPrefix(argument, "--destination=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && argument == "--report":
			value, next, err := diagnosticOptionValue(args, index, "--report")
			if err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
			index = next
			if err := setRecoveryPathOption(&options.report, seen, "report", "--report", value); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		case kind == backupRecoveryApplyCommand && strings.HasPrefix(argument, "--report="):
			if err := setRecoveryPathOption(&options.report, seen, "report", "--report", strings.TrimPrefix(argument, "--report=")); err != nil {
				return backupRecoveryCommandOptions{}, true, err
			}
		default:
			return backupRecoveryCommandOptions{}, true, errors.New("unsupported backup recovery argument")
		}
	}

	if options.store == "" {
		return backupRecoveryCommandOptions{}, true, errors.New("--store is required")
	}
	if kind == backupRecoveryPlanCommand {
		if options.output == "" {
			return backupRecoveryCommandOptions{}, true, errors.New("--output is required")
		}
		if _, err := backupstore.NormalizeRecoveryBounds(backupstore.RecoveryBounds{
			MaxManifests: options.maxManifests,
			MaxObjects:   options.maxObjects,
			MaxBytes:     options.maxBytes,
		}); err != nil {
			return backupRecoveryCommandOptions{}, true, err
		}
		return options, true, nil
	}
	if options.plan == "" || options.destination == "" || options.report == "" {
		return backupRecoveryCommandOptions{}, true, errors.New("--plan, --destination, and --report are required")
	}
	return options, true, nil
}

func setRecoveryPathOption(target *string, seen map[string]bool, key, name, value string) error {
	if seen[key] {
		return errors.New(name + " may be specified only once")
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') {
		return errors.New(name + " requires a non-empty absolute path")
	}
	clean := filepath.Clean(value)
	if !filepath.IsAbs(clean) {
		return errors.New(name + " requires an absolute path")
	}
	seen[key] = true
	*target = clean
	return nil
}

func setRecoveryPositiveIntOption(target *int, seen map[string]bool, key, name, value string) error {
	if seen[key] {
		return errors.New(name + " may be specified only once")
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return errors.New(name + " must be a positive integer")
	}
	seen[key] = true
	*target = parsed
	return nil
}

func setRecoveryPositiveInt64Option(target *int64, seen map[string]bool, key, name, value string) error {
	if seen[key] {
		return errors.New(name + " may be specified only once")
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return errors.New(name + " must be a positive integer")
	}
	seen[key] = true
	*target = parsed
	return nil
}
