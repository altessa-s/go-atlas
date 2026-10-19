// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
)

type position struct {
	Time int64  `json:"t"`
	ID   string `json:"i"`
}

func TestNewBindings_LengthPrefixesFilterParts(t *testing.T) {
	t.Parallel()

	b := keyset.NewBindings("list", "name:1", "ab", "c")
	assert.Equal(t, keyset.Bindings{Subject: "list", Sort: "name:1", Filter: "2:ab1:c"}, b)
	assert.NotEqual(t, b, keyset.NewBindings("list", "name:1", "a", "bc"))
	assert.Equal(t, keyset.Bindings{Subject: "list"}, keyset.NewBindings("list", ""))
	// The layout is persisted in issued tokens: an empty part and a multi-digit length stay framed as before.
	assert.Equal(t, "0:10:0123456789", keyset.NewBindings("list", "", "", "0123456789").Filter)
}

func TestTyped_RoundTrip(t *testing.T) {
	t.Parallel()

	typed := keyset.NewTyped[position](newCodec(t))
	token, err := typed.Issue(position{Time: 42, ID: "x"}, bindings)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, first, err := typed.Resolve(token, bindings)
	require.NoError(t, err)
	assert.False(t, first)
	assert.Equal(t, position{Time: 42, ID: "x"}, got)
}

func TestTyped_EmptyTokenIsFirstPage(t *testing.T) {
	t.Parallel()

	for name, typed := range map[string]keyset.Typed[position]{
		"codec":    keyset.NewTyped[position](newCodec(t)),
		"no codec": keyset.NewTyped[position](nil),
		"zero":     {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, first, err := typed.Resolve("", bindings)
			require.NoError(t, err)
			assert.True(t, first)
			assert.Zero(t, got)
		})
	}
}

func TestTyped_WithoutCodec(t *testing.T) {
	t.Parallel()

	typed := keyset.NewTyped[int](nil)
	token, err := typed.Issue(1, bindings)
	require.NoError(t, err)
	assert.Empty(t, token, "a list without a codec serves a single page")

	signed, err := keyset.NewTyped[int](newCodec(t)).Issue(1, bindings)
	require.NoError(t, err)
	_, first, err := typed.Resolve(signed, bindings)
	require.ErrorIs(t, err, keyset.ErrCodecRequired)
	assert.False(t, first)
	assert.Equal(t, keyset.FailureMalformed, keyset.Classify(err))
}

func TestTyped_Rejected(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	typed := keyset.NewTyped[int](c)
	token, err := typed.Issue(7, bindings)
	require.NoError(t, err)
	notJSON, err := c.Issue([]byte("{"), bindings)
	require.NoError(t, err)
	otherType, err := keyset.NewTyped[string](c).Issue("seven", bindings)
	require.NoError(t, err)

	tests := map[string]struct {
		token string
		b     keyset.Bindings
		want  error
	}{
		"garbage":        {token: "!!!", b: bindings, want: keyset.ErrInvalidToken},
		"not json":       {token: notJSON, b: bindings, want: keyset.ErrInvalidToken},
		"other type":     {token: otherType, b: bindings, want: keyset.ErrInvalidToken},
		"sort changed":   {token: token, b: keyset.Bindings{Sort: "x", Filter: bindings.Filter, Subject: bindings.Subject}, want: keyset.ErrSortChanged},
		"filter changed": {token: token, b: keyset.Bindings{Sort: bindings.Sort, Filter: "x", Subject: bindings.Subject}, want: keyset.ErrFilterChanged},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, first, err := typed.Resolve(tc.token, tc.b)
			require.ErrorIs(t, err, tc.want)
			assert.Zero(t, got)
			assert.False(t, first)
			assert.Equal(t, keyset.FailureMalformed, keyset.Classify(err))
		})
	}
}

func TestTyped_IssueErrors(t *testing.T) {
	t.Parallel()

	_, err := keyset.NewTyped[func()](newCodec(t)).Issue(func() {}, bindings)
	require.Error(t, err, "an unmarshalable cursor fails")

	_, err = keyset.NewTyped[string](newCodec(t)).Issue(string(make([]byte, keyset.MaxPayloadLength)), bindings)
	require.ErrorIs(t, err, keyset.ErrPayloadTooLarge)
}

func TestOffset(t *testing.T) {
	t.Parallel()

	c := newCodec(t)
	off := keyset.NewOffset(c)
	token, err := off.Issue(25, bindings)
	require.NoError(t, err)
	got, err := off.Resolve(token, bindings)
	require.NoError(t, err)
	assert.Equal(t, 25, got)

	got, err = off.Resolve("", bindings)
	require.NoError(t, err)
	assert.Zero(t, got, "an empty token starts at offset 0")

	_, err = off.Issue(-1, bindings)
	require.ErrorIs(t, err, keyset.ErrNegativeOffset)
	assert.Equal(t, keyset.FailureNone, keyset.Classify(err), "a negative offset is a caller bug, not a token failure")

	negative, err := keyset.NewTyped[int](c).Issue(-3, bindings)
	require.NoError(t, err)
	_, err = off.Resolve(negative, bindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken)

	empty, err := keyset.NewOffset(nil).Issue(5, bindings)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestClassify(t *testing.T) {
	t.Parallel()

	wrap := func(err error) error { return fmt.Errorf("list: %w", err) }
	tests := map[string]struct {
		err  error
		want keyset.Failure
	}{
		"nil":              {err: nil, want: keyset.FailureNone},
		"other":            {err: errors.New("boom"), want: keyset.FailureNone},
		"invalid":          {err: wrap(keyset.ErrInvalidToken), want: keyset.FailureMalformed},
		"sort changed":     {err: wrap(keyset.ErrSortChanged), want: keyset.FailureMalformed},
		"filter changed":   {err: wrap(keyset.ErrFilterChanged), want: keyset.FailureMalformed},
		"subject mismatch": {err: wrap(keyset.ErrSubjectMismatch), want: keyset.FailureMalformed},
		"codec required":   {err: wrap(keyset.ErrCodecRequired), want: keyset.FailureMalformed},
		"expired":          {err: wrap(keyset.ErrExpiredToken), want: keyset.FailureExpired},
		"payload too big":  {err: wrap(keyset.ErrPayloadTooLarge), want: keyset.FailureNone},
		"weak key":         {err: wrap(keyset.ErrWeakKey), want: keyset.FailureNone},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, keyset.Classify(tc.err))
		})
	}
	assert.Equal(t, "malformed", keyset.FailureMalformed.String())
	assert.Equal(t, "expired", keyset.FailureExpired.String())
	assert.Empty(t, keyset.FailureNone.String())
}

func TestTyped_Expired(t *testing.T) {
	t.Parallel()

	issued := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	token, err := keyset.NewTyped[int](newCodec(t, keyset.WithClock(fixedClock(issued)))).Issue(1, bindings)
	require.NoError(t, err)

	later := keyset.NewTyped[int](newCodec(t, keyset.WithClock(fixedClock(issued.Add(keyset.DefaultTTL+time.Second)))))
	_, _, err = later.Resolve(token, bindings)
	require.ErrorIs(t, err, keyset.ErrExpiredToken)
	assert.Equal(t, keyset.FailureExpired, keyset.Classify(err))
}
