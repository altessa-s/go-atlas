// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Instants round-trip at full precision in UTC, the zero time as 0.
func TestUnixNanoRoundTrip(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 16, 8, 0, 0, 123456789, time.FixedZone("x", 3600))
	require.Equal(t, at.UTC(), fromUnixNano(unixNano(at)))
	require.Equal(t, int64(0), unixNano(time.Time{}))
	require.True(t, fromUnixNano(0).IsZero())
}

type pgErr struct{ code string }

func (e pgErr) Error() string    { return e.code }
func (e pgErr) SQLState() string { return e.code }

type myErr struct{ Number uint16 }

func (e *myErr) Error() string { return "mysql" }

// A duplicate key is recognized by the shape of either driver's error, also
// when wrapped.
func TestIsDuplicateKey(t *testing.T) {
	t.Parallel()
	require.True(t, isDuplicateKey(pgErr{"23505"}))
	require.True(t, isDuplicateKey(fmt.Errorf("wrapped: %w", pgErr{"23505"})))
	require.False(t, isDuplicateKey(pgErr{"40001"}))
	require.True(t, isDuplicateKey(&myErr{Number: 1062}))
	require.True(t, isDuplicateKey(fmt.Errorf("wrapped: %w", &myErr{Number: 1062})))
	require.False(t, isDuplicateKey(&myErr{Number: 1213}))
	require.False(t, isDuplicateKey(errors.New("Error 1062")))
}
