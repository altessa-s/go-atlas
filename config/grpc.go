// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Grpc configuration.
const (
	defaultGrpcListenAddress = "0.0.0.0:7777"
	defaultGrpcReflection    = false
)

// GrpcTls represents TLS configuration for gRPC servers.
// It extends TlsServer configuration with optional client authentication settings.
type GrpcTls struct {
	// TlsServer contains the base server TLS configuration (embedded inline).
	*TlsServer `yaml:",inline"`

	// Client contains optional client certificate authentication settings.
	// If nil, client authentication is disabled.
	Client *TlsClientAuth `yaml:"client" default:"-"`
}

// Validate performs validation on the GrpcTls configuration.
// It validates the embedded TLS server configuration and client authentication
// settings if present.
//
// Returns an error if any validation rules fail.
func (c *GrpcTls) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Client, validation.Required.When(c.Client != nil)),
	)
}

// GrpcEnforcementPolicy represents keepalive enforcement policy for gRPC server.
// It defines rules for connection keepalive behavior and stream requirements.
type GrpcEnforcementPolicy struct {
	// MinTime is the minimum interval a client must wait between keepalive
	// pings. Pings sent more often than MinTime cause the server to send
	// GOAWAY and close the connection — this is the primary ping-flood
	// defense. Defaults to 30s, matching the explicit safe baseline applied
	// by the gRPC factory; bring it down (e.g. to 10s) only for clients
	// whose keepalive cadence requires it, and never raise it past the
	// expected client cadence (the stdlib's 5m default is permissive in
	// disguise — it lets malicious clients flood the connection table
	// because gRPC servers without an explicit policy reach it through a
	// silent fallback).
	MinTime time.Duration `yaml:"minTime" default:"30s"`

	// PermitWithoutStream allows clients to send keepalive pings even when
	// there are no active streams. Defaults to false: when no streams are
	// active, keepalive pings are rejected (the connection is closed). Set
	// to true for long-lived idle connections (e.g. pub/sub streams) that
	// rely on keepalive to detect dead peers.
	PermitWithoutStream bool `yaml:"permitWithoutStream" default:"false"`
}

// GrpcKeepAlive represents keepalive parameters for gRPC server connections.
// These parameters control connection lifecycle and health checking behavior.
type GrpcKeepAlive struct {
	// MaxConnectionIdle is a duration for the amount of time after which an
	// idle connection would be closed by sending a GoAway. Idleness duration is
	// defined since the most recent time the number of outstanding RPCs became
	// zero or the connection establishment.
	// Defaults to 15m (15 minutes).
	MaxConnectionIdle time.Duration `yaml:"maxConnectionIdle" default:"15m"`

	// MaxConnectionAge is a duration for the maximum amount of time a
	// connection may exist before it will be closed by sending a GoAway. A
	// random jitter of +/-10% will be added to MaxConnectionAge to spread out
	// connection storms.
	// Defaults to 30m (30 minutes).
	MaxConnectionAge time.Duration `yaml:"maxConnectionAge" default:"30m"`

	// MaxConnectionAgeGrace is an additive period after MaxConnectionAge after
	// which the connection will be forcibly closed.
	// Defaults to 5s (5 seconds).
	MaxConnectionAgeGrace time.Duration `yaml:"maxConnectionAgeGrace" default:"5s"`

	// Time specifies the duration after which if the server doesn't see any
	// activity it pings the client to see if the transport is still alive.
	// If set below 1s, a minimum value of 1s will be used instead.
	// Defaults to 5s (5 seconds).
	Time time.Duration `yaml:"time" default:"30s"`

	// Timeout specifies the duration the server waits for a response after
	// having pinged for keepalive check. If no activity is seen even after
	// that, the connection is closed.
	// Defaults to 1s (1 second).
	Timeout time.Duration `yaml:"timeout" default:"10s"`

	// EnforcementPolicy contains optional keepalive enforcement rules.
	// If nil, default enforcement policy is used.
	EnforcementPolicy *GrpcEnforcementPolicy `yaml:"enforcementPolicy" default:"-"`
}

