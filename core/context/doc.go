// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package context provides helpers for working with contexts.
//
// # Usage
//
//	ctx, cancel := context.ApplyTimeout(ctx, 5*time.Second)
//	defer cancel()
package context
