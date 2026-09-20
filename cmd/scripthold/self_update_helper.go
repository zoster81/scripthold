package main

import (
	"context"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/updater"
)

func tryRunSelfUpdateHelper(ctx context.Context, args []string, stderr io.Writer) (int, bool) {
	return tryRunSelfUpdateHelperWith(ctx, args, stderr, updater.RunDetachedHelper)
}

func tryRunSelfUpdateHelperWith(
	ctx context.Context,
	args []string,
	stderr io.Writer,
	run func(context.Context, string) error,
) (int, bool) {
	transactionID, matched, err := updater.ParseDetachedHelperInvocation(args)
	if !matched {
		return 0, false
	}
	if err != nil {
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
