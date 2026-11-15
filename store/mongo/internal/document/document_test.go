// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
)

func TestEncryptionBoundary(t *testing.T) {
	t.Parallel()
	type entity struct {
		Secret string `bson:"secret" encryption:"deterministic,key"`
	}
	input := entity{}
	t.Run("reject without encryptor", func(t *testing.T) {
		t.Parallel()
		set, unset, err := document.New(document.NewParser(), nil).Convert(t.Context(), input, false)
		require.ErrorIs(t, err, document.ErrEncryptionNotEnabled)
		require.Nil(t, set)
		require.Nil(t, unset)
	})
	t.Run("encrypt zero value", func(t *testing.T) {
		t.Parallel()
		calls := 0
		encrypted := &bson.Binary{Subtype: 6, Data: []byte("ciphertext")}
		engine := document.New(document.NewParser(), func(ctx context.Context, value any, algorithm, key string) (*bson.Binary, error) {
			calls++
			require.Equal(t, t.Context(), ctx)
			require.Equal(t, "", value)
			require.Equal(t, "deterministic", algorithm)
			require.Equal(t, "key", key)
			return encrypted, nil
		})
		set, unset, err := engine.Convert(t.Context(), input, false)
		require.NoError(t, err)
		require.Equal(t, bson.M{"secret": encrypted}, set)
		require.Empty(t, unset)
		require.Equal(t, 1, calls)
	})
	t.Run("propagate encryptor error", func(t *testing.T) {
		t.Parallel()
		failure := errors.New("encryptor failed")
		engine := document.New(document.NewParser(), func(context.Context, any, string, string) (*bson.Binary, error) {
			return nil, failure
		})
		set, unset, err := engine.Convert(t.Context(), input, true)
		require.ErrorIs(t, err, failure)
		require.Nil(t, set)
		require.Nil(t, unset)
	})
}

func TestNestedUpdateReplacement(t *testing.T) {
	t.Parallel()
	type child struct {
		Name     string  `bson:"name"`
		Optional *string `bson:"optional"`
	}
	type entity struct {
		Child     *child  `bson:"child"`
		Removed   *string `bson:"removed"`
		Immutable string  `bson:"immutable,omitonupdate"`
	}
	set, unset, err := document.New(document.NewParser(), nil).Convert(t.Context(),
		entity{Child: &child{Name: "updated"}, Immutable: "keep"}, true)
	require.NoError(t, err)
	require.Equal(t, bson.M{"child": bson.M{"name": "updated"}}, set)
	require.Equal(t, bson.M{"removed": nil}, unset)
}
