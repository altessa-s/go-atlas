// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

const (
	// DefaultPoolTimeout is the default timeout for acquiring a connection from the
	// pool. Applied to all clients created by [Factory.CreateClientFromConfig].
	DefaultPoolTimeout = 10 * time.Second

	// DefaultMaxRetries is the default maximum number of retries for failed
	// commands. Applied to all clients created by [Factory.CreateClientFromConfig].
	DefaultMaxRetries = 5

	// DefaultPingTimeout is the timeout used for the initial health-check ping
	// in [Factory.CreateClientFromConfig]. If the ping does not respond within
	// this duration the client is closed and an error is returned.
	DefaultPingTimeout = 5 * time.Second
)

// Factory creates Redis [redis.UniversalClient] instances from [config.Redis].
type Factory struct {
	corefactory.Base
	opts *options
}

// New creates a new Factory with the given functional options.
// By default the factory uses a discard logger; provide [WithLogger]
// to override.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base: corefactory.NewBase(cfg.logger),
		opts: cfg,
	}
}

// UniversalOptionsFromConfig builds [redis.UniversalOptions] from configuration.
// When [config.Redis.ConnectionURI] is set, it is parsed via [redis.ParseURL] to
// extract address, auth, TLS, and database number; pool/timeout/sentinel fields
// from config are applied on top. Otherwise, options are built from individual
// config fields. Returns an error if cfg is nil or the URI cannot be parsed.
func (f *Factory) UniversalOptionsFromConfig(cfg *config.Redis) (*redis.UniversalOptions, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if cfg.UseConnectionURI() {
		return f.universalOptionsFromURI(cfg)
	}

	return f.universalOptionsFromFields(cfg), nil
}

// universalOptionsFromFields builds UniversalOptions from individual config fields.
func (f *Factory) universalOptionsFromFields(cfg *config.Redis) *redis.UniversalOptions {
	opts := &redis.UniversalOptions{
		Addrs:           cfg.Hosts,
		Password:        cfg.Password.Expose(),
		Username:        cfg.Username,
		DB:              cfg.Database,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConnections,
		DialTimeout:     cfg.ConnectTimeout,
		ReadTimeout:     cfg.SocketTimeout,
		WriteTimeout:    cfg.SocketTimeout,
		ConnMaxIdleTime: cfg.IdleTimeout,
		ConnMaxLifetime: cfg.MaxConnectionAge,
		MaxRedirects:    cfg.MaxRedirects,
		ReadOnly:        cfg.ReadOnly,
		RouteByLatency:  cfg.RouteByLatency,
		RouteRandomly:   cfg.RouteRandomly,
		PoolTimeout:     DefaultPoolTimeout,
		MaxRetries:      DefaultMaxRetries,
	}

	// Sentinel mode: when MasterName is specified
	if cfg.MasterName != "" {
		opts.MasterName = cfg.MasterName
		opts.SentinelPassword = cfg.SentinelPassword.Expose()
	}

	return opts
}

// universalOptionsFromURI parses the ConnectionURI to get addr/auth/db, then
// applies pool, timeout, sentinel, and routing fields from config on top.
func (f *Factory) universalOptionsFromURI(cfg *config.Redis) (*redis.UniversalOptions, error) {
	parsed, err := redis.ParseURL(cfg.ConnectionURI.Expose())
	if err != nil {
		return nil, f.WrapError(err, "failed to parse connection URI")
	}

	opts := &redis.UniversalOptions{
		Addrs:           []string{parsed.Addr},
		Username:        parsed.Username,
		Password:        parsed.Password,
		DB:              parsed.DB,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConnections,
		DialTimeout:     cfg.ConnectTimeout,
		ReadTimeout:     cfg.SocketTimeout,
		WriteTimeout:    cfg.SocketTimeout,
		ConnMaxIdleTime: cfg.IdleTimeout,
		ConnMaxLifetime: cfg.MaxConnectionAge,
		MaxRedirects:    cfg.MaxRedirects,
		ReadOnly:        cfg.ReadOnly,
		RouteByLatency:  cfg.RouteByLatency,
		RouteRandomly:   cfg.RouteRandomly,
		PoolTimeout:     DefaultPoolTimeout,
		MaxRetries:      DefaultMaxRetries,
	}

	if parsed.TLSConfig != nil {
		opts.TLSConfig = parsed.TLSConfig
	}

	// Sentinel mode: when MasterName is specified
	if cfg.MasterName != "" {
		opts.MasterName = cfg.MasterName
		opts.SentinelPassword = cfg.SentinelPassword.Expose()
	}

	return opts, nil
}

// CreateClientFromConfig creates a [redis.UniversalClient] from configuration.
// After creating the client it performs a health-check ping within
// [DefaultPingTimeout]; if the ping fails the client is closed and an error
// is returned. If a [health.Coordinator] was provided via
// [WithHealthCoordinator], a health checker is registered under the service
// name "redis".
func (f *Factory) CreateClientFromConfig(ctx context.Context, cfg *config.Redis) (redis.UniversalClient, error) {
	opts, err := f.UniversalOptionsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	client := redis.NewUniversalClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			f.Logger().Warn("failed to close Redis client after ping failure",
				slog.Any("error", closeErr))
		}
		return nil, f.WrapError(err, "health check failed")
	}

	mode := f.detectMode(cfg)
	f.Logger().Debug("Redis client created",
		slog.String("mode", mode),
		slog.Any("hosts", cfg.Hosts))

	if f.opts.healthCoordinator != nil {
		f.opts.healthCoordinator.RegisterService("redis", &redisHealthChecker{client: client})
	}

	return client, nil
}

// detectMode returns a string describing the Redis mode based on configuration.
func (f *Factory) detectMode(cfg *config.Redis) string {
	if cfg.MasterName != "" {
		return "sentinel"
	}
	if len(cfg.Hosts) > 1 {
		return "cluster"
	}
	return "standalone"
}
