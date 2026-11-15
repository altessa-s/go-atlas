// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas/store/mongo/keyset"
)

var _ keyset.TokenCodec = (*pagetoken.Codec)(nil)

type tokenCodecFake struct{ err error }

func (f tokenCodecFake) Issue([]byte, pagetoken.Bindings) (string, error) {
	return "", f.err
}

func (f tokenCodecFake) Resolve(string, pagetoken.Bindings) ([]byte, error) {
	return nil, f.err
}

func TestFindPage_UsesTokenCodecPort(t *testing.T) {
	t.Parallel()

	boom := errors.New("codec failure")
	_, err := keyset.FindPage[bson.M](t.Context(), nil, tokenCodecFake{err: boom}, keyset.Query{After: "token"})
	require.ErrorIs(t, err, boom)

	_, err = keyset.FindPage[bson.M](t.Context(), nil, (*pagetoken.Codec)(nil), keyset.Query{After: "token"})
	require.ErrorIs(t, err, keyset.ErrTokensRequired)
}
