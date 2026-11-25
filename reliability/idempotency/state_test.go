// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/idempotency"
)

func TestState_Accessors(t *testing.T) {
	t.Parallel()
	s := &idempotency.State{Status: idempotency.StatusInProgress}
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
	var s *idempotency.State
	s.SetLockToken([]byte("x"))
	s.SetFingerprint([]byte("x"))
	s.SetOwner("x")
	require.Nil(t, s.LockToken())
	require.Nil(t, s.Fingerprint())
	require.Empty(t, s.Owner())
}

func TestState_PrivateFieldsAreNotSerialized(t *testing.T) {
	t.Parallel()
	s := &idempotency.State{Status: idempotency.StatusSuccess, Data: "d"}
	s.SetLockToken([]byte("tok"))
	s.SetOwner("node-1")
	s.SetFingerprint([]byte("fp"))

	raw, err := json.Marshal(s)
	require.NoError(t, err)

	var back idempotency.State
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, idempotency.StatusSuccess, back.Status)
	require.Equal(t, "d", back.Data)
	require.Nil(t, back.LockToken())
	require.Empty(t, back.Owner())
	require.Nil(t, back.Fingerprint())
}
