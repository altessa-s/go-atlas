// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	cfg "github.com/altessa-s/go-atlas/config/http"
)

// Enable every configurable entry to pin the entire default registration chain.
func TestFullDefaultChainOrder(t *testing.T) {
	t.Parallel()
	c := cfg.DefaultMiddlewares()
	defaults := New(&cfg.Config{Middlewares: &c}).WithMiddlewares()
	order, err := defaults.ordered()
	require.NoError(t, err)
	require.Empty(t, order)
	c.BodyLimit.Enabled = true
	c.Cors.Enabled = true
	c.Idempotency.Enabled = true
	c.IpAcl.Enabled = true
	c.GeoAcl.Enabled = true
	c.Limiter.Enabled = true
	c.Logger.Enabled = true
	c.Metrics.Enabled = true
	c.RealIp.Enabled = true
	c.Recovery.Enabled = true
	c.RequestId.Enabled = true
	c.SecurityHeaders.Enabled = true
	c.Tracing.Enabled = true
	b := New(&cfg.Config{Middlewares: &c}).
		UseLimiter(struct{ limiters.Limiter }{}).
		UseGeoResolver(struct{ geoacl.GeoResolver }{}).
		UseIdempotency(struct{ idempotency.Idempotency }{}).
		UseTracer(tracing.Noop()).WithMiddlewares()
	require.Empty(t, b.errs)
	ordered, err := b.ordered()
	require.NoError(t, err)
	var names []string
	for _, m := range ordered {
		names = append(names, m.Name())
	}
	require.Equal(t, []string{"recovery", "realip", "requestid", "tracing", "logger", "metrics", "cors",
		"securityheaders", "bodylimit", "ipacl", "geoacl", "limiter", "idempotency"}, names)
}
