// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"errors"
	"testing"
)

func TestWrapCheckError(t *testing.T) {
	err := WrapCheckError(errors.New("ping failed"), "redis")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestWrapWatchError(t *testing.T) {
	err := WrapWatchError(errors.New("watch failed"), "svc")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapShutdownError(t *testing.T) {
	err := WrapShutdownError(errors.New("shutdown failed"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}
