// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"cmp"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
	corehttp "github.com/altessa-s/go-atlas/core/net/http"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// gobreakerStateToFloat converts a gobreaker state to a float64 for gauge metrics.
// 0=closed, 1=half-open, 2=open.
func gobreakerStateToFloat(s gobreaker.State) float64 {
	//nolint:mnd
	switch s {
	case gobreaker.StateClosed:
		return 0
	case gobreaker.StateHalfOpen:
		return 1
	case gobreaker.StateOpen:
		return 2
	default:
		return 0
	}
}

const (
	// DefaultBreakerTimeout is the default timeout for the circuit breaker to transition from open to half-open state
	DefaultBreakerTimeout = 30 * time.Second
	// DefaultBreakerInterval is the default interval for clearing failure counts in closed state
	DefaultBreakerInterval = 30 * time.Second
	// DefaultBreakerMaxRequests is the default number of requests allowed in half-open state
	DefaultBreakerMaxRequests = 3
	// hostnameInternerSize is the size of the string interner for hostnames.
	hostnameInternerSize = 1024

	// DefaultBreakerMinRequests is the minimum number of requests before evaluating failure ratio.
	// The circuit breaker requires at least this many requests in the closed state
	// before it will consider opening based on failure ratio.
	DefaultBreakerMinRequests = 3

	// DefaultBreakerFailureRatioThreshold is the failure ratio threshold (0.0 to 1.0).
	// When the failure ratio exceeds this threshold and minimum requests are met,
	// the circuit breaker will trip to open state. Default is 0.6 (60%).
	DefaultBreakerFailureRatioThreshold = 0.6

	// DefaultBreakerConsecutiveFailuresLimit is the maximum consecutive failures allowed.
	// When consecutive failures reach this limit, the circuit breaker will trip
	// to open state regardless of the failure ratio.
	DefaultBreakerConsecutiveFailuresLimit = 100

	// DefaultMaxHostBreakers is the maximum number of per-host circuit breakers
	// created lazily for hosts without explicit [CircuitBreakerSettings]. It
	// bounds memory and the cardinality of the breaker-state metric when
	// request URLs come from untrusted input; hosts beyond the cap share the
	// client-wide breaker.
	DefaultMaxHostBreakers = 1024
)

var (
	// circuitBreakerCounter is used to generate unique names for circuit breakers
	circuitBreakerCounter atomic.Uint64
)

type circuitBreakerClient struct {
	*gobreaker.CircuitBreaker[*http.Response]
	client          *http.Client
	logger          *slog.Logger
	metrics         *httpClientMetrics
	maxResponseSize int64
	// hostBreakers maps hostnames to their circuit breakers
	hostBreakers map[string]*gobreaker.CircuitBreaker[*http.Response]
	// hostBreakerSettings contains per-host circuit breaker settings
	hostBreakerSettings map[string]*CircuitBreakerSettings
	// mu protects access to hostBreakers map and lazyBreakers
	mu sync.RWMutex
	// lazyBreakers counts entries of hostBreakers created without explicit settings
	lazyBreakers int
	// maxHostBreakers caps lazyBreakers; see [WithMaxHostBreakers]
	maxHostBreakers int
	// sharedUsed reports whether any request has been routed through the
	// client-wide breaker, so health aggregation can ignore it while idle
	sharedUsed atomic.Bool
	// lazy breakers inherit the client-wide breaker settings
	breakerMaxRequests uint32
	breakerInterval    time.Duration
	breakerTimeout     time.Duration
	// breakerCache provides thread-safe caching for circuit breakers
	breakerCache sync.Map // map[string]*gobreaker.CircuitBreaker[*http.Response]
	// stringInterner provides efficient string interning for hostnames
	stringInterner *corestrings.Interner
	// health is set by [retractableClient] after construction so the global
	// breaker callback (already wired via the closure passed to gobreaker.NewCircuitBreaker)
	// and per-host callbacks (created lazily in [getBreakerForHost]) can both
	// reach the health helper without a circular dependency in construction.
	// atomic.Pointer ensures the load inside callbacks is race-clean.
	health atomic.Pointer[httpClientHealth]
}

