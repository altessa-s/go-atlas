// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory materializes HTTP client options from configuration.
//
// # Usage
//
//	opts := factory.HealthOptions(cfg.Health)
//
// HealthOptions configures health reporting and SSRFOptions validates and materializes CIDR exemptions.
// Proxy options are assembled in transport/proxydial/factory.
package factory
