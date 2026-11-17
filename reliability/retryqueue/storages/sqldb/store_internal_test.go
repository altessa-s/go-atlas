// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
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
