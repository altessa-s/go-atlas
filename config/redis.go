// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"errors"
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Redis configuration.
const (
	defaultRedisHost             = "localhost:6379"
	defaultRedisDatabase         = 0
	defaultRedisMinIdleConns     = 5
	defaultRedisMaxRedirects     = 3
	defaultRedisPoolSize         = 100
	defaultRedisMaxConnectionAge = 0
	defaultRedisConnectTimeout   = 5 * time.Second
	defaultRedisSocketTimeout    = 5 * time.Second
	defaultRedisIdleTimeout      = 5 * time.Second
)

// Redis represents the configuration for Redis database connections.
// It contains all necessary parameters for connecting to Redis including
// authentication, connection pooling, timeouts, and cluster/sentinel settings.
//
// When ConnectionURI is set it takes priority over Hosts and authentication fields
// (Username, Password). Fields not expressible through the URI (pool sizes, timeouts,
// sentinel, routing) are applied on top. Note: ConnectionURI with redis.ParseURL
// supports a single address only — for multi-node clusters use Hosts instead.
//
// Example:
//
//	redis := &config.Redis{
//		Hosts:    []string{"redis1:6379", "redis2:6379"},
//		Password: "secret",
//	}
type Redis struct {
	// KeysPrefix is an optional prefix to prepend to all Redis keys.
	// If nil, no prefix is used.
	KeysPrefix *string `yaml:"keysPrefix"`

	// ConnectionURI is a full Redis connection string (e.g. "redis://user:pass@host:6379/0").
	// When set, it replaces Hosts, Username, Password, and Database.
	// Mutually exclusive with Username and Password.
	// Supports a single address only; for clusters with multiple seeds use Hosts.
	ConnectionURI Secret `yaml:"connectionUri"`

	// Username is the username for Redis authentication (Redis 6.0+).
	// Leave empty for password-only authentication.
	Username string `yaml:"username"`

	// Password is the password for Redis authentication.
	// Leave empty for no authentication.
	Password Secret `yaml:"password"`

	// SentinelPassword is the password for Redis Sentinel authentication.
	// Used when connecting to Redis through Sentinel.
	SentinelPassword Secret `yaml:"sentinelPassword"`

	// MasterName is the name of the Redis master when using Sentinel.
	// Required for Sentinel-based configurations.
	MasterName string `yaml:"masterName"`

	// Hosts is the list of Redis server addresses.
	// Defaults to ["localhost:6379"].
	// For clusters, specify multiple nodes. For Sentinel, specify sentinel addresses.
	Hosts []string `yaml:"hosts" default:"localhost:6379"`

	// Database is the Redis database number to select.
	// Defaults to 0. Only applicable for standalone Redis (not clusters).
	Database int `yaml:"database" default:"0,skip_zero"`

	// MinIdleConnections is the minimum number of idle connections to maintain.
	// Defaults to 5.
	MinIdleConnections int `yaml:"minIdleConnections" default:"5"`

	// MaxRedirects is the maximum number of redirects for Redis Cluster.
	// Defaults to 3.
	MaxRedirects int `yaml:"maxRedirects" default:"3"`

	// PoolSize is the maximum number of connections in the connection pool.
	// Defaults to 100.
	PoolSize int `yaml:"poolSize" default:"100"`

	// MaxConnectionAge is the maximum lifetime of a connection.
	// Set to 0 for no age limit (default).
	MaxConnectionAge time.Duration `yaml:"maxConnectionAge" default:"0s"`

	// ConnectTimeout is the maximum time to wait for a connection to be established.
	// Defaults to 5 seconds.
	ConnectTimeout time.Duration `yaml:"connectTimeout" default:"5s"`

	// SocketTimeout is the maximum time to wait for socket reads and writes.
	// Defaults to 5 seconds.
	SocketTimeout time.Duration `yaml:"socketTimeout" default:"5s"`

	// IdleTimeout is the maximum time a connection can be idle before being closed.
	// Defaults to 5 seconds.
	IdleTimeout time.Duration `yaml:"idleTimeout" default:"5s"`

	// ReadOnly enables read-only mode for Redis Cluster connections.
	// When true, read commands are routed to replica nodes.
	ReadOnly bool `yaml:"readOnly"`

	// RouteByLatency enables routing to the Redis node with lowest latency.
	// Only applicable for Redis Cluster.
	RouteByLatency bool `yaml:"routeByLatency"`

	// RouteRandomly enables random routing to Redis nodes.
	// Only applicable for Redis Cluster.
	RouteRandomly bool `yaml:"routeRandomly"`
}

// UseConnectionURI reports whether ConnectionURI is set and should be used
// instead of Hosts and individual authentication fields.
func (r *Redis) UseConnectionURI() bool {
	return !r.ConnectionURI.IsEmpty()
}

// DefaultRedis returns a Redis configuration with default values.
func DefaultRedis() Redis {
	return Redis{
		Hosts:              []string{defaultRedisHost},
		Database:           defaultRedisDatabase,
		MinIdleConnections: defaultRedisMinIdleConns,
		MaxRedirects:       defaultRedisMaxRedirects,
		PoolSize:           defaultRedisPoolSize,
		MaxConnectionAge:   defaultRedisMaxConnectionAge,
		ConnectTimeout:     defaultRedisConnectTimeout,
		SocketTimeout:      defaultRedisSocketTimeout,
		IdleTimeout:        defaultRedisIdleTimeout,
	}
}

// Validate performs validation on the Redis configuration.
// It validates hosts, timeout durations, and key prefix settings
// to ensure they meet Redis client requirements.
//
// When ConnectionURI is set, Hosts are not required and authentication fields
// (Username, Password) must not be set (conflict error).
//
// Returns an error if any validation rules fail.
func (r *Redis) Validate() error {
	if err := r.validateConnectionURIConflicts(); err != nil {
		return err
	}

	return ValidateStruct(r,
		validation.Field(&r.Hosts,
			validation.When(!r.UseConnectionURI(),
				validation.Required.Error("connection address must be specified"))),
		validation.Field(&r.ConnectTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&r.SocketTimeout, ozzo_rules.Duration()),
		validation.Field(&r.IdleTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&r.MaxConnectionAge, ozzo_rules.DurationOrZero()),
		validation.Field(&r.KeysPrefix, validation.NilOrNotEmpty),
	)
}

// validateConnectionURIConflicts returns an error if ConnectionURI is set
// together with any individual authentication field.
func (r *Redis) validateConnectionURIConflicts() error {
	if !r.UseConnectionURI() {
		return nil
	}

	var errs []error
	if r.Username != "" {
		errs = append(errs, errors.New("username must not be set when connectionUri is used"))
	}
	if !r.Password.IsEmpty() {
		errs = append(errs, errors.New("password must not be set when connectionUri is used"))
	}

	return errors.Join(errs...)
}
