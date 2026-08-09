// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"
)

// quoteBytes are the identifier quotes the supported dialects use: double
// quotes for PostgreSQL and MariaDB's ANSI mode, backticks for MariaDB's
// default. Fuzzing an arbitrary byte instead would make the counting
// assertions meaningless — a letter used as a "quote" occurs inside ordinary
// identifiers, and nothing downstream ever passes one.
var quoteBytes = []byte{'"', '`'}

// identPattern is the grammar ValidateIdent documents: dot-separated segments
// of [A-Za-z_][A-Za-z0-9_]*. It is spelled out here rather than reused from the
// implementation on purpose — a fuzz oracle that shares code with the thing it
// checks agrees with it by construction, including when both are wrong.
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// identSeeds are the shapes worth starting from: legitimate names, and the
// injection attempts the validator exists to stop.
var identSeeds = []string{
	"name",
	"address.city",
	"_private",
	"",
	".",
	"a..b",
	"1name",
	`name"; DROP TABLE users; --`,
	"name`",
	`na"me`,
	"name; SELECT 1",
	"name/*comment*/",
	"name[0]",
	"COUNT(*)",
	"name\x00",
	"имя",
}

// FuzzValidateIdent checks the validator against an independent statement of
// the grammar it enforces.
//
// The column position is the one part of a generated SQL clause no placeholder
// can cover: a filter's field mapping is interpolated into the statement as
// text. So the question is not whether the validator rejects the payloads
// someone thought of, but whether "accepted" and "matches the grammar" are the
// same set for every string — which is what a fuzzer, unlike a table of cases,
// can actually put pressure on.
func FuzzValidateIdent(f *testing.F) {
	for _, seed := range identSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		err := sqlbase.ValidateIdent(name)
		require.Equal(t, identPattern.MatchString(name), err == nil,
			"ValidateIdent(%q) disagrees with the documented grammar: err=%v", name, err)
	})
}

// FuzzQuoteWhole pins the guarantee the quoters rest on: the quote byte cannot
// appear inside a name that reached a quoter, so the output has exactly the two
// quotes it added and no way to close the identifier early.
//
// Quoting that escaped its input would need doubling logic; quoting that does
// not escape needs a validator that makes escaping unnecessary. This target
// checks the second contract holds absolutely, because if it ever does not, the
// missing escape becomes an injection rather than a formatting bug.
func FuzzQuoteWhole(f *testing.F) {
	for _, seed := range identSeeds {
		f.Add(seed, uint8(0))
		f.Add(seed, uint8(1))
	}

	f.Fuzz(func(t *testing.T, name string, quoteSel uint8) {
		quote := quoteBytes[int(quoteSel)%len(quoteBytes)]

		out, err := sqlbase.QuoteWhole(name, quote)
		if err != nil {
			require.Empty(t, out, "a rejected name must not also be quoted")
			return
		}

		require.Equal(t, string(quote)+name+string(quote), out)
		require.Equal(t, 2, strings.Count(out, string(quote)),
			"the quoted identifier can be closed early: %q", out)
	})
}

// FuzzQuoteQualified is the same guarantee for the per-segment form
// ("table"."column"), where the arithmetic is easier to get wrong: the quote
// count scales with the number of dots, and a name that slipped a quote or an
// extra dot past the validator would show up as a mismatch here.
func FuzzQuoteQualified(f *testing.F) {
	for _, seed := range identSeeds {
		f.Add(seed, uint8(0))
		f.Add(seed, uint8(1))
	}

	f.Fuzz(func(t *testing.T, name string, quoteSel uint8) {
		quote := quoteBytes[int(quoteSel)%len(quoteBytes)]

		out, err := sqlbase.QuoteQualified(name, quote)
		if err != nil {
			require.Empty(t, out, "a rejected name must not also be quoted")
			return
		}

		segments := strings.Split(name, ".")
		require.Equal(t, 2*len(segments), strings.Count(out, string(quote)),
			"the qualified identifier can be closed early: %q", out)

		// Stripping the quoting must give back exactly what went in — no
		// segment dropped, merged, or reordered.
		require.Equal(t, name, strings.ReplaceAll(out, string(quote), ""))
	})
}
