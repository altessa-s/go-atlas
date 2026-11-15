// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/saga"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// TestSentinelsWrapCanonical asserts the storage sentinels keep their identity
// and match the canonical core/errors sentinel, and no other.
func TestSentinelsWrapCanonical(t *testing.T) {
	t.Parallel()

	canonical := []error{coreerrs.ErrNotFound, coreerrs.ErrAlreadyExists, coreerrs.ErrConflict, coreerrs.ErrBusy}
	tests := []struct {
		name string
		err  error
		want error
		msg  string
	}{
		{name: "not found", err: saga.ErrInstanceNotFound, want: coreerrs.ErrNotFound, msg: "saga: instance not found"},
		{name: "exists", err: saga.ErrInstanceExists, want: coreerrs.ErrAlreadyExists, msg: "saga: instance already exists"},
		{name: "conflict", err: saga.ErrVersionConflict, want: coreerrs.ErrConflict,
			msg: "saga: optimistic concurrency: version conflict"},
		{name: "busy", err: saga.ErrInstanceBusy, want: coreerrs.ErrBusy, msg: "saga: instance is executing: busy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wrapped := fmt.Errorf("store: %w", tc.err)
			require.ErrorIs(t, wrapped, tc.err)
			require.EqualError(t, tc.err, tc.msg)
			for _, c := range canonical {
				if c == tc.want {
					require.ErrorIs(t, wrapped, c)
				} else {
					require.NotErrorIs(t, wrapped, c)
				}
			}
		})
	}
}
