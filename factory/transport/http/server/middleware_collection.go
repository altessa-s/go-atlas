// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
	router "github.com/altessa-s/go-atlas/transport/http/server/router"
)

type middlewareCollection struct {
	tracer      tracing.Tracer
	collector   metrics.Meter
	limiter     sharedlimiter.Limiter
	idempotency idempotencydata.Idempotency
	geoResolver geoacl.GeoResolver
	// Middleware
	configMW     []middlewares.Middleware // from WithMiddlewares() / WithXMiddleware()
	additionalMW []middlewares.Middleware // from WithMiddleware()
	rawMW        []router.Middleware      // from WithRawMiddleware()
	disabledMW   map[string]struct{}      // from WithoutXMiddleware()
}

// registerMiddleware collects all middleware, filters by disabledMW, sorts,
// deduplicates, and registers them on the server.
func (b *middlewareCollection) registerMiddleware(srv *server.Server) error {
	ordered, err := b.ordered()
	if err != nil {
		return err
	}
	if len(ordered) > 0 {
		srv.RegisterMiddleware(slices.To(ordered, func(v middlewares.Middleware) router.Middleware {
			return v.Handler
		})...)
	}

	// Register raw middleware (bypasses sorting).
	if len(b.rawMW) > 0 {
		srv.RegisterMiddleware(b.rawMW...)
	}

	return nil
}

// ordered merges both sources of named middleware before dependency resolution.
func (b *middlewareCollection) ordered() ([]middlewares.Middleware, error) {
	// Merge config-based and additional middleware.
	allMW := make([]middlewares.Middleware, 0, len(b.configMW)+len(b.additionalMW))
	allMW = append(allMW, b.configMW...)
	allMW = append(allMW, b.additionalMW...)

	// Filter out disabled middleware.
	if len(b.disabledMW) > 0 {
		filtered := make([]middlewares.Middleware, 0, len(allMW))
		for _, mw := range allMW {
			if mw == nil {
				continue
			}
			if _, disabled := b.disabledMW[mw.Name()]; !disabled {
				filtered = append(filtered, mw)
			}
		}
		allMW = filtered
	}

	return middlewares.OrderMiddlewaresWithDedupe(allMW)
}

// bodyLimitWriter applies an explicit reader cap, or aligns it with an installed
// config bodylimit middleware. Disabled/uninstalled middleware cannot raise it.
func (b *ServerBuilder) bodyLimitWriter() server.ResponseWriter {
	if b.cfg.MaxBodySize != 0 {
		return writer.New(writer.WithMaxBodySize(b.cfg.MaxBodySize))
	}
	cfg := b.middlewaresCfg()
	if cfg == nil || !cfg.BodyLimit.IsEnabled() || cfg.BodyLimit.MaxSize <= 0 {
		return nil
	}
	if _, disabled := b.disabledMW[bodylimit.Name()]; disabled {
		return nil
	}
	for _, mw := range b.configMW {
		if mw.Name() == bodylimit.Name() {
			return writer.New(writer.WithMaxBodySize(cfg.BodyLimit.MaxSize))
		}
	}
	return nil
}
