// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	natsconfig "github.com/altessa-s/go-atlas/config/nats"
)

func TestConfig_SubscriberDrainTimeout(t *testing.T) {
	t.Parallel()

	c := natsconfig.Default()
	require.Equal(t, 30*time.Second, c.SubscriberDrainTimeout)

	c.SubscriberDrainTimeout = 0
	require.NoError(t, c.Validate(), "zero keeps the provider default")

	c.SubscriberDrainTimeout = -time.Second
	require.Contains(t, fieldErrors(t, c.Validate()), "SubscriberDrainTimeout")
}
