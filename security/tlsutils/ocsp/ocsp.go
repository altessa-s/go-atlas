// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/security/tlsutils"

	"golang.org/x/crypto/ocsp"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

const (
	// DefaultHTTPTimeout is the default HTTP client timeout for OCSP requests.
	DefaultHTTPTimeout = 10 * time.Second
	// DefaultOCSPExpiry is the default expiry time for OCSP responses when NextUpdate is not set.
	DefaultOCSPExpiry = 24 * time.Hour
	// defaultCleanupInterval determines how often lazy cleanup runs (every N GetOCSPStaple calls).
	defaultCleanupInterval = 100
	// handshakeOCSPTimeout is the maximum time for OCSP operations during TLS handshake.
	handshakeOCSPTimeout = 5 * time.Second
	// refreshBuffer is the time before nextUpdate when refresh is considered needed.
	// OCSP responses are refreshed when less than this duration remains until expiration.
	refreshBuffer = time.Hour
	// maxOCSPResponseSize caps how many bytes are read from an OCSP responder
	// (including after transparent gzip decompression). DER-encoded OCSP responses are a few KB;
	// 1 MiB is far above any legitimate response.
	maxOCSPResponseSize = 1 << 20 // 1 MiB
	// ocspClockSkew is the tolerance applied to a response's thisUpdate and
	// nextUpdate when checking that it is currently valid.
	ocspClockSkew = 5 * time.Minute
)

// ErrStaleResponse indicates an OCSP response outside its validity window: its
// nextUpdate has passed or its thisUpdate is in the future.
var ErrStaleResponse = errors.New("ocsp: response is not within its validity window")

var _ tlsutils.OCSPStapler = (*Stapler)(nil)

// Stapler manages OCSP stapling for TLS certificates.
// It provides automatic caching and refresh of OCSP responses.
// All methods are safe for concurrent use.
//
// For automatic refresh, use WithScheduler and WithRefreshSchedule options:
//
//	stapler := ocsp.NewOCSPStapler(
//	    ocsp.WithScheduler(sched),
//	    ocsp.WithRefreshSchedule("0 */30 * * * *"), // every 30 minutes
//	    ocsp.WithLogger(logger),
//	)
//
// Expired cache entries are cleaned up lazily during GetOCSPStaple calls.
type Stapler struct {
	mu             sync.RWMutex
	cache          map[string]*ocspCacheEntry
	httpClient     *http.Client
	retryPolicy    RetryPolicy
	logger         *slog.Logger
	cleanupCounter atomic.Uint64 // atomic counter for lazy cleanup
	scheduler      corescheduler.TaskRegistrar
	refreshAllTask corescheduler.ManagedTask // Guards RunRefreshAll and marks scheduler management.

	enableCompression bool
	// failureMode controls handshake behavior when GetOCSPStaple cannot
	// produce a valid response — see [FailureMode] for the rationale.
	failureMode FailureMode
	// maxCacheEntries bounds the OCSP response cache. When zero, the
	// cache is unbounded (legacy behavior; opt-out for callers who
	// know their cert population is small and want to skip eviction
	// bookkeeping).
	maxCacheEntries int
}

// FailureMode returns the configured [FailureMode]. [StapleOCSPToConfig]
// uses this to decide whether a failed staple-fetch should be served
// as a cert-without-staple (Soft) or rejected (Hard).
func (s *Stapler) FailureMode() FailureMode {
	if s.failureMode == "" {
		return DefaultFailureMode
	}
	return s.failureMode
}

// ocspCacheEntry holds the raw DER response. It is not compressed: signed DER
// OCSP responses are effectively incompressible, so gzip would save no memory
// while costing a decode on every handshake.
type ocspCacheEntry struct {
	response   []byte
	nextUpdate time.Time
	cert       *tls.Certificate
	mu         sync.RWMutex
}

// NewOCSPStapler creates a new OCSP stapler with the specified options.
// By default, compression is disabled and no retry policy is configured.
//
// Example:
//
//	stapler := ocsp.NewOCSPStapler(
//	    ocsp.WithScheduler(sched),
//	    ocsp.WithRefreshSchedule("0 */30 * * * *"),
//	    ocsp.WithLogger(slog.Default()),
//	)
func NewOCSPStapler(opts ...Option) *Stapler {
	o := newOptions(opts...)

	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultHTTPTimeout}
	}

	// Ensure logger is never nil
	logger := o.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	failureMode := o.failureMode
	if failureMode == "" {
		failureMode = DefaultFailureMode
	}

	s := &Stapler{
		cache:             make(map[string]*ocspCacheEntry),
		httpClient:        httpClient,
		retryPolicy:       o.retryPolicy,
		logger:            logger,
		enableCompression: o.enableCompression,
		failureMode:       failureMode,
		maxCacheEntries:   o.maxCacheEntries,
		scheduler:         o.scheduler,
	}

	// Register refresh task with scheduler if configured
	if err := s.registerSchedulerTask(o); err != nil {
		logger.Warn("failed to register OCSP refresh task", slog.Any("error", err))
	}

	return s
}

