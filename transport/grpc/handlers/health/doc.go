// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package health implements the gRPC health checking protocol (grpc_health_v1).
//
// [Handler] delegates to an [health.Coordinator] from the observability/health
// package, supporting Check (single service), List (all services), and Watch
// (server-streaming status updates). The Watch stream terminates with
// codes.Canceled when the server's stop channel is closed.
//
// # Usage
//
//	coord := health.NewCoordinator(checker)
//	handler := health.New(coord)
//	handler.Register(grpcServer, stopCh)
package health
