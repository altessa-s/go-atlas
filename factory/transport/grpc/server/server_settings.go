// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"time"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/factory/transport/internal/factoryconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
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

type serverSettings struct {
	cfg           *grpcconfig.Config
	tlsProviders  *tlsproviders.Providers
	serverOptions []grpcserver.Option
}

func (b *serverSettings) newServer(base *corefactory.Base) (*grpcserver.Server, error) {
	opts := make([]grpcserver.Option, 0, 4)
	opts = slices.AppendIf(opts, b.cfg.Reflection, grpcserver.WithReflection())

	opts = append(opts,
		grpcserver.WithAddress(b.cfg.ListenAddress),
		grpcserver.WithLogger(base.Logger()),
	)

	if b.cfg.TLS != nil {
		tlsConfig, err := b.buildServerTlsConfig(base)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpcserver.WithTLSConfig(tlsConfig))
	}

	// Add gRPC server options from config.
	grpcOpts := b.buildGrpcOptions()
	opts = slices.AppendIf(opts, len(grpcOpts) > 0, grpcserver.WithGrpcOptions(grpcOpts...))
	opts = append(opts, b.serverOptions...)

	return grpcserver.New(opts...)
}

// buildServerTlsConfig builds a TLS config from the builder's gRPC TLS configuration.
func (b *serverSettings) buildServerTlsConfig(base *corefactory.Base) (*tls.Config, error) {
	cfg := b.cfg.TLS
	tlsConfig, err := factoryconv.ServerTLS(base, b.tlsProviders, cfg.Server)
	if err != nil {
		return nil, err
	}
	// mTLS client authentication
	if cfg.Client != nil && cfg.Client.AllowCertAuth {
		caPool, err := tlsutils.BuildCAPool(false, cfg.Client.CACerts...)
		if err != nil {
			return nil, base.WrapError(err, "failed to build client CA pool")
		}
		tlsConfig.ClientCAs = caPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// buildGrpcOptions builds gRPC server options from the builder's configuration.
func (b *serverSettings) buildGrpcOptions() []grpc.ServerOption {
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
