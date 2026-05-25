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
	coreio "github.com/altessa-s/go-atlas/core/io"
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
)

// gzip pools for reduced allocations
var (
	gzipWriterPool = sync.Pool{
		New: func() any {
			return gzip.NewWriter(io.Discard)
		},
	}

	gzipReaderPool = sync.Pool{
		New: func() any {
			r, err := gzip.NewReader(bytes.NewReader(nil))
			if err != nil {
				// This should never happen with an empty reader,
				// but return nil to be handled by getGzipReader
				return nil
			}
			return r
		},
	}
)

func getGzipWriter(w io.Writer) *gzip.Writer {
	gw, ok := gzipWriterPool.Get().(*gzip.Writer)
	if !ok || gw == nil {
		return gzip.NewWriter(w)
	}
	gw.Reset(w)
	return gw
}

func putGzipWriter(gw *gzip.Writer) {
	gzipWriterPool.Put(gw)
}

func getGzipReader(r io.Reader) (*gzip.Reader, error) {
	gr, ok := gzipReaderPool.Get().(*gzip.Reader)
	if !ok || gr == nil {
		return gzip.NewReader(r)
	}
	if err := gr.Reset(r); err != nil {
		gzipReaderPool.Put(gr)
		return nil, err
	}
	return gr, nil
}

func putGzipReader(gr *gzip.Reader) {
	gzipReaderPool.Put(gr)
}

var _ tlsutils.OCSPStapler = (*Stapler)(nil)

// Stapler manages OCSP stapling for TLS certificates.
// It provides automatic caching, compression, and refresh of OCSP responses.
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
	mu                sync.RWMutex
	cache             map[string]*ocspCacheEntry
	httpClient        *http.Client
	retryPolicy       RetryPolicy
	logger            *slog.Logger
	cleanupCounter    atomic.Uint64 // atomic counter for lazy cleanup
	scheduler         corescheduler.TaskRegistrar
	refreshAllRunning atomic.Bool // Guards against concurrent RunRefreshAll calls.

	schedulerRefreshAllRegistered atomic.Bool // Marks if RunRefreshAll is managed by scheduler.
	enableCompression             bool
	// failureMode controls handshake behavior when GetOCSPStaple cannot
	// produce a valid response — see [FailureMode] for the rationale.
	failureMode FailureMode
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

type ocspCacheEntry struct {
	response       []byte
	nextUpdate     time.Time
	originalSize   int
	compressedSize int
	cert           *tls.Certificate
	mu             sync.RWMutex
	isCompressed   bool
}

// NewOCSPStapler creates a new OCSP stapler with the specified options.
// By default, compression is enabled and no retry policy is configured.
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
		scheduler:         o.scheduler,
	}

	// Register refresh task with scheduler if configured
	if err := s.registerSchedulerTask(o); err != nil {
		logger.Warn("failed to register OCSP refresh task", slog.Any("error", err))
	}

	return s
}

// compressData compresses data using gzip with pooled buffers and writers.
// Returns the original data if it's empty.
func compressData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	buf := coreio.GetBuffer()
	defer coreio.PutBuffer(buf)

	writer := getGzipWriter(buf)
	defer putGzipWriter(writer)

	if _, err := writer.Write(data); err != nil {
		_ = writer.Close() //nolint:errcheck
		return nil, coreerrs.WrapOperation(err, "write compressed data")
	}

	if err := writer.Close(); err != nil {
		return nil, coreerrs.WrapOperation(err, "close gzip writer")
	}

	// Return a copy since we're returning the buffer to the pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

// decompressData decompresses gzip data using pooled readers.
// Returns the original data if it's empty.
func decompressData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	reader, err := getGzipReader(bytes.NewReader(data))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create gzip reader")
	}
	defer putGzipReader(reader)

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "decompress data")
	}

	return decompressed, nil
}

// GetOCSPStaple returns the OCSP staple for the given certificate.
// It checks the cache first and fetches a new response if necessary.
// The response is automatically compressed if compression is enabled.
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
		entry.mu.RLock()
		defer entry.mu.RUnlock()

		// Return cached response if still valid
		if time.Now().Before(entry.nextUpdate) {
			if entry.isCompressed {
				// Decompress before returning
				decompressed, err := decompressData(entry.response)
				if err != nil {
					s.logger.WarnContext(ctx, "failed to decompress cached OCSP response", slog.Any("error", err))
					// Fall through to fetch new response
				} else {
					s.logger.DebugContext(ctx, "returning decompressed OCSP response from cache",
						"original_size", entry.originalSize,
						"compressed_size", entry.compressedSize,
						"compression_ratio", float64(entry.compressedSize)/float64(entry.originalSize))
					return decompressed, nil
				}
			} else {
				return entry.response, nil
			}
		}
	}

	// Fetch new OCSP response
	response, nextUpdate, err := s.fetchOCSPResponse(ctx, cert, leafCert)
	if err != nil {
		return nil, err
	}

	cacheEntry := s.prepareCacheEntry(ctx, response, nextUpdate)

	s.mu.Lock()
	s.cache[cacheKey] = cacheEntry
	cacheEntry.cert = cert
	s.mu.Unlock()

	return response, nil
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
				httpReq.Header.Set("Accept-Encoding", "gzip, deflate")
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

			ocspResp, err = io.ReadAll(responseBody)
			if err != nil {
				return coreerrs.WrapOperation(err, "read OCSP response")
			}

			parsedResp, err = ocsp.ParseResponse(ocspResp, issuerCert)
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

	// Calculate next update time
	nextUpdate := parsedResp.NextUpdate
	if nextUpdate.IsZero() || nextUpdate.After(time.Now().Add(7*24*time.Hour)) {
		// If no next update or too far in the future, refresh in 24 hours
		nextUpdate = time.Now().Add(DefaultOCSPExpiry)
	}

	return ocspResp, nextUpdate, nil
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

	// Update cache entry with optional compression
	newEntry := s.prepareCacheEntry(ctx, response, nextUpdate)

	entry.mu.Lock()
	entry.response = newEntry.response
	entry.nextUpdate = newEntry.nextUpdate
	entry.isCompressed = newEntry.isCompressed
	entry.originalSize = newEntry.originalSize
	entry.compressedSize = newEntry.compressedSize
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

// prepareCacheEntry creates a cache entry with optional compression.
func (s *Stapler) prepareCacheEntry(ctx context.Context, response []byte, nextUpdate time.Time) *ocspCacheEntry {
	originalSize := len(response)
	isCompressed := false
	compressedSize := originalSize
	data := response

	if s.enableCompression && originalSize > 0 {
		if compressed, err := compressData(response); err == nil {
			data = compressed
			isCompressed = true
			compressedSize = len(compressed)

			s.logger.DebugContext(ctx, "compressed OCSP response",
				slog.Int("original_size", originalSize),
				slog.Int("compressed_size", compressedSize),
				slog.Float64("compression_ratio", float64(compressedSize)/float64(originalSize)))
		} else {
			s.logger.WarnContext(ctx, "failed to compress OCSP response, storing uncompressed",
				slog.Any("error", err))
		}
	}

	return &ocspCacheEntry{
		response:       data,
		nextUpdate:     nextUpdate,
		isCompressed:   isCompressed,
		originalSize:   originalSize,
		compressedSize: compressedSize,
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
