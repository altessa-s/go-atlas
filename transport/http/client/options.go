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
	"net/netip"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/http/client/limiters"
	"github.com/altessa-s/go-atlas/transport/internal/proxydial"
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
// Dial-level timeouts (Timeout, KeepAlive) live in
// [transport/internal/proxydial] so the pooled transport, the proxy
// dialer, and the gRPC proxy dialer all share one source of truth.
const (
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 90 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultExpectContinueTimeout = 1 * time.Second
)

func defaultPooledTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           proxydial.DefaultDialer().DialContext,
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
	ssrfAllowedCIDRs []netip.Prefix    `optgen:"manual"`
	collector        metrics.Collector `optgen:"notnil"`
	metricsSubsystem string            `optgen:"default=DefaultMetricsSubsystem"`
	// proxy resolves the proxy URL for outgoing requests. The auto-generated
	// setter is named WithProxyFunc to free the WithProxy name for the more
	// common host/port/auth convenience setter declared below.
	proxy ProxyFunc `opt:"ProxyFunc"`
	// proxyTLSConfig customizes the TLS handshake to https:// proxies
	// (e.g. a corporate proxy fronted by a self-signed cert). When set,
	// the client routes outbound requests through a custom DialContext
	// that performs TCP+TLS+CONNECT manually, isolating proxy TLS from
	// the destination's TLSClientConfig. Has no effect for plain http://
	// or socks5:// proxies.
	proxyTLSConfig *tls.Config
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

// ProxyFunc resolves the proxy URL for an outgoing request. It matches the
// signature of [http.Transport.Proxy]: returning (nil, nil) means "no proxy
// for this request"; returning a non-nil error aborts the request before
// dialing.
//
// When [WithSSRFProtection] is also enabled, note that the SSRF check
// runs against the dialed address — which becomes the proxy itself when
// a proxy is configured. Add the proxy host's CIDR to
// [WithSSRFAllowedCIDRs] if it lives on a private network.
type ProxyFunc = func(*http.Request) (*url.URL, error)

// WithProxy routes all requests through an HTTP proxy at host:port with
// optional credentials. Pass nil auth for an anonymous proxy; build auth
// with [url.UserPassword] or [url.User]. The scheme is always http — for
// https/socks5 proxies use [WithProxyURL] instead. Setting this disables
// the default HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment lookup.
//
// Invalid host or port produce a resolver that fails on the first request
// rather than silently falling back to the default proxy. The proxy is
// only applied when the underlying transport is a [*http.Transport] —
// custom round-trippers supplied via [WithClient] are left untouched.
//
// Example:
//
//	httpclient.New(httpclient.WithProxy("proxy.corp", 3128, url.UserPassword("svc", token)))
func WithProxy(host string, port int, auth *url.Userinfo) Option {
	if host == "" {
		return WithProxyFunc(failingProxy(errors.New("httpclient: WithProxy: empty host")))
	}
	if port < 1 || port > 65535 {
		return WithProxyFunc(failingProxy(fmt.Errorf("httpclient: WithProxy: invalid port %d", port)))
	}
	u := &url.URL{
		Scheme: "http",
		User:   auth,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	return WithProxyFunc(http.ProxyURL(u))
}

// WithProxyURL routes all requests through the given proxy URL. Supported
// schemes are http, https, socks5, and socks5h. Credentials embedded in
// the URL (http://user:pass@host:port) trigger Proxy-Authorization
// automatically. Setting this disables the default
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment lookup.
//
// A nil URL produces a resolver that fails on the first request rather
// than silently falling back to the default proxy.
func WithProxyURL(u *url.URL) Option {
	if u == nil {
		return WithProxyFunc(failingProxy(errors.New("httpclient: WithProxyURL: nil URL")))
	}
	return WithProxyFunc(http.ProxyURL(u))
}

// WithoutProxy disables proxy resolution entirely, including the default
// environment-based lookup. Use when the process runs in an environment
// where HTTP_PROXY is set but specific clients must connect directly.
func WithoutProxy() Option {
	return WithProxyFunc(noProxyResolver)
}

// noProxyResolver always reports "no proxy", overriding the default
// environment-based lookup baked into [defaultPooledTransport].
// Returning (nil, nil) is the contract documented on [http.Transport.Proxy]
// — a nil URL means "use no proxy for this request".
//
//nolint:nilnil // (nil, nil) is the http.Transport.Proxy "no proxy" contract.
func noProxyResolver(*http.Request) (*url.URL, error) { return nil, nil }

// failingProxy returns a resolver that always fails with err. Used by
// the convenience proxy options when their inputs are invalid so that
// misconfiguration surfaces at request time instead of being masked by
// the default env-based proxy.
func failingProxy(err error) ProxyFunc {
	return func(*http.Request) (*url.URL, error) { return nil, err }
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
