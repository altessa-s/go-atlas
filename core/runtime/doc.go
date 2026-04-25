// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package runtime provides low-level runtime utilities wrapping Go's runtime package.
// Offers type-safe generics for cleanup and finalizer operations.
//
// Example:
//
//	cleanup := runtime.AddCleanup(obj, func(data string) {
//	    log.Println("cleaning up:", data)
//	}, "resource-id")
//	defer cleanup.Stop()
package runtime
