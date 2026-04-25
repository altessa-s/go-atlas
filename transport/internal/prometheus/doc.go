// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prometheus provides shared Prometheus metrics building blocks
// for both HTTP and gRPC transports.
//
// Key facilities:
//
//   - Default histogram bucket sets for request duration and message size
//   - [BuildMetricName] / [BuildMetricNameWithDefault] for assembling
//     namespace_subsystem_suffix metric names
//   - Interned label name strings to avoid per-recording allocations
//   - [GetMessageSize] for extracting byte size from protobuf and custom
//     message types
//   - [ValidateBuckets] / [MustValidateBuckets] for compile-time bucket
//     validation
//
// This is an internal package. Transport-specific public APIs live in
// transport/http and transport/grpc.
package prometheus
