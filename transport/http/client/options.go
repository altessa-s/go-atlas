// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"
)

func defaultClient() *http.Client {
	return defaultPooledClient()
}

// defaultPooledClient returns a new [http.Client] with a pooled transport
// configured identically to go-cleanhttp's DefaultPooledClient: connection
// pooling, keep-alive, TLS handshake timeout, HTTP/2, and per-host idle
// connections scaled to GOMAXPROCS.
func defaultPooledClient() *http.Client {
	return &http.Client{
		Transport: defaultPooledTransport(),
	}
}

// Transport pool defaults matching go-cleanhttp's DefaultPooledClient.
const (
	defaultDialTimeout           = 30 * time.Second
	defaultDialKeepAlive         = 30 * time.Second
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 90 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultExpectContinueTimeout = 1 * time.Second
)

func defaultPooledTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultDialTimeout,
			KeepAlive: defaultDialKeepAlive,
		}).DialContext,
		MaxIdleConns:          defaultMaxIdleConns,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ExpectContinueTimeout: defaultExpectContinueTimeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
	}
}

// CircuitBreakerSettings defines per-host circuit breaker configuration.
// Supply it via [WithCircuitBreakerSettings] to override the global breaker
// defaults for a specific hostname. Zero-value fields fall back to the
// package-level defaults ([DefaultBreakerMaxRequests], [DefaultBreakerInterval],
// [DefaultBreakerTimeout]). Use [DefaultOnStateChange] for a ready-made
// state-transition logging callback.
type CircuitBreakerSettings struct {
	// MaxRequests is the maximum number of requests allowed in half-open state
	MaxRequests uint32
	// Interval is the period to clear failure counts in closed state
	Interval time.Duration
	// Timeout is the period after which circuit breaker transitions from open to half-open
	Timeout time.Duration
	// Name is the circuit breaker name for identification
	Name string
	// ReadyToTrip is the function to determine when the circuit breaker should trip
	ReadyToTrip func(counts gobreaker.Counts) bool
	// OnStateChange is called when the circuit breaker state changes
	OnStateChange func(name string, from gobreaker.State, to gobreaker.State)
}

// options holds the internal configuration state for the HTTP client.
// This struct is not exported and is modified through Option functions.
type options struct {
	client             *http.Client `optgen:"default=defaultClient()"`
	transport          *http.Transport
	retryMax           int           `optgen:"default=DefaultRetryMax"`
	breakerMaxRequests uint32        `optgen:"default=DefaultBreakerMaxRequests"`
	retryWaitMin       time.Duration `optgen:"default=DefaultRetryWaitMin"`
	retryWaitMax       time.Duration `optgen:"default=DefaultRetryWaitMax"`
	breakerInterval    time.Duration `optgen:"default=DefaultBreakerInterval"`
	breakerTimeout     time.Duration `optgen:"default=DefaultBreakerTimeout"`
	breakerName        string
	maxResponseSize    int64
	errorHandler       ErrorHandler
	retryPolicyHandler RetryPolicyHandler
	logger             *slog.Logger
	limiter            limiters.RequestsLimiter `optgen:"notnil"`
	// hostBreakerSettings maps hostnames to their circuit breaker settings
	hostBreakerSettings map[string]*CircuitBreakerSettings `opt:"-"`
	// ssrfProtection enables blocking connections to private/local IP addresses
	ssrfProtection bool `optgen:"manual"`
	// ssrfAllowedCIDRs contains CIDR prefixes exempted from SSRF blocking
	ssrfAllowedCIDRs []netip.Prefix `optgen:"manual"`
}

// WithRetryWait configures the minimum and maximum wait times between retry attempts.
// The actual wait time uses exponential backoff with jitter between these bounds.
// Defaults are 500ms minimum and 10s maximum.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithRetryWait(1*time.Second, 30*time.Second),
//	)
func WithRetryWait(minDur, maxDur time.Duration) Option {
	return func(opts *options) {
		opts.retryWaitMin = minDur
		opts.retryWaitMax = maxDur
	}
}

// WithCircuitBreakerSettings configures circuit breaker settings for a specific hostname.
// This allows different hosts to have different circuit breaker configurations.
// The hostname should match the host part of the URL (e.g., "api.example.com").
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithCircuitBreakerSettings("api.example.com", &httpclient.CircuitBreakerSettings{
//	        MaxRequests: 5,
//	        Interval:    60 * time.Second,
//	        Timeout:     30 * time.Second,
//	        Name:        "api-example-cb",
//	    }),
//	    httpclient.WithCircuitBreakerSettings("backup.example.com", &httpclient.CircuitBreakerSettings{
//	        MaxRequests: 3,
//	        Interval:    30 * time.Second,
//	        Timeout:     15 * time.Second,
//	        Name:        "backup-example-cb",
//	    }),
//	)
func WithCircuitBreakerSettings(hostname string, settings *CircuitBreakerSettings) Option {
	return func(opts *options) {
		if opts.hostBreakerSettings == nil {
			opts.hostBreakerSettings = make(map[string]*CircuitBreakerSettings)
		}
		opts.hostBreakerSettings[hostname] = settings
	}
}

// WithSSRFProtection enables SSRF protection that blocks connections to
// private and local IP addresses (RFC1918, loopback, link-local, etc.).
// The check runs after DNS resolution but before the TCP connection is established,
// preventing both direct private-IP requests and DNS rebinding attacks.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithSSRFProtection(),
//	)
func WithSSRFProtection() Option {
	return func(opts *options) {
		opts.ssrfProtection = true
	}
}

// WithSSRFAllowedCIDRs sets CIDR prefixes that are exempted from SSRF blocking.
// Use this for known internal service-to-service communication that must reach
// private addresses. Has no effect unless WithSSRFProtection is also set.
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithSSRFProtection(),
//	    httpclient.WithSSRFAllowedCIDRs(
//	        netip.MustParsePrefix("10.0.1.0/24"), // internal service network
//	    ),
//	)
func WithSSRFAllowedCIDRs(cidrs ...netip.Prefix) Option {
	return func(opts *options) {
		opts.ssrfAllowedCIDRs = append(opts.ssrfAllowedCIDRs, cidrs...)
	}
}

// DefaultOnStateChange returns a [gobreaker.Settings.OnStateChange] callback that
// logs circuit breaker state transitions at debug level using the provided logger.
// Pass this to [CircuitBreakerSettings.OnStateChange] to get visibility into breaker
// behavior without writing a custom callback.
func DefaultOnStateChange(logger *slog.Logger) func(name string, from, to gobreaker.State) {
	return func(name string, from, to gobreaker.State) {
		if logger != nil {
			//nolint:contextcheck // callback from gobreaker library has no request context
			logger.DebugContext(context.Background(),
				fmt.Sprintf("circuit [%s] change state from %s to %s", name, from, to))
		}
	}
}
