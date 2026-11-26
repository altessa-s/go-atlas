// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	atlas "github.com/altessa-s/go-atlas/store/mongo"
	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
)

func TestEncryptionBeforeConnect(t *testing.T) {
	t.Parallel()

	provider, err := kmslocal.New(kmslocal.WithMasterKey(strings.Repeat("k", kmslocal.RequiredMasterKeyLength)))
	require.NoError(t, err)
	t.Cleanup(provider.Clear)
	m, err := atlas.New("db", atlas.WithEncryptionEnabled(), atlas.WithKMS(provider))
	require.NoError(t, err)
	t.Run("encrypt", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() {
			_, err := m.Encrypt(t.Context(), "value", atlas.EncryptionAlgDeterministic, "key")
			require.ErrorIs(t, err, atlas.ErrEncryptionNotEnabled)
		})
	})
	t.Run("data key", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() {
			_, err := m.DataKey(t.Context(), "key", false)
			require.ErrorIs(t, err, atlas.ErrEncryptionNotEnabled)
		})
	})
	t.Run("create data key", func(t *testing.T) {
		t.Parallel()
		require.NotPanics(t, func() {
			_, err := m.CreateDataKey(t.Context(), "key")
			require.ErrorIs(t, err, atlas.ErrEncryptionNotEnabled)
		})
	})
}
