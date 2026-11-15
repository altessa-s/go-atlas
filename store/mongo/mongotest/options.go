// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest

//go:generate go tool optgen generate

import "time"

// DefaultEnv is the environment variable [New] reads the MongoDB connection URI from.
const DefaultEnv = "MONGO_URI"

// DefaultDatabasePrefix starts the name of every throwaway database [New] creates.
const DefaultDatabasePrefix = "it_"

// DefaultReadinessTimeout bounds how long [New] waits for a new client's first server handshake. A loaded integration suite can
// delay the handshake well past the fixed ping of [datamongo.Mongo.Connect]; a server that never answers still fails the test.
const DefaultReadinessTimeout = 2 * time.Minute

// DefaultStaleAge is the age past which [New] drops a leftover throwaway database of its prefix: one whose run was killed before
// its cleanup. It must outlast the longest test run sharing the server, so a concurrent run never loses its databases.
const DefaultStaleAge = 2 * time.Hour

// options configures [New].
type options struct {
	// env names the environment variable holding the connection URI; the test is skipped when it is unset or empty.
	env string `optgen:"default=DefaultEnv"`
	// databasePrefix starts the throwaway database name, followed by its creation time and 8 random hex digits.
	databasePrefix string `optgen:"default=DefaultDatabasePrefix"`
	// readinessTimeout bounds the wait for the client's first server handshake before Connect runs.
	readinessTimeout time.Duration `optval:"positive" optgen:"default=DefaultReadinessTimeout"`
	// staleAge is the age past which the first New of the process for a URI and prefix drops a throwaway database of that prefix;
	// zero disables the sweep.
	staleAge time.Duration `optval:"positive=allow_zero" optgen:"default=DefaultStaleAge"`
}
