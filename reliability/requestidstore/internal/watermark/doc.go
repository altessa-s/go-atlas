// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package watermark tracks the latest caller time observed by a request ID store.
// Background garbage collection uses that clock to select expired entries.
//
// # Usage
//
//	var clock watermark.Clock
//	clock.Observe(now)
//	cutoff := clock.Now()
package watermark
