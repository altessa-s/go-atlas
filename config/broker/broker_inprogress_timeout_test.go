// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

func TestInProgress_HeartbeatTimeout(t *testing.T) {
	t.Parallel()

	c := brokerconfig.Default().InProgress
	require.Equal(t, 5*time.Second, c.HeartbeatTimeout)

	c.HeartbeatTimeout = 0
	require.NoError(t, c.Validate(), "zero keeps the manager default")

	c.HeartbeatTimeout = -time.Second
	require.Contains(t, brokerFieldErrors(t, c.Validate()), "HeartbeatTimeout")
}
