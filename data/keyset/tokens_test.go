// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
)

type testPosition struct {
	CreatedAt int64  `json:"c"`
	ID        string `json:"i"`
}

func testBindings() keyset.Bindings {
	return keyset.Bindings{
		Sort:   keyset.Fingerprint([]byte("created_at:-1")),
		Filter: keyset.Fingerprint([]byte(`{"tenant":"acme"}`)),
	}
}

func TestTokensRoundTrip(t *testing.T) {
	t.Parallel()

	tokens := keyset.NewTokens[testPosition](nil)
	want := testPosition{CreatedAt: 1740830400000, ID: "row-42"}

	token, err := tokens.Issue(t.Context(), want, testBindings())
	require.NoError(t, err)

	got, err := tokens.Resolve(t.Context(), token, testBindings())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// The same Bindings value serves both directions; a query that moved out
// from under the cursor is rejected with the keyset sentinels.
func TestTokensChecksBindings(t *testing.T) {
	t.Parallel()

	tokens := keyset.NewTokens[testPosition](nil)

	token, err := tokens.Issue(t.Context(), testPosition{ID: "row-42"}, testBindings())
	require.NoError(t, err)

	changedFilter := testBindings()
	changedFilter.Filter = keyset.Fingerprint([]byte(`{"tenant":"other"}`))

	_, err = tokens.Resolve(t.Context(), token, changedFilter)
	require.ErrorIs(t, err, keyset.ErrFilterChanged)

	changedSort := testBindings()
	changedSort.Sort = keyset.Fingerprint([]byte("created_at:1"))

	_, err = tokens.Resolve(t.Context(), token, changedSort)
	require.ErrorIs(t, err, keyset.ErrSortChanged)
}

// Bindings.Subject is recorded at issue: the cursor becomes that
// principal's alone.
func TestTokensSubjectBinding(t *testing.T) {
	t.Parallel()

	tokens := keyset.NewTokens[testPosition](nil)

	bound := testBindings()
	bound.Subject = "alice"

	token, err := tokens.Issue(t.Context(), testPosition{ID: "row-42"}, bound)
	require.NoError(t, err)

	_, err = tokens.Resolve(t.Context(), token, bound)
	require.NoError(t, err)

	other := bound
	other.Subject = "bob"

	_, err = tokens.Resolve(t.Context(), token, other)
	require.ErrorIs(t, err, keyset.ErrSubjectMismatch)
}

// A token minted for one position shape must not silently decode as
// another.
func TestTokensRejectsForeignPositionShape(t *testing.T) {
	t.Parallel()

	type otherShape struct {
		CreatedAt string `json:"c"` // same tag, incompatible type
	}

	token, err := keyset.NewTokens[testPosition](nil).
		Issue(t.Context(), testPosition{CreatedAt: 7, ID: "row"}, testBindings())
	require.NoError(t, err)

	_, err = keyset.NewTokens[otherShape](nil).Resolve(t.Context(), token, testBindings())

	require.ErrorIs(t, err, keyset.ErrInvalidToken)
}

// Release without a storage is a documented no-op, so callers need not
// branch on the mode.
func TestTokensReleaseIsNoOpWithoutStorage(t *testing.T) {
	t.Parallel()

	tokens := keyset.NewTokens[testPosition](nil)

	token, err := tokens.Issue(t.Context(), testPosition{ID: "row-42"}, testBindings())
	require.NoError(t, err)

	require.NoError(t, tokens.Release(t.Context(), token))
}

// Length prefixes keep two different field sets from rendering to the same
// bytes; order matters because an ordering is part of what is fingerprinted.
func TestFingerprintBuilder(t *testing.T) {
	t.Parallel()

	sum := func(build func(*keyset.FingerprintBuilder)) string {
		fp := keyset.NewFingerprintBuilder()
		build(fp)

		return fp.Sum()
	}

	base := sum(func(fp *keyset.FingerprintBuilder) { fp.Field("a", "bc").Field("d", "") })

	require.Equal(t, base,
		sum(func(fp *keyset.FingerprintBuilder) { fp.Field("a", "bc").Field("d", "") }),
		"identical fields must fingerprint alike")

	require.NotEqual(t, base,
		sum(func(fp *keyset.FingerprintBuilder) { fp.Field("a", "b").Field("cd", "") }),
		"value bytes must not bleed into the next key")

	require.NotEqual(t, base,
		sum(func(fp *keyset.FingerprintBuilder) { fp.Field("d", "").Field("a", "bc") }),
		"field order is part of the fingerprint")
}

func TestFingerprintBuilderTimePtr(t *testing.T) {
	t.Parallel()

	sum := func(v *time.Time) string {
		fp := keyset.NewFingerprintBuilder()
		fp.TimePtr("start", v)

		return fp.Sum()
	}

	require.NotEqual(t, sum(nil), sum(&time.Time{}), "unset and the zero time are different bounds")

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.Equal(t, sum(&at), sum(&at))
}

func TestFingerprintJSON(t *testing.T) {
	t.Parallel()

	first, err := keyset.FingerprintJSON(map[string]any{"b": 2, "a": 1})
	require.NoError(t, err)

	second, err := keyset.FingerprintJSON(map[string]any{"a": 1, "b": 2})
	require.NoError(t, err)
	require.Equal(t, first, second, "JSON encoding sorts map keys, so equal maps fingerprint alike")

	_, err = keyset.FingerprintJSON(map[string]any{"ch": make(chan int)})
	require.Error(t, err, "an unencodable filter must not silently share a fingerprint")
}
