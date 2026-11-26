// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	cfg "github.com/altessa-s/go-atlas/config/grpc"
)

type chainAuth = auth.Auth
type chainHealth = health.Health

// Enable every configurable entry to pin the entire default registration chain.
func TestFullDefaultChainOrder(t *testing.T) {
	t.Parallel()
	c := cfg.DefaultInterceptors()
	defaults := New(&cfg.Config{Interceptors: &c}).WithInterceptors()
	order, err := defaults.dependencyOrder()
	require.NoError(t, err)
	require.Equal(t, []string{"metadata", "fieldbehavior", "protovalidator"}, order)
	c.Cache.Enabled = true
	c.RealIp.Enabled = true
	c.RequestId.Enabled = true
	c.Recovery.Enabled = true
	c.Idempotency.Enabled = true
	c.Auth.Enabled = true
	c.Health.Enabled = true
	c.IpAcl.Enabled = true
	c.GeoAcl.Enabled = true
	c.Limiter.Enabled = true
	c.Logger.Enabled = true
	c.Metrics.Enabled = true
	c.Tracing.Enabled = true
	c.ErrStatus.Enabled = true
	c.FieldMask.Enabled = true
	b := New(&cfg.Config{Interceptors: &c}).
		UseLimiter(struct{ limiters.Limiter }{}).
		UseGeoResolver(struct{ geoacl.GeoResolver }{}).
		UseIdempotency(struct{ idempotency.Idempotency }{}).
		UseCacher(struct{ cache.Cacher }{}).
		UseAuth(struct{ chainAuth }{}, nil).
		UseHealthChecker(struct{ chainHealth }{}).
		WithInterceptors()
	require.Empty(t, b.errs)
	names, err := b.dependencyOrder()
	require.NoError(t, err)
	require.Equal(t, []string{"metadata", "realip", "requestid", "tracing", "logger", "metrics", "errstatus",
		"recovery", "ipacl", "geoacl", "auth", "limiter", "idempotency", "cache",
		"fieldbehavior", "fieldmask", "protovalidator", "health"}, names)
}
