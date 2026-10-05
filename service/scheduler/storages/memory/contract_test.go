// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
	"github.com/altessa-s/go-atlas/service/scheduler/storagetest"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	for name, check := range map[string]func(*testing.T, scheduler.Storage){
		"claim_run":  storagetest.ClaimRun,
		"due_tasks":  storagetest.DueTasks,
		"identity":   storagetest.Identity,
		"pagination": storagetest.Pagination,
		"history":    storagetest.History,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, err := memory.New(100)
			require.NoError(t, err)
			check(t, store)
		})
	}
}
