// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package singleflight deduplicates concurrent work using comparable keys.
//
// # Admission and cancellation
//
// Distinct keys are admitted before goroutines are created. Overload is reported
// immediately; callers sharing a key wait independently on their own contexts.
//
//	g := singleflight.New[string, string]()
//	value, err := g.Do(ctx, "key", func(work context.Context) (string, error) {
//		return fetch(work, "key")
//	})
package singleflight
