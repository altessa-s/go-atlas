// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
)

func TestState_Accessors(t *testing.T) {
	t.Parallel()
	s := &storages.State{Status: storages.StatusInProgress}
	require.Nil(t, s.LockToken())
	require.Nil(t, s.Fingerprint())
	require.Empty(t, s.Owner())

	s.SetLockToken([]byte("tok"))
	s.SetOwner("node-1")
	fp := []byte("fp")
	s.SetFingerprint(fp)
	fp[0] = 'X' // the state keeps its own copy

	require.Equal(t, []byte("tok"), s.LockToken())
	require.Equal(t, "node-1", s.Owner())
	got := s.Fingerprint()
	require.Equal(t, []byte("fp"), got)
	got[0] = 'Y' // and hands out copies
	require.Equal(t, []byte("fp"), s.Fingerprint())
}

func TestState_NilReceiverIsSafe(t *testing.T) {
	t.Parallel()
	var s *storages.State
	s.SetLockToken([]byte("x"))
	s.SetFingerprint([]byte("x"))
	s.SetOwner("x")
	require.Nil(t, s.LockToken())
	require.Nil(t, s.Fingerprint())
	require.Empty(t, s.Owner())
}

func TestState_PrivateFieldsAreNotSerialized(t *testing.T) {
	t.Parallel()
	s := &storages.State{Status: storages.StatusSuccess, Data: "d"}
	s.SetLockToken([]byte("tok"))
	s.SetOwner("node-1")
	s.SetFingerprint([]byte("fp"))

	raw, err := json.Marshal(s)
	require.NoError(t, err)

	var back storages.State
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, storages.StatusSuccess, back.Status)
	require.Equal(t, "d", back.Data)
	require.Nil(t, back.LockToken())
	require.Empty(t, back.Owner())
	require.Nil(t, back.Fingerprint())
}
