package updater

import (
	"context"
	"errors"
)

// RunDetachedRecoveryHelper executes explicit recovery from the fixed helper
// copy after acquiring bounded exclusive installation ownership.
func RunDetachedRecoveryHelper(ctx context.Context, transactionID string) (err error) {
	ownership, err := AcquireCurrentRecoveryHelperOwnership(ctx, transactionID)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, ownership.Close())
	}()
	return ownership.RecoverTransaction(ctx)
}
