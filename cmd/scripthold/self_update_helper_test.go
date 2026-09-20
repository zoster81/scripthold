package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTryRunSelfUpdateHelperWithRoutesOnlyPrivateInvocation(t *testing.T) {
	transactionID := strings.Repeat("a", 64)
	var called string
	var stderr bytes.Buffer
	code, matched := tryRunSelfUpdateHelperWith(
		context.Background(),
		[]string{"_self-update-helper", transactionID},
		&stderr,
		func(_ context.Context, got string) error {
			called = got
			return nil
		},
	)
	if !matched || code != 0 || called != transactionID || stderr.Len() != 0 {
		t.Fatalf("matched=%v code=%d called=%q stderr=%q", matched, code, called, stderr.String())
	}
	if code, matched := tryRunSelfUpdateHelperWith(
		context.Background(), []string{"--version"}, &stderr, nil,
	); matched || code != 0 {
		t.Fatalf("ordinary args matched helper: matched=%v code=%d", matched, code)
	}
}

func TestTryRunSelfUpdateHelperWithHidesInternalFailureDetail(t *testing.T) {
	transactionID := strings.Repeat("b", 64)
	secret := "internal-detail-not-for-output"
	var stderr bytes.Buffer
	code, matched := tryRunSelfUpdateHelperWith(
		context.Background(),
		[]string{"_self-update-helper", transactionID},
		&stderr,
		func(context.Context, string) error { return errors.New(secret) },
	)
	if !matched || code != 1 {
		t.Fatalf("matched=%v code=%d", matched, code)
	}
	if strings.Contains(stderr.String(), secret) {
		t.Fatalf("helper error leaked internal detail: %q", stderr.String())
	}
}
