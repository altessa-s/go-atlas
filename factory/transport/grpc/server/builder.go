// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
)

// ServerBuilder assembles a gRPC [grpcserver.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	errs []error
	serverSettings
	interceptorCollection
}

// New creates a [ServerBuilder] for the given gRPC config.
// Config can be nil -- the error surfaces at [ServerBuilder.Build] time.
func New(cfg *grpcconfig.Config) *ServerBuilder {
	return &ServerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the gRPC server. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ServerBuilder) Build() (*grpcserver.Server, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	// Resolve the interceptor chain now, so a missing required interceptor
	// (the limiter without realIP, for example), a duplicate or a cycle is
	// reported by Build instead of by Server.Start.
	if _, err := b.dependencyOrder(); err != nil {
		return nil, b.WrapError(err, "invalid interceptor chain")
	}

	srv, err := b.newServer(&b.Base)
	if err != nil {
		return nil, err
	}

	// Register interceptors (nil interceptors are filtered by RegisterInterceptors).
	if chain := b.resolveCacheInterceptors(); len(chain) > 0 {
		if err := srv.RegisterInterceptors(chain...); err != nil {
			return nil, err
		}
	}

	return srv, nil
}
