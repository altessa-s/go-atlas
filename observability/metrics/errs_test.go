// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"errors"
	"testing"
)

func TestSentinelErrors_NotNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrEmptyMetricName", ErrEmptyMetricName},
		{"ErrEmptyLabelName", ErrEmptyLabelName},
		{"ErrLabelCountMismatch", ErrLabelCountMismatch},
		{"ErrMissingLabel", ErrMissingLabel},
		{"ErrCollectorShutdown", ErrCollectorShutdown},
		{"ErrAdapterClosed", ErrAdapterClosed},
	}
	for _, tt := range sentinels {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Error("sentinel error should not be nil")
			}
		})
	}
}

func TestWrapAdapterError(t *testing.T) {
	err := WrapAdapterError(errors.New("fail"), "prometheus")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapMetricError(t *testing.T) {
	err := WrapMetricError(errors.New("fail"), "counter inc")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapFlushError(t *testing.T) {
	err := WrapFlushError(errors.New("fail"), "prometheus")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapShutdownError(t *testing.T) {
	err := WrapShutdownError(errors.New("fail"), "prometheus")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}
