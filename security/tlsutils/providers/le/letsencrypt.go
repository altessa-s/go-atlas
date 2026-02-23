// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsle

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/security/tlsutils"

	"golang.org/x/crypto/acme/autocert"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// LetsEncrypt provides automatic TLS certificate management using the ACME protocol
// and Let's Encrypt certificate authority. Use New to create a new instance.
type LetsEncrypt struct {
	opts    *options
	am      *autocert.Manager
	httpSrv *http.Server
	mu      sync.Mutex
}

// New creates a new Let's Encrypt TLS certificate provider.
// The provider uses HTTP-01 challenges which require port 80 to be accessible.
//
// Example:
//
//	provider, err := tlsle.New(
//		tlsle.WithDomains("example.com"),
//		tlsle.WithEmail("admin@example.com"),
//		tlsle.WithCacheDir("./certs"),
//	)
func New(opt ...Option) (*LetsEncrypt, error) {
	opts, err := newOptions(opt...)
	if err != nil {
		return nil, err
	}
	if opts.logger == nil {
		opts.logger = slog.New(slog.DiscardHandler)
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}

	le := &LetsEncrypt{opts: opts}
	le.initAutocertManager()

	return le, nil
}

func (le *LetsEncrypt) initAutocertManager() {
	am := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      le.opts.email,
		HostPolicy: autocert.HostWhitelist(le.opts.domains...),
		Client:     le.opts.client,
	}
	if le.opts.renewBefore > 0 {
		am.RenewBefore = le.opts.renewBefore
	}

	if le.opts.cacheDir != "" {
		am.Cache = autocert.DirCache(le.opts.cacheDir)
	}

	le.am = am
}

// StartHTTPServer starts the ACME HTTP server for handling HTTP-01 challenges.
// The server must be accessible on port 80 from the internet for challenge validation.
// The fallback handler receives non-ACME requests.
//
// Example:
//
//	go provider.StartHTTPServer(":80", nil)
//
// Example with HTTP to HTTPS redirect:
//
//	redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		http.Redirect(w, r, "https://"+r.Host+r.URL.Path, http.StatusMovedPermanently)
//	})
//	go provider.StartHTTPServer(":80", redirect)
func (le *LetsEncrypt) StartHTTPServer(addr string, fallback http.Handler) error {
	return le.StartHTTPServerWithContext(context.Background(), addr, fallback)
}

var (
	// ErrNilHTTPServerContext is returned when a nil context is provided to StartHTTPServerWithContext.
	ErrNilHTTPServerContext = errors.New("acme http server: context cannot be nil")
	// ErrHTTPServerStarted is returned when attempting to start an already running HTTP server.
	ErrHTTPServerStarted = errors.New("acme http server: already started")
)

func hostFromAddr(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	// Accept host-only addresses too (e.g. "0.0.0.0", "localhost", "::").
	return strings.Trim(addr, "[]")
}

// StartHTTPServerWithContext starts the ACME HTTP server with context support.
// The context enables graceful shutdown and cancellation.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	go provider.StartHTTPServerWithContext(ctx, ":80", nil)
func (le *LetsEncrypt) StartHTTPServerWithContext(ctx context.Context, addr string, fallback http.Handler) error {
	if ctx == nil {
		return ErrNilHTTPServerContext
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	host := hostFromAddr(addr)

	le.mu.Lock()
	if le.httpSrv != nil {
		le.mu.Unlock()
		return ErrHTTPServerStarted
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(host, "http"))
	if err != nil {
		le.mu.Unlock()
		return fmt.Errorf("acme http server: %w", err) //nolint:goerr113
	}

	srv := &http.Server{
		Handler:        le.am.HTTPHandler(fallback),
		MaxHeaderBytes: 1 << 16,           //nolint:mnd
		ReadTimeout:    30 * time.Second,  //nolint:mnd
		WriteTimeout:   30 * time.Second,  //nolint:mnd
		IdleTimeout:    120 * time.Second, //nolint:mnd
	}
	le.httpSrv = srv
	le.mu.Unlock()

	// ACME HTTP server
	go func() {
		defer func() {
			le.mu.Lock()
			if le.httpSrv == srv {
				le.httpSrv = nil
			}
			le.mu.Unlock()
		}()

		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			le.opts.logger.ErrorContext(ctx, "acme http server stopped with error", slog.Any("error", err))
		}
	}()

	// Stop server on context cancellation.
	go func() {
		<-ctx.Done()

		// Inherit values from the original context (tracing/log fields), but don't inherit cancellation.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), le.opts.httpShutdownTimeout)
		defer cancel()

		// Try graceful shutdown first, then hard close.
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close() // #nosec G104 -- best effort close
		}
	}()

	le.opts.logger.InfoContext(ctx, "acme HTTP server started", slog.String("address", listener.Addr().String()))

	return nil
}

// HTTPHandler returns an HTTP handler that responds to ACME HTTP-01 challenges.
// Non-ACME requests are forwarded to the fallback handler.
//
// Example:
//
//	mux := http.NewServeMux()
//	http.ListenAndServe(":80", provider.HTTPHandler(mux))
func (le *LetsEncrypt) HTTPHandler(fallback http.Handler) http.Handler {
	return le.am.HTTPHandler(fallback)
}

// TLSConfig returns a TLS configuration with automatic certificate management.
// The configuration uses secure defaults and handles certificate renewal automatically.
//
// Example:
//
//	tlsConfig, err := provider.TLSConfig()
//	if err != nil {
//		log.Fatal(err)
//	}
//	server := &http.Server{
//		Addr:      ":443",
//		TLSConfig: tlsConfig,
//		Handler:   handler,
//	}
//	server.ListenAndServeTLS("", "")
func (le *LetsEncrypt) TLSConfig() (*tls.Config, error) {
	// Get autocert manager's TLS config
	tlsConfig := le.am.TLSConfig()

	// Apply secure defaults from tlsutils
	tlsConfig.MinVersion = tls.VersionTLS12
	tlsConfig.CipherSuites = tlsutils.DefaultTLSConfig().CipherSuites

	return tlsConfig, nil
}

// GetCertificate retrieves or requests a certificate for the given domain.
// This method implements the tls.Config.GetCertificate callback.
//
// Example:
//
//	config := &tls.Config{GetCertificate: provider.GetCertificate}
func (le *LetsEncrypt) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return le.am.GetCertificate(hello)
}

// Close stops the HTTP server gracefully using the provided context.
// The context controls the graceful shutdown timeout.
// Returns context.DeadlineExceeded if shutdown doesn't complete within the context timeout.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	provider.Close(ctx)
func (le *LetsEncrypt) Close(ctx context.Context) error {
	le.mu.Lock()
	srv := le.httpSrv
	le.httpSrv = nil
	le.mu.Unlock()

	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// Type returns the provider type identifier.
// Always returns ProviderTypeLetsEncrypt.
//
// Example:
//
//	fmt.Println(provider.Type()) // "letsencrypt"
func (le *LetsEncrypt) Type() tlsproviders.ProviderType {
	return tlsproviders.ProviderTypeLetsEncrypt
}

// Ensure LetsEncrypt implements required interfaces
var _ tlsproviders.Certificate = (*LetsEncrypt)(nil)
var _ tlsproviders.Provider = (*LetsEncrypt)(nil)
