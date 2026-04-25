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
// Lifecycle methods use atomic flags to prevent double-start and
// double-shutdown. Shutdown progress is logged at warning level on a
// configurable interval (see [timeouts.Config]).
//
// Example:
//
//	base := server.NewBaseServer(
//	    server.WithAddress(":8080"),
//	    server.WithLogger(slog.Default()),
//	)
//	base.Start("http", func(ln net.Listener, errCh chan<- error) {
//	    go httpServer.Serve(ln)
//	})
package server
