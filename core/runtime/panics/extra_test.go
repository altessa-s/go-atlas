// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

func TestSetReallyPanic(t *testing.T) {
	// Just verify it doesn't panic
	panics.SetReallyPanic(false)
	defer panics.SetReallyPanic(false) // reset
}

func TestSetLogger(t *testing.T) {
	panics.SetLogger(slog.Default())
	// restore
	panics.SetLogger(slog.New(slog.DiscardHandler))
}

func TestSetLoggerFromContext(t *testing.T) {
	panics.SetLoggerFromContext(func(_ context.Context) *slog.Logger {
		return slog.Default()
	})
	// restore
	panics.SetLoggerFromContext(nil)
}

func TestMustNonZero_Zero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustNonZero(0) should panic")
		}
	}()
	panics.MustNonZero(0, "must be nonzero")
}

func TestMustNonZero_NonZero(t *testing.T) {
	// Should not panic
	panics.MustNonZero(42, "must be nonzero")
}

func TestMustNonZero_EmptyString(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustNonZero('') should panic")
		}
	}()
	panics.MustNonZero("", "must be nonzero")
}
