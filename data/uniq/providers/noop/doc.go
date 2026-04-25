// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package noop provides no-operation provider for testing without external dependencies.
// All operations succeed immediately without actual storage. Ideal for unit tests and development environments.
//
// Example:
//
//	provider := noop.New()
//	u := uniq.New(provider)
//	u.Add(ctx, "key") // Always succeeds
package noop
