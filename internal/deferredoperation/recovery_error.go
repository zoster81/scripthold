package deferredoperation

import (
	"sort"
	"strings"
)

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

// RecoveryFailureReason returns a bounded path-free category signature suitable
// for lifecycle diagnostics. Joined failures retain every distinct category so
// a persistent condition cannot hide a newly occurring recovery failure.
func RecoveryFailureReason(err error) string {
	reasons := make(map[string]struct{}, 4)
	collectRecoveryFailureReasons(err, reasons)
	if len(reasons) == 0 {
		return recoveryFailureUnknown
	}
	values := make([]string, 0, len(reasons))
	for reason := range reasons {
		values = append(values, reason)
	}
	sort.Strings(values)
	return strings.Join(values, "+")
}

func collectRecoveryFailureReasons(err error, reasons map[string]struct{}) {
	if err == nil {
		return
	}
	if failure, ok := err.(*recoveryFailure); ok {
		reasons[boundedRecoveryFailureReason(failure.reason)] = struct{}{}
		collectRecoveryFailureReasons(failure.err, reasons)
		return
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			collectRecoveryFailureReasons(child, reasons)
		}
		return
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		collectRecoveryFailureReasons(wrapped.Unwrap(), reasons)
	}
}

func boundedRecoveryFailureReason(reason string) string {
	switch reason {
	case recoveryFailureRootPolicy,
		recoveryFailureControlLock,
		recoveryFailureStoreScan,
		recoveryFailureRecordRead,
		recoveryFailureStateWrite,
		recoveryFailureHeartbeat,
		recoveryFailureDispatch:
		return reason
	default:
		return recoveryFailureUnknown
	}
}
