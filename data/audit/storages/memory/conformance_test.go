// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/data/audit/storages/storagetest"
)

func TestConformance(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(*testing.T) audit.Storage { return memory.New() }, nil)
}
