// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/http/server"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// ServerBuilder assembles an HTTP [server.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	errs []error
	serverSettings
	middlewareCollection
	endpoints
}

// New creates a [ServerBuilder] for the given HTTP config.
// Config can be nil — the error surfaces at [ServerBuilder.Build] time.
func New(cfg *httpconfig.Config) *ServerBuilder {
	return &ServerBuilder{
		Base:           corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:            cfg,
		builtinEnabled: true,
		metricsEnabled: true,
		internalPrefix: "/internal",
	}
}

// --- Build ---

// Build assembles the server. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ServerBuilder) Build() (*server.Server, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	rootRouter, routerInterface := b.buildRouter()

	srv, err := b.newServer(&b.Base, routerInterface, b.bodyLimitWriter())
	if err != nil {
		return nil, err
	}

	// Collect, filter, sort and register middleware.
	if err := b.registerMiddleware(srv); err != nil {
		return nil, coreerrs.WrapOperation(err, "register HTTP middlewares")
	}

	// Register built-in handlers.
	b.registerBuiltinHandlers(srv, rootRouter, b.cfg.Pprof)

	// Register custom handlers.
	srv.RegisterHandlers(b.handlers...)

	return srv, nil
}
