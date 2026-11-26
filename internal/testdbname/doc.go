// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package testdbname defines the name grammar and staleness rule of throwaway test databases, shared by the Mongo and PostgreSQL test
// helpers: <prefix><creation time as 8 hex digits of Unix seconds>_<8 random hex digits>.
package testdbname