// GetOCSPStaple returns the OCSP staple for the given certificate.
// It checks the cache first and fetches a new response if necessary.
// The context controls the HTTP request timeout and cancellation.
//
// Example:
//
//	staple, err := stapler.GetOCSPStaple(ctx, &cert)
//	if err == nil {
//		cert.OCSPStaple = staple
//	}
func (s *Stapler) GetOCSPStaple(ctx context.Context, cert *tls.Certificate) ([]byte, error) {
	if cert == nil || len(cert.Certificate) == 0 {
		return nil, errors.New("invalid certificate")
	}

	// Lazy cleanup of expired entries every N calls
	if s.cleanupCounter.Add(1)%defaultCleanupInterval == 0 {
		s.removeExpiredEntries()
	}

	// Parse the certificate if not already parsed
	var leafCert *x509.Certificate
	if cert.Leaf != nil {
		leafCert = cert.Leaf
	} else {
		var err error
		leafCert, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "parse certificate")
		}
	}

	cacheKey := base64.StdEncoding.EncodeToString(cert.Certificate[0])

	s.mu.RLock()
	entry, exists := s.cache[cacheKey]
	s.mu.RUnlock()

	if exists {
		// Snapshot the entry under a read lock, then release it immediately so
		// the read lock is not held across the network fetch below (which would
		// serialize RunRefreshCycle behind handshake traffic).
		entry.mu.RLock()
		valid := time.Now().Before(entry.nextUpdate)
		response := entry.response
		entry.mu.RUnlock()

		// Return cached response if still valid
		if valid {
			return response, nil
		}
	}

	// Fetch new OCSP response
	response, nextUpdate, err := s.fetchOCSPResponse(ctx, cert, leafCert)
	if err != nil {
		return nil, err
	}

	cacheEntry := &ocspCacheEntry{response: response, nextUpdate: nextUpdate}

	s.mu.Lock()
	// Enforce the cap before inserting. evictOldestLocked picks the
	// entry whose nextUpdate is closest to "now" — expired entries go
	// first, then the soonest-to-expire — so the eviction does
	// minimum damage to the cache's hit rate.
	if s.maxCacheEntries > 0 {
		for len(s.cache) >= s.maxCacheEntries {
			if !s.evictOldestLocked() {
				break // pathological: no entries available to evict
			}
		}
	}
	s.cache[cacheKey] = cacheEntry
	cacheEntry.cert = cert
	s.mu.Unlock()

	return response, nil
}

// evictOldestLocked removes the cache entry with the earliest nextUpdate.
// Caller must hold s.mu in write mode. Returns true when an entry was
// removed, false when the cache is already empty.
func (s *Stapler) evictOldestLocked() bool {
	var (
		oldestKey  string
		oldestTime time.Time
		first      = true
	)
	for key, entry := range s.cache {
		entry.mu.RLock()
		t := entry.nextUpdate
		entry.mu.RUnlock()
		if first || t.Before(oldestTime) {
			oldestKey = key
			oldestTime = t
			first = false
		}
	}
	if first {
		return false
	}
	delete(s.cache, oldestKey)
	return true
}

