// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/idempotency"
)

func TestDefaultKeyValidator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "valid lowercase UUID v4", key: "550e8400-e29b-41d4-a716-446655440000", wantErr: false},
		{name: "uppercase UUID v4 rejected", key: "550E8400-E29B-41D4-A716-446655440000", wantErr: true},
		{name: "mixed case rejected", key: "550e8400-E29b-41d4-a716-446655440000", wantErr: true},
		{name: "UUID v1 rejected", key: "550e8400-e29b-11d4-a716-446655440000", wantErr: true},
		{name: "empty key rejected", key: "", wantErr: true},
		{name: "arbitrary string rejected", key: "not-a-uuid", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := idempotency.DefaultKeyValidator(tc.key)
			if tc.wantErr {
				require.ErrorIs(t, err, idempotency.ErrInvalidFormat)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFinalizeContextOutlivesCancellation(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(t.Context())
	cancel()

	ctx, release := idempotency.FinalizeContext(parent)
	defer release()

	require.NoError(t, ctx.Err())
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), idempotency.FinalizeTimeout)
}

func TestHeaderNames(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Idempotency-Key", idempotency.DefaultKeyHeader)
	require.Equal(t, "Idempotency-Key-Status", idempotency.DefaultKeyStatusHeader)
	require.Equal(t, "Idempotency-Key-Entity-Id", idempotency.DefaultKeyEntityIDHeader)
}
