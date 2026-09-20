package updater

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

// RunDetachedHelper executes the private detached self-update lifecycle for one
// transaction. A failed candidate verification is rolled back only when the
// installed bytes are still exactly the pinned candidate bytes.
func RunDetachedHelper(ctx context.Context, transactionID string) (err error) {
	ownership, err := AcquireCurrentDetachedHelperOwnership(ctx, transactionID)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, ownership.Close())
	}()
	return runDetachedHelperOwnershipWith(
		ctx,
		ownership,
		runtime.GOOS,
		runtime.GOARCH,
		reconciliationDeps{},
	)
}

func runDetachedHelperOwnershipWith(
	ctx context.Context,
	ownership *DetachedHelperOwnership,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if ownership == nil {
		return errors.New("detached helper ownership is required")
	}
	commitErr := ownership.commitExecutableReplacementWith(ctx, goos, goarch, deps)
	if commitErr == nil {
		return ownership.finalizeTransactionWith(ctx, goos, goarch, deps)
	}
	if ctx != nil && ctx.Err() != nil {
		return errors.Join(commitErr, ctx.Err())
	}
	if err := ownership.refreshTargetInspection(goos); err != nil {
		return errors.Join(commitErr, fmt.Errorf("refresh failed update target: %w", err))
	}
	result, reconcileErr := reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		deps,
		ownership.controlLock,
	)
	if reconcileErr != nil {
		return errors.Join(commitErr, reconcileErr)
	}
	if result.Status != ReconciliationRecoveryRequired || !result.candidateBytesInstalled {
		return commitErr
	}

	rollbackErr := ownership.rollbackCommittedReplacementWith(
		ctx,
		goos,
		goarch,
		deps,
		rollbackDeps{},
	)
	if rollbackErr != nil {
		return errors.Join(commitErr, fmt.Errorf("automatic rollback failed: %w", rollbackErr))
	}
	finalizeErr := ownership.finalizeTransactionWith(ctx, goos, goarch, deps)
	return errors.Join(commitErr, finalizeErr)
}
