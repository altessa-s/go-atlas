// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow

//go:generate go tool optgen generate

import "time"

// DefaultCompensationTimeout bounds the entire post-commit rollback sequence.
const DefaultCompensationTimeout = 30 * time.Second

type options struct {
	compensationTimeout time.Duration `optgen:"default=DefaultCompensationTimeout" optval:"positive"`

	// WithDetector overrides detection of active transactions and their identity.
	// A nil detector uses the committer's [TransactionDetector] when provided.
	detector Detector

	// runHooksOutsideTransaction makes [Runner.AfterCommit] run its hook at once when the context carries
	// no unit of work and no transaction, instead of returning [ErrNoUnitOfWork].
	runHooksOutsideTransaction bool
}
