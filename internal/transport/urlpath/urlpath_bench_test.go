// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package urlpath_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/urlpath"
)

func BenchmarkIsCanonical(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		urlpath.IsCanonical("/api/v1/tenants/42/users/7/sessions")
	}
}

func BenchmarkIsCanonicalURL(b *testing.B) {
	plain := &url.URL{Path: "/api/v1/tenants/42/users/7/sessions"}
	escaped, err := url.ParseRequestURI("/api/v1/tenants/42/users/caf%C3%A9%21/sessions")
	require.NoError(b, err)

	b.Run("plain", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			urlpath.IsCanonicalURL(plain)
		}
	})
	b.Run("escaped", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			urlpath.IsCanonicalURL(escaped)
		}
	})
}
