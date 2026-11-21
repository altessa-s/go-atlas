// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds distributed AIP-155 stores from configuration.
//
// # Usage
//
//	store, err := factory.New(cfg).UseStorage(requestidstoreconfig.StorageTypeRedis, redisfactory.Builder(client)).Build(ctx)
package factory
