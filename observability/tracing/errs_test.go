// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"errors"
	"testing"
)

func TestSentinelErrors_NotNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrInvalidTraceID", ErrInvalidTraceID},
		{"ErrInvalidSpanID", ErrInvalidSpanID},
		{"ErrTracerShutdown", ErrTracerShutdown},
		{"ErrEmptySpanName", ErrEmptySpanName},
		{"ErrNilAdapter", ErrNilAdapter},
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
	err := WrapAdapterError(errors.New("fail"), "otlp")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapSpanError(t *testing.T) {
	err := WrapSpanError(errors.New("fail"), "start span")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapFlushError(t *testing.T) {
	err := WrapFlushError(errors.New("fail"), "otlp")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestWrapShutdownError(t *testing.T) {
	err := WrapShutdownError(errors.New("fail"), "otlp")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}
