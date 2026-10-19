// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

func TestFromVersion(t *testing.T) {
	t.Parallel()

	for v, want := range map[int64]string{0: `W/"0"`, 1: `W/"1"`, 42: `W/"42"`, math.MaxInt64: `W/"9223372036854775807"`} {
		tag := etag.FromVersion(v)
		assert.Equal(t, want, tag.String())
		assert.True(t, tag.IsWeak())
		got, err := tag.Version()
		require.NoError(t, err)
		assert.Equal(t, v, got)

		parsed, err := etag.Parse(want)
		require.NoError(t, err)
		got, err = parsed.Version()
		require.NoError(t, err)
		assert.Equal(t, v, got, "a parsed version tag round-trips")
	}
	assert.True(t, etag.FromVersion(-1).IsZero(), "a negative version is absent")
}

func TestVersion_Rejects(t *testing.T) {
	t.Parallel()

	tests := map[string]etag.Tag{
		"zero tag":     {},
		"strong":       etag.Strong("7"),
		"non-numeric":  etag.Weak("x"),
		"negative":     etag.Weak("-1"),
		"plus sign":    etag.Weak("+7"),
		"leading zero": etag.Weak("07"),
		"overflow":     etag.Weak("9223372036854775808"),
		"decimal":      etag.Weak("1.5"),
		"space":        etag.Weak("1 "),
	}
	for name, tag := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v, err := tag.Version()
			require.ErrorIs(t, err, etag.ErrNotVersion)
			assert.Zero(t, v)
		})
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		singleton bool
		want      int64
		err       error // nil when valid
	}{
		{"weak", `W/"7"`, false, 7, nil},
		{"weak singleton", `W/"7"`, true, 7, nil},
		{"unsaved singleton", `W/"0"`, true, 0, nil},
		{"zero not singleton", `W/"0"`, false, 0, etag.ErrMalformed},
		{"max minus one", `W/"9223372036854775806"`, false, 9223372036854775806, nil},
		{"empty", "", false, 0, etag.ErrMissing},
		{"empty singleton", "", true, 0, etag.ErrMissing},
		{"bare number", "7", false, 0, etag.ErrMalformed},
		{"bare zero singleton", "0", true, 0, etag.ErrMalformed},
		{"strong", `"7"`, false, 0, etag.ErrMalformed},
		{"empty weak", `W/""`, false, 0, etag.ErrMalformed},
		{"non-numeric", `W/"x"`, false, 0, etag.ErrMalformed},
		{"negative", `W/"-1"`, false, 0, etag.ErrMalformed},
		{"max int64", `W/"9223372036854775807"`, false, 0, etag.ErrMalformed},
		{"overflow", `W/"9223372036854775808"`, false, 0, etag.ErrMalformed},
		{"plus sign", `W/"+7"`, false, 0, etag.ErrMalformed},
		{"leading zero", `W/"07"`, false, 0, etag.ErrMalformed},
		{"whitespace", ` W/"7" `, false, 0, etag.ErrMalformed},
		{"lowercase prefix", `w/"7"`, false, 0, etag.ErrMalformed},
		{"trailing content", `W/"7"x`, false, 0, etag.ErrMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var opts []etag.VersionOption
			if tc.singleton {
				opts = append(opts, etag.WithSingleton())
			}
			got, err := etag.ParseVersion(tc.in, opts...)
			if tc.err == nil {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				return
			}
			require.ErrorIs(t, err, tc.err)
			assert.Zero(t, got)
		})
	}
}

func TestParseVersion_RoundTripsFromVersion(t *testing.T) {
	t.Parallel()

	for _, v := range []int64{0, 1, 42} {
		got, err := etag.ParseVersion(etag.FromVersion(v).String(), etag.WithSingleton())
		require.NoError(t, err)
		assert.Equal(t, v, got)
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()

	require.NoError(t, etag.CheckVersion(`W/"3"`, 3))
	require.NoError(t, etag.CheckVersion(`W/"0"`, 0, etag.WithSingleton()))

	err := etag.CheckVersion(`W/"2"`, 3)
	require.ErrorIs(t, err, etag.ErrMismatch)
	require.ErrorIs(t, err, coreerrs.ErrConflict, "a mismatch is an optimistic-concurrency conflict")

	require.ErrorIs(t, etag.CheckVersion("", 3), etag.ErrMissing)
	require.ErrorIs(t, etag.CheckVersion(`"3"`, 3), etag.ErrMalformed)
	require.ErrorIs(t, etag.CheckVersion(`W/"0"`, 0), etag.ErrMalformed)
	require.NotErrorIs(t, etag.CheckVersion(`"3"`, 3), coreerrs.ErrConflict)
}
