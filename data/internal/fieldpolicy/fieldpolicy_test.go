// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldpolicy_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/fieldpolicy"
)

func TestAllowList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		list       fieldpolicy.AllowList
		field      string
		want       bool
		configured bool
		empty      bool
		all        bool
	}{
		{name: "zero value allows anything", field: "x", want: true, empty: true, all: true},
		{name: "empty configured denies", list: fieldpolicy.NewAllowList(), field: "x", configured: true, empty: true},
		{name: "exact hit", list: fieldpolicy.NewAllowList("a"), field: "a", want: true, configured: true},
		{name: "exact miss", list: fieldpolicy.NewAllowList("a"), field: "b", configured: true},
		{name: "prefix subpath", list: fieldpolicy.NewAllowList("a.*"), field: "a.b.c", want: true, configured: true},
		{name: "prefix excludes parent", list: fieldpolicy.NewAllowList("a.*"), field: "a", configured: true},
		{name: "prefix boundary", list: fieldpolicy.NewAllowList("a.*"), field: "ab.c", configured: true},
		{name: "lone star", list: fieldpolicy.NewAllowList("*"), field: "q.w", want: true, configured: true, all: true},
		{name: "embedded star is literal", list: fieldpolicy.NewAllowList("a.*.b"), field: "a.x.b", configured: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.list.Allows(tc.field))
			require.Equal(t, tc.configured, tc.list.IsConfigured())
			require.Equal(t, tc.empty, tc.list.IsEmpty())
			require.Equal(t, tc.all, tc.list.MatchesAll())
		})
	}
}

func TestAllowListRoots(t *testing.T) {
	t.Parallel()

	require.Nil(t, fieldpolicy.AllowList{}.Roots())
	require.Nil(t, fieldpolicy.NewAllowList("a", "*").Roots())
	require.Equal(t, []string{"a", "b", "c.d"},
		fieldpolicy.NewAllowList("b", "c.d.*", "a", "b.*").Roots())
}

func TestMapping(t *testing.T) {
	t.Parallel()

	m := fieldpolicy.Mapping{}.
		WithExact(map[string]string{"createdAt": "created_at", "user.profile.x": "px"}).
		WithPrefixes(map[string]string{
			"user.":         "u.",
			"user.profile.": "up.",
			"bad":           "worse.",
		})

	tests := []struct{ in, want string }{
		{"createdAt", "created_at"},
		{"user.profile.x", "px"},
		{"user.profile.name", "up.name"},
		{"user.name", "u.name"},
		{"badge", "badge"},
		{"other", "other"},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, m.Apply(tc.in), tc.in)
	}
	require.Equal(t, "x", fieldpolicy.Mapping{}.Apply("x"))
}

func TestOverlaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want bool
	}{
		{"a", "a", true},
		{"a", "a.b", true},
		{"a.b", "a", true},
		{"a", "ab", false},
		{"a.b", "a.bc", false},
		{"a.bc", "a.b", false},
		{"a.b", "a.c", false},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, fieldpolicy.Overlaps(tc.a, tc.b), "%s vs %s", tc.a, tc.b)
	}
}

func TestMappingSubtreeAndReach(t *testing.T) {
	t.Parallel()

	m := fieldpolicy.Mapping{}.
		WithExact(map[string]string{
			"address.city": "city_col",
			"address.zip":  "address.zip",
			"credentials":  "c",
			"createTime":   "created_at",
		}).
		WithPrefixes(map[string]string{"address.": "addr.", "address.geo.": "g."})

	tests := []struct {
		path        string
		wantSubtree []string
		wantReach   []string
	}{
		{path: "name", wantSubtree: []string{"name"}, wantReach: []string{"name"}},
		{path: "createTime", wantSubtree: []string{"created_at"}, wantReach: []string{"created_at"}},
		{path: "address", wantSubtree: []string{"addr", "city_col", "address.zip", "g"}},
		{path: "address.geo", wantSubtree: []string{"g"}, wantReach: []string{"addr.geo", "g"}},
		{path: "address.geo.lat", wantSubtree: []string{"g.lat"}, wantReach: []string{"addr.geo.lat", "g.lat"}},
		{path: "credentials", wantSubtree: []string{"c"}},
		{path: "credentials.password", wantSubtree: []string{"credentials.password"}, wantReach: []string{"credentials.password", "c.password"}},
	}
	for _, tc := range tests {
		got := m.Subtree(tc.path)
		require.ElementsMatch(t, tc.wantSubtree, got, tc.path)
		want := tc.wantReach
		if want == nil {
			want = tc.wantSubtree
		}
		require.ElementsMatch(t, want, m.Reach(tc.path), tc.path)
	}
	require.Equal(t, []string{"x"}, fieldpolicy.Mapping{}.Subtree("x"))
}

func TestMappingReachIncludesShadowedRules(t *testing.T) {
	t.Parallel()

	m := fieldpolicy.Mapping{}.
		WithExact(map[string]string{"credentials.password": "secret"}).
		WithPrefixes(map[string]string{"credentials.": "c.", "credentials.password.": "pw."})

	require.ElementsMatch(t, []string{"secret", "pw"}, m.Subtree("credentials.password"),
		"descendants are stored under the deeper prefix target")
	require.Equal(t, []string{"c.password", "pw", "secret"}, m.Reach("credentials.password"),
		"the shadowed ancestor prefix still says where the subtree is stored")
	require.Equal(t, []string{"c.password.hash", "pw.hash", "secret.hash"}, m.Reach("credentials.password.hash"))
}

// TestMappingSubtreeMatchesApply pins the subtree root to its definition —
// the exact entry for path, else TrimSuffix(Apply(path+"."), ".") — including
// exact keys that end in a dot.
func TestMappingSubtreeMatchesApply(t *testing.T) {
	t.Parallel()

	m := fieldpolicy.Mapping{}.
		WithExact(map[string]string{"a.": "b", "c": "d", "x.y": "z"}).
		WithPrefixes(map[string]string{"x.": "u.", "x.y.": "v.", "p.q.": "r."})

	exact := map[string]string{"c": "d", "x.y": "z"}
	for _, path := range []string{"a", "c", "x", "x.y", "x.y.z", "x.w", "p", "p.q", "p.q.s", "other"} {
		want, ok := exact[path]
		if !ok {
			want = strings.TrimSuffix(m.Apply(path+"."), ".")
		}
		require.Equal(t, want, m.Subtree(path)[0], path)
	}
	require.Equal(t, []string{"b"}, m.Subtree("a"))
}
