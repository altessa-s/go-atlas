// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package driver defines the core interfaces for the driven interceptor pattern.
// It decouples interceptor logic from gRPC-specific function signatures.
//
// The key interfaces are:
//
//   - [DrivenInterceptor] -- entry point; creates a per-call [Driver] from
//     the request context.
//   - [Driver] -- hooks for unary and streaming RPCs ([Driver.PreCall],
//     [Driver.PostCall]).
//   - [DriverStream] -- optional extension of [Driver] with per-message hooks
//     ([DriverStream.PostMsgSent], [DriverStream.PostMsgReceive]).
//
// Use [NoopDriver] to obtain a pass-through [Driver] for cases where no
// interception is needed.
package driver
