// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package orderby_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
)

// mustContext builds a TranslatorContext and fails the test on any
// construction error. Keeps option-helper tests free of error-wiring
// noise; tests that exercise construction failures call
// NewTranslatorContext directly.
func mustContext(tb testing.TB, opts ...orderby.TranslatorOption) *orderby.TranslatorContext {
	tb.Helper()
	ctx, err := orderby.NewTranslatorContext(opts...)
	require.NoError(tb, err)
	return ctx
}

// TestApplyFieldMapping exercises each branch of ApplyFieldMapping:
// no-mapping passthrough, exact hit, prefix hit (longest first),
// missed prefix, no-match fallthrough.
func TestApplyFieldMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  []orderby.TranslatorOption
		field string
		want  string
	}{
		{
			name:  "no mapping passes through unchanged",
			field: "anything.here",
			want:  "anything.here",
		},
		{
			name:  "exact mapping hit",
			opts:  []orderby.TranslatorOption{orderby.WithFieldMapping(map[string]string{"createdAt": "created_at"})},
			field: "createdAt",
			want:  "created_at",
		},
		{
			name:  "exact mapping miss returns input unchanged",
			opts:  []orderby.TranslatorOption{orderby.WithFieldMapping(map[string]string{"createdAt": "created_at"})},
			field: "slug",
			want:  "slug",
		},
		{
			name:  "prefix mapping rewrites leading segment",
			opts:  []orderby.TranslatorOption{orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."})},
			field: "address.zip.code",
			want:  "addr.zip.code",
		},
		{
			name: "longest prefix wins",
			opts: []orderby.TranslatorOption{orderby.WithFieldPrefixMapping(map[string]string{
				"user.":         "users.",
				"user.profile.": "users.prof.",
			})},
			field: "user.profile.name",
			want:  "users.prof.name",
		},
		{
			name: "exact mapping overrides prefix",
			opts: []orderby.TranslatorOption{
				orderby.WithFieldMapping(map[string]string{"address.city": "addrCity"}),
				orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."}),
			},
			field: "address.city",
			want:  "addrCity",
		},
		{
			name:  "no prefix match returns input unchanged",
			opts:  []orderby.TranslatorOption{orderby.WithFieldPrefixMapping(map[string]string{"address.": "addr."})},
			field: "billing.city",
			want:  "billing.city",
		},
		{
			name:  "WithFieldPrefixMapping silently drops entries without trailing dot",
			opts:  []orderby.TranslatorOption{orderby.WithFieldPrefixMapping(map[string]string{"addr": "ad"})},
			field: "address.city",
			want:  "address.city",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := mustContext(t, tt.opts...)
			require.Equal(t, tt.want, ctx.ApplyFieldMapping(tt.field))
		})
	}
}

// TestIsFieldAllowed covers the allow-list decision matrix: no
// allow-list (fast path), exact-only allow-list, wildcard-only
// allow-list, and the mixed case where exact and wildcard collaborate.
func TestIsFieldAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  []orderby.TranslatorOption
		field string
		want  bool
	}{
		{
			name:  "no allow-list permits everything",
			field: "anything.here",
			want:  true,
		},
		{
			name:  "exact allow-list hit",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("createdAt", "slug")},
			field: "slug",
			want:  true,
		},
		{
			name:  "exact allow-list miss",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("createdAt", "slug")},
			field: "secret",
			want:  false,
		},
		{
			name:  "wildcard prefix matches subtree",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("address.*")},
			field: "address.zip.code",
			want:  true,
		},
		{
			name:  "wildcard prefix does not match bare parent",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("address.*")},
			field: "address",
			want:  false,
		},
		{
			name:  "bare asterisk matches anything",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("*")},
			field: "any.thing.at.all",
			want:  true,
		},
		{
			name:  "mixed exact + wildcard",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("slug", "address.*")},
			field: "address.city",
			want:  true,
		},
		{
			name:  "embedded asterisk treated as exact (never matches real keys)",
			opts:  []orderby.TranslatorOption{orderby.WithAllowedFields("foo.*.bar")},
			field: "foo.x.bar",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := mustContext(t, tt.opts...)
			require.Equal(t, tt.want, ctx.IsFieldAllowed(tt.field))
		})
	}
}
