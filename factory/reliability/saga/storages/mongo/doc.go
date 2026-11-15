// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the saga storage configuration to
// a MongoDB saga store for factory/reliability/saga.
//
// The saga factory builds only the memory and sqldb stores; register this one
// for the mongo type:
//
//	orch, err := sagafactory.New(&cfg, def).
//		UseStorage(sagaconfig.StorageTypeMongo, mongofactory.Builder(db)).
//		Build(ctx)
package factory