// fetchOCSPResponse fetches a fresh OCSP response for the certificate.
// It tries each OCSP server URL in the certificate with retry policy if configured.
// Returns the OCSP response bytes and the next update time.
func (s *Stapler) fetchOCSPResponse(ctx context.Context, cert *tls.Certificate, leafCert *x509.Certificate) ([]byte, time.Time, error) {
	if len(leafCert.OCSPServer) == 0 {
		return nil, time.Time{}, errors.New("no OCSP server specified in certificate")
	}

	// Find issuer certificate
	var issuerCert *x509.Certificate
	if len(cert.Certificate) > 1 {
		var err error
		issuerCert, err = x509.ParseCertificate(cert.Certificate[1])
		if err != nil {
			return nil, time.Time{}, coreerrs.WrapOperation(err, "parse issuer certificate")
		}
	} else {
		return nil, time.Time{}, errors.New("issuer certificate not found in chain")
	}

	// Create OCSP request
	ocspReq, err := ocsp.CreateRequest(leafCert, issuerCert, nil)
	if err != nil {
		return nil, time.Time{}, coreerrs.WrapOperation(err, "create OCSP request")
	}

	var ocspResp []byte
	var parsedResp *ocsp.Response

	// Try each OCSP server with retry policy
	for i, ocspURL := range leafCert.OCSPServer {
		// Check context before trying next server
		select {
		case <-ctx.Done():
			return nil, time.Time{}, ctx.Err()
		default:
		}

		s.logger.DebugContext(ctx, "trying OCSP server",
			slog.Int("index", i+1),
			slog.Int("total", len(leafCert.OCSPServer)),
			slog.String("url", ocspURL))

		var retryErr error

		requestFunc := func() error {
			httpReq, err := http.NewRequestWithContext(ctx, "POST", ocspURL, bytes.NewReader(ocspReq))
			if err != nil {
				return coreerrs.WrapOperation(err, "create HTTP request")
			}
			httpReq.Header.Set("Content-Type", "application/ocsp-request")

			// Request gzip compression if enabled
			if s.enableCompression {
				httpReq.Header.Set("Accept-Encoding", "gzip")
			}

			resp, err := s.httpClient.Do(httpReq)
			if err != nil {
				return coreerrs.WrapOperation(err, "execute OCSP request")
			}
			defer resp.Body.Close() //nolint:errcheck

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("OCSP server returned status %d", resp.StatusCode)
			}

			var responseBody io.Reader = resp.Body

			// Handle compressed response
			if s.enableCompression {
				contentEncoding := resp.Header.Get("Content-Encoding")
				if strings.Contains(contentEncoding, "gzip") {
					gzipReader, gzipErr := gzip.NewReader(resp.Body)
					if gzipErr != nil {
						return coreerrs.WrapOperation(gzipErr, "create gzip reader for OCSP response")
					}
					defer gzipReader.Close() //nolint:errcheck
					responseBody = gzipReader

					s.logger.DebugContext(ctx, "received gzip-compressed OCSP response")
				}
			}

			ocspResp, err = io.ReadAll(io.LimitReader(responseBody, maxOCSPResponseSize+1))
			if err != nil {
				return coreerrs.WrapOperation(err, "read OCSP response")
			}
			if len(ocspResp) > maxOCSPResponseSize {
				return fmt.Errorf("OCSP response exceeds %d bytes", maxOCSPResponseSize)
			}

			// ParseResponseForCert also binds the response to the leaf's serial:
			// a CA-signed response for a sibling certificate must not be stapled.
			parsedResp, err = ocsp.ParseResponseForCert(ocspResp, leafCert, issuerCert)
			if err != nil {
				return coreerrs.WrapOperation(err, "parse OCSP response")
			}

			return nil
		}

		if s.retryPolicy != nil {
			// Use retry policy if available
			retryErr = ExecuteWithRetry(requestFunc, &RetryConfig{
				Policy:  s.retryPolicy,
				Context: ctx,
				OnRetry: func(attempt int, err error, nextDelay time.Duration) {
					s.logger.ErrorContext(ctx, "ocsp request failed, retrying",
						slog.Int("attempt", attempt+1),
						slog.Duration("next_delay", nextDelay),
						slog.Any("error", err))
				},
			})
		} else {
			// Single attempt without retry
			retryErr = requestFunc()
		}

		if retryErr == nil {
			// Success with this OCSP server
			break
		}

		// Log failure and try next server
		s.logger.ErrorContext(ctx, "failed to get OCSP response",
			slog.String("url", ocspURL),
			slog.Any("error", retryErr))
		if i == len(leafCert.OCSPServer)-1 {
			// Last server failed
			return nil, time.Time{}, coreerrs.WrapOperation(retryErr, "fetch from all OCSP servers")
		}
	}

	// Check certificate status
	if parsedResp.Status != ocsp.Good {
		return nil, time.Time{}, fmt.Errorf("certificate status is not good: %v", parsedResp.Status)
	}

	// The parser verifies the signature but not freshness: a replayed or
	// not-yet-valid "good" response must not be stapled (clients reject it, and
	// in Hard mode it would mask a revocation that happened after NextUpdate).
	now := time.Now()
	if err := checkFreshness(parsedResp, now); err != nil {
		return nil, time.Time{}, err
	}

	// Calculate next update time
	nextUpdate := parsedResp.NextUpdate
	if nextUpdate.IsZero() || nextUpdate.After(now.Add(7*24*time.Hour)) {
		// If no next update or too far in the future, refresh in 24 hours
		nextUpdate = now.Add(DefaultOCSPExpiry)
	}

	return ocspResp, nextUpdate, nil
}

