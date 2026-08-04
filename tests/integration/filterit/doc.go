// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package filterit holds the shared corpus the data/filter integration suite runs against every backend.
//
// The package itself is data: a seven-row dataset, a list of filter expressions with the rows each must
// select, and a list of expressions that must be refused with a named sentinel. The backend adapters that
// load the dataset and execute the translated query live in the _test.go files beside it, one per target.
//
// # Why one corpus
//
// A translator can pass every unit test — matching a golden string byte for byte — and still be wrong.
// The string may be a syntax error, name a function the server does not have, or type-mismatch on the
// wire. Running one dataset and one set of expressions through all six backends catches that, and turns
// the places where the backends genuinely disagree into assertions instead of folklore.
//
// # Divergence, skips and gaps
//
// Case.Differs records a backend that returns a different but correct answer — MariaDB's default
// collation folds case, so `name == "Alice"` also matches "alice". Case.Skip records a backend the case
// does not apply to at all, each with its reason, and the reasons fall into three kinds:
//
//   - The operation has no counterpart in the backend's query language (RediSearch has no regex predicate).
//   - The backend imposes a limit of its own (RediSearch drops a prefix shorter than MINPREFIX).
//   - The backend cannot represent the question at all (RediSearch has no null).
//
// A fourth kind used to be here — a gap in a translator — and is the reason the suite exists. The first
// full run turned up three: a bare boolean identifier that three translators failed to render, an `in`
// that ignored the RediSearch field schema, and a Meilisearch `!= null` that matched every document
// including the ones without the attribute. All three are fixed, and the cases that exposed them are
// ordinary corpus entries now.
package filterit
