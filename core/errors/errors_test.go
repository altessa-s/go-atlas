// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

func TestContextChecks(t *testing.T) {
	t.Run("IsContextDeadlineExceeded", func(t *testing.T) {
		if !errors.IsContextDeadlineExceeded(context.DeadlineExceeded) {
			t.Error("Should detect DeadlineExceeded")
		}
		if errors.IsContextDeadlineExceeded(std_errors.New("other")) {
			t.Error("Should not detect other")
		}
		if errors.IsContextDeadlineExceeded(nil) {
			t.Error("Should not detect nil")
		}
	})

	t.Run("IsContextCanceled", func(t *testing.T) {
		if !errors.IsContextCanceled(context.Canceled) {
			t.Error("Should detect Canceled")
		}
	})

	t.Run("IsContextCanceledOrDeadlineExceeded", func(t *testing.T) {
		if !errors.IsContextCanceledOrDeadlineExceeded(context.Canceled) {
			t.Error("Should detect Canceled")
		}
		if !errors.IsContextCanceledOrDeadlineExceeded(context.DeadlineExceeded) {
			t.Error("Should detect DeadlineExceeded")
		}
	})
}

func TestWrappers(t *testing.T) {
	base := std_errors.New("base")

	t.Run("Wrapf", func(t *testing.T) {
		err := errors.Wrapf(base, "context %s", "foo")
		if err.Error() != "context foo: base" {
			t.Errorf("Wrapf message = %q", err.Error())
		}
		if !std_errors.Is(err, base) {
			t.Error("Wrapf should wrap base")
		}
		if errors.Wrapf(nil, "foo") != nil {
			t.Error("Wrapf(nil) should be nil")
		}
	})

	t.Run("Wrap", func(t *testing.T) {
		err := errors.Wrap(base, "context")
		if err.Error() != "context: base" {
			t.Errorf("Wrap message = %q", err.Error())
		}
	})

	t.Run("WrapOperation", func(t *testing.T) {
		err := errors.WrapOperation(base, "read")
		if err.Error() != "failed to read: base" {
			t.Errorf("WrapOperation message = %q", err.Error())
		}
	})

	t.Run("WrapField", func(t *testing.T) {
		err := errors.WrapField(base, "name")
		if err.Error() != "field 'name': base" {
			t.Errorf("WrapField message = %q", err.Error())
		}
	})

	t.Run("WrapOperationWithContext", func(t *testing.T) {
		err := errors.WrapOperationWithContext(base, "read", "file")
		if err.Error() != "failed to read on file: base" {
			t.Errorf("WrapOperationWithContext message = %q", err.Error())
		}
	})
}

func TestStandardErrors(t *testing.T) {
	if errors.Required("db", "app").Error() != "db is required for app" {
		t.Error("Required message mismatch")
	}
	base := std_errors.New("oops")
	if errors.Provider("Redis", base).Error() != "failed to create Redis provider: oops" {
		t.Error("Provider message mismatch")
	}
}
