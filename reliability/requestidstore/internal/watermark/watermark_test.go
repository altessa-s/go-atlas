// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package watermark_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/requestidstore/internal/watermark"
)

func TestClock(t *testing.T) {
	t.Parallel()
	var clock watermark.Clock
	require.True(t, clock.Now().IsZero())
	first := time.Time{}.Add(-time.Hour)
	clock.Observe(first)
	require.Equal(t, first, clock.Now())
	now := time.Now()
	clock.Observe(now)
	clock.Observe(now.Add(-time.Hour))
	require.True(t, now.Equal(clock.Now()))
}