// checkFreshness rejects a response whose validity window does not contain
// now, tolerating ocspClockSkew on both ends.
func checkFreshness(resp *ocsp.Response, now time.Time) error {
	if resp.ThisUpdate.After(now.Add(ocspClockSkew)) {
		return fmt.Errorf("%w: thisUpdate %s is in the future", ErrStaleResponse, resp.ThisUpdate.Format(time.RFC3339))
	}
	if !resp.NextUpdate.IsZero() && now.After(resp.NextUpdate.Add(ocspClockSkew)) {
		return fmt.Errorf("%w: nextUpdate %s has passed", ErrStaleResponse, resp.NextUpdate.Format(time.RFC3339))
	}
	return nil
}

// NeedsRefresh returns true if the certificate's OCSP response needs refreshing.
// A response needs refresh if:
//   - It doesn't exist in the cache
//   - It will expire within the configured refresh period
//
// This method is useful for determining whether to call RunRefreshCycle.
//
// Example:
//
//	if stapler.NeedsRefresh(cert) {
//	    if err := stapler.RunRefreshCycle(ctx, cert); err != nil {
//	        log.Printf("OCSP refresh failed: %v", err)
//	    }
//	}
func (s *Stapler) NeedsRefresh(cert *tls.Certificate) bool {
	if cert == nil || len(cert.Certificate) == 0 {
		return false
	}

	cacheKey := base64.StdEncoding.EncodeToString(cert.Certificate[0])

	s.mu.RLock()
	entry, exists := s.cache[cacheKey]
	s.mu.RUnlock()

	if !exists {
		return true
	}

	entry.mu.RLock()
	needsRefresh := time.Now().Add(refreshBuffer).After(entry.nextUpdate)
	entry.mu.RUnlock()

	return needsRefresh
}

// RunRefreshCycle checks and refreshes the OCSP response for a certificate if needed.
// This method is designed to be called periodically via an external scheduler.
//
// The method:
//   - Checks if the cached response exists and is approaching expiration
//   - Fetches a new OCSP response if needed
//   - Updates the cache with the new response
//
// For periodic refresh, register this method with service/scheduler:
//
//	sched.Register(ctx, corescheduler.TaskConfig{
//	    ID:       "ocsp-refresh-" + certID,
//	    Interval: 30 * time.Minute,
//	    Func:     func(ctx context.Context) error {
//	        return stapler.RunRefreshCycle(ctx, cert)
//	    },
//	})
//
// Returns nil if the response is still valid and doesn't need refresh,
// or if the refresh was successful. Returns an error if the refresh fails.
func (s *Stapler) RunRefreshCycle(ctx context.Context, cert *tls.Certificate) error {
	if cert == nil || len(cert.Certificate) == 0 {
		return errors.New("invalid certificate")
	}

	cacheKey := base64.StdEncoding.EncodeToString(cert.Certificate[0])

	// Check if refresh is needed
	s.mu.RLock()
	entry, exists := s.cache[cacheKey]
	s.mu.RUnlock()

	// If no cache entry exists, fetch and cache
	if !exists {
		_, err := s.GetOCSPStaple(ctx, cert)
		return err
	}

	// Check if refresh is needed based on expiration time
	entry.mu.RLock()
	needsRefresh := time.Now().Add(refreshBuffer).After(entry.nextUpdate)
	entry.mu.RUnlock()

	if !needsRefresh {
		s.logger.DebugContext(ctx, "OCSP response still valid, skipping refresh")
		return nil
	}

	// Parse the certificate
	leafCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return coreerrs.WrapOperation(err, "parse certificate for refresh")
	}

	// Fetch new response
	response, nextUpdate, err := s.fetchOCSPResponse(ctx, cert, leafCert)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to refresh OCSP response", slog.Any("error", err))
		return err
	}

	entry.mu.Lock()
	entry.response = response
	entry.nextUpdate = nextUpdate
	entry.mu.Unlock()

	s.logger.DebugContext(ctx, "OCSP response refreshed successfully",
		slog.Time("next_update", nextUpdate))

	return nil
}

// removeExpiredEntries removes all expired cache entries.
func (s *Stapler) removeExpiredEntries() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.cache {
		entry.mu.RLock()
		expired := now.After(entry.nextUpdate)
		entry.mu.RUnlock()

		if expired {
			delete(s.cache, key)
		}
	}
}

