package main

import (
	"context"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/updater"
)

func tryRunSelfUpdateHelper(ctx context.Context, args []string, stderr io.Writer) (int, bool) {
	return tryRunSelfUpdateHelperWith(
		ctx,
		args,
		stderr,
		updater.RunDetachedHelper,
		updater.RunDetachedRecoveryHelper,
	)
}

func tryRunSelfUpdateHelperWith(
	ctx context.Context,
	args []string,
	stderr io.Writer,
	runAutomatic func(context.Context, string) error,
	runRecovery func(context.Context, string) error,
) (int, bool) {
	if transactionID, matched, err := updater.ParseDetachedHelperInvocation(args); matched {
		return runPrivateSelfUpdateHelper(ctx, transactionID, err, stderr, runAutomatic)
	}
	if transactionID, matched, err := updater.ParseDetachedRecoveryHelperInvocation(args); matched {
		return runPrivateSelfUpdateHelper(ctx, transactionID, err, stderr, runRecovery)
	}
	return 0, false
}

func runPrivateSelfUpdateHelper(
	ctx context.Context,
	transactionID string,
	parseErr error,
	stderr io.Writer,
	run func(context.Context, string) error,
) (int, bool) {
	if parseErr != nil {
		fmt.Fprintln(stderr, "Error: invalid internal self-update helper invocation")
		return 1, true
	}
	if run == nil {
		fmt.Fprintln(stderr, "Error: self-update helper is unavailable")
		return 1, true
	}
	if err := run(ctx, transactionID); err != nil {
		fmt.Fprintln(stderr, "Error: self-update helper failed")
		return 1, true
	}
	return 0, true
}
