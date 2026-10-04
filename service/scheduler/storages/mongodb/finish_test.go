// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongodb_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/service/scheduler/internal/storagetest"
)

func TestFinishRun(t *testing.T) {
	t.Parallel()
	store := newClaimIT(t)
	storagetest.FinishRun(t, store)
}

func TestReplaceTaskIf(t *testing.T) {
	t.Parallel()
	store := newClaimIT(t)
	storagetest.ReplaceTaskIf(t, store)
}
