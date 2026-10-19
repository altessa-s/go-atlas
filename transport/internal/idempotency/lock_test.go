// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/idempotency"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
)

func TestClassifyLock(t *testing.T) {
	t.Parallel()

	completed := &idempotencydata.State{Status: idempotencydata.StatusSuccess, Data: "entity-1"}
	tests := []struct {
		name   string
		locked bool
		state  *idempotencydata.State
		err    error
		want   idempotency.LockOutcome
		record idempotency.Record
	}{
		{name: "acquired", locked: true, state: &idempotencydata.State{Status: idempotencydata.StatusInProgress},
			want: idempotency.LockAcquired},
		{name: "fingerprint mismatch", err: fmt.Errorf("lock: %w", idempotencydata.ErrFingerprintMismatch),
			want: idempotency.LockFingerprintMismatch},
		{name: "owner mismatch", err: fmt.Errorf("lock: %w", idempotencydata.ErrOwnerMismatch), want: idempotency.LockOwnerMismatch},
		{name: "storage error wins over locked", locked: true, err: errors.New("down"), want: idempotency.LockStorageError},
		{name: "nil state", want: idempotency.LockNilState},
		{name: "in progress", state: &idempotencydata.State{Status: idempotencydata.StatusInProgress}, want: idempotency.LockInProgress},
		{name: "completed decodes the record", state: completed, want: idempotency.LockCompleted,
			record: idempotency.Record{EntityID: "entity-1"}},
		{name: "completed without data", state: &idempotencydata.State{Status: idempotencydata.StatusSuccess},
			want: idempotency.LockCompleted},
		{name: "unknown status runs the request", state: &idempotencydata.State{Status: "other"}, want: idempotency.LockAcquired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, record := idempotency.ClassifyLock(tc.locked, tc.state, tc.err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.record, record)
		})
	}
}

func TestEntityIDData(t *testing.T) {
	t.Parallel()

	require.Nil(t, idempotency.EntityIDData(""))
	require.Equal(t, any("entity-1"), idempotency.EntityIDData("entity-1"))

	record, ok := idempotency.DecodeRecord(idempotency.EntityIDData("entity-1"))
	require.True(t, ok)
	require.Equal(t, idempotency.Record{EntityID: "entity-1"}, record, "DecodeRecord reads the pre-replay form back")
}

func TestNewRecord(t *testing.T) {
	t.Parallel()

	require.Equal(t, idempotency.Record{EntityID: "e", Binding: "POST /orders"}, idempotency.NewRecord("e", "POST /orders"))
	require.False(t, idempotency.NewRecord("e", "POST /orders").HasHTTPResponse("POST /orders"), "no response attached yet")
}
