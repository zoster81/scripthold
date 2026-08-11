package handler

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/concurrency"
	"github.com/zoster81/scripthold/internal/operation"
)

type batchReadPlan struct {
	path              string
	worstDecodedBytes int64
}

// HandleReadMultipleFiles uses bounded parallelism when the aggregate decoded
// worst case fits the configured memory budget. Larger batches are processed
// one at a time with the exact remaining output budget.
func (h *Handler) HandleReadMultipleFiles(ctx context.Context, req *mcp.CallToolRequest, input ReadMultipleFilesInput) (*mcp.CallToolResult, ReadMultipleFilesOutput, error) {
	if len(input.Paths) == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "paths array is required and must contain at least one path"), ReadMultipleFilesOutput{}, nil
	}
	if len(input.Paths) > h.maxBatchFiles() {
		return errorResultWithCode(
			ErrCodeLimit,
			fmt.Sprintf("paths contains %d entries, exceeding the configured batch limit %d", len(input.Paths), h.maxBatchFiles()),
		), ReadMultipleFilesOutput{}, nil
	}

	budget := h.maxOutputBytes()
	plans, worstTotal := h.planBatchReads(input.Paths)
	maxWorkers := 0
	if worstTotal > budget {
		maxWorkers = 1
	}

	results := make([]FileReadResult, len(input.Paths))
	var remaining atomic.Int64
	remaining.Store(budget)
	concurrency.ProcessOrdered(ctx, plans, concurrency.Options{
		MaxWorkers:             maxWorkers,
		ContinueOnCancellation: true,
	}, func(ctx context.Context, _ int, plan batchReadPlan) FileReadResult {
		if err := ctx.Err(); err != nil {
			mapped := mapOperationError(err, plan.path)
			return FileReadResult{
				Path:      plan.path,
				Error:     mapped.Message,
				ErrorCode: mapped.BatchCode,
			}
		}

		fileBudget := plan.worstDecodedBytes
		if maxWorkers == 1 || fileBudget <= 0 {
			fileBudget = remaining.Load()
		}
		if fileBudget <= 0 {
			return batchBudgetError(plan.path, 0)
		}
		return h.readSingleFile(ctx, plan.path, input.Encoding, clampBudgetToInt(fileBudget))
	}, func(index int, result FileReadResult) bool {
		results[index] = result
		if result.Error == "" {
			remaining.Add(-int64(len(result.Content)))
		}
		return true
	})

	var successCount, errorCount int
	errorSummary := newBoundedErrorSummaryWithin(budget, remaining.Load())
	for _, r := range results {
		if r.Error != "" {
			errorCount++
			errorSummary.Add(fmt.Sprintf("%s: %s", r.Path, r.Error))
		} else {
			successCount++
		}
	}
	omitted := errorSummary.Omitted()

	return &mcp.CallToolResult{}, ReadMultipleFilesOutput{
		Results:         results,
		SuccessCount:    successCount,
		ErrorCount:      errorCount,
		Errors:          errorSummary.Items(),
		ErrorsTruncated: omitted > 0,
		ErrorsOmitted:   omitted,
	}, nil
}

func (h *Handler) planBatchReads(paths []string) ([]batchReadPlan, int64) {
	plans := make([]batchReadPlan, len(paths))
	var total int64
	for index, path := range paths {
		plans[index].path = path
		validated := h.ValidatePath(path)
		if !validated.Ok() {
			plans[index].worstDecodedBytes = math.MaxInt64
			total = math.MaxInt64
			continue
		}
		info, err := os.Stat(validated.Path)
		if err != nil || !info.Mode().IsRegular() {
			plans[index].worstDecodedBytes = math.MaxInt64
			total = math.MaxInt64
			continue
		}
		plans[index].worstDecodedBytes = worstDecodedBytes(info.Size())
		if math.MaxInt64-total < plans[index].worstDecodedBytes {
			total = math.MaxInt64
		} else {
			total += plans[index].worstDecodedBytes
		}
	}
	return plans, total
}

func worstDecodedBytes(size int64) int64 {
	if size <= 0 {
		return 0
	}
	if size > math.MaxInt64/3 {
		return math.MaxInt64
	}
	return size * 3
}

func batchBudgetError(path string, budget int64) FileReadResult {
	err := operation.Wrap(
		operation.KindLimit,
		"read_multiple_files",
		path,
		fmt.Errorf("decoded content exceeds the %d-byte batch output budget", budget),
	)
	mapped := mapOperationError(err, path)
	return FileReadResult{Path: path, Error: mapped.Message, ErrorCode: mapped.BatchCode}
}

// readSingleFile maps the shared streaming document pipeline into a batch result.
func (h *Handler) readSingleFile(ctx context.Context, path, requestedEncoding string, maxOutputBytes int) FileReadResult {
	result := FileReadResult{Path: path}

	v := h.ValidatePath(path)
	if !v.Ok() {
		mapped := mapOperationError(v.Err, path)
		result.Error = mapped.Message
		result.ErrorCode = mapped.BatchCode
		result.EncodingErrorCode = encodingErrorCode(v.Err)
		return result
	}

	output, err := h.readTextFileStream(ctx, v.Path, ReadTextFileInput{
		Encoding:        requestedEncoding,
		maxOutputBytes:  maxOutputBytes,
		outputLimitName: "batch output budget",
	})
	if err != nil {
		mapped := mapOperationError(err, v.Path)
		result.Error = mapped.Message
		result.ErrorCode = mapped.BatchCode
		result.EncodingErrorCode = encodingErrorCode(err)
		return result
	}

	result.Content = output.Content
	result.HasBOM = output.HasBOM
	result.BOMType = output.BOMType
	result.DetectedEncoding = output.DetectedEncoding
	result.EncodingConfidence = output.EncodingConfidence

	return result
}
