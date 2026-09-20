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
		next, handled, err := parseBackupRecoveryCommonArgument(&options, seen, args, index)
		if err != nil {
			return backupRecoveryCommandOptions{}, true, err
		}
		if handled {
			index = next
			continue
		}

		switch kind {
		case backupRecoveryPlanCommand:
			next, handled, err = parseBackupRecoveryPlanArgument(&options, seen, args, index)
		case backupRecoveryApplyCommand:
			next, handled, err = parseBackupRecoveryApplyArgument(&options, seen, args, index)
		}
		if err != nil {
			return backupRecoveryCommandOptions{}, true, err
		}
		if !handled {
			return backupRecoveryCommandOptions{}, true, errors.New("unsupported backup recovery argument")
		}
		index = next
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

func parseBackupRecoveryCommonArgument(options *backupRecoveryCommandOptions, seen map[string]bool, args []string, index int) (int, bool, error) {
	argument := args[index]
	switch {
	case argument == "--pretty":
		if seen["pretty"] {
			return index, true, errors.New("--pretty may be specified only once")
		}
		seen["pretty"] = true
		options.pretty = true
		return index, true, nil
	case argument == "--store":
		value, next, err := diagnosticOptionValue(args, index, "--store")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPathOption(&options.store, seen, "store", "--store", value)
	case strings.HasPrefix(argument, "--store="):
		err := setRecoveryPathOption(&options.store, seen, "store", "--store", strings.TrimPrefix(argument, "--store="))
		return index, true, err
	default:
		return index, false, nil
	}
}

func parseBackupRecoveryPlanArgument(options *backupRecoveryCommandOptions, seen map[string]bool, args []string, index int) (int, bool, error) {
	argument := args[index]
	switch {
	case argument == "--output":
		value, next, err := diagnosticOptionValue(args, index, "--output")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPathOption(&options.output, seen, "output", "--output", value)
	case strings.HasPrefix(argument, "--output="):
		err := setRecoveryPathOption(&options.output, seen, "output", "--output", strings.TrimPrefix(argument, "--output="))
		return index, true, err
	case argument == "--max-manifests":
		value, next, err := diagnosticOptionValue(args, index, "--max-manifests")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPositiveIntOption(&options.maxManifests, seen, "max-manifests", "--max-manifests", value)
	case strings.HasPrefix(argument, "--max-manifests="):
		err := setRecoveryPositiveIntOption(&options.maxManifests, seen, "max-manifests", "--max-manifests", strings.TrimPrefix(argument, "--max-manifests="))
		return index, true, err
	case argument == "--max-objects":
		value, next, err := diagnosticOptionValue(args, index, "--max-objects")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPositiveIntOption(&options.maxObjects, seen, "max-objects", "--max-objects", value)
	case strings.HasPrefix(argument, "--max-objects="):
		err := setRecoveryPositiveIntOption(&options.maxObjects, seen, "max-objects", "--max-objects", strings.TrimPrefix(argument, "--max-objects="))
		return index, true, err
	case argument == "--max-bytes":
		value, next, err := diagnosticOptionValue(args, index, "--max-bytes")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPositiveInt64Option(&options.maxBytes, seen, "max-bytes", "--max-bytes", value)
	case strings.HasPrefix(argument, "--max-bytes="):
		err := setRecoveryPositiveInt64Option(&options.maxBytes, seen, "max-bytes", "--max-bytes", strings.TrimPrefix(argument, "--max-bytes="))
		return index, true, err
	default:
		return index, false, nil
	}
}

func parseBackupRecoveryApplyArgument(options *backupRecoveryCommandOptions, seen map[string]bool, args []string, index int) (int, bool, error) {
	argument := args[index]
	switch {
	case argument == "--plan":
		value, next, err := diagnosticOptionValue(args, index, "--plan")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPathOption(&options.plan, seen, "plan", "--plan", value)
	case strings.HasPrefix(argument, "--plan="):
		err := setRecoveryPathOption(&options.plan, seen, "plan", "--plan", strings.TrimPrefix(argument, "--plan="))
		return index, true, err
	case argument == "--destination":
		value, next, err := diagnosticOptionValue(args, index, "--destination")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPathOption(&options.destination, seen, "destination", "--destination", value)
	case strings.HasPrefix(argument, "--destination="):
		err := setRecoveryPathOption(&options.destination, seen, "destination", "--destination", strings.TrimPrefix(argument, "--destination="))
		return index, true, err
	case argument == "--report":
		value, next, err := diagnosticOptionValue(args, index, "--report")
		if err != nil {
			return next, true, err
		}
		return next, true, setRecoveryPathOption(&options.report, seen, "report", "--report", value)
	case strings.HasPrefix(argument, "--report="):
		err := setRecoveryPathOption(&options.report, seen, "report", "--report", strings.TrimPrefix(argument, "--report="))
		return index, true, err
	default:
		return index, false, nil
	}
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