// defaultReadyToTrip determines when the circuit breaker should trip to open state.
// It trips when minimum requests are met and either failure ratio exceeds threshold
// or consecutive failures reach the limit.
func defaultReadyToTrip(counts gobreaker.Counts) bool {
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return counts.Requests >= DefaultBreakerMinRequests &&
		(failureRatio >= DefaultBreakerFailureRatioThreshold ||
			counts.ConsecutiveFailures >= DefaultBreakerConsecutiveFailuresLimit)
}

func newCircuitBreakerClient(opts options, m *httpClientMetrics) *circuitBreakerClient {
	httpClient := opts.client
	if httpClient == nil {
		httpClient = defaultPooledClient()
	}

	// Apply transport if provided
	if opts.transport != nil {
		httpClient = &http.Client{
			Transport:     opts.transport,
			CheckRedirect: httpClient.CheckRedirect,
			Jar:           httpClient.Jar,
			Timeout:       httpClient.Timeout,
		}
	}

	// Apply proxy override if configured. Clone the underlying transport
	// before mutating its Proxy field so neither a *http.Transport supplied
	// via WithTransport nor a *http.Client supplied via WithClient is
	// modified in place. A nil Transport is normalized to
	// http.DefaultTransport so callers passing &http.Client{} still see
	// the proxy applied (otherwise net/http would fall back to
	// http.DefaultTransport with http.ProxyFromEnvironment, silently
	// defeating WithoutProxy / WithProxy / WithProxyURL).
	// Custom round-trippers that are not *http.Transport are left alone.
	if opts.proxy != nil {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		if t, ok := transport.(*http.Transport); ok {
			cloned := t.Clone()
			if opts.proxyTLSConfig != nil {
				// Custom DialContext handles TCP+TLS+CONNECT manually
				// so the proxy TLS material stays isolated from the
				// destination's TLSClientConfig. Stdlib's
				// Transport.Proxy must be nil — our dialer resolves
				// the proxy via opts.proxy itself.
				cloned.Proxy = nil
				cloned.DialContext = proxyAwareDialer(opts.proxy, opts.proxyTLSConfig)
			} else {
				cloned.Proxy = opts.proxy
			}
			if httpClient == opts.client {
				httpClient = &http.Client{
					Transport:     cloned,
					CheckRedirect: httpClient.CheckRedirect,
					Jar:           httpClient.Jar,
					Timeout:       httpClient.Timeout,
				}
			} else {
				httpClient.Transport = cloned
			}
		}
	}

	// Apply SSRF protection if enabled. Mirrors the proxy block above:
	// when httpClient still aliases opts.client (caller supplied via
	// WithClient and we did not allocate a fresh one upstream), clone
	// it before mutating Transport so the caller's *http.Client is
	// never modified in place.
	if opts.ssrfProtection {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		if t, ok := transport.(*http.Transport); ok {
			wrapped := newSSRFSafeTransport(t, opts.ssrfAllowedCIDRs)
			if httpClient == opts.client {
				httpClient = &http.Client{
					Transport:     wrapped,
					CheckRedirect: httpClient.CheckRedirect,
					Jar:           httpClient.Jar,
					Timeout:       httpClient.Timeout,
				}
			} else {
				httpClient.Transport = wrapped
			}
		}
	}

	// Generate circuit breaker name
	var cbName string
	if opts.breakerName != "" {
		cbName = opts.breakerName
	} else {
		cbName = fmt.Sprintf("httpclient-cb-%d", circuitBreakerCounter.Add(1))
	}

	client := &circuitBreakerClient{
		client:              httpClient,
		logger:              opts.logger,
		metrics:             m,
		maxResponseSize:     opts.maxResponseSize,
		hostBreakers:        make(map[string]*gobreaker.CircuitBreaker[*http.Response]),
		hostBreakerSettings: opts.hostBreakerSettings,
		maxHostBreakers:     opts.maxHostBreakers,
		breakerMaxRequests:  opts.breakerMaxRequests,
		breakerInterval:     opts.breakerInterval,
		breakerTimeout:      opts.breakerTimeout,

		// Create dedicated string interner for HTTP client with smaller size
		// HTTP client typically has predictable string patterns (hostnames, common headers)
		stringInterner: corestrings.NewInterner(hostnameInternerSize), // Smaller than global default (8192)
	}
	// Wire the global breaker after the struct is allocated so the closure
	// can reach client.health (set later by retractableClient).
	// nolint:bodyclose
	client.CircuitBreaker = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        cbName,
		MaxRequests: opts.breakerMaxRequests,
		Interval:    opts.breakerInterval,
		Timeout:     opts.breakerTimeout,
		ReadyToTrip: defaultReadyToTrip,
		OnStateChange: func(name string, _ gobreaker.State, to gobreaker.State) {
			m.circuitBreakerState.WithLabels(metrics.Labels{"host": name}).Set(gobreakerStateToFloat(to))
			if h := client.health.Load(); h != nil {
				h.notifyAggregate()
			}
		},
	})

	return client
}

