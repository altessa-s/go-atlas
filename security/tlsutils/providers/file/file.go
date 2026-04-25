// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/altessa-s/go-atlas/security/tlsutils"
	"github.com/altessa-s/go-atlas/security/tlsutils/ocsp"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coretime "github.com/altessa-s/go-atlas/core/time"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// File represents a file-based TLS provider with automatic certificate reloading.
// Use NewWithCertAndKey to create a new instance.
type File struct {
	certFile    string
	keyFile     string
	keyPassword string

	mu          sync.RWMutex
	tlsConfig   *tls.Config
	lastModTime time.Time

	watcher     *fsnotify.Watcher
	watcherDone chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc

	options     *options
	certificate *tls.Certificate

	// Frequently accessed fields copied from options
	ocspStapler tlsutils.OCSPStapler
	logger      *slog.Logger
}

// NewWithCertAndKey creates a new file-based TLS provider with automatic certificate reloading.
// The provider watches certificate and key files for changes and reloads them automatically.
// The keyPassword parameter is required for encrypted private keys.
//
// Example:
//
//	provider, err := tlsfile.NewWithCertAndKey("server.crt", "server.key", "password")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer provider.Close()
func NewWithCertAndKey(certFile, keyFile, keyPassword string, opts ...Option) (*File, error) {
	if certFile == "" {
		return nil, errors.New("invalid cert file: path cannot be empty")
	}

	if keyFile == "" {
		return nil, errors.New("invalid key file: path cannot be empty")
	}

	o := newOptions(opts...)
	f := &File{
		certFile:    certFile,
		keyFile:     keyFile,
		keyPassword: keyPassword,
		watcherDone: make(chan struct{}),
		options:     o,
		ocspStapler: o.ocspStapler,
		logger:      o.logger,
	}

	baseCtx := corecontext.OrBackground(f.options.ctx)
	f.ctx, f.cancel = context.WithCancel(baseCtx)

	// Load initial certificate
	if err := f.loadCertificate(); err != nil {
		_ = f.cleanup() //nolint:errcheck
		return nil, err
	}

	// Initialize OCSP stapler if provided
	if f.ocspStapler != nil {
		// Apply OCSP stapling to TLS config
		// GetOCSPStaple caches the certificate for scheduler-based refresh
		if err := ocsp.StapleOCSPToConfig(f.tlsConfig, f.ocspStapler); err != nil {
			_ = f.cleanup() //nolint:errcheck
			return nil, err
		}
	}

	if f.options.enableWatcher {
		// Initialize fsnotify watcher
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			f.cancel()
			return nil, err
		}
		f.watcher = watcher

		// Add files to watcher
		if err = f.watcher.Add(certFile); err != nil {
			_ = f.cleanup() //nolint:errcheck
			return nil, err
		}

		if keyFile != certFile {
			if err = f.watcher.Add(keyFile); err != nil {
				_ = f.cleanup() //nolint:errcheck
				return nil, err
			}
		}

		// Start watching goroutine
		go f.watchFiles()
	} else {
		// If watcher is not enabled, close the channel immediately
		close(f.watcherDone)
	}

	return f, nil
}

// Type returns the provider type identifier.
// Always returns ProviderTypeFile for file-based providers.
//
// Example:
//
//	fmt.Println(provider.Type()) // "file"
func (f *File) Type() tlsproviders.ProviderType {
	return tlsproviders.ProviderTypeFile
}

// TLSConfig returns the current TLS configuration with loaded certificates.
// The configuration is cloned to prevent external modifications.
// Returns an error if certificates are not loaded.
//
// Example:
//
//	config, err := provider.TLSConfig()
//	server := &http.Server{TLSConfig: config}
func (f *File) TLSConfig() (*tls.Config, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.tlsConfig == nil {
		return nil, errors.New("TLS config not loaded")
	}

	// Return a copy to prevent modifications
	config := f.tlsConfig.Clone()
	return config, nil
}

