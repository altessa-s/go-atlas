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
	// mu protects access to hostBreakers map
	mu sync.RWMutex
	// breakerCache provides thread-safe caching for circuit breakers
	breakerCache sync.Map // map[string]*gobreaker.CircuitBreaker[*http.Response]
	// stringInterner provides efficient string interning for hostnames
	stringInterner *corestrings.Interner
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

	// Apply SSRF protection if enabled
	if opts.ssrfProtection {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		if t, ok := transport.(*http.Transport); ok {
			httpClient.Transport = newSSRFSafeTransport(t, opts.ssrfAllowedCIDRs)
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
		// nolint:bodyclose
		CircuitBreaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
			Name:        cbName,
			MaxRequests: opts.breakerMaxRequests,
			Interval:    opts.breakerInterval,
			Timeout:     opts.breakerTimeout,
			ReadyToTrip: defaultReadyToTrip,
			OnStateChange: func(name string, _ gobreaker.State, to gobreaker.State) {
				m.circuitBreakerState.WithLabels(metrics.Labels{"host": name}).Set(gobreakerStateToFloat(to))
			},
		}),
		client:              httpClient,
		logger:              opts.logger,
		metrics:             m,
		maxResponseSize:     opts.maxResponseSize,
		hostBreakers:        make(map[string]*gobreaker.CircuitBreaker[*http.Response]),
		hostBreakerSettings: opts.hostBreakerSettings,

		// Create dedicated string interner for HTTP client with smaller size
		// HTTP client typically has predictable string patterns (hostnames, common headers)
		stringInterner: corestrings.NewInterner(hostnameInternerSize), // Smaller than global default (8192)
	}

	return client
}

// internHostname returns an interned version of the hostname for memory optimization.
// Uses a dedicated string interner for HTTP client to ensure identical hostnames share the same memory location.
// Provides LRU eviction and hot caching specifically for HTTP hostnames.
func (c *circuitBreakerClient) internHostname(hostname string) string {
	return c.stringInterner.String(hostname)
}

// getBreakerForHost returns the circuit breaker for a given hostname.
// Uses sync.Map for thread-safe caching and falls back to the original map-based approach with locking.
// If no specific breaker exists for the host, it returns the default breaker.
func (c *circuitBreakerClient) getBreakerForHost(hostname string) *gobreaker.CircuitBreaker[*http.Response] {
	// Intern hostname for memory optimization
	hostname = c.internHostname(hostname)

	// Check if we have host-specific settings with single lock
	c.mu.RLock()
	if c.hostBreakerSettings == nil {
		c.mu.RUnlock()
		return c.CircuitBreaker
	}
	settings, hasSettings := c.hostBreakerSettings[hostname]
	c.mu.RUnlock()
	if !hasSettings {
		return c.CircuitBreaker
	}

	// Check sync.Map cache for thread-safe lookup
	if breaker, ok := c.breakerCache.Load(hostname); ok {
		if cb, ok := breaker.(*gobreaker.CircuitBreaker[*http.Response]); ok {
			return cb
		}
	}

	// Slow path: Create new breaker with locking (original implementation)
	c.mu.RLock()
	breaker, exists := c.hostBreakers[hostname]
	c.mu.RUnlock()

	if exists {
		// Cache the existing breaker for future requests
		c.breakerCache.Store(hostname, breaker)
		return breaker
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

	// Set defaults if not provided
	maxRequests := cmp.Or(settings.MaxRequests, DefaultBreakerMaxRequests)
	interval := cmp.Or(settings.Interval, DefaultBreakerInterval)
	timeout := cmp.Or(settings.Timeout, DefaultBreakerTimeout)

	name := settings.Name
	if name == "" {
		name = fmt.Sprintf("httpclient-cb-%s-%d", hostname, circuitBreakerCounter.Add(1))
	}

	readyToTrip := settings.ReadyToTrip
	if readyToTrip == nil {
		readyToTrip = defaultReadyToTrip
	}

	onStateChange := settings.OnStateChange
	if c.metrics != nil {
		userCb := onStateChange
		onStateChange = func(n string, from gobreaker.State, to gobreaker.State) {
			c.metrics.circuitBreakerState.WithLabels(metrics.Labels{"host": hostname}).Set(gobreakerStateToFloat(to))
			if userCb != nil {
				userCb(n, from, to)
			}
		}
	}

	// nolint:bodyclose
	breaker = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:          name,
		MaxRequests:   maxRequests,
		Interval:      interval,
		Timeout:       timeout,
		ReadyToTrip:   readyToTrip,
		OnStateChange: onStateChange,
	})

	c.hostBreakers[hostname] = breaker
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

// newCircuitBreakerRoundTripper creates a new round tripper with host-specific circuit breaker support
func (c *circuitBreakerClient) newCircuitBreakerRoundTripper(next http.RoundTripper) corehttp.RoundTripperFunc {
	return func(req *http.Request) (*http.Response, error) {
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
	}
}
