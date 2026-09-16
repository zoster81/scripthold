package deferredoperation

import "errors"

const (
	recoveryFailureRootPolicy  = "root_policy"
	recoveryFailureControlLock = "control_lock"
	recoveryFailureStoreScan   = "store_scan"
	recoveryFailureRecordRead  = "record_read"
	recoveryFailureStateWrite  = "state_write"
	recoveryFailureHeartbeat   = "heartbeat_write"
	recoveryFailureDispatch    = "dispatch"
	recoveryFailureUnknown     = "unknown"
)

type recoveryFailure struct {
	reason string
	err    error
}

func (failure *recoveryFailure) Error() string {
	return failure.err.Error()
}

func (failure *recoveryFailure) Unwrap() error {
	return failure.err
}

func wrapRecoveryFailure(reason string, err error) error {
	if err == nil {
		return nil
	}
	return &recoveryFailure{reason: reason, err: err}
}

// RecoveryFailureReason returns a bounded path-free category suitable for
// lifecycle diagnostics. The underlying error remains available to internal
// callers through errors.Is/errors.As but must not be logged directly.
func RecoveryFailureReason(err error) string {
	var failure *recoveryFailure
	if !errors.As(err, &failure) {
		return recoveryFailureUnknown
	}
	switch failure.reason {
	case recoveryFailureRootPolicy,
		recoveryFailureControlLock,
		recoveryFailureStoreScan,
		recoveryFailureRecordRead,
		recoveryFailureStateWrite,
		recoveryFailureHeartbeat,
		recoveryFailureDispatch:
		return failure.reason
	default:
		return recoveryFailureUnknown
	}
}
