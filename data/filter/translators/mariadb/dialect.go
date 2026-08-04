// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mariadb

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

// dateTimeLayout renders a [time.Time] as a MariaDB DATETIME literal at
// microsecond precision, the most DATETIME(6) can hold.
const dateTimeLayout = "2006-01-02 15:04:05.000000"

// dialect implements [sqlbase.Dialect] for MariaDB and MySQL.
type dialect struct{}

// QuoteIdent renders a column name as a backtick-quoted identifier,
// quoting each dot-separated segment on its own to produce the standard
// qualified form `table`.`column`.
func (dialect) QuoteIdent(name string) (string, error) {
	return sqlbase.QuoteQualified(name, '`')
}

// Placeholder renders MariaDB's positional bind marker.
func (dialect) Placeholder(int) string { return "?" }

// SizeExpr renders size() as CHAR_LENGTH, which counts characters rather
// than the bytes LENGTH would report on a multi-byte charset.
func (dialect) SizeExpr(col string) string { return "CHAR_LENGTH(" + col + ")" }

// stringPredicates maps the string predicates onto MariaDB built-ins.
//
// contains and startsWith both go through LOCATE, which takes the needle
// as a plain string — no LIKE pattern to escape, so a `%` or `_` in the
// operand stays literal. LOCATE takes the needle first, which is why the
// templates index their verbs. startsWith is LOCATE = 1 rather than a
// separate function because MariaDB has none; the two are equivalent,
// including for an empty needle, where LOCATE returns 1.
//
// endsWith has no built-in either and compiles to a suffix comparison,
// which needs the operand twice — once to size the suffix, once to
// compare against it. Hence EndsWithBindsTwice.
var stringPredicates = sqlbase.StringPredicates{
	Contains:           "LOCATE(%[2]s, %[1]s) > 0",
	StartsWith:         "LOCATE(%[2]s, %[1]s) = 1",
	EndsWith:           "RIGHT(%[1]s, CHAR_LENGTH(%[2]s)) = %[3]s",
	Matches:            "%[1]s REGEXP %[2]s",
	EndsWithBindsTwice: true,
}

// StringPredicate renders one of the four string predicates.
func (dialect) StringPredicate(op filter.Operator, col, needle string, value sqlbase.ValueFunc) (string, error) {
	return sqlbase.RenderStringPredicate(op, col, needle, value, stringPredicates)
}

// FormatLiteral renders a Go value as MariaDB SQL text.
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
		return formatFloat(val)
	case string:
		return quoteString(val), nil
	case []byte:
		// X'...' is MariaDB's hex literal; it sidesteps having to reason
		// about which byte sequences survive a string literal.
		return "X'" + hex.EncodeToString(val) + "'", nil
	case time.Time:
		return "'" + val.UTC().Format(dateTimeLayout) + "'", nil
	default:
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "%T", v)
	}
}

// formatFloat renders a float64 as a MariaDB numeric literal. MariaDB has
// no spelling for the non-finite values — DOUBLE cannot hold them at all
// — so those are rejected rather than approximated.
func formatFloat(v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", coreerrs.Wrapf(filter.ErrUnsupportedType, "MariaDB has no literal for %v", v)
	}
	return strconv.FormatFloat(v, 'g', -1, 64), nil
}

// quoteString wraps a string in single quotes, doubling the quote and
// the backslash.
//
// Doubling the quote is correct in every sql_mode, so the literal cannot
// be broken out of regardless of server configuration. Doubling the
// backslash is correct under the default mode, where it is an escape
// character; under NO_BACKSLASH_ESCAPES it is not, and a value
// containing a backslash comes out with it doubled. That costs fidelity,
// never safety — the quote is already handled — and it is the price of
// rendering without a server round-trip. Use the parameterized form
// where exact values matter.
//
// Iteration is byte-wise: every escaped byte is ASCII, and a UTF-8
// continuation byte is always >= 0x80, so multi-byte runes pass through
// untouched.
func quoteString(s string) string {
	const surroundingQuotes = 2 // opening + closing `'`

	var b strings.Builder
	b.Grow(len(s) + surroundingQuotes)
	b.WriteByte('\'')
	for i := range len(s) {
		switch c := s[i]; c {
		case '\'':
			b.WriteString(`''`)
		case '\\':
			b.WriteString(`\\`)
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
