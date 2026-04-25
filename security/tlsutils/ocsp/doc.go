// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ocsp provides OCSP stapling for TLS certificates with caching and compression.
//
// OCSP (Online Certificate Status Protocol) stapling improves TLS handshake performance
// by including certificate revocation status in the TLS handshake, eliminating the need
// for clients to contact the CA directly.
//
// # Features
//
//   - Automatic caching of OCSP responses with lazy expiration cleanup
//   - Scheduler-based refresh via RunRefreshCycle for integration with service/scheduler
//   - Built-in gzip compression to reduce memory usage by 60-95%
//   - Configurable retry policies for network failures
//   - Thread-safe concurrent operations
//   - Structured logging support
//
// # Basic Usage
//
//	stapler := ocsp.NewOCSPStapler(
//	    ocsp.WithRefreshPeriod(30*time.Minute),
//	    ocsp.WithCompression(),
//	    ocsp.WithLogger(slog.Default()),
//	)
//
//	// Apply to TLS config
//	config := &tls.Config{Certificates: []tls.Certificate{cert}}
//	ocsp.StapleOCSPToConfig(config, stapler)
//
// # Periodic Refresh with Scheduler
//
//	// Register periodic OCSP refresh with service/scheduler
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:       "ocsp-refresh",
//	    Interval: 30 * time.Minute,
//	    Func: func(ctx context.Context) error {
//	        return stapler.RunRefreshCycle(ctx, cert)
//	    },
//	})
//
// # Manual Refresh
//
//	// Check if refresh is needed
//	if stapler.NeedsRefresh(cert) {
//	    if err := stapler.RunRefreshCycle(ctx, cert); err != nil {
//	        log.Printf("OCSP refresh failed: %v", err)
//	    }
//	}
//
// # Lifecycle Management
//
// The package uses context-based lifecycle management. Each operation accepts a context
// that controls cancellation and timeout. For periodic refresh, register RunRefreshCycle
// with an external scheduler like service/scheduler.
package ocsp
