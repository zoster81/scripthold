package updater

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

// RecoverTransaction reconciles and repairs one explicitly selected pending
// transaction while the caller already owns exclusive control and use
// authority. It never performs network work.
func (ownership *DetachedHelperOwnership) RecoverTransaction(ctx context.Context) error {
	return ownership.recoverTransactionWith(ctx, runtime.GOOS, runtime.GOARCH, reconciliationDeps{})
}

func (ownership *DetachedHelperOwnership) recoverTransactionWith(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if ownership == nil || ownership.controlLock == nil || ownership.useLock == nil ||
		ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("exclusive update recovery ownership is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate recovery control lock: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("validate recovery use lock: %w", err)
	}
	if err := ownership.refreshTargetInspection(goos); err != nil {
		return fmt.Errorf("refresh recovery target inspection: %w", err)
	}

	result, err := reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		deps,
		ownership.controlLock,
	)
	if err != nil {
		return err
	}
	ownership.reconciliation = result

	switch result.Status {
	case ReconciliationStable:
		return nil
	case ReconciliationPrepared:
		return ownership.recoverPreparedTransaction(ctx, goos, goarch, deps)
	case ReconciliationCommitted:
		if result.rollbackPrepared {
			return ownership.rollbackAndFinalizeRecovery(ctx, goos, goarch, deps)
		}
		return ownership.finalizeTransactionWith(ctx, goos, goarch, deps)
	case ReconciliationRolledBack:
		return ownership.finalizeTransactionWith(ctx, goos, goarch, deps)
	case ReconciliationRecoveryRequired:
		if result.candidateBytesInstalled {
			return ownership.rollbackAndFinalizeRecovery(ctx, goos, goarch, deps)
		}
		if result.Problem != "" {
			return fmt.Errorf("self-update recovery requires manual intervention: %s", result.Problem)
		}
		return errors.New("self-update recovery requires manual intervention")
	default:
		return fmt.Errorf("unsupported self-update recovery state %q", result.Status)
	}
}

func (ownership *DetachedHelperOwnership) recoverPreparedTransaction(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	commitErr := ownership.commitExecutableReplacementWith(ctx, goos, goarch, deps)
	if commitErr == nil {
		return ownership.finalizeTransactionWith(ctx, goos, goarch, deps)
	}
	if ctx.Err() != nil {
		return errors.Join(commitErr, ctx.Err())
	}
	if err := ownership.refreshTargetInspection(goos); err != nil {
		return errors.Join(commitErr, fmt.Errorf("refresh failed recovery target: %w", err))
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
	ownership.reconciliation = result
	if !result.candidateBytesInstalled {
		return commitErr
	}
	if err := ownership.rollbackAndFinalizeRecovery(ctx, goos, goarch, deps); err != nil {
		return errors.Join(commitErr, err)
	}
	// Explicit recovery succeeded by restoring the verified source. The failed
	// candidate verification remains represented by the rolled-back terminal
	// outcome rather than being returned as an operation failure.
	return nil
}

func (ownership *DetachedHelperOwnership) rollbackAndFinalizeRecovery(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if err := ownership.rollbackCommittedReplacementWith(
		ctx,
		goos,
		goarch,
		deps,
		rollbackDeps{},
	); err != nil {
		return fmt.Errorf("recover self-update by rollback: %w", err)
	}
	if err := ownership.finalizeTransactionWith(ctx, goos, goarch, deps); err != nil {
		return fmt.Errorf("finalize recovered self-update rollback: %w", err)
	}
	return nil
}
