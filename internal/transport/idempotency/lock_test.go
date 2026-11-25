// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/idempotency"
)

func TestClassifyLock(t *testing.T) {
	t.Parallel()

	fingerprintMismatch := errors.New("fingerprint mismatch")
	ownerMismatch := errors.New("owner mismatch")
	tests := []struct {
		name             string
		locked, hasState bool
		status           idempotency.LockStatus
		data             any
		err              error
		want             idempotency.LockOutcome
		record           idempotency.Record
	}{
		{name: "acquired", locked: true, hasState: true, status: idempotency.StatusInProgress, want: idempotency.LockAcquired},
		{name: "fingerprint mismatch", err: fmt.Errorf("lock: %w", fingerprintMismatch), want: idempotency.LockFingerprintMismatch},
		{name: "owner mismatch", err: fmt.Errorf("lock: %w", ownerMismatch), want: idempotency.LockOwnerMismatch},
		{name: "storage error wins over locked", locked: true, err: errors.New("down"), want: idempotency.LockStorageError},
		{name: "nil state", want: idempotency.LockNilState},
		{name: "in progress", hasState: true, status: idempotency.StatusInProgress, want: idempotency.LockInProgress},
		{name: "completed decodes the record", hasState: true, status: idempotency.StatusSuccess, data: "entity-1",
			want: idempotency.LockCompleted, record: idempotency.Record{EntityID: "entity-1"}},
		{name: "completed without data", hasState: true, status: idempotency.StatusSuccess, want: idempotency.LockCompleted},
		{name: "unknown status runs the request", hasState: true, status: "other", want: idempotency.LockAcquired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kind := idempotency.ClassifyLockError(tc.err, fingerprintMismatch, ownerMismatch)
			got, record := idempotency.ClassifyLock(tc.locked, tc.hasState, tc.status, tc.data, kind)
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
