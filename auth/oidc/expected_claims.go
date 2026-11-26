// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
)

// claimMatches reports whether a decoded claim value satisfies an expected
// value from [WithValidationExpectedClaims].
//
// A bool, number or nil expectation matches only a claim of the same JSON
// type and value: the string "true" does not satisfy true. A string
// expectation matches an equal string, or a bool or number whose canonical
// text equals it (string-only config formats such as YAML maps cannot
// express the type). An array claim matches when any element matches.
// Objects never match.
func claimMatches(actual, expected any) bool {
	if elems, ok := claimArray(actual); ok {
		for _, elem := range elems {
			if scalarMatches(elem, expected) {
				return true
			}
		}
		return false
	}
	return scalarMatches(actual, expected)
}

func scalarMatches(actual, expected any) bool {
	if expected == nil {
		return actual == nil
	}
	if want, ok := expected.(string); ok {
		return stringMatches(actual, want)
	}
	if want, ok := expected.(bool); ok {
		got, isBool := actual.(bool)
		return isBool && got == want
	}
	return numbersEqual(actual, expected)
}

func stringMatches(actual any, want string) bool {
	switch got := actual.(type) {
	case string:
		return got == want
	case bool:
		return strconv.FormatBool(got) == want
	}
	if !isNumeric(actual) {
		return false
	}
	if !reflect.ValueOf(actual).CanFloat() && !json.Valid([]byte(want)) {
		// ParseFloat also accepts strings such as "+01" and "0x1p0".
		// Check that syntax, then convert the configured text exactly.
		if _, err := strconv.ParseFloat(want, 64); err != nil {
			return false
		}
		r, ok := new(big.Rat).SetString(want)
		if !ok {
			return false
		}
		places, _ := r.FloatPrec()
		want = r.FloatString(places)
	}
	return numbersEqual(actual, json.Number(want))
}

func isNumeric(v any) bool {
	if _, ok := v.(json.Number); ok {
		return true
	}
	kind := reflect.ValueOf(v).Kind()
	return (kind >= reflect.Int && kind <= reflect.Uint64) || kind == reflect.Float32 || kind == reflect.Float64
}

// numbersEqual compares two Go floats in their native precision. Any other
// pair, a json.Number or an integer on either side, is compared by exact
// decimal value, so an integer is never rounded to a float and a json.Number
// is never equated with a float it merely rounds to.
func numbersEqual(a, b any) bool {
	if !isNumeric(a) || !isNumeric(b) {
		return false
	}
	if reflect.ValueOf(a).CanFloat() && reflect.ValueOf(b).CanFloat() {
		x, ok := floatNumber(a)
		y, valid := floatNumber(b)
		return ok && valid && x == y
	}
	x, ok := exactNumber(a)
	y, valid := exactNumber(b)
	return ok && valid && x.Cmp(y) == 0
}

// exactNumber returns the exact value of a numeric operand. A float that holds
// an integer is that integer exactly; any other float stands for its shortest
// round-trip decimal, as it does when it is decoded from a token.
func exactNumber(v any) (*big.Rat, bool) {
	if n, ok := v.(json.Number); ok {
		if !json.Valid([]byte(n)) {
			return nil, false
		}
		return new(big.Rat).SetString(string(n))
	}
	r := reflect.ValueOf(v)
	switch {
	case r.CanFloat():
		f := r.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, false
		}
		if f == math.Trunc(f) {
			return new(big.Rat).SetFloat64(f), true
		}
		return new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, r.Type().Bits()))
	case r.CanInt():
		return new(big.Rat).SetInt64(r.Int()), true
	default:
		return new(big.Rat).SetUint64(r.Uint()), true
	}
}

func floatNumber(v any) (float64, bool) {
	f := reflect.ValueOf(v).Float()
	return f, !math.IsInf(f, 0)
}

func claimArray(v any) ([]any, bool) {
	switch arr := v.(type) {
	case []any:
		return arr, true
	case []string:
		out := make([]any, len(arr))
		for i, s := range arr {
			out[i] = s
		}
		return out, true
	default:
		return nil, false
	}
}

// ClaimValueEquals matches a claim against a typed preset condition value.
// Numeric expectations retain their JSON precision. String expectations match strings only.
func ClaimValueEquals(name string, expected any) PresetMatcherFunc {
	return func(claims map[string]any) bool {
		actual, exists := claims[name]
		return exists && conditionValueEquals(actual, expected)
	}
}

// conditionValueEquals compares a claim with a claim_equals value: strings,
// numbers, bools and null by JSON type; any other value by its %v text.
func conditionValueEquals(actual, expected any) bool {
	switch exp := expected.(type) {
	case string:
		got, ok := actual.(string)
		return ok && got == exp
	case bool:
		got, ok := actual.(bool)
		return ok && got == exp
	case nil:
		return actual == nil
	default:
		if isNumeric(expected) || isNumeric(actual) {
			return numbersEqual(actual, expected)
		}
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
	}
}
