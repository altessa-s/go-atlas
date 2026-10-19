// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultEnv is the environment variable [New] reads the MongoDB connection URI from.
const DefaultEnv = "MONGO_URI"

// DefaultDatabasePrefix starts the name of every throwaway database [New] creates.
const DefaultDatabasePrefix = "it_"

// options configures [New].
type options struct {
	// env names the environment variable holding the connection URI; the test is skipped when it is unset or empty.
	env string `optgen:"default=DefaultEnv"`
	// databasePrefix starts the throwaway database name, followed by 8 random hex digits.
	databasePrefix string `optgen:"default=DefaultDatabasePrefix"`
}
