// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"

	corectx "github.com/altessa-s/go-atlas/core/context"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

const (
	// DefaultPoolTimeout is the default timeout for acquiring a connection from the
	// pool. Applied to all clients created by [ClientBuilder.Build].
	DefaultPoolTimeout = 10 * time.Second

	// DefaultMaxRetries is the default maximum number of retries for failed
	// commands. Applied to all clients created by [ClientBuilder.Build].
	DefaultMaxRetries = 5

	// DefaultPingTimeout is the timeout used for the initial health-check ping
	// in [ClientBuilder.Build]. If the ping does not respond within
	// this duration the client is closed and an error is returned.
	DefaultPingTimeout = 5 * time.Second
)

// ClientBuilder assembles a Redis [redis.UniversalClient] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ClientBuilder.Build] time.
// The builder is not safe for concurrent use.
type ClientBuilder struct {
	corefactory.Base
	cfg  *config.Redis
	errs []error

	// Dependencies
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [ClientBuilder] for the given Redis config.
// Config can be nil — the error surfaces at [ClientBuilder.Build] time.
func New(cfg *config.Redis) *ClientBuilder {
	return &ClientBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a [redis.UniversalClient] from configuration.
// After creating the client it performs a health-check ping within
// [DefaultPingTimeout]; if the ping fails the client is closed and an error
// is returned. If a [health.Coordinator] was provided via
// [ClientBuilder.UseHealthCoordinator], a health checker is registered under
// the service name "redis".
func (b *ClientBuilder) Build(ctx context.Context) (redis.UniversalClient, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts, err := b.UniversalOptions()
	if err != nil {
		return nil, err
	}
	client := redis.NewUniversalClient(opts)

	pingCtx, cancel := corectx.ApplyTimeout(ctx, DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			b.Logger().Warn("failed to close Redis client after ping failure",
				slog.Any("error", closeErr))
		}
		return nil, b.WrapError(err, "health check failed")
	}

	mode := b.detectMode()
	b.Logger().Debug("Redis client created",
		slog.String("mode", mode),
		slog.Any("hosts", b.cfg.Hosts))

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(cmp.Or(b.healthServiceName, "redis"), &redisHealthChecker{
			client: client,
			logger: b.Logger(),
		})
	}

	return client, nil
}

// UniversalOptions builds [redis.UniversalOptions] from configuration.
// When [config.Redis.ConnectionURI] is set, it is parsed via [redis.ParseURL] to
// extract address, auth, TLS, and database number; pool/timeout/sentinel fields
// from config are applied on top. Otherwise, options are built from individual
// config fields. Returns an error if cfg is nil or the URI cannot be parsed.
func (b *ClientBuilder) UniversalOptions() (*redis.UniversalOptions, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if b.cfg.UseConnectionURI() {
		return b.universalOptionsFromURI()
	}

	return b.universalOptionsFromFields(), nil
}

// universalOptionsFromFields builds UniversalOptions from individual config fields.
func (b *ClientBuilder) universalOptionsFromFields() *redis.UniversalOptions {
	opts := &redis.UniversalOptions{
		Addrs:           b.cfg.Hosts,
		Password:        b.cfg.Password.Expose(),
		Username:        b.cfg.Username,
		DB:              b.cfg.Database,
		PoolSize:        b.cfg.PoolSize,
		MinIdleConns:    b.cfg.MinIdleConnections,
		DialTimeout:     b.cfg.ConnectTimeout,
		ReadTimeout:     b.cfg.SocketTimeout,
		WriteTimeout:    b.cfg.SocketTimeout,
		ConnMaxIdleTime: b.cfg.IdleTimeout,
		ConnMaxLifetime: b.cfg.MaxConnectionAge,
		MaxRedirects:    b.cfg.MaxRedirects,
		ReadOnly:        b.cfg.ReadOnly,
		RouteByLatency:  b.cfg.RouteByLatency,
		RouteRandomly:   b.cfg.RouteRandomly,
		PoolTimeout:     DefaultPoolTimeout,
		MaxRetries:      DefaultMaxRetries,
	}

	// Sentinel mode: when MasterName is specified
	if b.cfg.MasterName != "" {
		opts.MasterName = b.cfg.MasterName
		opts.SentinelPassword = b.cfg.SentinelPassword.Expose()
	}

	return opts
}

// universalOptionsFromURI parses the ConnectionURI to get addr/auth/db, then
// applies pool, timeout, sentinel, and routing fields from config on top.
func (b *ClientBuilder) universalOptionsFromURI() (*redis.UniversalOptions, error) {
	parsed, err := redis.ParseURL(b.cfg.ConnectionURI.Expose())
	if err != nil {
		return nil, b.WrapError(err, "failed to parse connection URI")
	}

	opts := &redis.UniversalOptions{
		Addrs:           []string{parsed.Addr},
		Username:        parsed.Username,
		Password:        parsed.Password,
		DB:              parsed.DB,
		PoolSize:        b.cfg.PoolSize,
		MinIdleConns:    b.cfg.MinIdleConnections,
		DialTimeout:     b.cfg.ConnectTimeout,
		ReadTimeout:     b.cfg.SocketTimeout,
		WriteTimeout:    b.cfg.SocketTimeout,
		ConnMaxIdleTime: b.cfg.IdleTimeout,
		ConnMaxLifetime: b.cfg.MaxConnectionAge,
		MaxRedirects:    b.cfg.MaxRedirects,
		ReadOnly:        b.cfg.ReadOnly,
		RouteByLatency:  b.cfg.RouteByLatency,
		RouteRandomly:   b.cfg.RouteRandomly,
		PoolTimeout:     DefaultPoolTimeout,
		MaxRetries:      DefaultMaxRetries,
	}

	if parsed.TLSConfig != nil {
		opts.TLSConfig = parsed.TLSConfig
	}

	// Sentinel mode: when MasterName is specified
	if b.cfg.MasterName != "" {
		opts.MasterName = b.cfg.MasterName
		opts.SentinelPassword = b.cfg.SentinelPassword.Expose()
	}

	return opts, nil
}

// detectMode returns a string describing the Redis mode based on configuration.
func (b *ClientBuilder) detectMode() string {
	if b.cfg.MasterName != "" {
		return "sentinel"
	}
	if len(b.cfg.Hosts) > 1 {
		return "cluster"
	}
	return "standalone"
}
