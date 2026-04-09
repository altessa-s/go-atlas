// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

// TestProvider_NilReturnsNil asserts that Provider, like every sibling Wrap*
// helper in this package, is safe to call with a nil error: it must return nil
// rather than producing a malformed "failed to create X provider: %!w(<nil>)"
// message.
//
// Regression for the missing nil guard previously present in Provider.
func TestProvider_NilReturnsNil(t *testing.T) {
	if err := errors.Provider("Redis", nil); err != nil {
		t.Fatalf("Provider(_, nil) = %v, want nil", err)
	}
}

// TestProvider_WrapsCauseAndPreservesChain asserts that a non-nil cause is
// wrapped with the %w verb so errors.Is still matches the original cause.
func TestProvider_WrapsCauseAndPreservesChain(t *testing.T) {
	cause := std_errors.New("connection refused")

	err := errors.Provider("Redis", cause)
	if err == nil {
		t.Fatal("Provider(_, cause) = nil, want wrapped error")
	}

	if !std_errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want true (chain broken)")
	}

	const want = "failed to create Redis provider: connection refused"
	if got := err.Error(); got != want {
		t.Errorf("err.Error() = %q, want %q", got, want)
	}
}

// BenchmarkProvider measures the happy path — a non-nil cause being wrapped.
// Useful to confirm the nil guard adds no measurable overhead to the common
// path.
func BenchmarkProvider(b *testing.B) {
	cause := std_errors.New("connection refused")
	b.ReportAllocs()
	for b.Loop() {
		_ = errors.Provider("Redis", cause)
	}
}

// BenchmarkProvider_NilFastPath measures the new nil guard path. It should be
// near-zero cost and allocation-free.
func BenchmarkProvider_NilFastPath(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = errors.Provider("Redis", nil)
	}
}
