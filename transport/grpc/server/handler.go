// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	stdGrpc "google.golang.org/grpc"
)

// Handler defines the interface for gRPC service handlers.
// Implementations register their services with the provided registrar and can
// use the stop channel to detect graceful shutdown for streaming RPCs.
type Handler interface {
	// Register synchronously submits service descriptors during Start. The
	// registrar is valid only during this call. The stop channel closes when
	// the running server begins shutting down.
	Register(registrar stdGrpc.ServiceRegistrar, stop <-chan struct{})
}
