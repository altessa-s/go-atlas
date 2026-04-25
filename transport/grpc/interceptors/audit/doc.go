// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package audit provides a gRPC server interceptor that emits audit events
// for every processed RPC. It captures method name, status code, duration,
// caller identity, and request metadata via the [audit.Auditor] pipeline.
package audit
