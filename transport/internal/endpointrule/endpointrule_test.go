// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointrule_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/endpointrule"
)

type rule struct{ name string }

func TestRegistry_Lookup(t *testing.T) {
	t.Parallel()

	exact := &rule{name: "exact"}
	shared := &rule{name: "shared"}
	first := &rule{name: "first-pattern"}
	second := &rule{name: "second-pattern"}
	fallback := &rule{name: "default"}

	newRegistry := func(withDefault bool) *endpointrule.Registry[rule] {
		r := &endpointrule.Registry[rule]{}
		r.Register("/svc.Exact", exact)
		r.Register("/svc.Nil", nil)
		r.RegisterEndpoints(shared, "/svc.A", "/svc.B")
		r.RegisterPattern(regexp.MustCompile(`^/svc\.`), first)
		r.RegisterPattern(regexp.MustCompile(`^/svc\.Second`), second)
		if withDefault {
			r.SetDefault(fallback)
		}
		return r
	}

	tests := []struct {
		name        string
		withDefault bool
		endpoint    string
		want        *rule
		wantFound   bool
	}{
		{name: "exact wins over pattern", endpoint: "/svc.Exact", want: exact, wantFound: true},
		{name: "exact nil rule is still a match", endpoint: "/svc.Nil", want: nil, wantFound: true},
		{name: "endpoint list", endpoint: "/svc.B", want: shared, wantFound: true},
		{name: "first pattern in insertion order", endpoint: "/svc.Second", want: first, wantFound: true},
		{name: "default fallback", withDefault: true, endpoint: "/other.Method", want: fallback, wantFound: true},
		{name: "missing rule", endpoint: "/other.Method", want: nil, wantFound: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, found := newRegistry(tc.withDefault).Lookup(tc.endpoint)
			require.Equal(t, tc.wantFound, found)
			require.Same(t, tc.want, got)
		})
	}
}

func TestRegistry_ZeroValueLookup(t *testing.T) {
	t.Parallel()

	var r endpointrule.Registry[rule]
	got, found := r.Lookup("/svc.Method")
	require.False(t, found)
	require.Nil(t, got)
}
