// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestRegisteredUnknownIsUnhealthy(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	c.RegisterService("dependency", health.Func(func(context.Context) health.ServingStatus { return health.StatusServiceUnknown }))
	require.Equal(t, health.StatusNotServing, c.CheckHealth(t.Context()))
	require.ErrorIs(t, c.Health(t.Context()), health.ErrNotServing)
	statuses, err := c.ListStatuses(t.Context())
	require.NoError(t, err)
	require.Equal(t, health.StatusServiceUnknown, statuses["dependency"])
}

func TestUnregisteredDuringCheckIsIgnored(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	entered, resume := make(chan struct{}), make(chan struct{})
	c.RegisterService("dependency", health.Func(func(context.Context) health.ServingStatus {
		close(entered)
		<-resume
		return health.StatusNotServing
	}))
	result := make(chan health.ServingStatus, 1)
	go func() { result <- c.CheckHealth(t.Context()) }()
	<-entered
	c.UnregisterService("dependency")
	close(resume)
	require.Equal(t, health.StatusServing, <-result)
}
