// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the nats block of the saga storage configuration to
// a NATS JetStream KV saga store for factory/reliability/saga.
//
// The saga factory builds only the memory and sqldb stores; register this one
// for the nats type:
//
//	orch, err := sagafactory.New(&cfg, def).
//		UseStorage(sagaconfig.StorageTypeNATS, natsfactory.Builder(js)).
//		Build(ctx)
package factory
