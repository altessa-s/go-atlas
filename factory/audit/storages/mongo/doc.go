// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the mongo block of the audit storage configuration to
// a MongoDB audit storage for factory/audit, creating the collection
// indexes at startup.
//
// The audit factory builds only the memory and sqldb storages; register this
// one for the mongo type:
//
//	auditor, err := auditfactory.New(&cfg.Audit).
//		UseStorage(auditconfig.StorageTypeMongo, auditmongofactory.Builder(db)).
//		Build(ctx)
package factory
