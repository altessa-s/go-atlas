// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

// ProxyFunc resolves the proxy URL for an outgoing connection. It
// matches the signature of [http.Transport.Proxy] and the
// [transport/http/client.ProxyFunc] alias, so resolvers are
// interchangeable between the two clients.
//
// Returning (nil, nil) means "no proxy for this connection";
// returning a non-nil error aborts the dial.
//
// When [WithProxyTLSConfig] is also enabled, note that the dialed
// address becomes the proxy itself when one is configured — set the
// proxy host's CIDR allowlist accordingly if it sits behind a private
// network filter.
type ProxyFunc = func(*http.Request) (*url.URL, error)

// options holds the internal configuration state for the gRPC client.
// This struct is not exported and is modified through Option functions.
type options struct {
	tlsConfig        *tls.Config  `optgen:"default=defaultSecureTLSConfig()"`
	insecure         bool         `optgen:"manual"`
	logger           *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler)"`
	appName          string
	pool             *pool.ConnectionPool
	enableRetry      bool              `optgen:"manual"`
	retryConfig      *RetryConfig      `optgen:"manual"`
	mutationTimeout  time.Duration     `optgen:"default=DefaultMutationTimeout"`
	queryTimeout     time.Duration     `optgen:"default=DefaultQueryTimeout"`
	extraDialOptions []grpc.DialOption `opt:"DialOptions" optgen:"append"`
	errorConverter   ErrorConverter
	// proxy resolves the proxy URL for outgoing connections. The auto-generated
	// setter is named WithProxyFunc to free the WithProxy name for the more
	// common host/port/auth convenience setter declared below.
	proxy ProxyFunc `opt:"ProxyFunc"`
	// proxyTLSConfig customizes the TLS handshake to https:// proxies
	// (e.g. a corporate proxy fronted by a self-signed cert). Has no
	// effect for plain http:// or socks5:// proxies, which never
	// TLS-wrap the proxy connection.
	proxyTLSConfig *tls.Config
}

// WithInsecure disables TLS and uses an insecure plaintext connection.
// This should only be used for local development or testing.
// Never use this in production environments.
//
// Example:
//
//	client, err := client.New(ctx, "localhost:8080",
//		client.WithInsecure(),
//	)
func WithInsecure() Option {
	return func(o *options) {
		o.tlsConfig = nil
		o.insecure = true
	}
}

// WithRetry enables automatic retry with the default configuration.
// Uses exponential backoff and retries on UNAVAILABLE, DEADLINE_EXCEEDED, and RESOURCE_EXHAUSTED.
//
// Example:
//
//	client, err := client.New(ctx, "localhost:8080",
//		client.WithRetry(),
//		client.WithInsecure(),
//	)
func WithRetry() Option {
	return func(o *options) {
		o.enableRetry = true
		o.retryConfig = DefaultRetryConfig()
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
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithRetryConfig(retryConfig),
//		client.WithInsecure(),
//	)
func WithRetryConfig(config *RetryConfig) Option {
	return func(o *options) {
		if config == nil {
			return
		}
		o.enableRetry = true
		o.retryConfig = config
	}
}

// WithProxy routes connections through an HTTP proxy at host:port with
// optional credentials. Pass nil auth for an anonymous proxy; build auth
// with [url.UserPassword] or [url.User]. The scheme is always http —
// for https/socks5 proxies use [WithProxyURL]. Setting this overrides
// grpc-go's default HTTPS_PROXY/HTTP_PROXY/NO_PROXY environment lookup.
//
// Invalid host or port produce a resolver that fails on the first dial
// rather than silently falling back to the env-based default.
//
// Example:
//
//	c, err := client.New(ctx, "service:443",
//	    client.WithProxy("proxy.corp", 3128, url.UserPassword("svc", token)),
//	)
func WithProxy(host string, port int, auth *url.Userinfo) Option {
	if host == "" {
		return WithProxyFunc(failingProxy(errors.New("grpcclient: WithProxy: empty host")))
	}
	if port < 1 || port > 65535 {
		return WithProxyFunc(failingProxy(fmt.Errorf("grpcclient: WithProxy: invalid port %d", port)))
	}
	u := &url.URL{
		Scheme: "http",
		User:   auth,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	return WithProxyFunc(http.ProxyURL(u))
}

// WithProxyURL routes connections through the given proxy URL. Supported
// schemes are http, https, socks5, and socks5h. Credentials embedded
// in the URL (http://user:pass@proxy:port) trigger Proxy-Authorization
// automatically. Setting this overrides grpc-go's default
// HTTPS_PROXY/HTTP_PROXY/NO_PROXY environment lookup.
//
// A nil URL produces a resolver that fails on the first dial rather
// than silently falling back to the env-based default.
func WithProxyURL(u *url.URL) Option {
	if u == nil {
		return WithProxyFunc(failingProxy(errors.New("grpcclient: WithProxyURL: nil URL")))
	}
	return WithProxyFunc(http.ProxyURL(u))
}

// WithoutProxy disables proxy resolution entirely, including grpc-go's
// default HTTPS_PROXY/HTTP_PROXY/NO_PROXY environment lookup. Use when
// the process runs in an environment where HTTPS_PROXY is set but a
// specific gRPC client must connect directly.
func WithoutProxy() Option {
	return WithProxyFunc(noProxyResolver)
}

// noProxyResolver always reports "no proxy", overriding grpc-go's
// env-based lookup. Returning (nil, nil) is the contract documented
// on [http.Transport.Proxy].
//
//nolint:nilnil // (nil, nil) is the http.Transport.Proxy "no proxy" contract.
func noProxyResolver(*http.Request) (*url.URL, error) { return nil, nil }

// failingProxy returns a resolver that always fails with err. Used by
// the convenience proxy options when their inputs are invalid so that
// misconfiguration surfaces at first dial instead of being masked by
// the default env-based proxy.
func failingProxy(err error) ProxyFunc {
	return func(*http.Request) (*url.URL, error) { return nil, err }
}
