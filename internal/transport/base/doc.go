// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package base provides the shared foundation for HTTP middlewares and gRPC
// interceptors. It contains the common [Base] struct with constructors,
// endpoint filtering and parameterized structured logging.
//
// This is an internal package — import it only from
// transport/http/server/middlewares and transport/grpc/interceptors.
package base
