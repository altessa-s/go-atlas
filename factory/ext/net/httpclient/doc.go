// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory materializes HTTP client options from configuration.
//
// # Usage
//
//	obs := observe.New(meter, coord, factory.HealthOptions(cfg.Health)...)
//	defer obs.Close()
//	c := httpclient.New(httpclient.WithObserver(obs))
//
// HealthOptions configures the health reporting of an observe.Observer and SSRFOptions validates and materializes
// CIDR exemptions and the allow/deny egress policy. Close the observer returned
// by observe.New when the client is retired.
// Proxy options are assembled in factory/ext/net/proxydial.
package factory