// internHostname returns an interned version of the hostname for memory optimization.
// Uses a dedicated string interner for HTTP client to ensure identical hostnames share the same memory location.
// Provides LRU eviction and hot caching specifically for HTTP hostnames.
func (c *circuitBreakerClient) internHostname(hostname string) string {
	return c.stringInterner.String(hostname)
}

// getBreakerForHost returns the circuit breaker for a given hostname.
// Every host gets its own breaker so failures of one endpoint never reject
// requests to another: hosts with explicit [CircuitBreakerSettings] use those
// settings, other hosts get a lazily created breaker that inherits the
// client-wide settings. Once [WithMaxHostBreakers] lazy breakers exist, new
// hosts share the client-wide breaker.
func (c *circuitBreakerClient) getBreakerForHost(hostname string) *gobreaker.CircuitBreaker[*http.Response] {
	// Intern hostname for memory optimization
	hostname = c.internHostname(hostname)

	// Fast path: thread-safe lookup of an already created breaker
	if breaker, ok := c.breakerCache.Load(hostname); ok {
		if cb, ok := breaker.(*gobreaker.CircuitBreaker[*http.Response]); ok {
			return cb
		}
	}

	settings, explicit := c.hostBreakerSettings[hostname]

	c.mu.RLock()
	breaker, exists := c.hostBreakers[hostname]
	full := !explicit && c.lazyBreakers >= c.maxHostBreakers
	c.mu.RUnlock()

	if exists {
		// Cache the existing breaker for future requests
		c.breakerCache.Store(hostname, breaker)
		return breaker
	}
	if full {
		c.sharedUsed.Store(true)
		return c.CircuitBreaker
	}

	// Create new breaker for this host
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check in case another goroutine created it
	breaker, exists = c.hostBreakers[hostname]
	if exists {
		// Cache the breaker created by another goroutine
		c.breakerCache.Store(hostname, breaker)
		return breaker
	}
	if !explicit && c.lazyBreakers >= c.maxHostBreakers {
		c.sharedUsed.Store(true)
		return c.CircuitBreaker
	}

	var st gobreaker.Settings
	var userCb func(name string, from gobreaker.State, to gobreaker.State)
	if explicit {
		// Set defaults if not provided
		st = gobreaker.Settings{
			Name:        settings.Name,
			MaxRequests: cmp.Or(settings.MaxRequests, DefaultBreakerMaxRequests),
			Interval:    cmp.Or(settings.Interval, DefaultBreakerInterval),
			Timeout:     cmp.Or(settings.Timeout, DefaultBreakerTimeout),
			ReadyToTrip: settings.ReadyToTrip,
		}
		if st.Name == "" {
			st.Name = fmt.Sprintf("httpclient-cb-%s-%d", hostname, circuitBreakerCounter.Add(1))
		}
		userCb = settings.OnStateChange
	} else {
		st = gobreaker.Settings{
			Name:        c.Name() + "-" + hostname,
			MaxRequests: c.breakerMaxRequests,
			Interval:    c.breakerInterval,
			Timeout:     c.breakerTimeout,
		}
	}
	if st.ReadyToTrip == nil {
		st.ReadyToTrip = defaultReadyToTrip
	}

	st.OnStateChange = func(n string, from gobreaker.State, to gobreaker.State) {
		if c.metrics != nil {
			c.metrics.circuitBreakerState.WithLabels(metrics.Labels{"host": hostname}).Set(gobreakerStateToFloat(to))
		}
		if h := c.health.Load(); h != nil {
			h.notifyAggregate()
			h.notifyHost(hostname)
		}
		if userCb != nil {
			userCb(n, from, to)
		}
	}

	// nolint:bodyclose
	breaker = gobreaker.NewCircuitBreaker[*http.Response](st)

	c.hostBreakers[hostname] = breaker
	if !explicit {
		c.lazyBreakers++
	}
	// Cache the newly created breaker
	c.breakerCache.Store(hostname, breaker)
	return breaker
}

