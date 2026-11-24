// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest

//go:generate go tool optgen generate --no-default-modifiers

import "time"

// DefaultEnv is the environment variable [New] reads the PostgreSQL DSN from; the go-atlas integration suite reads the same one.
const DefaultEnv = "POSTGRES_DSN"

// DefaultDatabasePrefix starts the name of every throwaway database [New] creates.
const DefaultDatabasePrefix = "it_"

// DefaultReadinessTimeout bounds how long [New] waits for the server to answer: a container still starting refuses connections for a
// moment; a server that never answers still fails the test.
const DefaultReadinessTimeout = time.Minute

// DefaultStaleAge is the minimum age at which New checks a marked database for abandonment.
const DefaultStaleAge = 2 * time.Hour

// options configures [New].
type options struct {
	// WithEnv names the environment variable holding the DSN; the test is skipped when it is unset or empty.
	env string `optgen:"default=DefaultEnv"`
	// WithDatabasePrefix starts the throwaway database name, followed by its creation time and 8 random hex digits. It must be a lower
	// case identifier.
	databasePrefix string `optgen:"default=DefaultDatabasePrefix"`
	// WithReadinessTimeout bounds the wait for the server to answer the first ping.
	readinessTimeout time.Duration `optval:"positive" optgen:"default=DefaultReadinessTimeout"`
	// WithStaleAge is the age past which New sweeps a throwaway database of that prefix;
	// zero disables the sweep.
	staleAge time.Duration `optval:"positive=allow_zero" optgen:"default=DefaultStaleAge"`
}
