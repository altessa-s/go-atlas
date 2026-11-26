// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimnum

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"

	"github.com/altessa-s/go-atlas/auth/internal/numericdate"
)

// ErrOutOfRange reports a number outside the float64 range.
var ErrOutOfRange = errors.New("custom numeric claim outside float64 range")

// Number returns n as a float64 when that value is exactly the decimal n
// spells, and as n itself otherwise.
func Number(n json.Number) (any, error) {
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) {
		return nil, ErrOutOfRange
	}
	if exact(f, n) {
		return f, nil
	}
	return n, nil
}

// exact reports whether f stands for the decimal n: an integer-valued n must
// be the float's exact integer value (the shortest spelling of a large float
// can name a different integer); any other n must equal the float's shortest
// round-trip spelling, so ordinary decimals such as 0.1 keep float64.
func exact(f float64, n json.Number) bool {
	want, ok := new(big.Rat).SetString(string(n))
	if !ok {
		return false
	}
	if want.IsInt() {
		return new(big.Rat).SetFloat64(f).Cmp(want) == 0
	}
	return numericdate.EqualNumbers(json.Number(strconv.FormatFloat(f, 'g', -1, 64)), n)
}

// Value normalizes every number inside v, in place for maps and slices.
func Value(v any) (any, error) {
	switch t := v.(type) {
	case json.Number:
		return Number(t)
	case map[string]any:
		for k, item := range t {
			decoded, err := Value(item)
			if err != nil {
				return nil, err
			}
			t[k] = decoded
		}
	case []any:
		for i, item := range t {
			decoded, err := Value(item)
			if err != nil {
				return nil, err
			}
			t[i] = decoded
		}
	}
	return v, nil
}

// Decode decodes one JSON value into v with number spelling preserved, then
// normalizes it with [Value].
func Decode(data []byte, v *map[string]any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	for k, item := range *v {
		decoded, err := Value(item)
		if err != nil {
			return err
		}
		(*v)[k] = decoded
	}
	return nil
}
