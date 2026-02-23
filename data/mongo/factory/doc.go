// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of cursor storages.
// It integrates cursor storage implementations with memory, Redis, and NATS backends
// to create cursor storages with consistent defaults.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	storage, err := f.CreateCursorStorageFromConfig(ctx, cfg.Storage, time.Hour)
//	if err != nil {
//		log.Fatal(err)
//	}
package factory
