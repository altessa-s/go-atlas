// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package server provides the shared lifecycle primitives used by both
// the HTTP and gRPC server implementations.
//
// [BaseServer] owns listener creation, TLS wrapping, startup verification,
// and graceful shutdown with progress logging. Protocol-specific servers
// embed it and supply [StartFunc] / [ShutdownFunc] callbacks for their
// own Serve/GracefulStop logic.
//
// Lifecycle methods serialize startup, shutdown, and failure recording.
// Shutdown progress is logged at warning level on a configurable interval
// (see [timeouts.Config]).
//
// Example:
//
//	base := server.NewBaseServer(
//	    server.WithAddress(":8080"),
//	    server.WithLogger(slog.Default()),
//	)
//	base.Start("http", func(ln net.Listener, errCh chan<- error) {
//	    go func() {
//	        if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
//	            errCh <- err
//	        }
//	    }()
//	})
//
// After Start succeeds, wait on Done to observe a post-start Serve failure or
// completion of lifecycle shutdown, including cancellation after the lifecycle
// claim. Protocol cleanup may finish after Done closes on cancellation.
// Err returns the first post-start Serve error; it remains nil on normal shutdown.
// Early Start errors are returned synchronously and leave Done open. Retrying is
// meaningful for listen/bind or pre-Serve errors. Auto-created listeners are
// cleared after early startup errors; a WithListener listener may be consumed by
// a failed Serve and is never replaced.
//
//	if err := base.Start("http", startHTTP); err != nil {
//	    return err // Do not wait on Done for an early startup failure.
//	}
//	<-base.Done()
//	return base.Err()
package server
