// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package audittest provides an in-memory audit.Auditor for tests of packages that emit audit events (HTTP and gRPC audit middleware, auth sinks).
//
// Example:
//
//	auditor, store, shutdown := audittest.NewTestAuditor(t)
package audittest
