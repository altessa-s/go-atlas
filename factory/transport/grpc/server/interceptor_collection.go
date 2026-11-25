// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	originalCache "github.com/altessa-s/go-atlas/cache"
	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
)

type interceptorCollection struct {
	cacheMetadataProcessor cache.MetadataProcessor
	cacheMethodPolicies    []cache.MethodPolicy
	tracer                 tracing.Tracer
	collector              metrics.Meter
	limiter                sharedlimiter.Limiter
	idempotency            idempotencydata.Idempotency
	cacher                 originalCache.Cacher
	authFn                 auth.Auth
	clientAuth             auth.ClientAuth
	healthChecker          health.Health
	geoResolver            geoacl.GeoResolver
	reasonCode             bufhelpers.ReasonCoder
	errStatusOptions       []errstatus.Option
	loggerOptions          []logger.Option
	fieldBehaviorOptions   []fieldbehavior.Option
	idempotencyOptions     []idempotency.Option
	fieldMaskOptions       []fieldmask.Option
	// Interceptors
	interceptors []interceptors.ServerInterceptor
}

// dependencyOrder resolves the same chain that the server installs on startup.
func (c *interceptorCollection) dependencyOrder() ([]string, error) {
	present := make([]any, 0, len(c.interceptors))
	for _, i := range c.interceptors {
		present = slices.AppendIf(present, nilcheck.IsNotNil(i), any(i))
	}
	if len(present) == 0 {
		return nil, nil
	}
	return interceptors.NewChain(present...).DependencyOrder()
}
