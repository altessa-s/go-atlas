// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Decimal literal bounds.
const (
	// MaxExactFloatDigits is the most significant digits a float literal may have to be read as an exact decimal: every decimal with
	// at most 15 significant digits survives the round trip through a float64, so the shortest representation of the parsed float
	// is the decimal that was written.
	MaxExactFloatDigits = 15

	// Decimal literals must be representable as an IEEE 754 Decimal128 (34 significant digits, exponent -6176..6111), which keeps
	// comparisons cheap and every literal storable.
	decimalMaxDigits = 34
	decimalMinExp    = -6176
	decimalMaxExp    = 6111
)

// DecimalLiteral returns v as an exact decimal, the way [FieldKindDecimal] reads an operand:
//
//   - a decimal.Decimal;
//   - an integer (int64, uint64, int32, int) exactly;
//   - a float64 as the shortest decimal that round-trips to it ("0.1" for the float parsed from 0.1), provided it has at most
//     [MaxExactFloatDigits] significant digits — a longer one cannot be told apart from its neighbors, so write it as a string;
//     NaN and infinities are rejected. A float parsed from a filter expression is read from its source spelling instead
//     ([DecimalOperand]);
//   - a string in decimal notation ("12.30", "-1e-2"), with up to 34 significant digits.
//
// Every result, a decimal.Decimal operand included, must be representable as a Decimal128 (34 significant digits, exponent
// -6176..6111) and is normalized (zero as 0, trailing zeros folded into the exponent), which bounds the cost of comparing it: an
// extreme exponent such as "0e-999999999" can never reach a comparison. Anything else fails with [ErrFieldTypeMismatch].
func DecimalLiteral(v any) (decimal.Decimal, error) {
	switch n := v.(type) {
	case decimal.Decimal:
		return boundedDecimal(n)
	case int64:
		return decimal.NewFromInt(n), nil
	case int32:
		return decimal.NewFromInt32(n), nil
	case int:
		return decimal.NewFromInt(int64(n)), nil
	case uint64:
		return decimal.NewFromUint64(n), nil
	case float64:
		return floatDecimal(n)
	case string:
		return stringDecimal(n)
	default:
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch, "%s is not a decimal", valueKindName(v))
	}
}

// DecimalOperand is [DecimalLiteral] for an operand produced by node: a float literal parsed from an expression is read from its
// exact source spelling (`1.00000000000000001` stays 1.00000000000000001 although the float it parsed to is 1), bounded like a
// decimal string; any other node uses v. Translators of [FieldKindDecimal] fields call it with the literal node and its value.
func DecimalOperand(node Node, v any) (decimal.Decimal, error) {
	if lit, ok := node.(*LiteralNode); ok && lit.Text != "" {
		return stringDecimal(lit.Text)
	}
	return DecimalLiteral(v)
}

// stringDecimal parses a decimal string within the Decimal128 bounds.
func stringDecimal(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch, "%q is not a decimal number", s)
	}
	return boundedDecimal(d)
}

// floatDecimal reads f as the shortest decimal that round-trips to it.
func floatDecimal(f float64) (decimal.Decimal, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch, "%v is not a finite decimal", f)
	}
	d, err := decimal.NewFromString(strconv.FormatFloat(f, 'g', -1, 64))
	if err != nil {
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch, "%v: %v", f, err)
	}
	if digits, _, _ := normalizeDecimal(d); digits > MaxExactFloatDigits {
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch,
			"float %v has %d significant digits, more than the %d that are exact; write it as a decimal string", f, digits, MaxExactFloatDigits)
	}
	return boundedDecimal(d)
}

// maxCoefficientBits caps the coefficient inspected by normalizeDecimal: 34 digits need 113 bits, the rest is room for trailing
// zeros. A larger coefficient is rejected without formatting it.
const maxCoefficientBits = 4096

// decimalBase is the radix of a decimal exponent.
const decimalBase = 10

// boundedDecimal accepts d when a Decimal128 can hold it — at most 34 significant digits, and an exponent that some scaling of the
// coefficient brings into -6176..6111 — and returns it normalized: zero as 0 (exponent 0), otherwise the coefficient without trailing
// zeros and the matching exponent.
func boundedDecimal(d decimal.Decimal) (decimal.Decimal, error) {
	digits, exp, ok := normalizeDecimal(d)
	switch {
	case !ok:
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch, "decimal coefficient exceeds %d bits", maxCoefficientBits)
	case digits == 0:
		return decimal.New(0, 0), nil
	case digits > decimalMaxDigits || exp < decimalMinExp || exp > int64(decimalMaxExp+decimalMaxDigits-digits):
		return decimal.Decimal{}, coreerrs.Wrapf(ErrFieldTypeMismatch,
			"decimal with %d significant digits and exponent %d is out of the Decimal128 range", digits, exp)
	}
	c := d.Coefficient()
	if shift := exp - int64(d.Exponent()); shift > 0 {
		c.Quo(c, new(big.Int).Exp(big.NewInt(decimalBase), big.NewInt(shift), nil)) // shift <= the coefficient's digit count
	}
	return decimal.NewFromBigInt(c, int32(exp)), nil //nolint:gosec // exp is within the Decimal128 range
}

// normalizeDecimal returns the number of digits of d's coefficient without trailing zeros and the exponent that goes with that
// coefficient (0 digits for zero, whatever its exponent). ok is false for a coefficient above [maxCoefficientBits].
func normalizeDecimal(d decimal.Decimal) (digits int, exp int64, ok bool) {
	c := d.Coefficient()
	if c.Sign() == 0 {
		return 0, 0, true
	}
	if c.BitLen() > maxCoefficientBits {
		return 0, 0, false
	}
	s := strings.TrimLeft(c.String(), "-")
	trimmed := strings.TrimRight(s, "0")
	return len(trimmed), int64(d.Exponent()) + int64(len(s)-len(trimmed)), true
}

// exactLiteral returns the operand v produced by node for a comparison against other: when other is a decimal.Decimal and node is
// a float literal with a recorded source spelling, the spelling (a decimal string read exactly by [DecimalLiteral]); otherwise v.
func exactLiteral(other any, node Node, v any) any {
	if _, ok := other.(decimal.Decimal); !ok {
		return v
	}
	if lit, ok := node.(*LiteralNode); ok && lit.Text != "" {
		return lit.Text
	}
	return v
}

// compareDecimal compares two operands at least one of which is a decimal.Decimal, exactly. ok is false when the other operand
// cannot be read as a decimal ([DecimalLiteral]).
func compareDecimal(left, right any) (c int, ok bool, err error) {
	_, lok := left.(decimal.Decimal)
	_, rok := right.(decimal.Decimal)
	if !lok && !rok {
		return 0, false, nil
	}
	l, err := DecimalLiteral(left)
	if err != nil {
		return 0, true, err
	}
	r, err := DecimalLiteral(right)
	if err != nil {
		return 0, true, err
	}
	return l.Cmp(r), true, nil
}
