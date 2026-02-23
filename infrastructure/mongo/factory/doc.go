// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of MongoDB clients.
// It integrates with [config.Mongodb] to produce driver-level
// [go.mongodb.org/mongo-driver/v2/mongo/options.ClientOptions] and
// higher-level [mongo.Mongo] wrappers with authentication, TLS, connection
// pooling, compression, and client-side field level encryption (CSFLE).
//
// The factory supports two configuration modes: connection-URI based
// (when [config.Mongodb.ConnectionURI] is set) and field-based (individual
// host, credential, and pool settings). Both paths converge in
// [Factory.ClientOptionsFromConfig].
//
// When a [health.Coordinator] is provided via [WithHealthCoordinator],
// the factory automatically registers a health checker that pings the
// primary on each check.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	mongo, err := f.CreateMongoFromConfig(cfg.Mongodb)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := mongo.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer mongo.Close()
package factory
