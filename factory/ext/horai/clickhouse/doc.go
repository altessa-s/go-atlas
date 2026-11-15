// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the clickhouse block of the scheduler history storage
// configuration to a ClickHouse history storage for
// factory/ext/horai, creating the table at startup when
// ensureSchema is set.
//
// The scheduler factory builds no history storage of its own; register this
// one for the clickhouse type:
//
//	sched, err := schedulerfactory.New(&cfg.Scheduler).
//		UseHistoryStorage(schedulerconfig.HistoryStorageTypeClickHouse, chfactory.Builder(conn)).
//		Build(ctx)
package factory
