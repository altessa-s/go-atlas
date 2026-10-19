// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=verbOptions --option-type=VerbOption --output=verb_options_gen.go

// verbOptions tunes [VerbMethodClassifier]. Configure it through the
// generated [VerbOption] values.
type verbOptions struct {
	// skipVerbs are the method-name prefixes whose methods are skipped;
	// every other method gets the classifier's policy. Blank entries are
	// dropped.
	skipVerbs []string `optgen:"default=DefaultSkipVerbs" optval:"nonempty"`
}
