// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler/internal/storagetest"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

func TestFinishRun(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.FinishRun(t, store)
}

func TestReplaceTaskIf(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.ReplaceTaskIf(t, store)
}

func TestCreateTask(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.CreateTask(t, store)
}

func TestRenewRun(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.RenewRun(t, store)
}

func TestClaimRunRequiresFinishedRun(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.ClaimRunRequiresFinishedRun(t, store)
}

func TestClaimRunFencesOccurrence(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.ClaimRunFencesOccurrence(t, store)
}

func TestRunIDRoundTrip(t *testing.T) {
	t.Parallel()
	store, err := memory.New(10)
	require.NoError(t, err)
	storagetest.RunIDRoundTrip(t, store)
}
