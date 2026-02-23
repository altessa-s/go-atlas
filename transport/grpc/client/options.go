// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"crypto/tls"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Default configuration values.
const (
	// DefaultMutationTimeout is the default timeout for mutation operations (Create/Update/Delete).
	DefaultMutationTimeout = 30 * time.Second

	// DefaultQueryTimeout is the default timeout for query operations (Get/List).
	DefaultQueryTimeout = 5 * time.Second

	// DefaultMaxRetryAttempts is the default maximum number of retry attempts.
	DefaultMaxRetryAttempts = 5

	// DefaultInitialBackoff is the default initial backoff duration between retries.
	DefaultInitialBackoff = 1 * time.Second

	// DefaultMaxBackoff is the default maximum backoff duration.
	DefaultMaxBackoff = 10 * time.Second

	// DefaultBackoffMultiplier is the default multiplier for exponential backoff.
	DefaultBackoffMultiplier = 2.0

	// MinConnectTimeout is the minimum connection timeout.
	MinConnectTimeout = 10 * time.Second

	// KeepaliveTime is the interval at which keepalive pings are sent.
	KeepaliveTime = 30 * time.Second

	// KeepaliveTimeout is the timeout for keepalive ping acknowledgment.
	KeepaliveTimeout = 10 * time.Second
)

// DefaultRetryableStatusCodes lists the gRPC status code names that trigger an
// automatic retry when retry is enabled via [WithRetry] or [WithRetryConfig].
var DefaultRetryableStatusCodes = []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "RESOURCE_EXHAUSTED"}

// ErrorConverter transforms a gRPC [status.Status] into a domain-specific error.
// Set one via [WithErrorConverter] to override the default [ParseStatusError] behavior
// in the client's error-status interceptor.
type ErrorConverter func(ctx context.Context, st *status.Status) error

// RetryConfig controls the gRPC service-config-level retry policy injected
// into every connection. The policy uses exponential backoff and limits
// retries to the status codes listed in RetryableStatusCodes.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (including initial call).
	MaxAttempts int

	// InitialBackoff is the initial backoff duration between retries.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64

	// RetryableStatusCodes are the gRPC status codes that should trigger a retry.
	RetryableStatusCodes []string
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithTLSConfig sets a custom TLS configuration for the gRPC connection.
// Use this to configure custom certificates, cipher suites, or other TLS settings.
//
// Example:
//
//	tlsConfig := &tls.Config{
//		MinVersion: tls.VersionTLS13,
//		ServerName: "service.example.com",
//	}
//	client, err := client.New("localhost:8080",
//		client.WithTLSConfig(tlsConfig),
//	)
func WithTLSConfig(t *tls.Config) Option {
	return func(c *Client) {
		c.tlsConfig = t
	}
}

// WithInsecure disables TLS and uses an insecure plaintext connection.
// This should only be used for local development or testing.
// Never use this in production environments.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithInsecure(),
//	)
func WithInsecure() Option {
	return func(c *Client) {
		c.tlsConfig = nil
		c.insecure = true
	}
}

// WithLogger sets a custom logger for the client.
// The logger will be used to log gRPC call details, connection events, and errors.
// If not set, a discard logger is used (no logging).
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	client, err := client.New("localhost:8080",
//		client.WithLogger(logger),
//	)
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithAppName sets the application name used in the gRPC User-Agent header.
// This helps identify the client application in server logs.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithAppName("my-service/v1.0.0"),
//	)
func WithAppName(appName string) Option {
	return func(c *Client) {
		c.appName = appName
	}
}

// WithConnectionPool enables connection pooling for the client.
// When enabled, the client will use a connection pool instead of a single persistent connection.
// This is recommended for high-throughput scenarios with many concurrent requests.
//
// Example:
//
//	p := pool.New(
//		pool.WithPoolSize(20),
//		pool.WithMaxIdleTime(time.Hour),
//		pool.WithLogger(logger),
//	)
//	client, err := client.New("localhost:8080",
//		client.WithConnectionPool(p),
//		client.WithLogger(logger),
//	)
func WithConnectionPool(p *pool.ConnectionPool) Option {
	return func(c *Client) {
		c.pool = p
	}
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:          DefaultMaxRetryAttempts,
		InitialBackoff:       DefaultInitialBackoff,
		MaxBackoff:           DefaultMaxBackoff,
		BackoffMultiplier:    DefaultBackoffMultiplier,
		RetryableStatusCodes: DefaultRetryableStatusCodes,
	}
}

// WithRetry enables automatic retry with the default configuration.
// Uses exponential backoff and retries on UNAVAILABLE, DEADLINE_EXCEEDED, and RESOURCE_EXHAUSTED.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithRetry(),
//		client.WithInsecure(),
//	)
func WithRetry() Option {
	return func(c *Client) {
		c.enableRetry = true
		c.retryConfig = DefaultRetryConfig()
	}
}

// WithRetryConfig enables automatic retry with custom configuration.
// Note: gRPC service config requires durations to be in seconds format.
//
// Example:
//
//	retryConfig := &client.RetryConfig{
//		MaxAttempts:          5,
//		InitialBackoff:       1 * time.Second,
//		MaxBackoff:           5 * time.Second,
//		BackoffMultiplier:    1.5,
//		RetryableStatusCodes: []string{"UNAVAILABLE", "INTERNAL"},
//	}
//
//	c, err := client.New("localhost:8080",
//		client.WithRetryConfig(retryConfig),
//		client.WithInsecure(),
//	)
func WithRetryConfig(config *RetryConfig) Option {
	return func(c *Client) {
		c.enableRetry = true
		c.retryConfig = config
	}
}

// WithMutationTimeout sets the default timeout for mutation operations (Create/Update/Delete).
// If not set, the default is 30 seconds.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithMutationTimeout(60*time.Second),
//		client.WithInsecure(),
//	)
func WithMutationTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.mutationTimeout = timeout
	}
}

// WithQueryTimeout sets the default timeout for query operations (Get/List).
// If not set, the default is 5 seconds.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithQueryTimeout(10*time.Second),
//		client.WithInsecure(),
//	)
func WithQueryTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.queryTimeout = timeout
	}
}

// WithDialOptions adds custom gRPC dial options to the client.
// These options are appended to the default options.
//
// Example:
//
//	client, err := client.New("localhost:8080",
//		client.WithDialOptions(
//			grpc.WithBlock(),
//			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(10*1024*1024)),
//		),
//	)
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(c *Client) {
		c.extraDialOptions = append(c.extraDialOptions, opts...)
	}
}

// WithErrorConverter sets a custom error converter function.
// This allows service-specific clients to provide custom error parsing logic.
//
// Example:
//
//	converter := func(ctx context.Context, st *status.Status) error {
//		return MyCustomError{Code: st.Code(), Message: st.Message()}
//	}
//	client, err := client.New("localhost:8080",
//		client.WithErrorConverter(converter),
//	)
func WithErrorConverter(converter ErrorConverter) Option {
	return func(c *Client) {
		c.errorConverter = converter
	}
}
