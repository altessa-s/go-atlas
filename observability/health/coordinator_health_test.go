// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
	grpchealth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
)

// The Coordinator plugs into the gRPC health interceptor as is.
var _ grpchealth.Health = (*health.Coordinator)(nil)

// staticChecker reports a fixed status.
type staticChecker health.ServingStatus

func (s staticChecker) CheckHealth(context.Context) health.ServingStatus {
	return health.ServingStatus(s)
}

func TestCoordinatorHealth(t *testing.T) {
	t.Parallel()

	c := health.New()
	t.Cleanup(c.Close)
	require.NoError(t, c.Health(t.Context()), "no services: serving")

	c.RegisterService("db", staticChecker(health.StatusServing))
	require.NoError(t, c.Health(t.Context()))

	c.RegisterService("cache", staticChecker(health.StatusNotServing))
	require.ErrorIs(t, c.Health(t.Context()), health.ErrNotServing)

	c.UnregisterService("cache")
	c.SetOverallStatus(health.StatusNotServing)
	require.ErrorIs(t, c.Health(t.Context()), health.ErrNotServing, "the override applies")
}
