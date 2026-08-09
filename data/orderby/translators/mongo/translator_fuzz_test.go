// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/orderby"
	orderbymongo "github.com/altessa-s/go-atlas/data/orderby/translators/mongo"
)

// allowedFields is the allowlist the targets translate against.
var allowedFields = []string{"name", "age", "created_at", "score"}

// FuzzTranslateHonorsTheAllowlist is the DoS oracle for the sort parameter.
//
// An order-by expression arrives from a client, and the allowlist is what keeps
// it pointed at indexed columns: an unindexed sort makes MongoDB do an
// in-memory sort that aborts past 32 MB, so a field that slips through is a
// request that can take the collection down. The parser has its own target;
// this one covers the translation, which is where the check actually runs, and
// feeds whole expressions so a field reaches it the way a client sends it.
func FuzzTranslateHonorsTheAllowlist(f *testing.F) {
	f.Add("name")
	f.Add("-age")
	f.Add("name,-created_at")
	f.Add("name asc, age desc")
	f.Add("secret")
	f.Add("$natural")
	f.Add("name,secret")
	f.Add("")
	f.Add(",,,")
	f.Add(strings.Repeat("name,", 64))

	parser, err := orderby.NewParser()
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, expression string) {
		spec, err := parser.Parse(t.Context(), expression)
		if err != nil {
			return // Rejected at parse; the translator never sees it.
		}

		translator, err := orderbymongo.NewTranslator(orderby.WithAllowedFields(allowedFields...))
		require.NoError(t, err)

		sort, err := translator.Translate(spec)
		if err != nil {
			require.Empty(t, sort, "a rejected spec must not also produce a sort document")
			return
		}

		for _, entry := range sort {
			require.True(t, slices.Contains(allowedFields, entry.Key),
				"a field outside the allowlist reached the sort: %q\n%v", entry.Key, sort)
			// int32 specifically: the driver encodes the Go type as written,
			// and MongoDB rejects a sort direction that is not a number.
			require.Contains(t, []any{int32(1), int32(-1)}, entry.Value,
				"sort direction must be int32 1 or -1, got %T(%v) for %q", entry.Value, entry.Value, entry.Key)
		}
	})
}
