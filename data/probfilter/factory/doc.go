// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of probabilistic filters.
// It integrates with the config.ProbabilisticFilter configuration to create
// Bloom and Cuckoo filters with their respective storage backends.
//
// Example:
//
//	f := factory.New(redisClient, cfg.Defaults)
//	mgr, err := f.CreateManagerFromConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
package factory