// failureModeAware is an optional interface satisfied by OCSP staplers
// that expose a [FailureMode]. [StapleOCSPToConfig] consults it to
// decide whether a failed staple-fetch should abort the handshake
// (Hard) or fall through with the unstapled certificate (Soft).
// Implemented by [*Stapler]; custom OCSPStapler implementations
// without this method fall back to Soft for backwards compatibility.
type failureModeAware interface {
	FailureMode() FailureMode
}

// resolveFailureMode reports the stapler's configured FailureMode,
// defaulting to Soft for staplers that don't expose one.
func resolveFailureMode(stapler tlsutils.OCSPStapler) FailureMode {
	if fma, ok := stapler.(failureModeAware); ok {
		return fma.FailureMode()
	}
	return FailureModeSoft
}

// stapleOrEnforce wraps the common "fetch + decide based on failure
// mode" flow used by both server and client certificate paths.
// Returns (certWithStaple, nil) on success, (cert, nil) in Soft mode
// when the staple is missing, or (nil, error) in Hard mode.
func stapleOrEnforce(ctx context.Context, stapler tlsutils.OCSPStapler, cert *tls.Certificate) (*tls.Certificate, error) {
	ocspResp, err := stapler.GetOCSPStaple(ctx, cert)
	if err == nil && len(ocspResp) > 0 {
		return tlsutils.CloneCertificateWithOCSPStaple(cert, ocspResp), nil
	}

	if resolveFailureMode(stapler) == FailureModeHard {
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "ocsp staple required (hard-fail mode)")
		}
		return nil, errors.New("ocsp staple required (hard-fail mode): empty response")
	}

	// Soft mode: surrender the staple but still serve the certificate.
	return cert, nil
}

// StapleCertificate fetches an OCSP staple for cert and returns a stapled copy,
// honoring the stapler's [FailureMode]: a Hard-mode stapler returns an error
// when no valid staple can be produced, while Soft mode returns the unstapled
// certificate. Use this from per-certificate callbacks (GetCertificate /
// GetClientCertificate) where [StapleOCSPToConfig] cannot be applied.
//
// Returns cert unchanged when stapler or cert is nil.
func StapleCertificate(ctx context.Context, stapler tlsutils.OCSPStapler, cert *tls.Certificate) (*tls.Certificate, error) {
	if stapler == nil || cert == nil {
		return cert, nil
	}
	return stapleOrEnforce(ctx, stapler, cert)
}

// StapleOCSPToConfig adds OCSP stapling to a TLS config.
// It wraps the GetCertificate and GetClientCertificate functions to add OCSP staples.
// If the config already has these functions, they will be wrapped to preserve existing behavior.
// The TLS handshake context is used for OCSP requests.
//
// Failure mode (see [FailureMode]) is read from the stapler: a Hard-mode
// stapler aborts the handshake when no valid OCSP response can be
// produced; the default Soft-mode behavior serves the certificate
// without a staple.
//
// Example:
//
//	config := tlsutils.DefaultTLSConfig()
//	ocsp.StapleOCSPToConfig(config, stapler)
func StapleOCSPToConfig(config *tls.Config, stapler tlsutils.OCSPStapler) error {
	if config == nil || stapler == nil {
		return errors.New("invalid config or stapler")
	}

	// Store original GetCertificate function
	originalGetCert := config.GetCertificate

	// Wrap GetCertificate to add OCSP stapling
	config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		var cert *tls.Certificate
		var err error

		switch {
		case originalGetCert != nil:
			cert, err = originalGetCert(hello)
		case len(config.Certificates) > 0:
			// Use first certificate if no GetCertificate function
			cert = &config.Certificates[0]
		default:
			return nil, errors.New("no certificates available")
		}

		if err != nil {
			return nil, err
		}

		// Apply timeout to prevent slow OCSP responses from blocking TLS handshake
		ctx, cancel := context.WithTimeout(hello.Context(), handshakeOCSPTimeout)
		defer cancel()

		return stapleOrEnforce(ctx, stapler, cert)
	}

	// Also handle client certificates if needed
	originalGetClientCert := config.GetClientCertificate
	if originalGetClientCert != nil {
		config.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := originalGetClientCert(info)
			if err != nil {
				return nil, err
			}

			// Apply timeout to prevent slow OCSP responses from blocking TLS handshake
			ctx, cancel := context.WithTimeout(info.Context(), handshakeOCSPTimeout)
			defer cancel()

			return stapleOrEnforce(ctx, stapler, cert)
		}
	}

	return nil
}
