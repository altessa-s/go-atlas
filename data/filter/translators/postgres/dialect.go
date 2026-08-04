// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

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

// dateTimeLayout renders a [time.Time] as a PostgreSQL timestamptz
// literal at microsecond precision, the most the type can hold. The
// trailing +00 is the offset, always UTC here.
const dateTimeLayout = "2006-01-02 15:04:05.000000-07"

// dialect implements [sqlbase.Dialect] for PostgreSQL.
type dialect struct{}

// QuoteIdent renders a column name as a double-quoted identifier,
// quoting each dot-separated segment on its own to produce the standard
// qualified form "table"."column".
//
// Quoting also pins the case: PostgreSQL folds unquoted identifiers to
// lower case, so a camelCase CEL field maps to a camelCase column, not
// to its lower-cased form. Use [filter.WithFieldMapping] where the table
// actually uses snake_case.
func (dialect) QuoteIdent(name string) (string, error) {
	return sqlbase.QuoteQualified(name, '"')
}

// Placeholder renders PostgreSQL's numbered bind marker.
func (dialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

// SizeExpr renders size() as length(), which counts characters of a text
// value. Array and jsonb columns need cardinality() and
// jsonb_array_length() instead — map those fields onto a generated
// column if you need to filter on their size.
func (dialect) SizeExpr(col string) string { return "length(" + col + ")" }

// StringPredicate maps the string predicates onto PostgreSQL built-ins.
//
// contains and startsWith take the needle as a plain string — no LIKE
// pattern to escape, so a `%` or `_` in the operand stays literal.
// starts_with() requires PostgreSQL 11 or newer.
//
// endsWith has no built-in and compiles to a suffix comparison, which
// needs the operand twice — once to size the suffix, once to compare it.
// value is therefore called twice and binds two arguments.
func (dialect) StringPredicate(op filter.Operator, col, needle string, value sqlbase.ValueFunc) (string, error) {
	arg, err := value(needle)
	if err != nil {
		return "", err
	}

	switch op {
	case filter.OpContains:
		return "strpos(" + col + ", " + arg + ") > 0", nil
	case filter.OpStartsWith:
		return "starts_with(" + col + ", " + arg + ")", nil
	case filter.OpEndsWith:
		second, err := value(needle)
		if err != nil {
			return "", err
		}
		return "right(" + col + ", length(" + arg + ")) = " + second, nil
	case filter.OpMatches:
		return col + " ~ " + arg, nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedOperation, "string predicate %v", op)
	}
}

// FormatLiteral renders a Go value as PostgreSQL SQL text.
func (dialect) FormatLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if val {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case uint64:
		return strconv.FormatUint(val, 10), nil
	case float64:
		return formatFloat(val), nil
	case string:
		return quoteString(val), nil
	case []byte:
		// decode() rather than the '\xdead'::bytea form, which relies on
		// standard_conforming_strings being on; this one has no
		// backslash to interpret and is correct either way.
		return "decode('" + hex.EncodeToString(val) + "', 'hex')", nil
	case time.Time:
		return "TIMESTAMP WITH TIME ZONE '" + val.UTC().Format(dateTimeLayout) + "'", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// formatFloat renders a float64 as a PostgreSQL numeric literal. The
// non-finite values are representable, but only as quoted strings with
// an explicit cast — bare NaN or Infinity would parse as column names.
func formatFloat(v float64) string {
	switch {
	case math.IsNaN(v):
		return "'NaN'::double precision"
	case math.IsInf(v, 1):
		return "'Infinity'::double precision"
	case math.IsInf(v, -1):
		return "'-Infinity'::double precision"
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}

// quoteString renders a string as an escape-string literal, E'...',
// escaping the backslash and the quote.
//
// The E prefix is what makes this safe without knowing the server's
// configuration: inside a plain '...' literal the backslash is an escape
// character only when standard_conforming_strings is off, so no single
// plain-literal escaping is correct in both settings. An E-string always
// interprets backslashes, so one rule covers every server.
//
// A NUL byte is escaped to \000 rather than dropped. PostgreSQL text
// cannot hold one at all, so the server rejects the literal — which is
// the honest outcome, and better than silently truncating the value.
//
// Iteration is byte-wise: every escaped byte is ASCII, and a UTF-8
// continuation byte is always >= 0x80, so multi-byte runes pass through
// untouched.
func quoteString(s string) string {
	const prefixAndQuotes = 3 // `E` + opening + closing `'`

	var b strings.Builder
	b.Grow(len(s) + prefixAndQuotes)
	b.WriteString("E'")
	for i := range len(s) {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case 0:
			b.WriteString(`\000`)
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
