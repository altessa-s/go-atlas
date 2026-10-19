// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/security/tlsutils"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// Default keepalive enforcement policy applied by [ServerBuilder.Build]
// when the operator config does not provide explicit values. The stdlib
// gRPC server falls back to a permissive policy (5m MinTime) when none
// is registered with the server, and that fallback only triggers because
// the policy slot is empty — registering ANY policy is what matters.
// Picking explicit, conservative values keeps the ping-flood door shut
// regardless of whether the operator filled in [grpcconfig.Config.KeepAlive].
const (
	// DefaultGrpcEnforcementMinTime is the minimum ping interval clients
	// must respect. Pings under this cadence cause the server to send
	// GOAWAY and drop the connection.
	DefaultGrpcEnforcementMinTime = 30 * time.Second

	// DefaultGrpcEnforcementPermitWithoutStream rejects keepalive pings
	// from connections with no active RPCs. Operators serving long-lived
	// idle streams should override via config.
	DefaultGrpcEnforcementPermitWithoutStream = false
)

// ServerBuilder assembles a gRPC [grpcserver.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	cfg  *grpcconfig.Config
	errs []error

	// Dependencies
	tlsProviders           *tlsproviders.Providers
	cacheMetadataProcessor cache.MetadataProcessor
	cacheMethodConfigs     []cache.MethodConfig
	tracer                 tracing.Tracer
	collector              metrics.Collector
	limiter                sharedlimiter.Limiter
	idempotency            idempotencydata.Idempotency
	cacher                 cache.Cacher
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
	serverOptions          []grpcserver.Option

	// Interceptors
	interceptors []interceptors.ServerInterceptor
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
		return nil, fmt.Errorf("configuration is required")
	}

	// Resolve the interceptor chain now, so a missing required interceptor
	// (the limiter without realIP, for example), a duplicate or a cycle is
	// reported by Build instead of by Server.Start.
	if err := validateChain(b.interceptors); err != nil {
		return nil, b.WrapError(err, "invalid interceptor chain")
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
		baseOpts = append(baseOpts, baseserver.WithTLSConfig(tlsConfig))
	}

	opts = append(opts, grpcserver.WithBaseOptions(baseOpts...))

	// Add gRPC server options from config.
	grpcOpts := b.buildGrpcOptions()
	opts = slices.AppendIf(opts, len(grpcOpts) > 0, grpcserver.WithGrpcOptions(grpcOpts...))
	opts = append(opts, b.serverOptions...)

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

// validateChain resolves the dependency order of list the way the server does
// when it starts and returns the resolution error, if any.
func validateChain(list []interceptors.ServerInterceptor) error {
	present := make([]any, 0, len(list))
	for _, i := range list {
		present = slices.AppendIf(present, nilcheck.IsNotNil(i), any(i))
	}
	if len(present) == 0 {
		return nil
	}
	_, err := interceptors.NewChain(present...).DependencyOrder()
	return err
}

// buildServerTlsConfig builds a TLS config from the builder's gRPC TLS configuration.
func (b *ServerBuilder) buildServerTlsConfig() (*tls.Config, error) {
	if err := b.RequireDependency(b.tlsProviders, "tls providers"); err != nil {
		return nil, err
	}

	cfg := b.cfg.TLS
	providerType := tlsproviders.ProviderType(cfg.Provider)
	provider, ok := b.tlsProviders.Get(providerType)
	if !ok {
		return nil, b.Errorf("tls provider %q not configured", cfg.Provider)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, b.WrapError(err, "failed to create TLS config from provider")
	}

	if err := tlsutils.ApplyMinTLSVersion(tlsConfig, cfg.MinTLSVersion); err != nil {
		return nil, b.WrapError(err, "tls.minVersion")
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

	opts = slices.AppendIfFunc(opts, cfg.ConnectionTimeout != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.ConnectionTimeout(*cfg.ConnectionTimeout)}
	})
	opts = slices.AppendIfFunc(opts, cfg.MaxConcurrentStreams != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.MaxConcurrentStreams(*cfg.MaxConcurrentStreams)}
	})
	opts = slices.AppendIfFunc(opts, cfg.MaxSendMsgSize != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.MaxSendMsgSize(*cfg.MaxSendMsgSize)}
	})
	opts = slices.AppendIfFunc(opts, cfg.MaxRecvMsgSize != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.MaxRecvMsgSize(*cfg.MaxRecvMsgSize)}
	})
	opts = slices.AppendIfFunc(opts, cfg.WriteBufferSize != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.WriteBufferSize(*cfg.WriteBufferSize)}
	})
	opts = slices.AppendIfFunc(opts, cfg.ReadBufferSize != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.ReadBufferSize(*cfg.ReadBufferSize)}
	})

	opts = slices.AppendIfFunc(opts, cfg.KeepAlive != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     cfg.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      cfg.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: cfg.KeepAlive.MaxConnectionAgeGrace,
			Time:                  cfg.KeepAlive.Time,
			Timeout:               cfg.KeepAlive.Timeout,
		})}
	})

	// Always install an enforcement policy — see the package-level
	// constants for the rationale. Without this, configs that leave
	// KeepAlive (or EnforcementPolicy inside it) nil silently fall back
	// to the permissive stdlib default and the server has no real
	// ping-flood defense.
	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepaliveEnforcementPolicy(cfg)))

	return opts
}

// keepaliveEnforcementPolicy resolves the enforcement policy to install
// on the gRPC server: the config value when supplied, otherwise the
// package-level safe defaults. Split out so unit tests can exercise the
// fallback paths without spinning up a full server.
func keepaliveEnforcementPolicy(cfg *grpcconfig.Config) keepalive.EnforcementPolicy {
	if cfg != nil && cfg.KeepAlive != nil && cfg.KeepAlive.EnforcementPolicy != nil {
		return keepalive.EnforcementPolicy{
			MinTime:             cfg.KeepAlive.EnforcementPolicy.MinTime,
			PermitWithoutStream: cfg.KeepAlive.EnforcementPolicy.PermitWithoutStream,
		}
	}
	return keepalive.EnforcementPolicy{
		MinTime:             DefaultGrpcEnforcementMinTime,
		PermitWithoutStream: DefaultGrpcEnforcementPermitWithoutStream,
	}
}
