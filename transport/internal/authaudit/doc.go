// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package authaudit records HTTP and gRPC authentication and authorization decisions.
//
// # Usage
//
//	err := authaudit.Record(ctx, recorder, "http", action, subject, allowed, reason)
package authaudit