// Close stops the file watcher and releases all resources.
// The context controls the graceful shutdown timeout.
// Returns context.DeadlineExceeded if cleanup doesn't complete within the context timeout.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	provider.Close(ctx)
func (f *File) Close(ctx context.Context) error {
	f.cancel()

	// If watcher is not enabled, return immediately
	if !f.options.enableWatcher {
		return f.cleanup()
	}

	ctx = corecontext.OrBackground(ctx)

	// Wait for watcher goroutine to finish or context to timeout
	select {
	case <-f.watcherDone:
	case <-ctx.Done():
		_ = f.cleanup() //nolint:errcheck // best-effort cleanup before returning context error
		return ctx.Err()
	}

	return f.cleanup()
}

func (f *File) cleanup() error {
	if f.watcher != nil {
		return f.watcher.Close()
	}
	return nil
}

func (f *File) loadCertificate() error {
	cert, err := tlsutils.LoadFromFile(f.certFile, f.keyFile, f.keyPassword)
	if err != nil {
		return err
	}

	// Start with secure default TLS configuration
	tlsConfig := tlsutils.DefaultTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{*cert}

	f.mu.Lock()
	f.tlsConfig = tlsConfig
	f.certificate = cert
	f.lastModTime = time.Now()
	f.mu.Unlock()

	// If OCSP stapler exists, refresh OCSP for new certificate
	// StapleOCSPToConfig calls GetOCSPStaple which caches the certificate for scheduler-based refresh
	if f.ocspStapler != nil {
		if err := ocsp.StapleOCSPToConfig(tlsConfig, f.ocspStapler); err != nil {
			f.logger.WarnContext(f.ctx, "failed to apply OCSP stapling to reloaded certificate", slog.Any("error", err))
		}
	}

	f.logger.DebugContext(f.ctx, "certificate loaded successfully",
		"cert_file", f.certFile,
		"key_file", f.keyFile,
		"ocsp_enabled", f.ocspStapler != nil)

	// Notify about reload if channel is set
	if f.options.reloadNotifyChan != nil {
		select {
		case f.options.reloadNotifyChan <- struct{}{}:
		default:
			// Don't block if channel is full
		}
	}

	return nil
}

func (f *File) watchFiles() {
	defer close(f.watcherDone)
	defer func() {
		_ = f.cleanup() //nolint:errcheck
	}()

	// Debounce timer to avoid multiple reloads for rapid file changes.
	// Implemented without time.AfterFunc to avoid a separate goroutine that can race with shutdown/cleanup.
	var debounceTimer *time.Timer
	const debounceDelay = 100 * time.Millisecond
	debounceC := (<-chan time.Time)(nil)

	for {
		select {
		case <-f.ctx.Done():
			if debounceTimer != nil {
				coretime.TimerStopAndDrain(debounceTimer)
			}
			return

		case event, ok := <-f.watcher.Events:
			if !ok {
				return
			}

			// Only react to write and create events
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				f.logger.DebugContext(f.ctx, "file change detected", "file", event.Name, "op", event.Op)

				// Reset debounce timer
				if debounceTimer != nil {
					coretime.TimerStopAndDrain(debounceTimer)
					debounceTimer.Reset(debounceDelay)
				} else {
					debounceTimer = time.NewTimer(debounceDelay)
					debounceC = debounceTimer.C
				}
			}

		case <-debounceC:
			if err := f.loadCertificate(); err != nil {
				f.logger.ErrorContext(f.ctx, "failed to reload certificate", slog.Any("error", err))
			} else {
				f.logger.InfoContext(f.ctx, "certificate reloaded successfully", "cert_file", f.certFile, "key_file", f.keyFile)
			}

		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			f.logger.ErrorContext(f.ctx, "file watcher error", slog.Any("error", err))
		}
	}
}

var _ tlsproviders.Provider = (*File)(nil)