func (c *circuitBreakerClient) standardClient() *http.Client {
	next := c.client.Transport
	if next == nil {
		next = http.DefaultTransport
	}

	return &http.Client{
		Transport:     c.newCircuitBreakerRoundTripper(next),
		CheckRedirect: c.client.CheckRedirect,
		Jar:           c.client.Jar,
		Timeout:       c.client.Timeout,
	}
}

// idleClosingRoundTripper pairs a wrapping round-tripper with the transport
// it wraps so [http.Client.CloseIdleConnections] reaches the pooled transport.
type idleClosingRoundTripper struct {
	corehttp.RoundTripperFunc
	next http.RoundTripper
}

// CloseIdleConnections forwards to the wrapped transport.
func (rt idleClosingRoundTripper) CloseIdleConnections() { closeIdleConnections(rt.next) }

// closeIdleConnections closes rt's idle connections when it supports it, the
// same contract [http.Client.CloseIdleConnections] applies to its Transport.
func closeIdleConnections(rt http.RoundTripper) {
	if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// newCircuitBreakerRoundTripper creates a new round tripper with host-specific circuit breaker support
func (c *circuitBreakerClient) newCircuitBreakerRoundTripper(next http.RoundTripper) http.RoundTripper {
	return idleClosingRoundTripper{next: next, RoundTripperFunc: func(req *http.Request) (*http.Response, error) {
		// Get the appropriate circuit breaker for this host
		breaker := c.getBreakerForHost(req.URL.Host)

		res, err := breaker.Execute(func() (*http.Response, error) {
			return next.RoundTrip(req) //nolint:bodyclose
		})

		if err != nil {
			return nil, coreerrs.Wrapf(err, "circuit breaker execution failed for %s %s", req.Method, req.URL.String())
		}

		// Apply response size limit if configured
		if c.maxResponseSize > 0 && res.ContentLength > c.maxResponseSize {
			_ = res.Body.Close() //nolint:errcheck
			return nil, &ResponseSizeError{
				Limit: c.maxResponseSize,
				Size:  res.ContentLength,
			}
		}

		// If the content length is unknown, wrap the body with a limiting reader
		if c.maxResponseSize > 0 && res.ContentLength < 0 {
			res.Body = coreio.NewLimitedReadCloser(res.Body, c.maxResponseSize)
		}

		return res, nil
	}}
}
