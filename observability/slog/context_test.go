// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"log/slog"
	"testing"
)

func TestContextWithLogger_NilContext_OK(t *testing.T) {
	l := slog.New(slog.DiscardHandler)

	var ctx context.Context
	ctx = ContextWithLogger(ctx, l)

	got := FromContext(ctx)
	if got != l {
		t.Fatalf("got logger %v, want %v", got, l)
	}
}

func TestFromContext_NilContext_ReturnsNil(t *testing.T) {
	var ctx context.Context
	if got := FromContext(ctx); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
