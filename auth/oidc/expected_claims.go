// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
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
	want, ok := claimNumber(expected)
	if !ok {
		return false
	}
	got, ok := claimNumber(actual)
	return ok && got == want
}

func stringMatches(actual any, want string) bool {
	switch got := actual.(type) {
	case string:
		return got == want
	case bool:
		return strconv.FormatBool(got) == want
	}
	got, ok := claimNumber(actual)
	if !ok {
		return false
	}
	parsed, err := strconv.ParseFloat(want, 64)
	return err == nil && parsed == got
}

// claimNumber converts a numeric claim or expectation to float64, the type
// encoding/json decodes JSON numbers into.
func claimNumber(v any) (float64, bool) {
	if n, ok := v.(json.Number); ok {
		f, err := n.Float64()
		return f, err == nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	default:
		return 0, false
	}
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
