// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package urlpath_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/urlpath"
)

// routerClean is the canonicalization net/http.ServeMux applies
// before dispatch: path.Clean on a rooted path, trailing slash kept.
func routerClean(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if strings.HasSuffix(p, "/") && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

func TestIsCanonical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/admin", true},
		{"/admin/", true},
		{"/api/v1/users", true},
		{"/files/.hidden", true},
		{"/files/...", true},
		{"/a..b/c.d", true},
		{"", false},
		{"admin", false},
		{"*", false},
		{"//", false},
		{"//admin", false},
		{"/admin//", false},
		{"/api//users", false},
		{"/.", false},
		{"/..", false},
		{"/./admin", false},
		{"/admin/.", false},
		{"/admin/./", false},
		{"/public/../admin", false},
		{"/healthz/../admin", false},
		{"/admin/..", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, urlpath.IsCanonical(tc.path))
			require.Equal(t, routerClean(tc.path) == tc.path, urlpath.IsCanonical(tc.path),
				"must agree with the routers' path cleaning")
		})
	}
}

func FuzzIsCanonical(f *testing.F) {
	for _, seed := range []string{"/", "/admin", "/admin/", "//admin", "/a/../b", "/./a", "", "a", "/a//", "/..."} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		require.Equal(t, routerClean(p) == p, urlpath.IsCanonical(p))
	})
}
