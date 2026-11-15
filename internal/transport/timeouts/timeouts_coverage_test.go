// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package timeouts_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	got := timeouts.Default()

	require.Equal(t, timeouts.Lifecycle{
		StartupVerification: 2 * time.Second,
		ShutdownGraceful:    time.Minute,
		ShutdownWarning:     15 * time.Second,
	}, got)
	require.Equal(t, timeouts.DefaultStartupVerification, got.StartupVerification)
	require.Equal(t, timeouts.DefaultShutdownGraceful, got.ShutdownGraceful)
	require.Equal(t, timeouts.DefaultShutdownWarning, got.ShutdownWarning)
}
