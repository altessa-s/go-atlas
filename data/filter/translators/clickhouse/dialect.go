// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/internal/sqlbase"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// dateTimeLayout renders a [time.Time] in the form ClickHouse parses as
// a DateTime64(3) literal. Precision below one millisecond is dropped.
const dateTimeLayout = "2006-01-02 15:04:05.000"

// dialect implements [sqlbase.Dialect] for ClickHouse.
type dialect struct{}

// QuoteIdent renders a column name as a single backtick-quoted
// identifier, dots included: a ClickHouse Nested column is literally
// stored under the name "address.city", so splitting it into a qualified
// reference would name something else.
func (dialect) QuoteIdent(name string) (string, error) {
	return sqlbase.QuoteWhole(name, '`')
}

// Placeholder renders ClickHouse's positional bind marker.
func (dialect) Placeholder(int) string { return "?" }

// SizeExpr renders size() as length(), which covers both String and
// Array columns.
func (dialect) SizeExpr(col string) string { return "length(" + col + ")" }

// stringPredicates maps the string predicates onto native ClickHouse
// functions. All four take the needle as a plain string, so nothing
// needs pattern escaping — unlike a LIKE-based rendering, a `%` or `_`
// in the operand stays literal. ClickHouse has endsWith(), so the
// operand is bound once.
var stringPredicates = sqlbase.StringPredicates{
	Contains:   "position(%[1]s, %[2]s) > 0",
	StartsWith: "startsWith(%[1]s, %[2]s)",
	EndsWith:   "endsWith(%[1]s, %[2]s)",
	Matches:    "match(%[1]s, %[2]s)",
}

// StringPredicate renders one of the four string predicates.
func (dialect) StringPredicate(op filter.Operator, col, needle string, value sqlbase.ValueFunc) (string, error) {
	return sqlbase.RenderStringPredicate(op, col, needle, value, stringPredicates)
}

// FormatLiteral renders a Go value as ClickHouse SQL text.
func (dialect) FormatLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float64:
		return formatFloat(val), nil
	case string:
		return quoteString(val), nil
	case []byte:
		// Hex avoids having to reason about which byte sequences survive
		// a string literal; unhex() yields a ClickHouse String either way.
		return "unhex('" + hex.EncodeToString(val) + "')", nil
	case time.Time:
		return "toDateTime64('" + val.UTC().Format(dateTimeLayout) + "', 3, 'UTC')", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// formatFloat renders a float64 as a ClickHouse numeric literal. The
// non-finite values have their own spellings and would otherwise come
// out as Go's "NaN" / "+Inf", which ClickHouse cannot parse.
func formatFloat(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}

// quoteString wraps a string in single quotes with ClickHouse backslash
// escaping. Iteration is byte-wise: every escaped byte is ASCII, and a
// UTF-8 continuation byte is always >= 0x80, so multi-byte runes pass
// through untouched.
func quoteString(s string) string {
	const surroundingQuotes = 2 // opening + closing `'`

	var b strings.Builder
	b.Grow(len(s) + surroundingQuotes)
	b.WriteByte('\'')
	for i := range len(s) {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// Ensure dialect implements sqlbase.Dialect.
var _ sqlbase.Dialect = dialect{}
