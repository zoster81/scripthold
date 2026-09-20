package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTryRunSelfUpdateHelperWithRoutesPrivateIntents(t *testing.T) {
	transactionID := strings.Repeat("a", 64)
	var automaticCalled string
	var recoveryCalled string
	var stderr bytes.Buffer
	automatic := func(_ context.Context, got string) error {
		automaticCalled = got
		return nil
	}
	recovery := func(_ context.Context, got string) error {
		recoveryCalled = got
		return nil
	}

	code, matched := tryRunSelfUpdateHelperWith(
		context.Background(),
		[]string{"_self-update-helper", transactionID},
		&stderr,
		automatic,
		recovery,
	)
	if !matched || code != 0 || automaticCalled != transactionID || recoveryCalled != "" || stderr.Len() != 0 {
		t.Fatalf("automatic matched=%v code=%d automatic=%q recovery=%q stderr=%q",
			matched, code, automaticCalled, recoveryCalled, stderr.String())
	}

	automaticCalled = ""
	code, matched = tryRunSelfUpdateHelperWith(
		context.Background(),
		[]string{"_self-update-recovery-helper", transactionID},
		&stderr,
		automatic,
		recovery,
	)
	if !matched || code != 0 || recoveryCalled != transactionID || automaticCalled != "" || stderr.Len() != 0 {
		t.Fatalf("recovery matched=%v code=%d automatic=%q recovery=%q stderr=%q",
			matched, code, automaticCalled, recoveryCalled, stderr.String())
	}

	if code, matched := tryRunSelfUpdateHelperWith(
		context.Background(), []string{"--version"}, &stderr, nil, nil,
	); matched || code != 0 {
		t.Fatalf("ordinary args matched helper: matched=%v code=%d", matched, code)
	}
}

func TestTryRunSelfUpdateHelperWithHidesInternalFailureDetail(t *testing.T) {
	transactionID := strings.Repeat("b", 64)
	secret := "internal-detail-not-for-output"
	for _, args := range [][]string{
		{"_self-update-helper", transactionID},
		{"_self-update-recovery-helper", transactionID},
	} {
		var stderr bytes.Buffer
		code, matched := tryRunSelfUpdateHelperWith(
			context.Background(),
			args,
			&stderr,
			func(context.Context, string) error { return errors.New(secret) },
			func(context.Context, string) error { return errors.New(secret) },
		)
		if !matched || code != 1 {
			t.Fatalf("args=%v matched=%v code=%d", args, matched, code)
		}
		if strings.Contains(stderr.String(), secret) {
			t.Fatalf("helper error leaked internal detail: %q", stderr.String())
		}
	}
}
