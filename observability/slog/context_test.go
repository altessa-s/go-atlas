// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextWithLogger_NilContext_OK(t *testing.T) {
	l := slog.New(slog.DiscardHandler)

	var ctx context.Context
	ctx = ContextWithLogger(ctx, l)

	got := FromContext(ctx)
	require.Same(t, l, got)
}

func TestFromContext_NilContext_ReturnsNil(t *testing.T) {
	var ctx context.Context
	require.Nil(t, FromContext(ctx))
}
