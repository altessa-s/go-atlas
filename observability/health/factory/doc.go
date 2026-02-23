// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of health [health.Coordinator] instances.
// It maps [config.Health] settings to [health.Option] values, applying the factory's
// logger as the default.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	coordinator, err := f.CreateCoordinatorFromConfig(cfg)
//	if err != nil {
//	    return err
//	}
//	coordinator.RegisterService("database", dbChecker)
package factory
