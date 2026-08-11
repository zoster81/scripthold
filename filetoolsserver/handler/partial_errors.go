package handler

import (
	"errors"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	maxPartialFailureDetails = 64
	maxPartialFailureBytes   = int64(64 * 1024)
	partialFailureOverhead   = int64(64)
)

// EncodingErrorCode refines the stable top-level errorCode without changing
// the existing 2.x error-code contract.
const (
	EncodingErrorAmbiguous       = "ENCODING_AMBIGUOUS"
	EncodingErrorMalformed       = "ENCODING_MALFORMED"
	EncodingErrorUnsupported     = "ENCODING_UNSUPPORTED"
	EncodingErrorBOMConflict     = "ENCODING_BOM_CONFLICT"
	EncodingErrorUnrepresentable = "ENCODING_UNREPRESENTABLE"
	EncodingErrorOther           = "ENCODING_OTHER"
)

type partialFileErrorCollector struct {
	details       []PartialFileError
	total         int
	retainedBytes int64
	byteLimit     int64
}

func newPartialFileErrorCollector(maxOutputBytes int64) *partialFileErrorCollector {
	return &partialFileErrorCollector{
		details:   make([]PartialFileError, 0, maxPartialFailureDetails),
		byteLimit: partialFailureByteBudget(maxOutputBytes),
	}
}

func (collector *partialFileErrorCollector) Add(path string, err error) {
	if collector == nil || err == nil {
		return
	}
	collector.total++
	mapped := mapOperationError(err, path)
	detail := PartialFileError{
		Path:              path,
		Error:             mapped.Message,
		ErrorCode:         mapped.BatchCode,
		EncodingErrorCode: encodingErrorCode(err),
	}
	cost := partialFileErrorCost(detail)
	if len(collector.details) >= maxPartialFailureDetails || cost > collector.byteLimit-collector.retainedBytes {
		return
	}
	collector.details = append(collector.details, detail)
	collector.retainedBytes += cost
}

func (collector *partialFileErrorCollector) Total() int {
	if collector == nil {
		return 0
	}
	return collector.total
}

func (collector *partialFileErrorCollector) DetailsWithinBudget(maxBytes int64) []PartialFileError {
	if collector == nil || len(collector.details) == 0 || maxBytes <= 0 {
		return nil
	}
	limit := min(collector.byteLimit, maxBytes)
	retained := make([]PartialFileError, 0, len(collector.details))
	var used int64
	for _, detail := range collector.details {
		cost := partialFileErrorCost(detail)
		if cost > limit-used {
			break
		}
		retained = append(retained, detail)
		used += cost
	}
	if len(retained) == 0 {
		return nil
	}
	return retained
}

func partialFileErrorCost(detail PartialFileError) int64 {
	return partialFailureOverhead + int64(len(detail.Path)+len(detail.Error)+len(detail.ErrorCode)+len(detail.EncodingErrorCode))
}

func (collector *partialFileErrorCollector) Omitted() int {
	if collector == nil {
		return 0
	}
	return collector.total - len(collector.details)
}

type boundedErrorSummary struct {
	items         []string
	total         int
	retainedBytes int64
	byteLimit     int64
}

func newBoundedErrorSummary(maxOutputBytes int64) *boundedErrorSummary {
	return newBoundedErrorSummaryWithin(maxOutputBytes, maxOutputBytes)
}

func newBoundedErrorSummaryWithin(maxOutputBytes, availableBytes int64) *boundedErrorSummary {
	byteLimit := partialFailureByteBudget(maxOutputBytes)
	if availableBytes < 0 {
		availableBytes = 0
	}
	if availableBytes < byteLimit {
		byteLimit = availableBytes
	}
	return &boundedErrorSummary{
		items:     make([]string, 0, maxPartialFailureDetails),
		byteLimit: byteLimit,
	}
}

func (summary *boundedErrorSummary) Add(message string) {
	if summary == nil {
		return
	}
	summary.total++
	cost := partialFailureOverhead + int64(len(message))
	if len(summary.items) >= maxPartialFailureDetails || cost > summary.byteLimit-summary.retainedBytes {
		return
	}
	summary.items = append(summary.items, message)
	summary.retainedBytes += cost
}

func (summary *boundedErrorSummary) Items() []string {
	if summary == nil || len(summary.items) == 0 {
		return nil
	}
	return append([]string(nil), summary.items...)
}

func (summary *boundedErrorSummary) Omitted() int {
	if summary == nil {
		return 0
	}
	return summary.total - len(summary.items)
}

func partialFailureByteBudget(maxOutputBytes int64) int64 {
	if maxOutputBytes <= 0 {
		return maxPartialFailureBytes
	}
	budget := maxOutputBytes / 8
	if budget > maxPartialFailureBytes {
		budget = maxPartialFailureBytes
	}
	return budget
}

func encodingErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrEncodingAmbiguous):
		return EncodingErrorAmbiguous
	case errors.Is(err, ErrEncodingUnsupported):
		return EncodingErrorUnsupported
	case errors.Is(err, ErrBOMEncodingConflict):
		return EncodingErrorBOMConflict
	case errors.Is(err, fileEncoding.ErrInvalidEncodedSequence):
		return EncodingErrorMalformed
	}

	switch operation.KindOf(err) {
	case operation.KindDecoding:
		return EncodingErrorMalformed
	case operation.KindEncodingOutput:
		return EncodingErrorUnrepresentable
	case operation.KindEncoding:
		return EncodingErrorOther
	default:
		return ""
	}
}
