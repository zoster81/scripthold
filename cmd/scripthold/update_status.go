package main

import (
	"context"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/updater"
)

const selfUpdateStatusUsage = "Usage: scripthold update status"

func tryRunSelfUpdateCommand(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	processVersion string,
) (int, bool) {
	return tryRunSelfUpdateCommandWith(
		ctx,
		args,
		stdout,
		stderr,
		processVersion,
		updater.ObserveSelfUpdateView,
	)
}

func tryRunSelfUpdateCommandWith(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	processVersion string,
	observe func(context.Context, string, bool) (updater.SelfUpdateView, error),
) (int, bool) {
	if len(args) == 0 || args[0] != "update" {
		return 0, false
	}
	if len(args) == 1 || (len(args) == 2 && (args[1] == "--help" || args[1] == "-h")) {
		fmt.Fprintln(stdout, selfUpdateStatusUsage)
		return 0, true
	}
	if len(args) != 2 || args[1] != "status" {
		fmt.Fprintln(stderr, "Error: unsupported update command")
		fmt.Fprintln(stderr, selfUpdateStatusUsage)
		return 1, true
	}
	if observe == nil {
		fmt.Fprintln(stderr, "Error: self-update status is unavailable")
		return 1, true
	}

	view, err := observe(ctx, processVersion, false)
	if err != nil {
		fmt.Fprintln(stderr, "Error: self-update status could not be observed safely")
		return 1, true
	}
	writeSelfUpdateStatus(stdout, processVersion, view)
	return 0, true
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
