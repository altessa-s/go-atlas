// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/data/projection/translators/mongo"
)

func newTranslator(tb testing.TB, opts ...projection.TranslatorOption) *mongo.Translator {
	tb.Helper()
	tr, err := mongo.NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []projection.TranslatorOption
		paths   []string
		want    bson.M
		wantErr error
	}{
		{name: "all fields without policy", want: nil},
		{
			name:  "inclusion suppresses _id",
			paths: []string{"name", "email"},
			want:  bson.M{"name": int32(1), "email": int32(1), "_id": int32(0)},
		},
		{
			name:  "required _id is kept",
			opts:  []projection.TranslatorOption{projection.WithRequiredFields("_id")},
			paths: []string{"name"},
			want:  bson.M{"name": int32(1), "_id": int32(1)},
		},
		{
			name:  "_id subpath keeps _id unsuppressed",
			paths: []string{"_id.tenant"},
			want:  bson.M{"_id.tenant": int32(1)},
		},
		{
			name:  "parent and child collapse",
			paths: []string{"address", "address.city", "addressee"},
			want:  bson.M{"address": int32(1), "addressee": int32(1), "_id": int32(0)},
		},
		{
			name: "collision created by mapping collapses",
			opts: []projection.TranslatorOption{projection.WithFieldMapping(map[string]string{
				"city": "addr.city", "address": "addr",
			})},
			paths: []string{"city", "address"},
			want:  bson.M{"addr": int32(1), "_id": int32(0)},
		},
		{
			name:  "collision created by required field collapses",
			opts:  []projection.TranslatorOption{projection.WithRequiredFields("meta")},
			paths: []string{"meta.version"},
			want:  bson.M{"meta": int32(1), "_id": int32(0)},
		},
		{
			name: "empty request excludes denied fields",
			opts: []projection.TranslatorOption{
				projection.WithDeniedFields("password"),
				projection.WithDeniedStorageFields("tokens", "tokens.refresh"),
			},
			want: bson.M{"password": int32(0), "tokens": int32(0)},
		},
		{
			name: "empty request uses allow-list roots",
			opts: []projection.TranslatorOption{projection.WithAllowedFields("name", "address.*")},
			want: bson.M{"name": int32(1), "address": int32(1), "_id": int32(0)},
		},
		{
			name:    "denied ancestor request rejected",
			opts:    []projection.TranslatorOption{projection.WithDeniedFields("credentials.password")},
			paths:   []string{"credentials"},
			wantErr: projection.ErrFieldNotAllowed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := newTranslator(t, tc.opts...).Translate(projection.Spec{Paths: tc.paths})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNewTranslatorPropagatesPolicyErrors(t *testing.T) {
	t.Parallel()

	_, err := mongo.NewTranslator(projection.WithUntrustedInput())
	require.ErrorIs(t, err, projection.ErrAllowlistRequired)
}

func TestTranslateReturnsFreshDocument(t *testing.T) {
	t.Parallel()

	tr := newTranslator(t, projection.WithAllowedFields("name"))
	first, err := tr.Translate(projection.Spec{})
	require.NoError(t, err)
	first["injected"] = int32(1)

	second, err := tr.Translate(projection.Spec{})
	require.NoError(t, err)
	require.NotContains(t, second, "injected")
}
