// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgrestest provides an env-gated PostgreSQL database for integration tests of services built on store/postgres.
//
// [New] opens a *sql.DB through store/postgres and drops its database during cleanup.
// Use a direct endpoint on a dedicated test cluster with CREATEDB access and access to the postgres maintenance database.
package postgrestest
