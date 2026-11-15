// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the saga storage configuration to
// a Redis saga store for factory/reliability/saga.
//
// The saga factory builds only the memory and sqldb stores; register this one
// for the redis type:
//
//	orch, err := sagafactory.New(&cfg, def).
//		UseStorage(sagaconfig.StorageTypeRedis, redisfactory.Builder(rdb)).
//		Build(ctx)
package factory
