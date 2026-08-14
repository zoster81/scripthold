package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/backupstore"
)

func runBackupRecoveryCommand(
	ctx context.Context,
	options backupRecoveryCommandOptions,
	stdout io.Writer,
	stderr io.Writer,
) int {
	switch options.kind {
	case backupRecoveryPlanCommand:
		return runBackupRecoveryPlanCommand(ctx, options, stdout, stderr)
	case backupRecoveryApplyCommand:
		return runBackupRecoveryApplyCommand(ctx, options, stdout, stderr)
	default:
		fmt.Fprintln(stderr, "Error: unsupported backup recovery command")
		return 1
	}
}

func runBackupRecoveryPlanCommand(
	ctx context.Context,
	options backupRecoveryCommandOptions,
	stdout io.Writer,
	stderr io.Writer,
) int {
	store, err := backupstore.OpenExistingForDiagnosis(backupstore.DiagnosticOpenOptions{Directory: options.store})
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	plan, planErr := store.CreateRecoveryPlan(ctx, backupstore.RecoveryBounds{
		MaxManifests: options.maxManifests,
		MaxObjects:   options.maxObjects,
		MaxBytes:     options.maxBytes,
	})
	if planErr == nil {
		planErr = store.WriteRecoveryPlan(ctx, options.output, plan, options.pretty)
	}
	closeErr := store.Close()
	if planErr != nil {
		fmt.Fprintf(stderr, "Error: %v\n", planErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "Error: backup recovery source lock could not be released")
		return 1
	}

	encoded, err := backupstore.EncodeRecoveryPlan(plan, options.pretty)
	if err != nil {
		fmt.Fprintln(stderr, "Error: backup recovery plan could not be encoded")
		return 1
	}
	if err := writeBackupRecoveryOutput(stdout, encoded); err != nil {
		fmt.Fprintln(stderr, "Error: backup recovery output could not be written")
		return 1
	}
	return backupRecoveryPlanExitCode(plan, nil)
}

func runBackupRecoveryApplyCommand(
	ctx context.Context,
	options backupRecoveryCommandOptions,
	stdout io.Writer,
	stderr io.Writer,
) int {
	plan, err := backupstore.LoadRecoveryPlan(options.plan)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if !plan.Applicable || !plan.CoverageComplete {
		fmt.Fprintln(stderr, "Error: recovery plan is not applicable")
		return 1
	}

	store, err := backupstore.OpenExistingForDiagnosis(backupstore.DiagnosticOpenOptions{
		Directory: options.store,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	closeSource := true
	defer func() {
		if closeSource {
			_ = store.Close()
		}
	}()

	evidence, err := store.ScanRecoveryEvidence(ctx, plan.Bounds)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	freshPlan, err := backupstore.BuildRecoveryPlan(evidence)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if freshPlan.PlanID != plan.PlanID {
		fmt.Fprintln(stderr, "Error: recovery source evidence changed after plan review")
		return 1
	}

	destination, err := store.PrepareRecoveryDestination(ctx, options.plan, options.destination, options.report, plan)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	closeDestination := true
	defer func() {
		if closeDestination {
			_ = destination.Close()
		}
	}()

	if !destinationIsPromoted(destination) {
		if _, err := store.ReconstructRecoveryObjects(ctx, destination, plan, evidence); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if _, err := store.ReconstructRecoveryManifests(ctx, destination, plan, evidence); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if _, err := store.RebuildAndAuditRecoveryDestination(ctx, destination, plan); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
	}

	report, err := store.FinalizeRecoveryDestination(ctx, destination, plan, options.pretty)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := destination.Close(); err != nil {
		fmt.Fprintln(stderr, "Error: recovery destination lock could not be released")
		return 1
	}
	closeDestination = false
	if err := store.Close(); err != nil {
		fmt.Fprintln(stderr, "Error: backup recovery source lock could not be released")
		return 1
	}
	closeSource = false

	encoded, err := backupstore.EncodeRecoveryReport(report, options.pretty)
	if err != nil {
		fmt.Fprintln(stderr, "Error: backup recovery report could not be encoded")
		return 1
	}
	if err := writeBackupRecoveryOutput(stdout, encoded); err != nil {
		fmt.Fprintln(stderr, "Error: backup recovery output could not be written")
		return 1
	}
	return backupRecoveryApplyExitCode(nil)
}

func destinationIsPromoted(destination *backupstore.RecoveryDestination) bool {
	return destination != nil && destination.IsPromoted()
}

func writeBackupRecoveryOutput(target io.Writer, data []byte) error {
	if target == nil {
		return errors.New("recovery output writer is unavailable")
	}
	written, err := target.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
