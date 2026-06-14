// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    etag.Tag
		wantErr bool
	}{
		{name: "strong", input: `"abc"`, want: etag.Strong("abc")},
		{name: "weak", input: `W/"abc"`, want: etag.Weak("abc")},
		{name: "empty value", input: `""`, want: etag.Strong("")},
		{name: "surrounding spaces", input: `  "abc"  `, want: etag.Strong("abc")},
		{name: "obs-text byte", input: "\"a\x80b\"", want: etag.Strong("a\x80b")},

		{name: "missing quotes", input: `abc`, wantErr: true},
		{name: "unterminated", input: `"abc`, wantErr: true},
		{name: "lowercase weak indicator", input: `w/"abc"`, wantErr: true},
		{name: "embedded quote", input: `"a"b"`, wantErr: true},
		{name: "control char", input: "\"a\x01b\"", wantErr: true},
		{name: "trailing junk", input: `"abc"x`, wantErr: true},
		{name: "empty input", input: ``, wantErr: true},
		{name: "weak without tag", input: `W/`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := etag.Parse(tc.input)
			if tc.wantErr {
				require.ErrorIs(t, err, etag.ErrInvalidTag)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.want.String(), got.String())
		})
	}
}

func TestParseList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantTags []etag.Tag
		wantStar bool
		wantErr  bool
	}{
		{name: "single", input: `"a"`, wantTags: []etag.Tag{etag.Strong("a")}},
		{
			name:     "multiple",
			input:    `"a", W/"b" ,"c"`,
			wantTags: []etag.Tag{etag.Strong("a"), etag.Weak("b"), etag.Strong("c")},
		},
		{name: "star", input: `*`, wantStar: true},
		{name: "star with spaces", input: `  *  `, wantStar: true},
		{name: "trailing comma tolerated", input: `"a",`, wantTags: []etag.Tag{etag.Strong("a")}},

		{name: "empty", input: ``, wantErr: true},
		{name: "missing separator", input: `"a" "b"`, wantErr: true},
		{name: "malformed member", input: `"a", bad`, wantErr: true},
		{name: "star is not a list member", input: `"a", *`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tags, star, err := etag.ParseList(tc.input)
			if tc.wantErr {
				require.ErrorIs(t, err, etag.ErrInvalidTag)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantStar, star)
			require.Equal(t, tc.wantTags, tags)
		})
	}
}

func TestParseErrorIsInvalidTag(t *testing.T) {
	t.Parallel()
	_, err := etag.Parse("nope")
	require.True(t, errors.Is(err, etag.ErrInvalidTag))
}
