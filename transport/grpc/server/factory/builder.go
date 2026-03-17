// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/security/tlsutils"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// ServerBuilder assembles a gRPC [grpcserver.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	cfg  *config.Grpc
	errs []error

	// Dependencies
	tlsProviders           *tlsproviders.Providers
	cacheMetadataProcessor cache.MetadataProcessor
	tracer                 tracing.Tracer
	limiter                sharedlimiter.Limiter
	idempotency            idempotencydata.Idempotency
	cacher                 cache.Cacher
	authFn                 auth.Auth
	clientAuth             auth.ClientAuth
	healthChecker          health.Health
	geoResolver            geoacl.GeoResolver

	// Interceptors
	interceptors []interceptors.ServerInterceptor
}

// New creates a [ServerBuilder] for the given gRPC config.
// Config can be nil -- the error surfaces at [ServerBuilder.Build] time.
func New(cfg *config.Grpc) *ServerBuilder {
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
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]grpcserver.Option, 0, 4)
	opts = slices.AppendIf(opts, b.cfg.Reflection, grpcserver.WithReflection())

	baseOpts := make([]baseserver.Option, 0, 4)
	baseOpts = append(baseOpts,
		baseserver.WithAddress(b.cfg.ListenAddress),
		baseserver.WithLogger(b.Logger()),
	)

	if b.cfg.TLS != nil {
		tlsConfig, err := b.buildServerTlsConfig()
		if err != nil {
			return nil, err
		}
		baseOpts = append(baseOpts, baseserver.WithTlsConfig(tlsConfig))
	}

	opts = append(opts, grpcserver.WithBaseOptions(baseOpts...))

	// Add gRPC server options from config.
	grpcOpts := b.buildGrpcOptions()
	opts = slices.AppendIf(opts, len(grpcOpts) > 0, grpcserver.WithGrpcOptions(grpcOpts...))

	srv, err := grpcserver.New(opts...)
	if err != nil {
		return nil, err
	}

	// Register interceptors (nil interceptors are filtered by RegisterInterceptors).
	if len(b.interceptors) > 0 {
		if err := srv.RegisterInterceptors(b.interceptors...); err != nil {
			return nil, err
		}
	}

	return srv, nil
}

// buildServerTlsConfig builds a TLS config from the builder's gRPC TLS configuration.
func (b *ServerBuilder) buildServerTlsConfig() (*tls.Config, error) {
	if err := b.RequireDependency(b.tlsProviders, "tls providers"); err != nil {
		return nil, err
	}

	cfg := b.cfg.TLS
	providerType := tlsproviders.ProviderType(cfg.ProviderType)
	provider, ok := b.tlsProviders.Get(providerType)
	if !ok {
		return nil, b.Errorf("tls provider %q not configured", cfg.ProviderType)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, b.WrapError(err, "failed to create TLS config from provider")
	}

	if cfg.MinTLSVersion == "1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	// mTLS client authentication
	if cfg.Client != nil && cfg.Client.AllowCertAuth {
		caPool, err := tlsutils.BuildCAPool(false, cfg.Client.CACerts...)
		if err != nil {
			return nil, b.WrapError(err, "failed to build client CA pool")
		}
		tlsConfig.ClientCAs = caPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// buildGrpcOptions builds gRPC server options from the builder's configuration.
func (b *ServerBuilder) buildGrpcOptions() []grpc.ServerOption {
	cfg := b.cfg
	opts := make([]grpc.ServerOption, 0, 8)

	if cfg.ConnectionTimeout != nil {
		opts = append(opts, grpc.ConnectionTimeout(*cfg.ConnectionTimeout))
	}

	if cfg.MaxConcurrentStreams != nil {
		opts = append(opts, grpc.MaxConcurrentStreams(*cfg.MaxConcurrentStreams))
	}

	if cfg.MaxSendMsgSize != nil {
		opts = append(opts, grpc.MaxSendMsgSize(*cfg.MaxSendMsgSize))
	}

	if cfg.MaxRecvMsgSize != nil {
		opts = append(opts, grpc.MaxRecvMsgSize(*cfg.MaxRecvMsgSize))
	}

	if cfg.WriteBufferSize != nil {
		opts = append(opts, grpc.WriteBufferSize(*cfg.WriteBufferSize))
	}

	if cfg.ReadBufferSize != nil {
		opts = append(opts, grpc.ReadBufferSize(*cfg.ReadBufferSize))
	}

	if cfg.KeepAlive != nil {
		opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     cfg.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      cfg.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: cfg.KeepAlive.MaxConnectionAgeGrace,
			Time:                  cfg.KeepAlive.Time,
			Timeout:               cfg.KeepAlive.Timeout,
		}))

		if cfg.KeepAlive.EnforcementPolicy != nil {
			opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             cfg.KeepAlive.EnforcementPolicy.MinTime,
				PermitWithoutStream: cfg.KeepAlive.EnforcementPolicy.PermitWithoutStream,
			}))
		}
	}

	return opts
}
