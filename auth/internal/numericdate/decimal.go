// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package numericdate

import (
	"cmp"
	"encoding/json"
	"math/big"
	"strings"
)

// decimal is sign × digits × 10^(point-len(digits)). Nonzero digits have
// neither leading nor trailing zeros. Storing the decimal point separately
// avoids allocating the zero digits implied by a large exponent.
type decimal struct {
	sign   int
	digits string
	point  big.Int
}

func parseDecimal(n json.Number) (decimal, bool) {
	s := n.String()
	if s == "" || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) || !json.Valid([]byte(s)) || strings.TrimSpace(s) != s {
		return decimal{}, false
	}
	var d decimal
	d.sign = 1
	if s[0] == '-' {
		d.sign = -1
		s = s[1:]
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		if _, ok := d.point.SetString(s[i+1:], decimalBase); !ok {
			return decimal{}, false
		}
		s = s[:i]
	}
	whole, frac, _ := strings.Cut(s, ".")
	digits := whole + frac
	sig := strings.TrimLeft(digits, "0")
	if sig == "" {
		return decimal{}, true
	}
	d.digits = strings.TrimRight(sig, "0")
	d.point.Add(&d.point, big.NewInt(int64(len(whole)-(len(digits)-len(sig)))))
	return d, true
}

func (d *decimal) compare(other *decimal) int {
	if c := cmp.Compare(d.sign, other.sign); c != 0 {
		return c
	}
	if c := d.point.Cmp(&other.point); c != 0 {
		return d.sign * c
	}
	return d.sign * strings.Compare(d.digits, other.digits)
}

// EqualNumbers compares exact JSON decimal values, independently of their spelling.
func EqualNumbers(a, b json.Number) bool {
	x, ok := parseDecimal(a)
	y, valid := parseDecimal(b)
	return ok && valid && x.compare(&y) == 0
}

// subNanosecond splits off the digits below one nanosecond. The residual is
// measured from the nearest nanosecond, with ties rounded toward +infinity.
// This makes residuals directly comparable even after shifting a date by a
// duration (an integral number of nanoseconds).
func (d *decimal) subNanosecond(cut int) (residual *decimal, roundAway bool) {
	if cut >= len(d.digits) {
		return nil, false
	}
	r := &decimal{sign: d.sign}
	digits := d.digits[cut:]
	// For |fraction| >= 0.5 there are no implicit leading zeros. Taking
	// its decimal complement therefore needs at most len(digits) bytes.
	roundAway = digits[0] > '5' || (digits[0] == '5' && (len(digits) > 1 || d.sign > 0))
	if roundAway {
		complement := []byte(digits)
		for i, c := range complement {
			complement[i] = '9' - (c - '0')
		}
		complement[len(complement)-1]++
		digits = string(complement)
		r.sign = -r.sign
	}
	r.digits = strings.TrimLeft(digits, "0")
	r.point.SetInt64(int64(len(r.digits) - len(digits)))
	return r, roundAway
}

// digit returns a significand digit, including the implicit zero padding.
func (d *decimal) digit(index int) uint64 {
	if index < 0 || index >= len(d.digits) {
		return 0
	}
	return uint64(d.digits[index] - '0')
}
