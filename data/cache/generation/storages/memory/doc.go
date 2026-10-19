// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-process generation.Store: bumps are visible to
// the same process at once and to no other process. Use it for a single
// replica, for tests, or as the default of cache.NewGenerations.
//
// Example:
//
//	store := memory.New()
//	gen, _ := store.Current(ctx, "catalog")
//	next, _ := store.Bump(ctx, "catalog") // next > gen
package memory
