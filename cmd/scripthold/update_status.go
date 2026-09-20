package main

import (
	"context"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/updater"
)

const selfUpdateUsage = "Usage:\n  scripthold update\n  scripthold update status\n  scripthold update --adopt\n\nBefore adoption, stop every other Scripthold process using this binary and keep them stopped until the adoption command exits."

type selfUpdateCommandDeps struct {
	observe func(context.Context, string, bool) (updater.SelfUpdateView, error)
	adopt   func(context.Context) error
	update  func(context.Context) (updater.UpdateLaunchResult, error)
}

func tryRunSelfUpdateCommand(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	processVersion string,
) (int, bool) {
	return tryRunSelfUpdateCommandWithDeps(
		ctx,
		args,
		stdout,
		stderr,
		processVersion,
		selfUpdateCommandDeps{
			observe: updater.ObserveSelfUpdateView,
			adopt:   updater.InitializeStableAdoption,
			update:  updater.LaunchCurrentUpdate,
		},
	)
}

func tryRunSelfUpdateCommandWith(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	processVersion string,
	observe func(context.Context, string, bool) (updater.SelfUpdateView, error),
) (int, bool) {
	return tryRunSelfUpdateCommandWithDeps(ctx, args, stdout, stderr, processVersion, selfUpdateCommandDeps{observe: observe})
}

func tryRunSelfUpdateCommandWithDeps(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	processVersion string,
	deps selfUpdateCommandDeps,
) (int, bool) {
	if len(args) == 0 || args[0] != "update" {
		return 0, false
	}
	if len(args) == 1 {
		return runSelfUpdate(ctx, stdout, stderr, deps.update), true
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Fprintln(stdout, selfUpdateUsage)
		return 0, true
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Error: unsupported update command")
		fmt.Fprintln(stderr, selfUpdateUsage)
		return 1, true
	}
	switch args[1] {
	case "status":
		if deps.observe == nil {
			fmt.Fprintln(stderr, "Error: self-update status is unavailable")
			return 1, true
		}
		view, err := deps.observe(ctx, processVersion, false)
		if err != nil {
			fmt.Fprintln(stderr, "Error: self-update status could not be observed safely")
			return 1, true
		}
		writeSelfUpdateStatus(stdout, processVersion, view)
		return 0, true
	case "--adopt":
		if deps.adopt == nil {
			fmt.Fprintln(stderr, "Error: self-update adoption is unavailable")
			return 1, true
		}
		if err := deps.adopt(ctx); err != nil {
			fmt.Fprintln(stderr, "Error: self-update adoption failed safely")
			return 1, true
		}
		fmt.Fprintln(stdout, "Adoption complete. The installed standalone binary was verified and adopted for self-update.")
		fmt.Fprintln(stdout, "No update was downloaded or installed.")
		fmt.Fprintln(stdout, "Every other Scripthold process using this binary must have been stopped before adoption began and must remain stopped until this command exits.")
		return 0, true
	default:
		fmt.Fprintln(stderr, "Error: unsupported update command")
		fmt.Fprintln(stderr, selfUpdateUsage)
		return 1, true
	}
}

func runSelfUpdate(
	ctx context.Context,
	stdout, stderr io.Writer,
	launch func(context.Context) (updater.UpdateLaunchResult, error),
) int {
	if launch == nil {
		fmt.Fprintln(stderr, "Error: self-update is unavailable")
		return 1
	}
	result, err := launch(ctx)
	if err != nil {
		if result.Status == updater.UpdateLaunchDispatched {
			fmt.Fprintf(stderr, "Error: self-update helper has started for %s -> %s, but launcher cleanup or release failed.\n", result.CurrentVersion, result.CandidateVersion)
			fmt.Fprintln(stderr, "Run 'scripthold update status' before retrying; do not start another update until installation state is known.")
			return 1
		}
		fmt.Fprintln(stderr, "Error: self-update failed safely; no update helper was started.")
		fmt.Fprintln(stderr, "Run 'scripthold update status' before retrying.")
		return 1
	}

	switch result.Status {
	case updater.UpdateLaunchCurrent:
		if result.CurrentVersion == "" || result.CandidateVersion != "" {
			fmt.Fprintln(stderr, "Error: self-update failed safely with an invalid internal result; run 'scripthold update status' before retrying.")
			return 1
		}
		fmt.Fprintf(stdout, "Scripthold is already up to date at version %s.\n", result.CurrentVersion)
		return 0
	case updater.UpdateLaunchDispatched:
		if result.CurrentVersion == "" || result.CandidateVersion == "" {
			fmt.Fprintln(stderr, "Error: self-update failed safely with an invalid internal result; run 'scripthold update status' before retrying.")
			return 1
		}
		fmt.Fprintf(stdout, "Self-update helper has started for %s -> %s.\n", result.CurrentVersion, result.CandidateVersion)
		fmt.Fprintln(stdout, "Run 'scripthold update status' to observe completion before starting another update.")
		return 0
	default:
		fmt.Fprintln(stderr, "Error: self-update failed safely with an invalid internal result; run 'scripthold update status' before retrying.")
		return 1
	}
}

func writeSelfUpdateStatus(writer io.Writer, processVersion string, view updater.SelfUpdateView) {
	state := "not_adopted"
	durableCurrent := "n/a"
	installed := "n/a"
	candidate := "n/a"
	if view.Installation.Adopted {
		state = string(view.Installation.State)
		durableCurrent = displayStatusValue(view.Installation.CurrentVersion, "unknown")
		installed = displayStatusValue(view.Installation.InstalledVersion, "unverified")
		candidate = displayStatusValue(view.Installation.CandidateVersion, "none")
	}
	comparison := displayStatusValue(view.ReleaseComparisonVersion, "suppressed")
	latest := displayStatusValue(view.Release.LatestVersion, "unknown")
	updateAvailable := "unknown"
	if view.Release.LatestVersion != "" {
		updateAvailable = yesNo(view.Release.UpdateAvailable)
	}

	fmt.Fprintf(writer, "Adopted: %s\n", yesNo(view.Installation.Adopted))
	fmt.Fprintf(writer, "State: %s\n", state)
	fmt.Fprintf(writer, "Process version: %s\n", displayStatusValue(processVersion, "unknown"))
	fmt.Fprintf(writer, "Durable current version: %s\n", durableCurrent)
	fmt.Fprintf(writer, "Verified installed version: %s\n", installed)
	fmt.Fprintf(writer, "Candidate version: %s\n", candidate)
	fmt.Fprintf(writer, "Release comparison version: %s\n", comparison)
	fmt.Fprintf(writer, "Latest version: %s\n", latest)
	fmt.Fprintf(writer, "Update available: %s\n", updateAvailable)
	fmt.Fprintf(writer, "Recovery available: %s\n", yesNo(view.Installation.RecoveryAvailable))
	if view.Installation.Problem != "" {
		fmt.Fprintf(writer, "Problem: %s\n", view.Installation.Problem)
	}
}

func displayStatusValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
