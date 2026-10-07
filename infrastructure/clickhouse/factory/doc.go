// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a ClickHouse driver connection from
// [clickhouseconfig.Config] using a fluent builder.
//
// The builder owns nothing beyond assembly: it turns configuration and
// injected dependencies (TLS, logger, health coordinator) into a
// driver.Conn and hands ownership to the caller, who must close it. Storage
// packages such as data/audit/storages/clickhouse take that connection as a
// parameter rather than dialing one themselves.
//
//	conn, err := factory.New(cfg).
//		UseDefaultLogger().
//		UseHealthCoordinator(coordinator).
//		Build(ctx)
//	if err != nil {
//		return err
//	}
//	defer conn.Close()
//
// The driver connects lazily, so a successful Build does not prove the
// server is reachable. Registering a health coordinator is what turns an
// unreachable server into an observable NOT_SERVING status.
package factory
