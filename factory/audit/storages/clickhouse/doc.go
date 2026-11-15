// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the clickhouse block of the audit storage
// configuration to a ClickHouse audit storage for factory/audit,
// applying the configured schema migration and checking the table schema at
// startup.
//
// The audit factory builds only the memory, mongo and sqldb storages;
// register this one for the clickhouse type:
//
//	conn, err := chfactory.New(&cfg.ClickHouse).Build(ctx) // factory/infrastructure/clickhouse
//	auditor, err := auditfactory.New(&cfg.Audit).
//		UseStorage(auditconfig.StorageTypeClickHouse, auditchfactory.Builder(conn)).
//		Build(ctx)
package factory
