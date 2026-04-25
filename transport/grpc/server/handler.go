// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	stdGrpc "google.golang.org/grpc"
)

// Handler defines the interface for gRPC service handlers.
// Implementations register their services with the provided gRPC server
// and use the stop channel to detect graceful shutdown (e.g. for streaming RPCs).
type Handler interface {
	// Register registers the handler's services with gs.
	// stop is closed when the server begins shutting down.
	Register(gs *stdGrpc.Server, stop <-chan struct{})
}
