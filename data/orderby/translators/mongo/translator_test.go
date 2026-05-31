// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/data/orderby/translators/mongo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// mustTranslator builds a translator and fails the test on any
// construction error. Keeps the success-path tests free of error-wiring
// noise; tests that exercise construction failures call NewTranslator
// directly.
func mustTranslator(tb testing.TB, opts ...orderby.TranslatorOption) *mongo.Translator {
	tb.Helper()
	tr, err := mongo.NewTranslator(opts...)
	require.NoError(tb, err)
	return tr
}

func TestTranslate_Multi(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "create_time desc, slug")

	got, err := mustTranslator(t).Translate(ob)
	require.NoError(t, err)
	require.Equal(t,
		bson.D{
			{Key: "create_time", Value: int32(-1)},
			{Key: "slug", Value: int32(1)},
		},
		got,
	)
}

func TestTranslate_Empty(t *testing.T) {
	t.Parallel()
	got, err := mustTranslator(t).Translate(orderby.Spec{})
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestTranslate_FieldMapping(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc")

	got, err := mustTranslator(t, orderby.WithFieldMapping(map[string]string{
		"createdAt": "created_at",
	})).Translate(ob)
	require.NoError(t, err)
	require.Equal(t,
		bson.D{{Key: "created_at", Value: int32(-1)}},
		got,
	)
}

func TestTranslate_AllowedFields(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "createdAt desc, secret")

	_, err := mustTranslator(t, orderby.WithAllowedFields("createdAt")).Translate(ob)
	require.ErrorIs(t, err, orderby.ErrFieldNotAllowed)
}

// TestNewTranslator_UntrustedInput_RequiresAllowlist locks the contract
// that misconfiguration is rejected at construction, not at the first
// Translate call.
func TestNewTranslator_UntrustedInput_RequiresAllowlist(t *testing.T) {
	t.Parallel()

	_, err := mongo.NewTranslator(orderby.WithUntrustedInput())
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)

	_, err = mongo.NewTranslator(
		orderby.WithUntrustedInput(),
		orderby.WithAllowedFields("createdAt"),
	)
	require.NoError(t, err)
}

// TestNewTranslator_UntrustedInput_EmptyAllowlist guards the
// misconfiguration path where WithAllowedFields was called with no
// arguments (or with an empty slice). The check has to happen at
// construction so the bug surfaces during boot, not on the first
// untrusted query.
func TestNewTranslator_UntrustedInput_EmptyAllowlist(t *testing.T) {
	t.Parallel()

	_, err := mongo.NewTranslator(
		orderby.WithUntrustedInput(),
		orderby.WithAllowedFields(),
	)
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)

	var empty []string
	_, err = mongo.NewTranslator(
		orderby.WithUntrustedInput(),
		orderby.WithAllowedFields(empty...),
	)
	require.ErrorIs(t, err, orderby.ErrAllowlistRequired)
}

func TestTranslate_NestedField(t *testing.T) {
	t.Parallel()
	ob := testhelpers.MustParseOrderBy(t, "address.city asc")

	got, err := mustTranslator(t, orderby.WithAllowedFields("address.city")).Translate(ob)
	require.NoError(t, err)
	require.Equal(t,
		bson.D{{Key: "address.city", Value: int32(1)}},
		got,
	)
}

// TestTranslate_AllowedFieldsWildcard covers the `prefix.*` form of
// WithAllowedFields. The options are translator-agnostic, so exercising
// them through the Mongo translator is enough.
func TestTranslate_AllowedFieldsWildcard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expr    string
		allow   []string
		wantErr error
		want    bson.D
	}{
		{
			name:  "subtree allowed via wildcard",
			expr:  "address.city desc",
			allow: []string{"address.*"},
			want:  bson.D{{Key: "address.city", Value: int32(-1)}},
		},
		{
			name:  "deep subtree allowed via wildcard",
			expr:  "address.zip.code asc",
			allow: []string{"address.*"},
			want:  bson.D{{Key: "address.zip.code", Value: int32(1)}},
		},
		{
			name:    "bare prefix rejected — wildcard does not cover it",
			expr:    "address asc",
			allow:   []string{"address.*"},
			wantErr: orderby.ErrFieldNotAllowed,
		},
		{
			name:    "sibling not covered",
			expr:    "billing.city asc",
			allow:   []string{"address.*"},
			wantErr: orderby.ErrFieldNotAllowed,
		},
		{
			name:  "bare star matches anything",
			expr:  "any.thing.here asc",
			allow: []string{"*"},
			want:  bson.D{{Key: "any.thing.here", Value: int32(1)}},
		},
		{
			name:  "mix of exact and wildcard",
			expr:  "createdAt desc, address.city asc",
			allow: []string{"createdAt", "address.*"},
			want: bson.D{
				{Key: "createdAt", Value: int32(-1)},
				{Key: "address.city", Value: int32(1)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ob := testhelpers.MustParseOrderBy(t, tt.expr)
			got, err := mustTranslator(t, orderby.WithAllowedFields(tt.allow...)).Translate(ob)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestTranslate_FieldPrefixMapping covers WithFieldPrefixMapping: longest
// prefix wins, exact mapping still overrides prefix, malformed entries
// (no trailing dot) are silently dropped.
func TestTranslate_FieldPrefixMapping(t *testing.T) {
	t.Parallel()

	t.Run("rewrites leading segment", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "address.city desc")
		got, err := mustTranslator(t,
			orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "addr.city", Value: int32(-1)}}, got)
	})

	t.Run("rewrites deep path", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "address.zip.code asc")
		got, err := mustTranslator(t,
			orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "addr.zip.code", Value: int32(1)}}, got)
	})

	t.Run("longest prefix wins", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "user.profile.name desc")
		got, err := mustTranslator(t,
			orderby.WithFieldPrefixMapping(map[string]string{
				"user.":         "users.",
				"user.profile.": "users.prof.",
			}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "users.prof.name", Value: int32(-1)}}, got)
	})

	t.Run("exact mapping overrides prefix", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "address.city desc")
		got, err := mustTranslator(t,
			orderby.WithFieldMapping(map[string]string{"address.city": "addrCity"}),
			orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "addrCity", Value: int32(-1)}}, got)
	})

	t.Run("entries without trailing dot are silently ignored", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "address.city asc")
		got, err := mustTranslator(t,
			orderby.WithFieldPrefixMapping(map[string]string{"addr": "ad"}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "address.city", Value: int32(1)}}, got)
	})

	t.Run("non-matching path passes through unchanged", func(t *testing.T) {
		t.Parallel()
		ob := testhelpers.MustParseOrderBy(t, "billing.city desc")
		got, err := mustTranslator(t,
			orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
		).Translate(ob)
		require.NoError(t, err)
		require.Equal(t, bson.D{{Key: "billing.city", Value: int32(-1)}}, got)
	})
}
