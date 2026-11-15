// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptTimeoutIsCappedByTimeout(t *testing.T) {
	t.Parallel()

	require.Equal(t, DefaultAttemptTimeout, New(nil).attemptTimeout)
	require.Equal(t, 300*time.Millisecond, New(nil, WithAttemptTimeout(300*time.Millisecond)).attemptTimeout)
	require.Equal(t, time.Second, New(nil, WithTimeout(time.Second), WithAttemptTimeout(time.Minute)).attemptTimeout)
	require.Equal(t, 500*time.Millisecond, New(nil, WithTimeout(500*time.Millisecond)).attemptTimeout)
	require.Equal(t, DefaultAttemptTimeout, New(nil, WithAttemptTimeout(0)).attemptTimeout)
}

// With the defaults, the first request and every retry fit into the check
// timeout even when each request runs into its attempt timeout and each
// backoff takes its longest, jittered delay.
func TestDefaultAttemptsFitIntoDefaultTimeout(t *testing.T) {
	t.Parallel()

	budget := time.Duration(DefaultMaxAttempts+1) * DefaultAttemptTimeout
	delay := retryBaseDelay
	for range DefaultMaxAttempts {
		budget += time.Duration(float64(min(delay, retryMaxDelay)) * (1 + retryJitter))
		delay *= 2
	}
	require.Less(t, budget, DefaultTimeout)
}
