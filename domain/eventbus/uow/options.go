// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

// DefaultCompensationTimeout bounds the entire post-commit rollback sequence.
const DefaultCompensationTimeout = 30 * time.Second

type options struct {
	compensationTimeout time.Duration `optgen:"default=DefaultCompensationTimeout" optval:"positive"`
}
