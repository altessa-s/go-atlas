// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// backend is one target of the shared corpus. Each implementation owns
// its schema, its seeding and its query execution; everything above that
// — which cases run, what they expect — is shared.
type backend interface {
	// name identifies the backend in the corpus tables.
	name() filterit.Backend

	// setup connects, creates a throwaway schema and seeds the dataset.
	// It skips the test when the server is not reachable, so a developer
	// without the compose stack running still gets a green build.
	setup(tb testing.TB)

	// search translates expr and runs the result, returning the matching
	// row IDs in ascending order.
	search(tb testing.TB, expr string) ([]int64, error)

	// translate runs only the translation half, layering extra options on
	// top of the backend's defaults. The rejection matrix uses it to
	// assert on errors without involving the server.
	translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error
}

// envOr returns the environment override for a connection setting, or
// the default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uniqueSuffix names a throwaway schema, index or database so that two
// runs against the same server — or two backends in one run — never
// collide.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// parse turns a corpus expression into an AST, failing the test on a
// parse error — a broken corpus entry is a bug in the suite, not a
// finding about the backend.
func parse(tb testing.TB, expr string) filter.Node {
	tb.Helper()

	parser, err := filter.NewParser()
	require.NoError(tb, err)

	node, err := parser.Parse(tb.Context(), expr)
	require.NoError(tb, err, "corpus expression %q does not parse", expr)
	return node
}

// runCorpus drives the shared success corpus against one backend.
func runCorpus(t *testing.T, b backend) {
	t.Helper()

	b.setup(t)

	for _, tc := range filterit.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			if reason, ok := tc.AppliesTo(b.name()); !ok {
				t.Skipf("not applicable to %s: %s", b.name(), reason)
			}

			// A NoError here is the assertion unit tests cannot make: a
			// clause can match a golden string byte for byte and still
			// be a syntax error, an unknown function or a type mismatch
			// once it reaches the server.
			got, err := b.search(t, tc.Expr)
			require.NoError(t, err)

			want := tc.WantFor(b.name())
			require.Equal(t, normalize(want), normalize(got),
				"expression %q selected the wrong rows", tc.Expr)
		})
	}
}

// runRejections drives the shared rejection matrix against one backend.
func runRejections(t *testing.T, b backend) {
	t.Helper()

	for _, tc := range filterit.Rejections() {
		t.Run(tc.Name, func(t *testing.T) {
			if reason, ok := tc.AppliesTo(b.name()); !ok {
				t.Skipf("not applicable to %s: %s", b.name(), reason)
			}

			err := b.translate(t, tc.Expr, tc.Options...)
			require.Error(t, err, "expression %q was expected to be rejected", tc.Expr)
			require.ErrorIs(t, err, tc.WantFor(b.name()))
		})
	}
}

// normalize sorts a result set and collapses empty to nil so that a
// backend returning []int64{} compares equal to one returning nil.
func normalize(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}