// Validate performs validation on the GrpcKeepAlive configuration.
// It validates all duration fields to ensure they are valid duration strings.
//
// Returns an error if any validation rules fail.
func (k *GrpcKeepAlive) Validate() error {
	return ValidateStruct(k,
		validation.Field(&k.MaxConnectionIdle, ozzo_rules.DurationOrZero()),
		validation.Field(&k.MaxConnectionAge, ozzo_rules.DurationOrZero()),
		validation.Field(&k.MaxConnectionAgeGrace, ozzo_rules.DurationOrZero()),
		validation.Field(&k.Time, ozzo_rules.DurationOrZero()),
		validation.Field(&k.Timeout, ozzo_rules.DurationOrZero()),
		validation.Field(&k.EnforcementPolicy, validation.NilOrNotEmpty),
	)
}

// Grpc represents the configuration for gRPC server settings.
// It contains all necessary parameters for configuring a gRPC server including
// network binding, TLS settings, and debugging features.
//
// Example:
//
//	grpcCfg := &config.Grpc{
//		ListenAddress: "0.0.0.0:9090",
//		Reflection:    true,
//	}
type Grpc struct {
	// TLS contains optional TLS configuration for secure gRPC connections.
	// If nil, the server will run without TLS encryption.
	TLS *GrpcTls `yaml:"tls" default:"-"`

	// ListenAddress specifies the address and port to bind the gRPC server to.
	// Defaults to "0.0.0.0:7777" which binds to all interfaces on port 7777.
	ListenAddress string `yaml:"listenAddress" default:"0.0.0.0:7777"`

	// Reflection enables or disables gRPC server reflection.
	// When enabled, clients can discover available services and methods.
	// Defaults to false for security reasons.
	Reflection bool `yaml:"reflection" default:"false"`

	// ConnectionTimeout specifies the timeout for establishing connections.
	// If not set, gRPC uses the default behavior.
	// A zero or negative value will result in an immediate timeout.
	// Default value: not set
	ConnectionTimeout *time.Duration `yaml:"connectionTimeout"`

	// MaxConcurrentStreams limits the maximum number of concurrent streams.
	// If not set, gRPC uses math.MaxUint32 as the default limit.
	// Default value: not set
	MaxConcurrentStreams *uint32 `yaml:"maxConcurrentStreams"`

	// MaxSendMsgSize sets the maximum message size in bytes that can be sent.
	// If not set, gRPC uses the default of 4MB (4194304 bytes).
	// Default value: not set
	MaxSendMsgSize *int `yaml:"maxSendMsgSize"`

	// MaxRecvMsgSize sets the maximum message size in bytes that can be received.
	// If not set, gRPC uses the default of 4MB (4194304 bytes).
	// Default value: not set
	MaxRecvMsgSize *int `yaml:"maxRecvMsgSize"`

	// WriteBufferSize sets the size of the write buffer in bytes.
	// If not set, gRPC uses the default of 32KB.
	// Zero or negative values will disable the write buffer.
	// Default value: not set
	WriteBufferSize *int `yaml:"writeBufferSize"`

	// ReadBufferSize sets the size of the read buffer in bytes.
	// If not set, gRPC uses the default of 32KB.
	// Zero or negative values will disable the read buffer.
	// Default value: not set
	ReadBufferSize *int `yaml:"readBufferSize"`

	// KeepAlive contains server-side keepalive parameters for connection management.
	KeepAlive *GrpcKeepAlive `yaml:"keepAlive" default:"-"`

	// Interceptors contains configuration for gRPC middleware components.
	// Includes settings for cache, realIp, requestId, recovery, idempotency, auth, and limiter interceptors.
	Interceptors *InterceptorsConfig `yaml:"interceptors" default:"-"`
}

// DefaultGrpc returns a Grpc configuration with default values.
func DefaultGrpc() Grpc {
	return Grpc{
		ListenAddress: defaultGrpcListenAddress,
		Reflection:    defaultGrpcReflection,
	}
}

// Validate performs validation on the Grpc configuration.
// It validates the listen address format, connection timeout duration,
// TLS configuration, keepalive parameters, and interceptors configuration if present.
//
// Returns an error if any validation rules fail.
func (g *Grpc) Validate() error {
	return ValidateStruct(g,
		validation.Field(&g.ListenAddress, ozzo_rules.ListenAddress()),
		validation.Field(&g.ConnectionTimeout, validation.NilOrNotEmpty,
			ozzo_rules.Duration().When(g.ConnectionTimeout != nil)),
		validation.Field(&g.TLS, validation.Required.When(g.TLS != nil)),
		validation.Field(&g.KeepAlive, validation.Required.When(g.KeepAlive != nil)),
		validation.Field(&g.Interceptors, validation.Required.When(g.Interceptors != nil)),
	)
}
