// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory materializes GRPC client options from configuration.
//
// # Usage
//
//	opts := factory.HealthOptions(cfg.Health)
//
// HealthOptions configures health reporting.
// Proxy options are assembled in transport/proxydial/factory.
package factory
